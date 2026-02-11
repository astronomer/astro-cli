package telemetry

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/version"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	// SegmentWriteKeyProd is the production Segment write key
	SegmentWriteKeyProd = "pHCtHpueRKSdaMgrACkHXFxMaKxovlLR"
	// SegmentWriteKeyDev is the development Segment write key
	SegmentWriteKeyDev = "J8yCxB2fAmKTwRWg8rjkgeDqxYfAuEQT"

	// Environment variable to disable telemetry
	envTelemetryDisabled = "ASTRO_TELEMETRY_DISABLED"
	// Environment variable to override Segment write key
	envSegmentWriteKey = "ASTRO_SEGMENT_WRITE_KEY"
)

// Event types
const (
	EventCommandExecution = "Command Execution"
)

// TelemetryPayload represents the data sent to Segment
type TelemetryPayload struct {
	AnonymousID string                 `json:"anonymous_id"`
	Event       string                 `json:"event"`
	Properties  map[string]interface{} `json:"properties"`
	WriteKey    string                 `json:"write_key"`
}

// agentEnvVars maps environment variables to agent names
var agentEnvVars = map[string]string{
	"CLAUDECODE":             "claude-code",
	"CLAUDE_CODE_ENTRYPOINT": "claude-code",
	"CURSOR_TRACE_ID":        "cursor",
	"AIDER_MODEL":            "aider",
	"CONTINUE_GLOBAL_DIR":    "continue",
}

// ciEnvVars maps environment variables to CI system names
var ciEnvVars = map[string]string{
	"GITHUB_ACTIONS": "github-actions",
	"GITLAB_CI":      "gitlab-ci",
	"JENKINS_URL":    "jenkins",
	"CIRCLECI":       "circleci",
	"CI":             "ci-unknown",
}

// IsEnabled checks if telemetry is enabled
func IsEnabled() bool {
	// Check environment variable first (takes precedence)
	envVal := os.Getenv(envTelemetryDisabled)
	if envVal == "1" || strings.EqualFold(envVal, "true") {
		return false
	}

	// Check config setting
	return config.CFG.TelemetryEnabled.GetBool()
}

// GetAnonymousID returns the anonymous user ID, creating one if it doesn't exist
func GetAnonymousID() string {
	existingID := config.CFG.TelemetryAnonymousID.GetHomeString()
	if existingID != "" {
		return existingID
	}

	// Generate new UUID
	newID := uuid.New().String()
	_ = config.CFG.TelemetryAnonymousID.SetHomeString(newID)
	return newID
}

// GetSegmentWriteKey returns the appropriate Segment write key
func GetSegmentWriteKey() string {
	// Check for environment variable override
	if key := os.Getenv(envSegmentWriteKey); key != "" {
		return key
	}

	// Use dev key for SNAPSHOT builds, prod key otherwise
	if strings.Contains(version.CurrVersion, "SNAPSHOT") {
		return SegmentWriteKeyDev
	}
	return SegmentWriteKeyProd
}

// GetCommandPath extracts the command path from a cobra.Command
// Returns the full command path (e.g., "deploy", "dev start")
func GetCommandPath(cmd *cobra.Command) string {
	// Get the full command path
	path := cmd.CommandPath()
	// Remove the root command name ("astro")
	parts := strings.SplitN(path, " ", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	// Return empty string for root command
	return ""
}

// DetectContext detects the invocation context (agent, CI, or interactive)
func DetectContext() string {
	// Check for agent environment variables first
	for envVar, agentName := range agentEnvVars {
		if os.Getenv(envVar) != "" {
			return agentName
		}
	}

	// Check for CI environment variables
	for envVar, ciName := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return ciName
		}
	}

	return "interactive"
}

// TrackCommand sends telemetry data for a command execution
// It spawns a subprocess to send the data asynchronously
func TrackCommand(cmd *cobra.Command) {
	if !IsEnabled() {
		return
	}

	commandPath := GetCommandPath(cmd)
	// Don't track root command, hidden commands, or telemetry commands
	if commandPath == "" || cmd.Hidden || strings.HasPrefix(commandPath, "telemetry") || strings.HasPrefix(commandPath, "_telemetry") {
		return
	}

	payload := TelemetryPayload{
		AnonymousID: GetAnonymousID(),
		Event:       EventCommandExecution,
		WriteKey:    GetSegmentWriteKey(),
		Properties: map[string]interface{}{
			"command":        commandPath,
			"cli_version":    version.CurrVersion,
			"os":             runtime.GOOS,
			"os_version":     getOSVersion(),
			"go_version":     runtime.Version(),
			"context":        DetectContext(),
			"cli_name":       "astro-cli",
			"platform":       runtime.GOARCH,
		},
	}

	// Spawn subprocess to send telemetry
	spawnTelemetrySender(payload)
}

// getOSVersion returns the OS version string
func getOSVersion() string {
	// For simplicity, we use runtime.GOOS + runtime.GOARCH
	// A more detailed version could use platform-specific APIs
	return runtime.GOOS + "/" + runtime.GOARCH
}

// spawnTelemetrySender spawns a detached subprocess to send telemetry
func spawnTelemetrySender(payload TelemetryPayload) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return
	}

	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return
	}

	// Create command to run astro _telemetry-send
	cmd := exec.Command(executable, "_telemetry-send")
	cmd.Stdin = strings.NewReader(string(payloadJSON))

	// Detach the process so it doesn't block the main CLI
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Start the process without waiting
	_ = cmd.Start()

	// Don't wait for the process to complete
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

// CreateTrackingHook returns a RunE function that tracks command execution
func CreateTrackingHook() func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		TrackCommand(cmd)
		return nil
	}
}
