package airflow

import (
	"bufio"
	"fmt"
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
	standaloneDir          = ".astro/standalone"
	defaultStandalonePort  = "8080"
	standaloneIndexURL     = "https://pip.astronomer.io/v2/"
	standalonePythonVer    = "3.12"
	constraintsFileInImage = "/etc/pip-constraints.txt"
)

var (
	errStandaloneNotSupported    = errors.New("this command is not supported in standalone mode")
	errUnsupportedAirflowVersion = errors.New("standalone mode requires Airflow 2.2+ (runtime 4.0.0+) or Airflow 3 (runtime 3.x)")
	errUVNotFound                = errors.New("'uv' is required for standalone mode but was not found on PATH.\nInstall it with: curl -LsSf https://astral.sh/uv/install.sh | sh\nSee https://docs.astral.sh/uv/getting-started/installation/ for more options")

	// Function variables for testing
	lookPath              = exec.LookPath
	standaloneParseFile   = docker.ParseFile
	standaloneGetImageTag = docker.GetImageTagFromParsedFile
	runCommand            = execCommand
	startCommand          = startCmd
)

// Standalone implements ContainerHandler using `airflow standalone` instead of Docker Compose.
type Standalone struct {
	airflowHome  string
	envFile      string
	dockerfile   string
	airflowMajor string // "2" or "3", set during Start()
}

// StandaloneInit creates a new Standalone handler.
func StandaloneInit(airflowHome, envFile, dockerfile string) (*Standalone, error) {
	return &Standalone{
		airflowHome: airflowHome,
		envFile:     envFile,
		dockerfile:  dockerfile,
	}, nil
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

	// 2. Validate Airflow version (2.2+ or 3.x)
	s.airflowMajor = airflowversions.AirflowMajorVersionForRuntimeVersion(tag)
	if s.airflowMajor != "2" && s.airflowMajor != "3" {
		return errUnsupportedAirflowVersion
	}

	// 3. Check uv is on PATH
	_, err = lookPath("uv")
	if err != nil {
		return errUVNotFound
	}

	// 4. Extract constraints from runtime image (cached)
	constraintsPath, airflowVersion, err := s.getConstraints(tag)
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

	// 6. Install dependencies
	requirementsPath := filepath.Join(s.airflowHome, "requirements.txt")
	installArgs := []string{
		"pip", "install",
		fmt.Sprintf("apache-airflow==%s", airflowVersion),
		"-c", constraintsPath,
		"--index-url", standaloneIndexURL,
	}
	if exists, _ := fileutil.Exists(requirementsPath, nil); exists {
		installArgs = append(installArgs, "-r", requirementsPath)
	}

	err = runCommand(s.airflowHome, "uv", installArgs...)
	if err != nil {
		sp.Stop()
		return fmt.Errorf("error installing dependencies: %w", err)
	}

	spinner.StopWithCheckmark(sp, "Environment ready")

	// 7. Apply settings
	err = s.applySettings(settingsFile, envConns)
	if err != nil {
		fmt.Printf("Warning: could not apply airflow settings: %s\n", err.Error())
	}

	// 8. Build environment
	env := s.buildEnv()

	// 9. Start airflow standalone as foreground process
	fmt.Println("\nStarting Airflow in standalone mode…")

	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")
	airflowBin := filepath.Join(venvBin, "airflow")

	cmd := exec.Command(airflowBin, "standalone") //nolint:gosec
	cmd.Dir = s.airflowHome
	cmd.Env = env
	// Start the subprocess in its own process group so we can kill the entire
	// tree (scheduler, triggerer, api-server, etc.) when the user sends Ctrl+C.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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

	// Run health check in background (URL differs between Airflow 2 and 3)
	var healthURL, healthComp string
	switch s.airflowMajor {
	case "3":
		healthURL = "http://localhost:" + defaultStandalonePort + "/api/v2/monitor/health"
		healthComp = "api-server"
	default:
		healthURL = "http://localhost:" + defaultStandalonePort + "/health"
		healthComp = "webserver"
	}
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
		fmt.Printf("%sCredentials are printed above by `airflow standalone`\n\n", bullet)
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

// runtimeImageName returns the full Docker image name for the given runtime tag.
func (s *Standalone) runtimeImageName(tag string) string {
	switch s.airflowMajor {
	case "3":
		return fmt.Sprintf("%s/%s:%s", AstroImageRegistryBaseImageName, AstroRuntimeAirflow3ImageName, tag)
	default:
		return fmt.Sprintf("%s/%s:%s", QuayBaseImageName, AstroRuntimeAirflow2ImageName, tag)
	}
}

// getConstraints extracts pip constraints from the runtime Docker image.
// Results are cached in .astro/standalone/constraints-<tag>.txt.
func (s *Standalone) getConstraints(tag string) (string, string, error) {
	constraintsDir := filepath.Join(s.airflowHome, standaloneDir)
	constraintsFile := filepath.Join(constraintsDir, fmt.Sprintf("constraints-%s.txt", tag))

	// Check cache
	if exists, _ := fileutil.Exists(constraintsFile, nil); exists {
		airflowVersion, err := parseAirflowVersionFromConstraints(constraintsFile)
		if err == nil && airflowVersion != "" {
			return constraintsFile, airflowVersion, nil
		}
	}

	// Create directory
	err := os.MkdirAll(constraintsDir, os.FileMode(0o755))
	if err != nil {
		return "", "", fmt.Errorf("error creating standalone directory: %w", err)
	}

	// Determine full image name
	fullImageName := s.runtimeImageName(tag)

	// Run docker to extract constraints
	out, err := execDockerRun(fullImageName, constraintsFileInImage)
	if err != nil {
		return "", "", fmt.Errorf("error extracting constraints from runtime image %s: %w", fullImageName, err)
	}

	// Write constraints to cache file
	err = os.WriteFile(constraintsFile, []byte(out), os.FileMode(0o644))
	if err != nil {
		return "", "", fmt.Errorf("error caching constraints file: %w", err)
	}

	airflowVersion, err := parseAirflowVersionFromConstraints(constraintsFile)
	if err != nil {
		return "", "", err
	}

	return constraintsFile, airflowVersion, nil
}

// execDockerRun runs `docker run --rm --entrypoint cat <image> <path>` and returns stdout.
var execDockerRun = func(imageName, filePath string) (string, error) {
	cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", imageName, filePath) //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseAirflowVersionFromConstraints reads a constraints file and extracts the apache-airflow version.
func parseAirflowVersionFromConstraints(constraintsFile string) (string, error) {
	data, err := os.ReadFile(constraintsFile)
	if err != nil {
		return "", fmt.Errorf("error reading constraints file: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apache-airflow==") {
			return strings.TrimPrefix(line, "apache-airflow=="), nil
		}
	}
	return "", errors.New("could not find apache-airflow version in constraints file")
}

// buildEnv constructs the environment variables for the standalone process.
func (s *Standalone) buildEnv() []string {
	venvBin := filepath.Join(s.airflowHome, ".venv", "bin")

	// Build our override map — these take precedence over the inherited env.
	overrides := map[string]string{
		"PATH":                         fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH")),
		"AIRFLOW_HOME":                 s.airflowHome,
		"ASTRONOMER_ENVIRONMENT":       "local",
		"AIRFLOW__CORE__LOAD_EXAMPLES": "False",
		"AIRFLOW__CORE__DAGS_FOLDER":   filepath.Join(s.airflowHome, "dags"),
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
	var env []string
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

	airflowVersion := uint64(3) //nolint:mnd
	if s.airflowMajor == "2" {
		airflowVersion = 2 //nolint:mnd
	}
	return settings.ConfigSettings("standalone", settingsFile, envConns, airflowVersion, true, true, true)
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

// Stop is mostly a no-op for standalone mode since Ctrl+C is the primary mechanism.
func (s *Standalone) Stop(_ bool) error {
	fmt.Println("Standalone mode runs in the foreground. Use Ctrl+C to stop.")
	return nil
}

// Kill cleans up standalone state files.
func (s *Standalone) Kill() error {
	sp := spinner.NewSpinner("Cleaning up standalone environment…")
	sp.Start()
	defer sp.Stop()

	// Remove venv, standalone cache, airflow.db, logs, and credential files
	pathsToRemove := []string{
		filepath.Join(s.airflowHome, ".venv"),
		filepath.Join(s.airflowHome, standaloneDir),
		filepath.Join(s.airflowHome, "airflow.db"),
		filepath.Join(s.airflowHome, "logs"),
		filepath.Join(s.airflowHome, "simple_auth_manager_passwords.json.generated"), // Airflow 3
		filepath.Join(s.airflowHome, "standalone_admin_password.txt"),                // Airflow 2
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

func (s *Standalone) PS() error {
	return errStandaloneNotSupported
}

func (s *Standalone) Logs(_ bool, _ ...string) error {
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
func execCommand(dir string, name string, args ...string) error {
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
