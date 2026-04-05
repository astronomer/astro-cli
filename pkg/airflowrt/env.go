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
	overrides["AIRFLOW_HOME"] = standaloneHome
	overrides["ASTRONOMER_ENVIRONMENT"] = "local"
	overrides["AIRFLOW__CORE__LOAD_EXAMPLES"] = "False"
	overrides["AIRFLOW__CORE__DAGS_FOLDER"] = filepath.Join(projectPath, "dags")
	overrides["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS"] = "True"
	if port != DefaultPort {
		overrides["AIRFLOW__API__PORT"] = port
	}
	overrides["AIRFLOW__CORE__EXECUTION_API_SERVER_URL"] = "http://localhost:" + port + "/execution/"

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

// LoadEnvFile reads a .env file and returns key=value pairs.
// Values wrapped in matching single or double quotes are unquoted to match
// the behavior of Docker Compose's .env loader.
func LoadEnvFile(path string) ([]string, error) {
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
			val := StripQuotes(line[idx+1:])
			envVars = append(envVars, key+"="+val)
		}
	}
	return envVars, nil
}

// StripQuotes removes matching surrounding single or double quotes from a value.
func StripQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
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
