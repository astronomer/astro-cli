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
	"strings"
	"sync"
	"syscall"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/astronomer/astro-cli/settings"
	"github.com/pkg/errors"
)

const (
	standaloneDir            = ".astro/standalone"
	standalonePIDFile        = "airflow.pid"
	standaloneLogFile        = "airflow.log"
	defaultStandalonePort    = "8080"
	standaloneIndexURL       = "https://pip.astronomer.io/v2/"
	standalonePythonVer      = "3.12"
	constraintsBaseURL       = "https://cdn.astronomer.io/runtime-constraints"
	freezeBaseURL            = "https://cdn.astronomer.io/runtime-freeze"
	stopPollInterval         = 500 * time.Millisecond
	stopTimeout              = 10 * time.Second
	filePermissions          = os.FileMode(0o644)
	dirPermissions           = os.FileMode(0o755)
	standaloneAdminUser     = "admin"
	standaloneAdminPassword = "admin"
	// standalonePasswordsFile lives inside standaloneDir (.astro/standalone/) so it stays
	// out of the project root and is cleaned up automatically by Kill/reset.
	standalonePasswordsFile = "simple_auth_manager_passwords.json.generated"
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
	osReadFile            = os.ReadFile
	osFindProcess         = os.FindProcess
)

// Standalone implements ContainerHandler using `airflow standalone` instead of Docker Compose.
type Standalone struct {
	airflowHome string
	envFile     string
	dockerfile  string
	foreground  bool // if true, run in foreground (stream output, block on Wait)
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

// pidFilePath returns the full path to the PID file.
func (s *Standalone) pidFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standalonePIDFile)
}

// logFilePath returns the full path to the log file.
func (s *Standalone) logFilePath() string {
	return filepath.Join(s.airflowHome, standaloneDir, standaloneLogFile)
}

// passwordsFilePath returns the full path to the SimpleAuthManager passwords file.
// Keeping it inside standaloneDir means it stays out of the project root and is
// cleaned up automatically by Kill/reset along with other standalone state.
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

	// 2. Validate Airflow version (AF3 only)
	if airflowversions.AirflowMajorVersionForRuntimeVersion(tag) != "3" {
		return errUnsupportedAirflowVersion
	}

	// 3. Check uv is on PATH
	_, err = lookPath("uv")
	if err != nil {
		return errUVNotFound
	}

	// 3b. In background mode, bail early if already running (before any install work)
	if !s.foreground {
		if pid, alive := s.readPID(); alive {
			return fmt.Errorf("standalone Airflow is already running (PID %d). Run 'astro dev local stop' first", pid)
		}
	}

	// 4. Fetch constraints and freeze files from CDN (cached locally)
	freezePath, airflowVersion, taskSDKVersion, err := s.getConstraints(tag)
	if err != nil {
		return err
	}

	sp := spinner.NewSpinner("Setting up standalone environment…")
	sp.Start()

	// 5. Create venv
	err = runCommand(s.airflowHome, "uv", "venv", "--python", standalonePythonVer)
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

	// 7. Apply settings
	err = s.applySettings(settingsFile, envConns)
	if err != nil {
		fmt.Printf("Warning: could not apply airflow settings: %s\n", err.Error())
	}

	// 8. Seed credentials file (admin:admin) if this is a fresh environment
	if err = s.ensureCredentials(); err != nil {
		fmt.Printf("Warning: could not seed credentials file: %s\n", err.Error())
	}

	// 9. Build environment
	env := s.buildEnv()

	// 10. Start airflow standalone
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
		return s.startForeground(cmd, waitTime)
	}
	return s.startBackground(cmd, waitTime)
}

// startForeground runs the airflow process in the foreground, streaming output to the terminal.
func (s *Standalone) startForeground(cmd *exec.Cmd, waitTime time.Duration) error {
	// Set up pipes for stdout/stderr so we can stream output
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
			// Send SIGTERM to the entire process group (-pid).
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) //nolint:errcheck
		}
	}()
	defer signal.Stop(sigChan)

	// Stream output in background goroutines
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
		bullet := ansi.Cyan("\u27A4") + " "
		uiURL := "http://localhost:" + defaultStandalonePort
		fmt.Println("\n" + ansi.Green("\u2714") + " Airflow is ready!")
		fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
		if user, pass := s.readCredentials(); user != "" {
			fmt.Printf("%sUsername:   %s\n", bullet, ansi.Bold(user))
			fmt.Printf("%sPassword:   %s\n", bullet, ansi.Bold(pass))
		}
		fmt.Println()
	}()

	// Wait for the process to complete
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
func (s *Standalone) startBackground(cmd *exec.Cmd, waitTime time.Duration) error {
	// Check if already running
	if pid, alive := s.readPID(); alive {
		return fmt.Errorf("standalone Airflow is already running (PID %d). Run 'astro dev local stop' first", pid)
	}

	// Open log file for writing
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

	// Write PID file
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

	bullet := ansi.Cyan("\u27A4") + " "
	uiURL := "http://localhost:" + defaultStandalonePort
	fmt.Printf("\n%s Airflow is ready! (PID %d)\n", ansi.Green("\u2714"), cmd.Process.Pid)
	fmt.Printf("%sAirflow UI: %s\n", bullet, ansi.Bold(uiURL))
	if user, pass := s.readCredentials(); user != "" {
		fmt.Printf("%sUsername:   %s\n", bullet, ansi.Bold(user))
		fmt.Printf("%sPassword:   %s\n", bullet, ansi.Bold(pass))
	}
	fmt.Printf("%sView logs: %s\n", bullet, ansi.Bold("astro dev local logs -f"))
	fmt.Printf("%sStop:      %s\n", bullet, ansi.Bold("astro dev local stop"))

	return nil
}

// healthEndpoint returns the health check URL and component name.
func (s *Standalone) healthEndpoint() (url, component string) {
	return "http://localhost:" + defaultStandalonePort + "/api/v2/monitor/health", "api-server"
}

// ensureCredentials seeds the SimpleAuthManager passwords file with admin:admin
// if it doesn't already exist. Airflow's init() uses "a+" mode, so if the entry
// is already present it won't be overwritten — existing passwords survive a restart,
// but a fresh environment always gets the predictable admin/admin default.
func (s *Standalone) ensureCredentials() error {
	path := s.passwordsFilePath()
	if _, err := os.Stat(path); err == nil {
		return nil // already exists, leave it alone
	}
	// Ensure .astro/standalone/ exists before writing into it
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
	for u, p := range creds {
		return u, p
	}
	return "", ""
}

// getConstraints fetches pip constraints and freeze files from the CDN.
// The constraints file (small, 3 version pins) is used to extract version info.
// The freeze file (full package list) is used as pip constraints for the install.
// Both are cached in .astro/standalone/.
func (s *Standalone) getConstraints(tag string) (freezePath, airflowVersion, taskSDKVersion string, err error) {
	constraintsDir := filepath.Join(s.airflowHome, standaloneDir)
	constraintsFile := filepath.Join(constraintsDir, fmt.Sprintf("constraints-%s.txt", tag))
	freezeFile := filepath.Join(constraintsDir, fmt.Sprintf("freeze-%s.txt", tag))

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

	// Create directory
	err = os.MkdirAll(constraintsDir, dirPermissions)
	if err != nil {
		return "", "", "", fmt.Errorf("error creating standalone directory: %w", err)
	}

	// Fetch constraints file (small — version pins only, used for parsing)
	constraintsURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", constraintsBaseURL, tag, standalonePythonVer)
	constraintsContent, fetchErr := fetchConstraintsURL(constraintsURL)
	if fetchErr != nil {
		return "", "", "", fmt.Errorf("error fetching constraints from %s: %w", constraintsURL, fetchErr)
	}
	if err = os.WriteFile(constraintsFile, []byte(constraintsContent), filePermissions); err != nil {
		return "", "", "", fmt.Errorf("error caching constraints file: %w", err)
	}

	// Fetch freeze file (full package list, used as pip -c constraints)
	freezeURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", freezeBaseURL, tag, standalonePythonVer)
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

	// Build our override map — these take precedence over the inherited env.
	overrides := map[string]string{
		"PATH":                                               fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH")),
		"AIRFLOW_HOME":                                       standaloneHome,
		"ASTRONOMER_ENVIRONMENT":                             "local",
		"AIRFLOW__CORE__LOAD_EXAMPLES":                       "False",
		"AIRFLOW__CORE__DAGS_FOLDER":                         filepath.Join(s.airflowHome, "dags"),
		"AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_USERS":           standaloneAdminUser + ":admin",
		"AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_PASSWORDS_FILE":  s.passwordsFilePath(),
	}

	// Load .env file if it exists — these also override inherited env.
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

	// Append our overrides.
	for k, v := range overrides {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

// loadEnvFile reads a .env file and returns key=value pairs.
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
		if strings.Contains(line, "=") {
			envVars = append(envVars, line)
		}
	}
	return envVars, nil
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
	venvBash := filepath.Join(s.airflowHome, ".venv", "bin", "bash")

	cmd := exec.Command(venvBash, "-c", command) //nolint:gosec
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

	// Check if process is alive
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

	// Send SIGTERM to the process group
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
	// Stop the running process first
	s.Stop(false) //nolint:errcheck

	sp := spinner.NewSpinner("Cleaning up standalone environment…")
	sp.Start()
	defer sp.Stop()

	// Remove venv and the entire standaloneDir (.astro/standalone/).
	// Since AIRFLOW_HOME points at standaloneDir, all Airflow-generated files
	// (airflow.cfg, airflow.db, logs/, passwords file, constraint caches) live
	// there and are cleaned up in one shot.
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

// Stub methods — not supported in standalone mode.

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

func (s *Standalone) Build(_, _ string, _ bool) error {
	return errStandaloneNotSupported
}

func (s *Standalone) Run(_ []string, _ string) error {
	return errStandaloneNotSupported
}

func (s *Standalone) Bash(_ string) error {
	return errStandaloneNotSupported
}

func (s *Standalone) RunDAG(_, _, _, _ string, _, _ bool) error {
	return errStandaloneNotSupported
}

func (s *Standalone) ImportSettings(_, _ string, _, _, _ bool) error {
	return errStandaloneNotSupported
}

func (s *Standalone) ExportSettings(_, _ string, _, _, _, _ bool) error {
	return errStandaloneNotSupported
}

func (s *Standalone) ComposeExport(_, _ string) error {
	return errStandaloneNotSupported
}

func (s *Standalone) Pytest(_, _, _, _, _ string) (string, error) {
	return "", errStandaloneNotSupported
}

func (s *Standalone) Parse(_, _, _ string) error {
	return errStandaloneNotSupported
}

func (s *Standalone) UpgradeTest(_, _, _, _ string, _, _, _, _, _ bool, _ string, _ astroplatformcore.ClientWithResponsesInterface) error {
	return errStandaloneNotSupported
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
