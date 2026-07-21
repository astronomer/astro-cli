package airflowrt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := `# comment
FOO=bar
BAZ="quoted value"
SINGLE='single quoted'

EMPTY=
`
	require.NoError(t, os.WriteFile(envFile, []byte(content), 0o644))

	vars, err := LoadEnvFile(envFile)
	require.NoError(t, err)
	assert.Contains(t, vars, "FOO=bar")
	assert.Contains(t, vars, "BAZ=quoted value")
	assert.Contains(t, vars, "SINGLE=single quoted")
	assert.Contains(t, vars, "EMPTY=")
}

func TestLoadEnvFile_NotExists(t *testing.T) {
	_, err := LoadEnvFile("/nonexistent/.env")
	assert.Error(t, err)
}

func TestStripQuotes(t *testing.T) {
	assert.Equal(t, "hello", StripQuotes(`"hello"`))
	assert.Equal(t, "hello", StripQuotes(`'hello'`))
	assert.Equal(t, "hello", StripQuotes("hello"))
	assert.Equal(t, "", StripQuotes(`""`))
	assert.Equal(t, `"mismatched'`, StripQuotes(`"mismatched'`))
	assert.Equal(t, "a", StripQuotes("a"))
}

func TestBuildEnv(t *testing.T) {
	dir := t.TempDir()

	env := BuildEnv(dir, "9090", "")

	envMap := envToMap(env)
	assert.Equal(t, filepath.Join(dir, StandaloneDir), envMap["AIRFLOW_HOME"])
	assert.Equal(t, "local", envMap["ASTRONOMER_ENVIRONMENT"])
	assert.Equal(t, "False", envMap["AIRFLOW__CORE__LOAD_EXAMPLES"])
	assert.Equal(t, filepath.Join(dir, "dags"), envMap["AIRFLOW__CORE__DAGS_FOLDER"])
	assert.Equal(t, "True", envMap["AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS"])
	assert.Equal(t, "9090", envMap["AIRFLOW__API__PORT"])
	assert.Equal(t, "http://localhost:9090/execution/", envMap["AIRFLOW__CORE__EXECUTION_API_SERVER_URL"])
	assert.True(t, strings.HasPrefix(envMap["PATH"], filepath.Join(dir, ".venv", "bin")))
}

func TestBuildEnv_LoopbackDefaults(t *testing.T) {
	dir := t.TempDir()

	env := BuildEnv(dir, "", "")

	envMap := envToMap(env)
	// The api-server (AF3) and webserver (AF2) must default to loopback so a
	// standalone instance with all-admins auth is not reachable from the LAN.
	assert.Equal(t, "127.0.0.1", envMap["AIRFLOW__API__HOST"])
	assert.Equal(t, "127.0.0.1", envMap["AIRFLOW__WEBSERVER__WEB_SERVER_HOST"])
}

func TestBuildEnv_LoopbackOverriddenByEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "AIRFLOW__API__HOST=0.0.0.0\nAIRFLOW__WEBSERVER__WEB_SERVER_HOST=0.0.0.0\n"
	require.NoError(t, os.WriteFile(envFile, []byte(content), 0o644))

	env := BuildEnv(dir, "", envFile)

	envMap := envToMap(env)
	// A user's explicit .env setting wins over the loopback default.
	assert.Equal(t, "0.0.0.0", envMap["AIRFLOW__API__HOST"])
	assert.Equal(t, "0.0.0.0", envMap["AIRFLOW__WEBSERVER__WEB_SERVER_HOST"])
}

func TestBuildEnv_LoopbackOverriddenByInheritedEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIRFLOW__API__HOST", "0.0.0.0")
	t.Setenv("AIRFLOW__WEBSERVER__WEB_SERVER_HOST", "0.0.0.0")

	env := BuildEnv(dir, "", "")

	envMap := envToMap(env)
	// A user's setting in the inherited environment wins over the loopback default.
	assert.Equal(t, "0.0.0.0", envMap["AIRFLOW__API__HOST"])
	assert.Equal(t, "0.0.0.0", envMap["AIRFLOW__WEBSERVER__WEB_SERVER_HOST"])
}

func TestBuildEnv_DefaultPort(t *testing.T) {
	dir := t.TempDir()

	env := BuildEnv(dir, DefaultPort, "")

	envMap := envToMap(env)
	// Default port should NOT set AIRFLOW__API__PORT
	_, hasPort := envMap["AIRFLOW__API__PORT"]
	assert.False(t, hasPort)
}

func TestBuildEnv_WithEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("MY_VAR=hello\n"), 0o644))

	env := BuildEnv(dir, "8080", envFile)

	envMap := envToMap(env)
	assert.Equal(t, "hello", envMap["MY_VAR"])
}

func TestHasProxyConfigured(t *testing.T) {
	// No proxy set
	assert.False(t, HasProxyConfigured(map[string]string{}))

	// Proxy in overrides
	assert.True(t, HasProxyConfigured(map[string]string{"HTTP_PROXY": "http://proxy:8080"}))
	assert.True(t, HasProxyConfigured(map[string]string{"https_proxy": "http://proxy:8080"}))
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			m[kv[:idx]] = kv[idx+1:]
		}
	}
	return m
}
