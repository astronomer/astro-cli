package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/config"
	sharedtel "github.com/astronomer/astro-cli/pkg/telemetry"
	"github.com/astronomer/astro-cli/version"
)

const (
	// SourceName identifies this CLI as the telemetry source
	SourceName = "astro-cli"

	// SkipPreRunAnnotation is the cobra annotation key used to skip PersistentPreRunE
	SkipPreRunAnnotation = "skipPreRun"

	// DevModeAnnotation is the cobra annotation key for the resolved dev mode
	// ("standalone" or "docker"). Set by the dev command's pre-run hook.
	DevModeAnnotation = "dev_mode"

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

// currentToken returns the bearer token for the active context, or "" when the
// user is logged out — in which case the event still sends, unattributed.
//
// Deliberately does not refresh an expired token: the sender runs in a detached
// subprocess, so refreshing would race the main process for ~/.astro/config.yaml.
// A stale token costs one unattributed event.
// Returns the raw token: contexts persist it with a "Bearer " prefix already
// (see cmd/cloud/setup.go), while ASTRO_API_TOKEN holds it without one.
// SendWithToken supplies the scheme.
func currentToken() string {
	if t := os.Getenv("ASTRO_API_TOKEN"); t != "" {
		return strings.TrimPrefix(t, "Bearer ")
	}
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(ctx.Token, "Bearer ")
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

// noticeVersion tracks the wording of the notice below. Bump it whenever the
// notice makes a materially different claim about what is collected, so users
// who accepted earlier wording see the change instead of inheriting it silently.
// v1 predates this constant and is stored as "true".
const noticeVersion = "2"

// showFirstRunNotice prints a notice about telemetry on the first CLI invocation,
// and again whenever noticeVersion changes.
func showFirstRunNotice() {
	if config.CFG.TelemetryNoticeShown.GetHomeString() == noticeVersion {
		return
	}
	fmt.Fprintln(os.Stderr,
		"The Astro CLI collects usage data to help us prioritize and invest in CLI features.\n"+
			"Commands, OS, and CLI version are tracked — never arguments or their values.\n"+
			"While you are logged in, events are linked to your Astro organization.\n"+
			"Logged-out usage stays anonymous.\n"+
			"Opt out anytime: `astro telemetry disable` or ASTRO_TELEMETRY_DISABLED=1")
	_ = config.CFG.TelemetryNoticeShown.SetHomeString(noticeVersion)
}

// buildCommandProperties constructs the telemetry property map for a command.
// Extracted so it can be tested independently of the send path.
func buildCommandProperties(cmd *cobra.Command) map[string]interface{} {
	context := "non-interactive"
	if IsInteractive() {
		context = "interactive"
	}

	properties := map[string]interface{}{
		"command":      GetCommandPath(cmd),
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

	if mode := cmd.Annotations[DevModeAnnotation]; mode != "" {
		properties[DevModeAnnotation] = mode
	}

	return properties
}

// TrackCommand sends telemetry data for a command execution.
// It spawns a subprocess to send the data asynchronously.
func TrackCommand(cmd *cobra.Command) {
	if !IsEnabled() || isTestRun() {
		return
	}

	commandPath := GetCommandPath(cmd)
	if commandPath == "" || cmd.Hidden || strings.HasPrefix(commandPath, "telemetry") || strings.HasPrefix(commandPath, "_telemetry") {
		return
	}

	// After the filter above, so that commands which send nothing say nothing —
	// notably `astro telemetry disable`, which otherwise announces collection to
	// someone in the act of opting out.
	showFirstRunNotice()

	properties := buildCommandProperties(cmd)

	payload := sharedtel.TelemetryPayload{
		Source:      SourceName,
		Event:       EventCommandExecution,
		AnonymousID: GetAnonymousID(),
		Properties:  properties,
	}

	apiURL := sharedtel.GetTelemetryAPIURL()
	token := currentToken()

	if isDebugMode() {
		sendDebug(payload, apiURL, token)
		return
	}

	spawnTelemetrySender(payload, apiURL, token)
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

// sendDebug sends telemetry synchronously and prints debug output.
// It prints the event body and whether a token was attached, never the token.
func sendDebug(payload sharedtel.TelemetryPayload, apiURL, token string) {
	body, _ := json.MarshalIndent(payload, "", "  ")
	authState := "anonymous"
	if token != "" {
		authState = "authenticated"
	}
	fmt.Fprintf(os.Stderr, "[telemetry] POST %s (%s)\n%s\n", apiURL, authState, body)

	status, err := sharedtel.SendWithToken(payload, apiURL, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[telemetry] error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[telemetry] response: %d OK\n", status)
}

// spawnTelemetrySender spawns a detached subprocess to send telemetry
func spawnTelemetrySender(payload sharedtel.TelemetryPayload, apiURL, token string) {
	sp := sharedtel.SenderPayload{
		TelemetryPayload: payload,
		APIURL:           apiURL,
		Token:            token,
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
