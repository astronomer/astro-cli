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

// agentEnvVars is an ordered list of environment variables to detect agent
// contexts. Order matters: the first match wins. Otto is first because it
// passes its own environment through to every command it runs, so an Otto
// session started from inside another agent still carries that agent's marker.
// Otto is the proximate caller, so it takes precedence.
var agentEnvVars = []envMapping{
	{"OTTO", "otto"},
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
	// GitHub Copilot. COPILOT_CLI is set by the Copilot CLI, which is also the
	// harness behind the cloud coding agent and, as of 2026, Copilot for
	// JetBrains; github's own gh keys on it (cli/cli internal/agents/detect.go).
	// COPILOT_AGENT is set on terminals VS Code creates for agent mode
	// (microsoft/vscode toolTerminalCreator.ts), where it is described as
	// backward compatibility for exactly this kind of detection.
	{"COPILOT_CLI", "github-copilot"},
	{"COPILOT_AGENT", "github-copilot"},
}

// aiAgentEnvVar is a cross-vendor convention where the value names the agent
// rather than the variable: VS Code sets AI_AGENT=github_copilot_vscode_agent
// for agent sessions and the Copilot desktop app sets github_copilot_app_agent
// (microsoft/vscode aiAgentEnv.ts). gh reads it too.
//
// It is checked after agentEnvVars so a specific marker still wins, and only
// prefixes we recognise are mapped. Values are not passed through: some vendors
// put a version in theirs (claude-code_2-1-156_agent), which would spray one
// agent across dozens of distinct values in the telemetry. A new vendor adopting
// the convention is a one-line addition here.
const aiAgentEnvVar = "AI_AGENT"

var aiAgentPrefixes = []envMapping{
	{"github_copilot", "github-copilot"},
}

// detectAIAgent maps the AI_AGENT convention to an agent name, or "" if the
// value is empty or not one we recognise.
func detectAIAgent() string {
	value := strings.ToLower(os.Getenv(aiAgentEnvVar))
	if value == "" {
		return ""
	}
	for _, m := range aiAgentPrefixes {
		if strings.HasPrefix(value, m.envVar) {
			return m.name
		}
	}
	return ""
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
	return detectAIAgent()
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
