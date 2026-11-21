package airflowrt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultPort = "8080"

// BuildEnv constructs the environment for the standalone airflow process.
// projectPath is the project root, port is the webserver port, and envFilePath
// is the path to a .env file (pass "" to use projectPath/.env).
func BuildEnv(projectPath, port, envFilePath string) []string {
	venvBin := filepath.Join(projectPath, ".venv", "bin")
	standaloneHome := filepath.Join(projectPath, StandaloneDir)

	if port == "" {
		port = DefaultPort
	}
	if envFilePath == "" {
		envFilePath = filepath.Join(projectPath, ".env")
	}

	overrides := map[string]string{}

	// Layer 1: Load .env file
	if envVars, err := LoadEnvFile(envFilePath); err == nil {
		for _, kv := range envVars {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				overrides[kv[:idx]] = kv[idx+1:]
			}
		}
	}

	// Layer 2: Standalone-critical settings
	overrides["PATH"] = fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH"))
	// The runtime image sets PYTHONPATH to the project root, so a DAG's
	// `from include import x` resolves there and on Astro. Standalone must
	// match or every such DAG is an import error locally. A PYTHONPATH from
	// .env or the shell is kept, after the project root.
	pythonPath := projectPath
	if extra := userSetting(overrides, "PYTHONPATH"); extra != "" {
		pythonPath += string(os.PathListSeparator) + extra
	}
	overrides["PYTHONPATH"] = pythonPath
	overrides["AIRFLOW_HOME"] = standaloneHome
	overrides["ASTRONOMER_ENVIRONMENT"] = "local"
	overrides["AIRFLOW__CORE__LOAD_EXAMPLES"] = "False"
	overrides["AIRFLOW__CORE__DAGS_FOLDER"] = filepath.Join(projectPath, "dags")
	overrides["AIRFLOW__CORE__PLUGINS_FOLDER"] = filepath.Join(projectPath, "plugins")
	overrides["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS"] = "True"
	if port != DefaultPort {
		overrides["AIRFLOW__API__PORT"] = port
	}
	overrides["AIRFLOW__CORE__EXECUTION_API_SERVER_URL"] = "http://localhost:" + port + "/execution/"
	// The Astro theme, as the Runtime image's entrypoint sets it, so Airflow
	// looks the same opened on its own port as through the local proxy or on
	// Astro. A theme of the user's own, in .env or the shell, wins, as it does
	// there; an empty one is no theme to Airflow (it cannot parse ""), so it is
	// replaced, also as there. {} is how to turn the theme off. Airflow before
	// 3.2 ignores the setting.
	if userSetting(overrides, UIThemeEnv) == "" {
		overrides[UIThemeEnv] = UITheme()
	}

	// Layer 3: macOS proxy workaround.
	// Python's _scproxy calls SCDynamicStoreCopyProxies which is not fork-safe.
	// When Airflow's LocalExecutor forks, this can spin at 100% CPU indefinitely.
	// Setting NO_PROXY=* tells Python to skip _scproxy entirely.
	if !HasProxyConfigured(overrides) {
		overrides["NO_PROXY"] = "*"
		overrides["no_proxy"] = "*"
	}

	// Build final env: inherited + overrides
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
		env = append(env, k+"="+v)
	}
	return env
}

// userSetting is what the user set key to: its value in .env when .env has
// it, and otherwise the shell's.
func userSetting(overrides map[string]string, key string) string {
	if v, ok := overrides[key]; ok {
		return v
	}
	return os.Getenv(key)
}

// LoadEnvFile reads a .env file and returns its assignments as key=value
// pairs, in file order, decoded by ParseDotenv (Docker Compose's rules). A
// line ParseDotenv cannot read is skipped; LoadDotenvFile reports them.
func LoadEnvFile(path string) ([]string, error) {
	entries, _, err := LoadDotenvFile(path)
	if err != nil {
		return nil, err
	}
	envVars := make([]string, 0, len(entries))
	for _, e := range entries {
		envVars = append(envVars, e.Key+"="+e.Value)
	}
	return envVars, nil
}

// LoadDotenvFile reads and parses the .env file at path.
func LoadDotenvFile(path string) ([]DotenvEntry, []DotenvProblem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	entries, problems := ParseDotenv(data)
	return entries, problems, nil
}

// proxyEnvKeys lists environment variable names that indicate a proxy is configured.
var proxyEnvKeys = []string{
	"HTTP_PROXY", "http_proxy",
	"HTTPS_PROXY", "https_proxy",
	"ALL_PROXY", "all_proxy",
	"NO_PROXY", "no_proxy",
}

// HasProxyConfigured returns true if any proxy-related environment variable
// is set — either in the inherited environment, the .env overrides, or the
// macOS system proxy settings. When true, we leave proxy settings alone.
func HasProxyConfigured(overrides map[string]string) bool {
	// Check .env overrides
	for _, key := range proxyEnvKeys {
		if _, ok := overrides[key]; ok {
			return true
		}
	}
	// Check inherited environment
	for _, key := range proxyEnvKeys {
		if os.Getenv(key) != "" {
			return true
		}
	}
	// Check macOS system proxy (no-op on other platforms)
	return HasSystemProxy()
}
