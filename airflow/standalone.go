//go:build !windows

package airflow

import (
	"bufio"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/settings"
)

const osDarwin = "darwin"

var (
	standaloneDir         = airflowrt.StandaloneDir
	standalonePIDFile     = airflowrt.StandalonePIDFile
	standaloneLogFile     = airflowrt.StandaloneLogFile
	defaultStandalonePort = airflowrt.DefaultPort
	standaloneIndexURL    = airflowrt.StandaloneIndexURL
	defaultPythonVersion  = airflowrt.DefaultPython
	stopPollInterval      = airflowrt.StopPollInterval
	stopTimeout           = airflowrt.StopTimeout
	filePermissions       = airflowrt.FilePermissions
	dirPermissions        = airflowrt.DirPermissions
	standaloneVersionFile = airflowrt.StandaloneVersionFile
)

var (
	errStandaloneNotSupported    = errors.New("this command is not supported in standalone mode")
	errUnsupportedAirflowVersion = errors.New("standalone mode requires Airflow 2 (runtime 11.x-13.x) or Airflow 3 (runtime X.Y-Z format, e.g. 3.0-1)")
	errUVNotFound                = errors.New("'uv' is required for standalone mode but was not found on PATH.\nInstall it with: curl -LsSf https://astral.sh/uv/install.sh | sh\nSee https://docs.astral.sh/uv/getting-started/installation/ for more options")

	// Function variables for testing
	lookPath              = exec.LookPath
	standaloneParseFile   = docker.ParseFile
	standaloneGetImageTag = docker.GetImageTagFromParsedFile
	runCommand            = execCommand
	startCommand          = startCmd
	standaloneExec        = standaloneExecDefault
	osReadFile            = os.ReadFile
	resolveFloatingTag    = airflowversions.ResolveFloatingTag
	standaloneOpenURL     = browser.OpenURL
)

// parseRuntimeTagPython delegates to airflowrt.ParseRuntimeTagPython.
var parseRuntimeTagPython = airflowrt.ParseRuntimeTagPython

// writeDarwinForkSafetyPatch installs a .pth file into the venv's
// site-packages that neutralizes macOS fork-safety hazards at Python
// startup.  After os.fork() on macOS the Objective-C, CoreFoundation,
// and Network framework runtimes are in a corrupt state, causing forked
// children to spin at 100 % CPU on getaddrinfo, setproctitle, or any
// call that touches os_log / libdispatch.
//
// The patch does two things (only on macOS):
//
//  1. Removes os.fork from the Python namespace so that Airflow's
//     CAN_FORK = hasattr(os, "fork") evaluates to False.  This forces
//     standard_task_runner to use subprocess (_start_by_exec) instead of
//     the unsafe fork path (_start_by_fork).  subprocess.Popen uses
//     C-level fork+exec via _posixsubprocess, not os.fork, so it is
//     unaffected.
//
//  2. Replaces setproctitle.setproctitle with a no-op to prevent the
//     CoreFoundation CFBundleGetFunctionPointerForName spin in any
//     process that was still forked by other means.
//
// We use a .pth file (processed by Python's site module at startup)
// rather than sitecustomize.py because the venv's lib/pythonX.Y/
// directory is not on sys.path, whereas site-packages is.
func writeDarwinForkSafetyPatch(venvPath string) error {
	libDir := filepath.Join(venvPath, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "python") {
			continue
		}

		sitePackages := filepath.Join(libDir, e.Name(), "site-packages")

		// Skip if already patched. We treat the .pth file as the sentinel —
		// both files are always written together, so its presence implies
		// _fix_setproctitle.py is also in place.
		if _, err := os.Stat(filepath.Join(sitePackages, "_fix_setproctitle.pth")); err == nil {
			continue
		}

		if err := os.WriteFile(filepath.Join(sitePackages, "_fix_setproctitle.py"), darwinForkSafetyPy, filePermissions); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(sitePackages, "_fix_setproctitle.pth"), darwinForkSafetyPth, filePermissions); err != nil {
			return err
		}
	}
	return nil
}

// af2DarwinShim is the macOS shim that replaces `airflow standalone` for AF2,
// avoiding gunicorn's fork which crashes on macOS.
//
//go:embed standalone_scripts/af2_darwin_shim.py
var af2DarwinShim []byte

// af2PickleFixPlugin fixes QueuedLocalWorker pickling under macOS spawn-mode multiprocessing.
//
//go:embed standalone_scripts/af2_pickle_fix_plugin.py
var af2PickleFixPlugin []byte

// darwinForkSafetyPy / darwinForkSafetyPth are installed into the venv's
// site-packages to neutralize macOS fork-safety hazards at Python startup.
//
//go:embed standalone_scripts/fix_setproctitle.py
var darwinForkSafetyPy []byte

//go:embed standalone_scripts/fix_setproctitle.pth
var darwinForkSafetyPth []byte

// af2RuntimePythonDefaults maps AF2 runtime major versions to their default
// Python version.  The runtime versions JSON does not populate
// defaultPythonVersion for AF2 entries, so this serves as the authoritative
// lookup.
var af2RuntimePythonDefaults = map[string]string{
	"11": "3.11",
	"12": "3.12",
	"13": "3.12",
}

// resolvePythonVersion determines the Python version using a 4-tier strategy:
//  1. Explicit version from the Dockerfile image tag (-python-X.Y suffix)
//  2. defaultPythonVersion from the runtime versions JSON (updates.astronomer.io)
//  3. Known AF2 runtime-major → Python mapping
//  4. Hardcoded fallback (3.12)
var resolvePythonVersion = func(baseTag, tagPython string) string {
	// Tier 1: Dockerfile image tag had an explicit -python-X.Y suffix
	if tagPython != "" {
		return tagPython
	}

	// Tier 2: Fetch from runtime versions JSON
	if v := airflowversions.GetDefaultPythonVersion(baseTag); v != "" {
		return v
	}

	// Tier 3: AF2 runtime-major lookup
	if v, ok := af2RuntimePythonDefaults[airflowversions.RuntimeVersionMajor(baseTag)]; ok {
		return v
	}

	// Tier 4: Hardcoded fallback
	return defaultPythonVersion
}

// Standalone implements ContainerHandler using `airflow standalone` instead of Docker Compose.
type Standalone struct {
	airflowHome         string
	envFile             string
	dockerfile          string
	foreground          bool   // if true, run in foreground (stream output, block on Wait)
	noBrowser           bool   // if true, don't open the browser after startup
	port                string // webserver port; defaults to defaultStandalonePort
	useProxy            bool   // whether the reverse proxy is active
	proxyHostname       string // e.g. "my-project.localhost"
	proxyPort           string // proxy listener port (default 6563)
	airflowMajorVersion string // "2" or "3", determined from the runtime tag at Start()
}

// StandaloneInit creates a new Standalone handler.
func StandaloneInit(airflowHome, envFile, dockerfile string) (*Standalone, error) {
	return &Standalone{
		airflowHome:         airflowHome,
		envFile:             envFile,
		dockerfile:          dockerfile,
		airflowMajorVersion: readPersistedVersion(airflowHome),
	}, nil
}

func versionFilePath(airflowHome string) string {
	return filepath.Join(airflowHome, standaloneDir, standaloneVersionFile)
}

// persistVersion writes the Airflow major version ("2" or "3") to
// .astro/standalone/airflow_version so subcommands that create a fresh
// Standalone via StandaloneInit can recover it without re-parsing.
func (s *Standalone) persistVersion() {
	dir := filepath.Join(s.airflowHome, standaloneDir)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		logger.Debugf("Warning: could not create standalone dir for version file: %v", err)
		return
	}
	if err := os.WriteFile(versionFilePath(s.airflowHome), []byte(s.airflowMajorVersion), filePermissions); err != nil {
		logger.Debugf("Warning: could not persist airflow version: %v", err)
	}
}

// readPersistedVersion reads the Airflow major version from the version file.
// Falls back to parsing the Dockerfile if the file doesn't exist.
func readPersistedVersion(airflowHome string) string {
	data, err := os.ReadFile(versionFilePath(airflowHome))
	if err == nil {
		v := strings.TrimSpace(string(data))
		if v == "2" || v == "3" {
			return v
		}
	}
	return detectVersionFromDockerfile(airflowHome)
}

// detectVersionFromDockerfile parses the Dockerfile to extract the runtime
// tag and derive the Airflow major version.  Returns "" if anything fails.
func detectVersionFromDockerfile(airflowHome string) string {
	cmds, err := standaloneParseFile(filepath.Join(airflowHome, "Dockerfile"))
	if err != nil {
		return ""
	}
	_, tag := standaloneGetImageTag(cmds)
	if tag == "" {
		return ""
	}
	baseTag, _ := parseRuntimeTagPython(tag)
	return airflowversions.AirflowMajorVersionForRuntimeVersion(baseTag)
}

// webserverPort returns the configured port. It checks (in order):
// 1. Explicit --port flag
// 2. api-server.port (AF3) or webserver.port (AF2) from .astro/config.yaml
// 3. Default (8080)
func (s *Standalone) webserverPort() string {
	if s.port != "" {
		return s.port
	}
	if s.airflowMajorVersion == "3" {
		if p := config.CFG.APIServerPort.GetString(); p != "" && p != "0" {
			return p
		}
	} else {
		if p := config.CFG.WebserverPort.GetString(); p != "" && p != "0" {
			return p
		}
	}
	return defaultStandalonePort
}

// airflowMajorVersionUint returns the Airflow major version as a uint64 for settings APIs.
func (s *Standalone) airflowMajorVersionUint() uint64 {
	const (
		airflowV2 = 2
		airflowV3 = 3
	)
	if s.airflowMajorVersion == "2" {
		return airflowV2
	}
	return airflowV3
}

func (s *Standalone) pidFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standalonePIDFile)
}

func (s *Standalone) logFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standaloneLogFile)
}

// Start runs airflow standalone locally without Docker.
//
//nolint:gocognit,gocyclo
func (s *Standalone) Start(opts *types.StartOptions) error {
	settingsFile := opts.SettingsFile
	waitTime := opts.WaitTime
	envConns := opts.EnvConns
	s.foreground = opts.Foreground
	s.noBrowser = opts.NoBrowser
	if opts.Port != "" {
		s.port = opts.Port
	}
	useProxy := !opts.NoProxy

	fmt.Println(ansi.Bold("Note:") + " Standalone mode is experimental. Report issues at https://github.com/astronomer/astro-cli/issues")
	fmt.Println()

	// 1. Parse Dockerfile to get runtime image + tag
	cmds, err := standaloneParseFile(filepath.Join(s.airflowHome, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("error parsing Dockerfile: %w", err)
	}
	_, tag := standaloneGetImageTag(cmds)
	if tag == "" {
		return errors.New("could not determine runtime version from Dockerfile")
	}

	baseTag, tagPython := parseRuntimeTagPython(tag)

	// 2. Validate Airflow version (AF2 or AF3).
	// If the tag isn't a pinned runtime version (X.Y-Z or semver), try to resolve it
	// as a floating tag (e.g., "3.1" → "3.1-12") via the runtime versions JSON.
	if !airflowrt.IsValidRuntimeTag(baseTag) {
		resolved, resolveErr := resolveFloatingTag(baseTag)
		if resolveErr == nil {
			baseTag = resolved
		} else if airflowversions.AirflowMajorVersionForRuntimeVersion(baseTag) == "" {
			return fmt.Errorf("could not determine runtime version from Dockerfile image tag '%s'.\nStandalone mode requires a pinned Astronomer Runtime image (e.g., astro-runtime:3.1-12 or astro-runtime:13.5.0)", tag)
		}
	}
	afMajor := airflowversions.AirflowMajorVersionForRuntimeVersion(baseTag)
	if afMajor == "2" {
		rtMajor := airflowversions.RuntimeVersionMajor(baseTag)
		// Allow-list of AF2 runtime major versions supported by standalone mode.
		// When a future AF2 runtime version ships, add its major version here
		// and update the error message below so it doesn't silently fall through
		// to errUnsupportedAirflowVersion.
		if rtMajor != "11" && rtMajor != "12" && rtMajor != "13" {
			return fmt.Errorf("standalone mode supports Airflow 2 runtime versions 11.x through 13.x, got %s", baseTag)
		}
	} else if afMajor != "3" {
		return errUnsupportedAirflowVersion
	}
	s.airflowMajorVersion = afMajor
	s.persistVersion()

	pythonVersion := resolvePythonVersion(baseTag, tagPython)

	// 3. Check uv is on PATH
	_, err = lookPath("uv")
	if err != nil {
		return errUVNotFound
	}

	// 3b. In background mode, bail early if already running (before any install work)
	if !s.foreground {
		if pid, alive := s.readPID(); alive {
			return fmt.Errorf("standalone Airflow is already running (PID %d). Run 'astro dev stop' first", pid)
		}
	}

	// 3c. Determine port: try default port first, allocate random only if taken
	var proxyHostname, proxyPort string
	if useProxy {
		proxyPort = config.CFG.ProxyPort.GetString()
		if proxyPort == "" {
			proxyPort = proxy.DefaultPort
		}

		hostname, hErr := proxy.DeriveHostname(s.airflowHome)
		if hErr != nil {
			// Fall back to non-proxy mode if hostname derivation fails
			useProxy = false
		} else {
			proxyHostname = hostname

			// Only allocate a port if no explicit --port was set
			if opts.Port == "" {
				defaultPort := s.webserverPort()
				if !proxy.IsPortAvailable(defaultPort) {
					allocatedPort, aErr := proxy.AllocatePort()
					if aErr != nil {
						return fmt.Errorf("error allocating webserver port: %w", aErr)
					}
					s.port = allocatedPort
				}
				// else: keep defaultPort as-is
			}
		}
	}

	// 3d. Check if the port is already in use by another process.
	// airflow standalone doesn't crash when the port is taken — only the
	// api-server subprocess fails — so the health check would pass against
	// whichever service already occupies the port, misleading the user.
	port := s.webserverPort()
	if err := checkPortAvailable(port); err != nil {
		return err
	}

	s.proxyHostname = proxyHostname
	s.proxyPort = proxyPort
	s.useProxy = useProxy

	// 4. Fetch constraints and freeze files from CDN (cached locally)
	freezePath, airflowVersion, taskSDKVersion, err := s.getConstraints(baseTag, pythonVersion)
	if err != nil {
		return err
	}

	sp := spinner.NewSpinner("Setting up standalone environment…")
	sp.Start()

	// 5. Create venv
	err = runCommand(s.airflowHome, "uv", "venv", "--python", pythonVersion, "--allow-existing")
	if err != nil {
		sp.Stop()
		return fmt.Errorf("error creating virtual environment: %w", err)
	}

	// 6. Install dependencies (2-step install)
	// --python explicitly targets the project venv so uv never installs into
	// a parent venv even if VIRTUAL_ENV leaks through the environment.
	venvPython := filepath.Join(s.airflowHome, ".venv", "bin", "python")

	// Step 1: Install airflow with full freeze constraints (reproduces runtime env exactly)
	installArgs := []string{
		"pip", "install",
		"--python", venvPython,
		fmt.Sprintf("apache-airflow==%s", airflowVersion),
		"-c", freezePath,
		"--index-url", standaloneIndexURL,
	}
	err = runCommand(s.airflowHome, "uv", installArgs...)
	if err != nil {
		sp.Stop()
		return fmt.Errorf("error installing dependencies: %w", err)
	}

	// Step 2: Install user requirements with only airflow/sdk version locks
	requirementsPath := filepath.Join(s.airflowHome, "requirements.txt")
	if exists, _ := fileutil.Exists(requirementsPath, nil); exists {
		userInstallArgs := []string{
			"pip", "install",
			"--python", venvPython,
			"-r", requirementsPath,
			fmt.Sprintf("apache-airflow==%s", airflowVersion),
		}
		if taskSDKVersion != "" {
			userInstallArgs = append(userInstallArgs, fmt.Sprintf("apache-airflow-task-sdk==%s", taskSDKVersion))
		}
		userInstallArgs = append(userInstallArgs, "--index-url", standaloneIndexURL)
		err = runCommand(s.airflowHome, "uv", userInstallArgs...)
		if err != nil {
			sp.Stop()
			return fmt.Errorf("error installing user requirements: %w", err)
		}
	}

	spinner.StopWithCheckmark(sp, "Environment ready")

	// 7. Build environment
	env := s.buildEnv()

	// 9. Start airflow standalone
	fmt.Println("\nStarting Airflow in standalone mode…")

	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")
	airflowBin := filepath.Join(venvBin, "airflow")

	var cmd *exec.Cmd
	if s.airflowMajorVersion == "2" && runtime.GOOS == osDarwin {
		// On macOS, AF2's gunicorn webserver forks workers which inherit
		// corrupted ObjC runtime state → SIGSEGV.  Use a shim that runs
		// `airflow webserver --debug` (Flask dev server, no fork).
		shimPath := filepath.Join(venvBin, "_standalone_macos.py")
		if err := os.WriteFile(shimPath, af2DarwinShim, filePermissions); err != nil {
			return fmt.Errorf("error writing macOS standalone shim: %w", err)
		}

		// Disable os.fork and patch setproctitle on macOS.  After fork()
		// the ObjC/CF/Network runtimes are corrupt, causing children to
		// spin at 100 % CPU on getaddrinfo or setproctitle calls.
		// Without this patch, AF2 on macOS will hit SIGSEGV or 100% CPU spin.
		if err := writeDarwinForkSafetyPatch(filepath.Join(s.airflowHome, ".venv")); err != nil {
			return fmt.Errorf("could not write Darwin fork-safety patch (AF2 on macOS requires this to avoid SIGSEGV): %w", err)
		}

		// Write the LocalExecutor pickle fix plugin so QueuedLocalWorker
		// can be serialized across macOS spawn-mode multiprocessing.
		pluginsDir := filepath.Join(s.airflowHome, "plugins")
		if err := os.MkdirAll(pluginsDir, dirPermissions); err != nil {
			return fmt.Errorf("could not create plugins directory for LocalExecutor pickle fix: %w", err)
		}
		pickleFix := filepath.Join(pluginsDir, "fix_local_executor_pickle.py")
		if _, err := os.Stat(pickleFix); os.IsNotExist(err) {
			if err := os.WriteFile(pickleFix, af2PickleFixPlugin, filePermissions); err != nil {
				return fmt.Errorf("could not write LocalExecutor pickle fix plugin (AF2 on macOS requires this): %w", err)
			}
		}

		pythonBin := filepath.Join(venvBin, "python")
		cmd = exec.Command(pythonBin, shimPath) //nolint:gosec
	} else {
		cmd = exec.Command(airflowBin, "standalone") //nolint:gosec
	}
	cmd.Dir = s.airflowHome
	cmd.Env = env
	// Start the subprocess in its own process group so we can kill the entire
	// tree (scheduler, triggerer, api-server, etc.) when the user sends Ctrl+C.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if s.foreground {
		return s.startForeground(cmd, waitTime, settingsFile, envConns)
	}
	return s.startBackground(cmd, waitTime, settingsFile, envConns)
}

// startForeground runs the airflow process in the foreground, streaming output to the terminal.
func (s *Standalone) startForeground(cmd *exec.Cmd, waitTime time.Duration, settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	err = startCommand(cmd)
	if err != nil {
		return fmt.Errorf("error starting airflow standalone: %w", err)
	}

	// Forward signals to the entire process group so child processes
	// (scheduler, triggerer, api-server, etc.) are also terminated.
	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			if cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) //nolint:errcheck
			}
		case <-done:
		}
	}()
	defer func() {
		signal.Stop(sigChan)
		close(done)
	}()

	var wg sync.WaitGroup
	wg.Add(2) //nolint:mnd
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintln(os.Stderr, scanner.Text())
		}
	}()

	// Run health check in background
	healthURL, healthComp := s.healthEndpoint()
	go func() {
		err := checkWebserverHealth(healthURL, waitTime, healthComp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s\n", err.Error())
			return
		}
		// Apply settings now that Airflow is running and the DB is initialized
		if err := s.applySettings(settingsFile, envConns); err != nil {
			fmt.Printf("Warning: could not apply airflow settings: %s\n", err.Error())
		}
		bullet := ansi.Cyan("\u27A4") + " "
		uiURL := "http://localhost:" + s.webserverPort()

		// Register proxy route in foreground mode
		if proxyURL := s.registerProxyRoute(cmd.Process.Pid); proxyURL != "" {
			uiURL = proxyURL
		}

		fmt.Println("\n" + ansi.Green("\u2714") + " Airflow is ready!")
		fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
		fmt.Println()

		if !(s.noBrowser || util.CheckEnvBool(os.Getenv("ASTRONOMER_NO_BROWSER"))) {
			if err := standaloneOpenURL(uiURL); err != nil {
				fmt.Println("Unable to open the Airflow UI, please visit the following link: " + uiURL)
			}
		}
	}()

	wg.Wait()
	err = cmd.Wait()
	if err != nil {
		// If the process was killed by a signal (e.g. Ctrl+C), don't treat it as an error
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == -1 {
				fmt.Println("\nAirflow standalone stopped.")
				return nil
			}
		}
		return fmt.Errorf("airflow standalone exited with error: %w", err)
	}

	fmt.Println("\nAirflow standalone stopped.")
	return nil
}

// startBackground runs the airflow process in the background, writes a PID file,
// runs the health check, and returns.
func (s *Standalone) startBackground(cmd *exec.Cmd, waitTime time.Duration, settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection) error {
	logPath := s.logFilePath()
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("error creating log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = startCommand(cmd)
	if err != nil {
		return fmt.Errorf("error starting airflow standalone: %w", err)
	}

	// Reap the child process when it exits to prevent zombies.  In normal
	// usage the CLI exits right after Start() returns and the child is
	// reparented to init, but during tests (or if the parent stays alive)
	// an un-reaped child would appear alive to signal-0 checks in Stop().
	go cmd.Wait() //nolint:errcheck

	err = os.WriteFile(s.pidFilePath(), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), filePermissions)
	if err != nil {
		// Kill the process if we can't write the PID file
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) //nolint:errcheck
		return fmt.Errorf("error writing PID file: %w", err)
	}
	// Run health check (blocking — wait for healthy or timeout)
	healthURL, healthComp := s.healthEndpoint()
	err = checkWebserverHealth(healthURL, waitTime, healthComp)
	if err != nil {
		return fmt.Errorf("airflow did not become healthy: %w", err)
	}

	// Apply settings now that Airflow is running and the DB is initialized
	if err := s.applySettings(settingsFile, envConns); err != nil {
		fmt.Printf("Warning: could not apply airflow settings: %s\n", err.Error())
	}

	bullet := ansi.Cyan("\u27A4") + " "
	uiURL := "http://localhost:" + s.webserverPort()

	// Register proxy route and show proxy URL
	if proxyURL := s.registerProxyRoute(cmd.Process.Pid); proxyURL != "" {
		uiURL = proxyURL
	}

	fmt.Printf("\n%s Airflow is ready! (PID %d)\n", ansi.Green("\u2714"), cmd.Process.Pid)
	fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
	fmt.Printf("%sView logs: %s\n", bullet, ansi.Bold("astro dev logs -f"))
	fmt.Printf("%sStop:      %s\n", bullet, ansi.Bold("astro dev stop"))

	if !(s.noBrowser || util.CheckEnvBool(os.Getenv("ASTRONOMER_NO_BROWSER"))) {
		if err := standaloneOpenURL(uiURL); err != nil {
			fmt.Println("\nUnable to open the Airflow UI, please visit the following link: " + uiURL)
		}
	}

	return nil
}

// registerProxyRoute ensures the proxy daemon is running and registers a route
// for this standalone instance. Returns the proxy URL on success, or "" if the
// proxy is disabled or registration failed.
func (s *Standalone) registerProxyRoute(pid int) string {
	if !s.useProxy || s.proxyHostname == "" {
		return ""
	}
	if _, err := proxy.EnsureRunning(s.proxyPort); err != nil {
		fmt.Printf("Warning: could not start proxy: %s\n", err.Error())
		return ""
	}
	route := &proxy.Route{
		Hostname:   s.proxyHostname,
		Port:       s.webserverPort(),
		ProjectDir: s.airflowHome,
		PID:        pid,
	}
	if err := proxy.AddRoute(route); err != nil {
		fmt.Printf("Warning: could not register proxy route: %s\n", err.Error())
		return ""
	}
	return fmt.Sprintf("http://%s:%s", s.proxyHostname, s.proxyPort)
}

// healthEndpoint returns the health check URL and component name.
func (s *Standalone) healthEndpoint() (url, component string) {
	port := s.webserverPort()
	if s.airflowMajorVersion == "3" {
		return "http://localhost:" + port + "/api/v2/monitor/health", "api-server"
	}
	return "http://localhost:" + port + "/health", "webserver"
}

// checkPortAvailable delegates to airflowrt.CheckPortAvailable.
var checkPortAvailable = airflowrt.CheckPortAvailable

// getConstraints fetches pip constraints and freeze files from the CDN.
// Delegates to airflowrt.FetchConstraints for the actual fetch + cache logic.
func (s *Standalone) getConstraints(tag, pythonVersion string) (freezePath, airflowVersion, taskSDKVersion string, err error) {
	result, err := airflowrt.FetchConstraints(s.airflowHome, tag, pythonVersion)
	if err != nil {
		return "", "", "", err
	}
	return result.FreezePath, result.AirflowVersion, result.TaskSDKVersion, nil
}

// buildEnv constructs the environment variables for the standalone process.
func (s *Standalone) buildEnv() []string {
	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")

	// Point AIRFLOW_HOME at .astro/standalone/ so all Airflow-generated files
	// (airflow.cfg, airflow.db, logs/) land there rather than in the project root.
	// DAGS_FOLDER is pinned back to the project root so DAGs are still discovered.
	standaloneHome := filepath.Join(s.airflowHome, standaloneDir)

	// Layer 1: Load .env file if it exists — user-specified env vars that
	// override inherited env but NOT standalone-critical settings.
	overrides := map[string]string{}
	envFilePath := s.envFile
	if envFilePath == "" {
		envFilePath = filepath.Join(s.airflowHome, ".env")
	}
	if envVars, err := airflowrt.LoadEnvFile(envFilePath); err == nil {
		for _, kv := range envVars {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				overrides[kv[:idx]] = kv[idx+1:]
			}
		}
	}

	// Layer 2: Standalone-critical settings — these MUST take precedence over
	// both inherited env and .env to prevent standalone mode from breaking.
	overrides["PATH"] = fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH"))
	// Add include/ to PYTHONPATH so user modules are importable — mirrors
	// the Docker image which bakes /usr/local/airflow/include into PYTHONPATH.
	includePath := filepath.Join(s.airflowHome, "include")
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		overrides["PYTHONPATH"] = fmt.Sprintf("%s:%s", includePath, existing)
	} else {
		overrides["PYTHONPATH"] = includePath
	}
	overrides["AIRFLOW_HOME"] = standaloneHome
	overrides["ASTRONOMER_ENVIRONMENT"] = "local"
	overrides["AIRFLOW__CORE__LOAD_EXAMPLES"] = "False"
	overrides["AIRFLOW__CORE__DAGS_FOLDER"] = filepath.Join(s.airflowHome, "dags")
	port := s.webserverPort()
	if s.airflowMajorVersion == "3" {
		overrides["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS"] = "True"
		if port != defaultStandalonePort {
			overrides["AIRFLOW__API__PORT"] = port
		}
		overrides["AIRFLOW__CORE__EXECUTION_API_SERVER_URL"] = "http://localhost:" + port + "/execution/"
	} else {
		// AF2: point plugins at the project directory so the pickle-fix plugin
		// written during Start() is visible to Airflow.  AF3 intentionally
		// omits this override and falls back to $AIRFLOW_HOME/plugins
		// (.astro/standalone/plugins).
		overrides["AIRFLOW__CORE__PLUGINS_FOLDER"] = filepath.Join(s.airflowHome, "plugins")
		if port != defaultStandalonePort {
			overrides["AIRFLOW__WEBSERVER__WEB_SERVER_PORT"] = port
		}
		// On Linux, limit gunicorn to 1 worker as a defensive default for
		// local development.  On macOS the shim replaces gunicorn entirely
		// so these are not needed.
		if runtime.GOOS != osDarwin {
			overrides["AIRFLOW__WEBSERVER__WORKERS"] = "1"
		}
		// Use LocalExecutor so the scheduler can heartbeat while tasks run.
		// SQLite is fine for local dev; the skip-check flag allows it.
		// execute_tasks_new_python_interpreter runs each task as a fresh
		// subprocess instead of using multiprocessing fork/spawn.
		overrides["AIRFLOW__CORE__EXECUTOR"] = "LocalExecutor"
		overrides["_AIRFLOW__SKIP_DATABASE_EXECUTOR_COMPATIBILITY_CHECK"] = "1"
		overrides["AIRFLOW__CORE__EXECUTE_TASKS_NEW_PYTHON_INTERPRETER"] = "True"
	}

	// Layer 3: macOS fork-safety workarounds.
	// a) Python's _scproxy calls SCDynamicStoreCopyProxies which is not fork-safe.
	//    When Airflow forks, this can spin at 100% CPU indefinitely.
	//    Setting NO_PROXY=* tells Python to skip _scproxy entirely.
	//    We only do this when no proxy is configured so corporate proxy users
	//    aren't affected.
	if !airflowrt.HasProxyConfigured(overrides) {
		overrides["NO_PROXY"] = "*"
		overrides["no_proxy"] = "*"
		logger.Debugf("No proxy detected — setting NO_PROXY=* to avoid macOS _scproxy hang")
	}

	// Start with inherited env, filtering out keys we override.
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			if _, overridden := overrides[kv[:idx]]; overridden {
				continue
			}
		}
		env = append(env, kv)
	}

	for k, v := range overrides {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

// standaloneAPIURL returns the versioned API URL for the running standalone instance.
// AF2 exposes /api/v1, AF3 exposes /api/v2.
func (s *Standalone) standaloneAPIURL(port string) string {
	if s.airflowMajorVersion == "2" {
		return fmt.Sprintf("http://localhost:%s/api/v1", port)
	}
	return fmt.Sprintf("http://localhost:%s/api/v2", port)
}

// standaloneAuthHeader returns the Authorization header for the running standalone instance.
// AF2 uses Basic auth (admin:admin); AF3 uses JWT from /auth/token.
func (s *Standalone) standaloneAuthHeader(port string) string {
	if s.airflowMajorVersion == "2" {
		// Hardcoded to admin:admin, which matches the --password admin flag passed
		// by af2DarwinShim and the default AF2 user created during Start().
		// If the user changes the admin password after first start, settings
		// import/export will silently fail against the real credentials.
		// This is acceptable for local dev but means credentials are coupled.
		return "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:admin"))
	}
	token, err := fetchAirflowJWTToken(fmt.Sprintf("http://localhost:%s", port))
	if err != nil {
		logger.Debugf("Unable to fetch Airflow auth token for standalone: %s", err)
		return ""
	}
	return token
}

// applySettings imports airflow_settings.yaml using the Airflow REST API.
func (s *Standalone) applySettings(settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection) error {
	settingsExists, err := fileutil.Exists(settingsFile, nil)
	if err != nil || !settingsExists {
		if len(envConns) == 0 {
			return nil
		}
	}

	port := s.webserverPort()
	apiURL := s.standaloneAPIURL(port)
	authHeader := s.standaloneAuthHeader(port)
	return settings.ConfigSettings(apiURL, authHeader, settingsFile, envConns, true, true, true)
}

// standaloneExecAirflowCommand runs an airflow command via the local venv.
func (s *Standalone) standaloneExecAirflowCommand(_, command string) (string, error) {
	env := s.buildEnv()

	// Use system bash — Python venvs don't include bash; venv bin/ is on PATH.
	cmd := exec.Command("bash", "-c", command) //nolint:gosec
	cmd.Dir = s.airflowHome
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("error running airflow command: %w", err)
	}
	return string(out), nil
}

// readPID delegates to airflowrt.ReadPID.
func (s *Standalone) readPID() (int, bool) {
	return airflowrt.ReadPID(s.pidFilePath())
}

// Stop terminates the standalone Airflow process.
func (s *Standalone) Stop(_ bool) error {
	// Deregister proxy route
	s.removeProxyRoute()

	pid, alive := s.readPID()
	if pid == 0 {
		fmt.Println("No standalone Airflow process found.")
		return nil
	}

	if !alive {
		// Stale PID file — clean up
		os.Remove(s.pidFilePath())
		fmt.Println("No standalone Airflow process found (cleaned up stale PID file).")
		return nil
	}

	fmt.Printf("Stopping Airflow standalone (PID %d)…\n", pid)
	syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck

	// Poll for process exit
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollInterval)
		if _, stillAlive := s.readPID(); !stillAlive {
			break
		}
	}

	// If still alive, send SIGKILL
	if _, stillAlive := s.readPID(); stillAlive {
		syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
		time.Sleep(stopPollInterval)
	}

	os.Remove(s.pidFilePath())
	fmt.Println("Airflow standalone stopped.")
	return nil
}

// removeProxyRoute deregisters the proxy route for this project and
// stops the proxy daemon if no routes remain.
func (s *Standalone) removeProxyRoute() {
	hostname, err := proxy.DeriveHostname(s.airflowHome)
	if err != nil {
		logger.Debugf("could not derive proxy hostname: %s", err)
		return
	}
	remaining, err := proxy.RemoveRoute(hostname)
	if err != nil {
		logger.Debugf("could not remove proxy route for %s: %s", hostname, err)
		return
	}
	if remaining == 0 {
		proxy.StopIfEmpty()
	}
}

// Kill stops a running process (if any) and cleans up standalone state files.
func (s *Standalone) Kill() error {
	s.Stop(false) //nolint:errcheck

	sp := spinner.NewSpinner("Cleaning up standalone environment…")
	sp.Start()
	defer sp.Stop()

	pathsToRemove := []string{
		filepath.Join(s.airflowHome, ".venv"),
		filepath.Join(s.airflowHome, standaloneDir),
	}

	for _, p := range pathsToRemove {
		if exists, _ := fileutil.Exists(p, nil); exists {
			os.RemoveAll(p)
		}
	}

	spinner.StopWithCheckmark(sp, "Standalone environment cleaned up")
	return nil
}

// PS reports the status of the standalone Airflow process.
// PS returns structured process status data
func (s *Standalone) PS() (*types.PSStatus, error) {
	pid, alive := s.readPID()
	status := &types.PSStatus{
		Mode:    "standalone",
		Running: &alive,
	}
	if alive {
		status.PID = &pid
	}
	return status, nil
}

// standaloneLogPrefixMaps maps Docker container names (passed by the cmd layer)
// to the log prefixes used by `airflow standalone`, keyed by Airflow major version.
var standaloneLogPrefixMaps = map[string]map[string]string{
	"3": {
		WebserverDockerContainerName:    "api-server",
		APIServerDockerContainerName:    "api-server",
		SchedulerDockerContainerName:    "scheduler",
		TriggererDockerContainerName:    "triggerer",
		DAGProcessorDockerContainerName: "dag-processor",
	},
	"2": {
		WebserverDockerContainerName: "webserver",
		SchedulerDockerContainerName: "scheduler",
		TriggererDockerContainerName: "triggerer",
	},
}

// buildLogPrefixes converts Docker container names to standalone log prefix
// filters. Returns nil when no filtering should be applied.
func buildLogPrefixes(containerNames []string, airflowMajorVersion string) []string {
	if len(containerNames) == 0 {
		return nil
	}
	prefixMap := standaloneLogPrefixMaps[airflowMajorVersion]
	if prefixMap == nil {
		prefixMap = standaloneLogPrefixMaps["3"]
	}
	seen := map[string]bool{}
	var prefixes []string
	for _, name := range containerNames {
		if p, ok := prefixMap[name]; ok && !seen[p] {
			seen[p] = true
			prefixes = append(prefixes, p+" ")
		}
	}
	return prefixes
}

// matchesLogPrefix returns true if the line starts with any of the given prefixes,
// or if prefixes is nil (no filtering).
func matchesLogPrefix(line string, prefixes []string) bool {
	if prefixes == nil {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// Logs streams the standalone Airflow log file.
// When containerNames are provided (via --scheduler, --api-server, etc. flags),
// only lines matching those components are shown.
func (s *Standalone) Logs(follow bool, containerNames ...string) error {
	logPath := s.logFilePath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s — has standalone been started?", logPath)
	}

	prefixes := buildLogPrefixes(containerNames, s.airflowMajorVersion)

	if !follow {
		data, err := osReadFile(logPath)
		if err != nil {
			return fmt.Errorf("error reading log file: %w", err)
		}
		for _, line := range strings.SplitAfter(string(data), "\n") {
			if line != "" && matchesLogPrefix(line, prefixes) {
				fmt.Print(line)
			}
		}
		return nil
	}

	// Follow mode: read existing content then poll for new data
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	// Set up signal handling so Ctrl+C exits cleanly
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for {
		line, err := reader.ReadString('\n')
		if line != "" && matchesLogPrefix(line, prefixes) {
			fmt.Print(line)
		}
		if err != nil {
			// At EOF, poll for new data
			select {
			case <-sigChan:
				return nil
			case <-time.After(stopPollInterval):
				continue
			}
		}
	}
}

// ensureVenv validates the venv exists before running commands.
func (s *Standalone) ensureVenv() error {
	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")
	if _, err := os.Stat(venvBin); os.IsNotExist(err) {
		return fmt.Errorf("no virtual environment found — run 'astro dev start' first")
	}
	return nil
}

// standaloneExecDefault delegates to airflowrt.ExecWithEnv.
func standaloneExecDefault(dir string, env, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return airflowrt.ExecWithEnv(dir, env, args, stdin, stdout, stderr)
}

func (s *Standalone) Build(_, _ string, _ bool) error {
	return errors.New("astro dev build builds a Docker image and is not available in standalone mode")
}

// Run executes an arbitrary command in the Airflow venv environment.
// The args slice already contains the full command (e.g. ["airflow", "dags", "list"]).
func (s *Standalone) Run(args []string, _ string) error {
	if err := s.ensureVenv(); err != nil {
		return err
	}
	env := s.buildEnv()
	return standaloneExec(s.airflowHome, env, args, os.Stdin, os.Stdout, os.Stderr)
}

// Bash opens an interactive shell with the Airflow venv environment.
func (s *Standalone) Bash(_ string) error {
	if err := s.ensureVenv(); err != nil {
		return err
	}
	env := s.buildEnv()
	return standaloneExec(s.airflowHome, env, []string{"bash"}, os.Stdin, os.Stdout, os.Stderr)
}

func (s *Standalone) RunDAG(_, _, _, _ string, _, _ bool) error {
	return errStandaloneNotSupported
}

// ImportSettings imports connections/variables/pools from the settings file.
func (s *Standalone) ImportSettings(settingsFile, _ string, connections, variables, pools bool) error {
	// Verify standalone is actually running before attempting to import.
	if _, alive := s.readPID(); !alive {
		return errors.New("standalone Airflow is not running, run astro dev start --standalone to start it")
	}

	if !connections && !variables && !pools {
		connections = true
		variables = true
		pools = true
	}

	fileState, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return fmt.Errorf("error looking for settings file: %w", err)
	}
	if !fileState {
		return errors.New("file specified does not exist")
	}

	// If proxy mode allocated a random port, the actual port is stored in
	// the proxy route registered during Start. Fall back to config default.
	port := s.webserverPort()
	if route, rerr := proxy.GetRouteByProject(s.airflowHome); rerr == nil && route != nil && route.Port != "" {
		port = route.Port
	}

	apiURL := s.standaloneAPIURL(port)
	authHeader := s.standaloneAuthHeader(port)

	err = settings.ConfigSettings(apiURL, authHeader, settingsFile, nil, connections, variables, pools)
	if err != nil {
		return err
	}
	fmt.Println("\nAirflow objects created from settings file")
	return nil
}

// ExportSettings exports connections/variables/pools to a settings file or .env file.
func (s *Standalone) ExportSettings(settingsFile, envFile string, connections, variables, pools, envExport bool) error {
	if !connections && !variables && !pools {
		connections = true
		variables = true
		pools = true
	}

	origExec := settings.SetExecAirflowCommand(s.standaloneExecAirflowCommand)
	defer settings.SetExecAirflowCommand(origExec)

	afVersion := s.airflowMajorVersionUint()

	if envExport {
		err := settings.EnvExport("standalone", envFile, afVersion, connections, variables)
		if err != nil {
			return err
		}
		fmt.Println("\nAirflow objects exported to env file")
		return nil
	}

	fileState, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return fmt.Errorf("error looking for settings file: %w", err)
	}
	if !fileState {
		return errors.New("file specified does not exist")
	}

	err = settings.Export("standalone", settingsFile, afVersion, connections, variables, pools)
	if err != nil {
		return err
	}
	fmt.Println("\nAirflow objects exported to settings file")
	return nil
}

func (s *Standalone) ComposeExport(_, _ string) error {
	return errors.New("astro dev compose-export is not available in standalone mode")
}

// Pytest runs pytest on DAGs using the local venv.
func (s *Standalone) Pytest(pytestFile, _, _, pytestArgsString, _ string) (string, error) {
	if err := s.ensureVenv(); err != nil {
		return "", err
	}
	env := s.buildEnv()

	// Resolve pytest file path (same logic as Docker handler)
	if pytestFile != DefaultTestPath {
		if !strings.Contains(pytestFile, "tests") {
			pytestFile = "tests/" + pytestFile
		}
	}

	args := []string{"pytest", pytestFile}
	if pytestArgsString != "" {
		args = append(args, strings.Fields(pytestArgsString)...)
	}

	err := standaloneExec(s.airflowHome, env, args, nil, os.Stdout, os.Stderr)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("%d", exitErr.ExitCode()), errors.New("something went wrong while Pytesting your DAGs")
		}
		return "", err
	}
	return "", nil
}

// Parse validates DAGs by running the default integrity test.
func (s *Standalone) Parse(_, _, _ string) error {
	path := filepath.Join(s.airflowHome, DefaultTestPath)

	fileExist, err := fileutil.Exists(path, nil)
	if err != nil {
		return err
	}
	if !fileExist {
		fmt.Println("\nThe file " + path + " which is needed for `astro dev parse` does not exist. Please run `astro dev init` to create it")
		return nil
	}

	fmt.Println("Checking your DAGs for errors…")

	exitCode, err := s.Pytest(DefaultTestPath, "", "", "", "")
	if err != nil {
		if strings.Contains(exitCode, "1") {
			return errors.New("See above for errors detected in your DAGs")
		}
		return errors.Wrap(err, "something went wrong while parsing your DAGs")
	}
	fmt.Println(ansi.Green("\u2714") + " No errors detected in your DAGs ")
	return nil
}

func (s *Standalone) UpgradeTest(_, _, _, _ string, _, _, _, _, _ bool, _ string, _ astrov1.ClientWithResponsesInterface) error {
	return errors.New("astro dev upgrade-test is not available in standalone mode")
}

// execCommand runs a command in the given directory.
// Output is suppressed unless verbose (debug) logging is enabled.
func execCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec
	cmd.Dir = dir
	if logger.IsLevelEnabled(logrus.DebugLevel) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// startCmd starts a command without waiting for it to finish.
func startCmd(cmd *exec.Cmd) error {
	return cmd.Start()
}
