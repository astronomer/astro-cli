package airflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/astronomer/astro-cli/settings"
	"github.com/pkg/errors"
)

const (
	standaloneDir           = ".astro/standalone"
	standalonePIDFile       = "airflow.pid"
	standaloneLogFile       = "airflow.log"
	defaultStandalonePort   = "8080"
	standaloneIndexURL      = "https://pip.astronomer.io/v2/"
	defaultPythonVersion    = "3.12" // default Python version for all Runtime 3.x images
	constraintsBaseURL      = "https://cdn.astronomer.io/runtime-constraints"
	freezeBaseURL           = "https://cdn.astronomer.io/runtime-freeze"
	stopPollInterval        = 500 * time.Millisecond
	stopTimeout             = 10 * time.Second
	filePermissions         = os.FileMode(0o644)
	dirPermissions          = os.FileMode(0o755)
	standaloneAdminUser     = "admin"
	standaloneAdminPassword = "admin"
	standalonePasswordsFile = "simple_auth_manager_passwords.json.generated" //nolint:gosec
)

var (
	errStandaloneNotSupported    = errors.New("this command is not supported in standalone mode")
	errUnsupportedAirflowVersion = errors.New("standalone mode requires Airflow 3 (runtime 3.x)")
	errUVNotFound                = errors.New("'uv' is required for standalone mode but was not found on PATH.\nInstall it with: curl -LsSf https://astral.sh/uv/install.sh | sh\nSee https://docs.astral.sh/uv/getting-started/installation/ for more options")

	// Function variables for testing
	lookPath              = exec.LookPath
	standaloneParseFile   = docker.ParseFile
	standaloneGetImageTag = docker.GetImageTagFromParsedFile
	runCommand            = execCommand
	startCommand          = startCmd
	standaloneExec        = standaloneExecDefault
	osReadFile            = os.ReadFile
	osFindProcess         = os.FindProcess
)

// runtimePythonRe matches the optional -python-X.Y (and optional -base) suffix on a runtime tag.
var runtimePythonRe = regexp.MustCompile(`-python-(\d+\.\d+)(-base)?$`)

// parseRuntimeTagPython extracts the base runtime tag and the Python version from a
// full image tag. Returns an empty pythonVersion when the tag has no explicit
// `-python-X.Y` suffix so the caller can fall back to other sources.
//
//	"3.1-12"                    → base="3.1-12", python=""
//	"3.1-12-python-3.11"       → base="3.1-12", python="3.11"
//	"3.1-12-python-3.11-base"  → base="3.1-12", python="3.11"
func parseRuntimeTagPython(tag string) (baseTag, pythonVersion string) {
	loc := runtimePythonRe.FindStringSubmatchIndex(tag)
	if loc == nil {
		return strings.TrimSuffix(tag, "-base"), ""
	}
	return tag[:loc[0]], tag[loc[2]:loc[3]]
}

// resolvePythonVersion determines the Python version using a 3-tier strategy:
//  1. Explicit version from the Dockerfile image tag (-python-X.Y suffix)
//  2. defaultPythonVersion from the runtime versions JSON (updates.astronomer.io)
//  3. Hardcoded fallback (3.12)
var resolvePythonVersion = func(baseTag, tagPython string) string {
	// Tier 1: Dockerfile image tag had an explicit -python-X.Y suffix
	if tagPython != "" {
		return tagPython
	}

	// Tier 2: Fetch from runtime versions JSON
	if v := airflowversions.GetDefaultPythonVersion(baseTag); v != "" {
		return v
	}

	// Tier 3: Hardcoded fallback
	return defaultPythonVersion
}

// Standalone implements ContainerHandler using `airflow standalone` instead of Docker Compose.
type Standalone struct {
	airflowHome string
	envFile     string
	dockerfile  string
	foreground  bool   // if true, run in foreground (stream output, block on Wait)
	port        string // webserver port; defaults to defaultStandalonePort
}

// StandaloneInit creates a new Standalone handler.
func StandaloneInit(airflowHome, envFile, dockerfile string) (*Standalone, error) {
	return &Standalone{
		airflowHome: airflowHome,
		envFile:     envFile,
		dockerfile:  dockerfile,
	}, nil
}

// SetForeground controls whether Start() runs the process in the foreground.
func (s *Standalone) SetForeground(fg bool) {
	s.foreground = fg
}

// SetPort overrides the default webserver port.
func (s *Standalone) SetPort(port string) {
	s.port = port
}

// webserverPort returns the configured port. It checks (in order):
// 1. Explicit --port flag
// 2. api-server.port from .astro/config.yaml (same config used by `astro dev start`)
// 3. Default (8080)
func (s *Standalone) webserverPort() string {
	if s.port != "" {
		return s.port
	}
	if p := config.CFG.APIServerPort.GetString(); p != "" && p != "0" {
		return p
	}
	return defaultStandalonePort
}

func (s *Standalone) pidFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standalonePIDFile)
}

func (s *Standalone) logFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standaloneLogFile)
}

func (s *Standalone) passwordsFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standalonePasswordsFile)
}

// Start runs airflow standalone locally without Docker.
//
//nolint:gocognit,gocyclo
func (s *Standalone) Start(imageName, settingsFile, composeFile, buildSecretString string, noCache, noBrowser bool, waitTime time.Duration, envConns map[string]astrocore.EnvironmentObjectConnection) error {
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

	// 2. Validate Airflow version (AF3 only)
	if airflowversions.AirflowMajorVersionForRuntimeVersion(baseTag) != "3" {
		return errUnsupportedAirflowVersion
	}

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

	// 4. Fetch constraints and freeze files from CDN (cached locally)
	freezePath, airflowVersion, taskSDKVersion, err := s.getConstraints(baseTag, pythonVersion)
	if err != nil {
		return err
	}

	sp := spinner.NewSpinner("Setting up standalone environment…")
	sp.Start()

	// 5. Create venv
	err = runCommand(s.airflowHome, "uv", "venv", "--python", pythonVersion)
	if err != nil {
		sp.Stop()
		return fmt.Errorf("error creating virtual environment: %w", err)
	}

	// 6. Install dependencies (2-step install)
	// Step 1: Install airflow with full freeze constraints (reproduces runtime env exactly)
	installArgs := []string{
		"pip", "install",
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

	// 7. Seed credentials file (admin:admin) if this is a fresh environment
	if err = s.ensureCredentials(); err != nil {
		fmt.Printf("Warning: could not seed credentials file: %s\n", err.Error())
	}

	// 8. Build environment
	env := s.buildEnv()

	// 9. Start airflow standalone
	fmt.Println("\nStarting Airflow in standalone mode…")

	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")
	airflowBin := filepath.Join(venvBin, "airflow")

	cmd := exec.Command(airflowBin, "standalone") //nolint:gosec
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
func (s *Standalone) startForeground(cmd *exec.Cmd, waitTime time.Duration, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection) error {
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
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) //nolint:errcheck
		}
	}()
	defer signal.Stop(sigChan)

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
		fmt.Println("\n" + ansi.Green("\u2714") + " Airflow is ready!")
		fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
		if user, pass := s.readCredentials(); user != "" {
			fmt.Printf("%sUsername:   %s\n", bullet, ansi.Bold(user))
			fmt.Printf("%sPassword:   %s\n", bullet, ansi.Bold(pass))
		}
		fmt.Println()
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
func (s *Standalone) startBackground(cmd *exec.Cmd, waitTime time.Duration, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection) error {
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
	fmt.Printf("\n%s Airflow is ready! (PID %d)\n", ansi.Green("\u2714"), cmd.Process.Pid)
	fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
	if user, pass := s.readCredentials(); user != "" {
		fmt.Printf("%sUsername:   %s\n", bullet, ansi.Bold(user))
		fmt.Printf("%sPassword:   %s\n", bullet, ansi.Bold(pass))
	}
	fmt.Printf("%sView logs: %s\n", bullet, ansi.Bold("astro dev logs -f"))
	fmt.Printf("%sStop:      %s\n", bullet, ansi.Bold("astro dev stop"))

	return nil
}

// healthEndpoint returns the health check URL and component name.
func (s *Standalone) healthEndpoint() (url, component string) {
	return "http://localhost:" + s.webserverPort() + "/api/v2/monitor/health", "api-server"
}

// ensureCredentials seeds the passwords file with admin:admin on first run.
// If the file already exists it is left untouched, preserving custom credentials.
func (s *Standalone) ensureCredentials() error {
	path := s.passwordsFilePath()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPermissions); err != nil {
		return err
	}
	creds := map[string]string{standaloneAdminUser: standaloneAdminPassword}
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, filePermissions)
}

// readCredentials reads the SimpleAuthManager password file and returns (username, password).
// Returns empty strings if the file doesn't exist or can't be parsed.
func (s *Standalone) readCredentials() (username, password string) {
	data, err := osReadFile(s.passwordsFilePath())
	if err != nil {
		return "", ""
	}
	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", ""
	}
	// Return the well-known admin user if present; otherwise return the first entry.
	if p, ok := creds[standaloneAdminUser]; ok {
		return standaloneAdminUser, p
	}
	for u, p := range creds {
		return u, p
	}
	return "", ""
}

// getConstraints fetches pip constraints and freeze files from the CDN.
// The constraints file (small, 3 version pins) is used to extract version info.
// The freeze file (full package list) is used as pip constraints for the install.
// Both are cached in .astro/standalone/.
func (s *Standalone) getConstraints(tag, pythonVersion string) (freezePath, airflowVersion, taskSDKVersion string, err error) {
	constraintsDir := filepath.Join(s.airflowHome, standaloneDir)
	constraintsFile := filepath.Join(constraintsDir, fmt.Sprintf("constraints-%s-python-%s.txt", tag, pythonVersion))
	freezeFile := filepath.Join(constraintsDir, fmt.Sprintf("freeze-%s-python-%s.txt", tag, pythonVersion))

	// Check cache — both files must exist
	constraintsCached, _ := fileutil.Exists(constraintsFile, nil)
	freezeCached, _ := fileutil.Exists(freezeFile, nil)
	if constraintsCached && freezeCached {
		airflowVersion, err = parseAirflowVersionFromConstraints(constraintsFile)
		if err == nil && airflowVersion != "" {
			taskSDKVersion, _ = parsePackageVersionFromConstraints(constraintsFile, "apache-airflow-task-sdk")
			return freezeFile, airflowVersion, taskSDKVersion, nil
		}
	}

	err = os.MkdirAll(constraintsDir, dirPermissions)
	if err != nil {
		return "", "", "", fmt.Errorf("error creating standalone directory: %w", err)
	}

	// Fetch constraints file (small — version pins only, used for parsing)
	constraintsURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", constraintsBaseURL, tag, pythonVersion)
	constraintsContent, fetchErr := fetchConstraintsURL(constraintsURL)
	if fetchErr != nil {
		return "", "", "", fmt.Errorf("error fetching constraints from %s: %w", constraintsURL, fetchErr)
	}
	if err = os.WriteFile(constraintsFile, []byte(constraintsContent), filePermissions); err != nil {
		return "", "", "", fmt.Errorf("error caching constraints file: %w", err)
	}

	// Fetch freeze file (full package list, used as pip -c constraints)
	freezeURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", freezeBaseURL, tag, pythonVersion)
	freezeContent, fetchErr := fetchConstraintsURL(freezeURL)
	if fetchErr != nil {
		return "", "", "", fmt.Errorf("error fetching freeze file from %s: %w", freezeURL, fetchErr)
	}
	if err = os.WriteFile(freezeFile, []byte(freezeContent), filePermissions); err != nil {
		return "", "", "", fmt.Errorf("error caching freeze file: %w", err)
	}

	airflowVersion, err = parseAirflowVersionFromConstraints(constraintsFile)
	if err != nil {
		return "", "", "", err
	}

	taskSDKVersion, _ = parsePackageVersionFromConstraints(constraintsFile, "apache-airflow-task-sdk")

	return freezeFile, airflowVersion, taskSDKVersion, nil
}

// fetchConstraintsURL fetches constraints from a URL and returns the body as a string.
var fetchConstraintsURL = func(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch constraints: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parsePackageVersionFromConstraints reads a constraints file and extracts the version for a given package.
func parsePackageVersionFromConstraints(constraintsFile, packageName string) (string, error) {
	data, err := os.ReadFile(constraintsFile)
	if err != nil {
		return "", fmt.Errorf("error reading constraints file: %w", err)
	}

	prefix := packageName + "=="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", fmt.Errorf("could not find %s version in constraints file", packageName)
}

// parseAirflowVersionFromConstraints reads a constraints file and extracts the apache-airflow version.
func parseAirflowVersionFromConstraints(constraintsFile string) (string, error) {
	return parsePackageVersionFromConstraints(constraintsFile, "apache-airflow")
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
	if envVars, err := loadEnvFile(envFilePath); err == nil {
		for _, kv := range envVars {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				overrides[kv[:idx]] = kv[idx+1:]
			}
		}
	}

	// Layer 2: Standalone-critical settings — these MUST take precedence over
	// both inherited env and .env to prevent standalone mode from breaking.
	overrides["PATH"] = fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH"))
	overrides["AIRFLOW_HOME"] = standaloneHome
	overrides["ASTRONOMER_ENVIRONMENT"] = "local"
	overrides["AIRFLOW__CORE__LOAD_EXAMPLES"] = "False"
	overrides["AIRFLOW__CORE__DAGS_FOLDER"] = filepath.Join(s.airflowHome, "dags")
	overrides["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_USERS"] = standaloneAdminUser + ":admin"
	overrides["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_PASSWORDS_FILE"] = s.passwordsFilePath()
	if s.port != "" {
		overrides["AIRFLOW__WEBSERVER__WEB_SERVER_PORT"] = s.port
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

// loadEnvFile reads a .env file and returns key=value pairs.
// Values wrapped in matching single or double quotes are unquoted to match
// the behavior of Docker Compose's .env loader.
func loadEnvFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var envVars []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := line[:idx]
			val := line[idx+1:]
			val = stripQuotes(val)
			envVars = append(envVars, key+"="+val)
		}
	}
	return envVars, nil
}

// stripQuotes removes matching surrounding single or double quotes from a value.
func stripQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

// applySettings imports airflow_settings.yaml using airflow CLI commands run via the venv.
func (s *Standalone) applySettings(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection) error {
	settingsExists, err := fileutil.Exists(settingsFile, nil)
	if err != nil || !settingsExists {
		if len(envConns) == 0 {
			return nil
		}
	}

	// Temporarily swap the execAirflowCommand to use venv instead of docker
	origExec := settings.SetExecAirflowCommand(s.standaloneExecAirflowCommand)
	defer settings.SetExecAirflowCommand(origExec)

	return settings.ConfigSettings("standalone", settingsFile, envConns, 3, true, true, true) //nolint:mnd
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

// readPID reads the PID file and checks if the process is alive.
// Returns the PID and true if the process is running, or 0 and false otherwise.
func (s *Standalone) readPID() (int, bool) {
	data, err := osReadFile(s.pidFilePath())
	if err != nil {
		return 0, false
	}

	pid := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid <= 0 {
		return 0, false
	}

	proc, err := osFindProcess(pid)
	if err != nil {
		return pid, false
	}
	// On Unix, FindProcess always succeeds; use signal 0 to probe.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}

// Stop terminates the standalone Airflow process.
func (s *Standalone) Stop(_ bool) error {
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
func (s *Standalone) PS() error {
	pid, alive := s.readPID()
	if alive {
		fmt.Printf("Airflow standalone is running (PID %d)\n", pid)
	} else {
		fmt.Println("Airflow standalone is not running.")
	}
	return nil
}

// Logs streams the standalone Airflow log file.
func (s *Standalone) Logs(follow bool, _ ...string) error {
	logPath := s.logFilePath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s — has standalone been started?", logPath)
	}

	if !follow {
		data, err := osReadFile(logPath)
		if err != nil {
			return fmt.Errorf("error reading log file: %w", err)
		}
		fmt.Print(string(data))
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
		if line != "" {
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

// standaloneExecDefault runs a command with the given env, dir, and I/O streams.
// It resolves the binary using the env's PATH (not the parent process's PATH)
// so that venv binaries like "airflow" and "pytest" are found correctly.
func standaloneExecDefault(dir string, env, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	binary := resolveInEnvPath(args[0], env)
	cmd := exec.Command(binary, args[1:]...) //nolint:gosec
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// resolveInEnvPath looks up a binary name in the PATH from the given env slice.
// This is needed because exec.Command uses the parent process's PATH, not cmd.Env.
func resolveInEnvPath(binary string, env []string) string {
	if filepath.IsAbs(binary) || strings.Contains(binary, string(filepath.Separator)) {
		return binary
	}
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			for _, dir := range filepath.SplitList(e[5:]) {
				candidate := filepath.Join(dir, binary)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}
	return binary // fallback to original
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

	origExec := settings.SetExecAirflowCommand(s.standaloneExecAirflowCommand)
	defer settings.SetExecAirflowCommand(origExec)

	err = settings.ConfigSettings("standalone", settingsFile, nil, 3, connections, variables, pools) //nolint:mnd
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

	if envExport {
		err := settings.EnvExport("standalone", envFile, 3, connections, variables) //nolint:mnd
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

	err = settings.Export("standalone", settingsFile, 3, connections, variables, pools) //nolint:mnd
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

func (s *Standalone) UpgradeTest(_, _, _, _ string, _, _, _, _, _ bool, _ string, _ astroplatformcore.ClientWithResponsesInterface) error {
	return errors.New("astro dev upgrade-test is not available in standalone mode")
}

// execCommand runs a command in the given directory.
func execCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// startCmd starts a command without waiting for it to finish.
func startCmd(cmd *exec.Cmd) error {
	return cmd.Start()
}
