package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/version"

	sharedtel "github.com/astronomer/astro-cli/pkg/telemetry"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	// SourceName identifies this CLI as the telemetry source
	SourceName = "astro-cli"

	// SkipPreRunAnnotation is the cobra annotation key used to skip PersistentPreRunE
	SkipPreRunAnnotation = "skipPreRun"

	// EventCommandExecution is the event type for CLI command tracking
	EventCommandExecution = "CLI Command"
)

// IsEnabled checks if telemetry is enabled via env var and config
func IsEnabled() bool {
	if sharedtel.IsDisabledByEnv() {
		return false
	}
	return config.CFG.TelemetryEnabled.GetBool()
}

// GetAnonymousID returns the anonymous user ID, creating one if it doesn't exist
func GetAnonymousID() string {
	existingID := config.CFG.TelemetryAnonymousID.GetHomeString()
	if existingID != "" {
		return existingID
	}
	newID := sharedtel.NewAnonymousID()
	_ = config.CFG.TelemetryAnonymousID.SetHomeString(newID)
	return newID
}

// IsInteractive returns true if stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// GetCommandPath extracts the command path from a cobra.Command
// Returns the full command path (e.g., "deploy", "dev start")
func GetCommandPath(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	parts := strings.SplitN(path, " ", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// showFirstRunNotice prints a one-time notice about telemetry on the first CLI invocation.
func showFirstRunNotice() {
	if config.CFG.TelemetryNoticeShown.GetHomeString() != "" {
		return
	}
	fmt.Fprintln(os.Stderr,
		"The Astro CLI now collects anonymous usage data to help us prioritize and invest in CLI features.\n"+
			"Only commands, OS, and CLI version are tracked — no personal information is collected.\n"+
			"Opt out anytime: `astro telemetry disable` or ASTRO_TELEMETRY_DISABLED=1")
	_ = config.CFG.TelemetryNoticeShown.SetHomeString("true")
}

// TrackCommand sends telemetry data for a command execution.
// It spawns a subprocess to send the data asynchronously.
func TrackCommand(cmd *cobra.Command) {
	if !IsEnabled() || isTestRun() {
		return
	}

	showFirstRunNotice()

	commandPath := GetCommandPath(cmd)
	if commandPath == "" || cmd.Hidden || strings.HasPrefix(commandPath, "telemetry") || strings.HasPrefix(commandPath, "_telemetry") {
		return
	}

	context := "non-interactive"
	if IsInteractive() {
		context = "interactive"
	}

	properties := map[string]interface{}{
		"command":      commandPath,
		"cli_version":  version.CurrVersion,
		"os":           runtime.GOOS,
		"os_version":   sharedtel.GetOSVersion(),
		"go_version":   runtime.Version(),
		"context":      context,
		"architecture": runtime.GOARCH,
	}

	if agent := sharedtel.DetectAgent(); agent != "" {
		properties["agent"] = agent
	}
	if ciSystem := sharedtel.DetectCISystem(); ciSystem != "" {
		properties["ci_system"] = ciSystem
	}

	payload := sharedtel.TelemetryPayload{
		Source:      SourceName,
		Event:       EventCommandExecution,
		AnonymousID: GetAnonymousID(),
		Properties:  properties,
	}

	apiURL := sharedtel.GetTelemetryAPIURL()

	if isDebugMode() {
		sendDebug(payload, apiURL)
		return
	}

	spawnTelemetrySender(payload, apiURL)
}

// CreateTrackingHook returns a RunE function that tracks command execution
func CreateTrackingHook() func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		TrackCommand(cmd)
		return nil
	}
}

// SendEvent re-exports the shared package's SendEvent for the _telemetry-send command.
func SendEvent() error {
	return sharedtel.SendEvent()
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
	val := os.Getenv("ASTRO_TELEMETRY_DEBUG")
	return val == "1" || strings.EqualFold(val, "true")
}

// sendDebug sends telemetry synchronously and prints debug output
func sendDebug(payload sharedtel.TelemetryPayload, apiURL string) {
	body, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Fprintf(os.Stderr, "[telemetry] POST %s\n%s\n", apiURL, body)

	status, err := sharedtel.Send(payload, apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[telemetry] error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[telemetry] response: %d OK\n", status)
}

// spawnTelemetrySender spawns a detached subprocess to send telemetry
func spawnTelemetrySender(payload sharedtel.TelemetryPayload, apiURL string) {
	type senderPayload struct {
		sharedtel.TelemetryPayload
		APIURL string `json:"api_url"`
	}

	sp := senderPayload{
		TelemetryPayload: payload,
		APIURL:           apiURL,
	}
	payloadJSON, err := json.Marshal(sp)
	if err != nil {
		return
	}

	executable, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(executable, "_telemetry-send")
	cmd.Stdin = strings.NewReader(string(payloadJSON))
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		if isDebugMode() {
			fmt.Fprintf(os.Stderr, "[telemetry] failed to spawn sender: %v\n", err)
		}
		return
	}

	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}
