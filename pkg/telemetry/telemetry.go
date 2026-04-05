package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

const (
	// TelemetryAPIURL is the telemetry API endpoint
	TelemetryAPIURL = "https://api.astronomer.io/v1alpha1/telemetry"

	// Environment variable to disable telemetry
	envTelemetryDisabled = "ASTRO_TELEMETRY_DISABLED"
	// Environment variable to override telemetry API URL
	envTelemetryAPIURL = "ASTRO_TELEMETRY_API_URL"
	// Environment variable to enable synchronous debug mode
	envTelemetryDebug = "ASTRO_TELEMETRY_DEBUG"
)

// TelemetryPayload represents the data sent to the telemetry API
type TelemetryPayload struct {
	Source      string                 `json:"source"`
	Event       string                 `json:"event"`
	AnonymousID string                 `json:"anonymousId"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

// senderPayload wraps TelemetryPayload with the API URL for the subprocess
type senderPayload struct {
	TelemetryPayload
	APIURL string `json:"api_url"`
}

// envMapping pairs an environment variable with a context name
type envMapping struct {
	envVar string
	name   string
}

// agentEnvVars is an ordered list of environment variables to detect agent contexts
var agentEnvVars = []envMapping{
	{"CLAUDECODE", "claude-code"},
	{"CLAUDE_CODE_ENTRYPOINT", "claude-code"},
	{"CURSOR_TRACE_ID", "cursor"},
	{"CURSOR_AGENT", "cursor"},
	{"AIDER_MODEL", "aider"},
	{"CONTINUE_GLOBAL_DIR", "continue"},
	{"CORTEX_SESSION_ID", "snowflake-cortex"},
	{"GEMINI_CLI", "gemini-cli"},
	{"OPENCODE", "opencode"},
	{"CODEX_API_KEY", "codex"},
}

// ciEnvVars is an ordered list of environment variables to detect CI contexts.
// Generic "CI" must be last so specific providers take precedence.
var ciEnvVars = []envMapping{
	{"GITHUB_ACTIONS", "github-actions"},
	{"GITLAB_CI", "gitlab-ci"},
	{"JENKINS_URL", "jenkins"},
	{"HUDSON_URL", "jenkins"},
	{"CIRCLECI", "circleci"},
	{"TF_BUILD", "azure-devops"},
	{"BITBUCKET_BUILD_NUMBER", "bitbucket-pipelines"},
	{"CODEBUILD_BUILD_ID", "aws-codebuild"},
	{"TEAMCITY_VERSION", "teamcity"},
	{"BUILDKITE", "buildkite"},
	{"CF_BUILD_ID", "codefresh"},
	{"TRAVIS", "travis-ci"},
	{"CI", "ci-unknown"},
}

// IsDisabledByEnv checks if telemetry is disabled via the ASTRO_TELEMETRY_DISABLED env var.
func IsDisabledByEnv() bool {
	envVal := os.Getenv(envTelemetryDisabled)
	return envVal == "1" || strings.EqualFold(envVal, "true")
}

// NewAnonymousID generates a new anonymous UUID. Callers are responsible for persistence.
func NewAnonymousID() string {
	return uuid.New().String()
}

// GetTelemetryAPIURL returns the telemetry API URL, allowing override via env var
func GetTelemetryAPIURL() string {
	if url := os.Getenv(envTelemetryAPIURL); url != "" {
		return url
	}
	return TelemetryAPIURL
}

// DetectAgent returns the name of the detected agent (e.g. "claude-code"), or "" if none.
func DetectAgent() string {
	for _, m := range agentEnvVars {
		if os.Getenv(m.envVar) != "" {
			return m.name
		}
	}
	return ""
}

// DetectCISystem returns the name of the detected CI system (e.g. "github-actions"), or "" if none.
func DetectCISystem() string {
	for _, m := range ciEnvVars {
		if os.Getenv(m.envVar) != "" {
			return m.name
		}
	}
	return ""
}

// GetOSVersion returns the OS version string (e.g., "Darwin 24.3.0", "Linux 6.5.0")
func GetOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("cmd", "/c", "ver").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux", "darwin":
		out, err := exec.Command("uname", "-sr").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOOS
}

// isTestRun returns true if the current process is a Go test binary.
func isTestRun() bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasSuffix(executable, ".test") || strings.HasSuffix(executable, ".test.exe")
}

// isDebugMode returns true if synchronous debug mode is enabled
func isDebugMode() bool {
	val := os.Getenv(envTelemetryDebug)
	return val == "1" || strings.EqualFold(val, "true")
}

// sendDebug sends telemetry synchronously and prints debug output
func sendDebug(payload TelemetryPayload, apiURL string) {
	body, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Fprintf(os.Stderr, "[telemetry] POST %s\n%s\n", apiURL, body)

	status, err := Send(payload, apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[telemetry] error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[telemetry] response: %d OK\n", status)
}

// Track sends a telemetry event asynchronously in a goroutine.
// This is the main entry point for non-CLI callers (e.g., Desktop).
// It checks the env-var disable flag and skips sending during test runs.
func Track(payload TelemetryPayload) {
	if IsDisabledByEnv() || isTestRun() {
		return
	}

	apiURL := GetTelemetryAPIURL()

	if isDebugMode() {
		sendDebug(payload, apiURL)
		return
	}

	go func() {
		_, _ = Send(payload, apiURL)
	}()
}
