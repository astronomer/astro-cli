package cmd

import (
	"bufio"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/telemetry"
	sharedtel "github.com/astronomer/astro-cli/pkg/telemetry"
)

// telemetryOutput is the -o of `astro telemetry` and its two subcommands,
// registered once on the group.
var telemetryOutput string

// telemetryState is what `astro telemetry`, `enable` and `disable` publish:
// whether this CLI sends telemetry now, after the command ran.
// ASTRO_TELEMETRY_DISABLED outranks the setting, so after an enable with it
// set, enabled is false and disabled_by_env says why.
type telemetryState struct {
	Enabled       bool `json:"enabled"`
	DisabledByEnv bool `json:"disabled_by_env"`
}

func currentTelemetryState() telemetryState {
	return telemetryState{Enabled: telemetry.IsEnabled(), DisabledByEnv: sharedtel.IsDisabledByEnv()}
}

func newTelemetryCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage anonymous telemetry settings",
		// A local setting: nothing here needs a login.
		Annotations: map[string]string{astroCmd.NoLoginAnnotation: "true"},
		Long: `Manage anonymous telemetry settings for the Astro CLI.

Telemetry helps us understand how the CLI is used and improve it. We collect anonymous usage data including:
- Commands used (not arguments or values)
- CLI version
- Operating system
- Invocation context (CI, interactive, etc.)

No personally identifiable information is collected. You can opt out at any time using 'astro telemetry disable' or by setting the ASTRO_TELEMETRY_DISABLED=1 environment variable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelemetry(out, telemetryStatus)
		},
	}
	cliout.AddOutputFlag(cmd, &telemetryOutput)
	cmd.AddCommand(
		newTelemetryEnableCmd(out),
		newTelemetryDisableCmd(out),
	)
	return cmd
}

func newTelemetryEnableCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable anonymous telemetry",
		Long:  "Enable anonymous telemetry collection for the Astro CLI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelemetry(out, telemetryEnable)
		},
		Example: `  # Turn telemetry back on
  astro telemetry enable`,
	}
	return cmd
}

func newTelemetryDisableCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable anonymous telemetry",
		Long:  "Disable anonymous telemetry collection for the Astro CLI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelemetry(out, telemetryDisable)
		},
		Example: `  # Stop sending anonymous telemetry
  astro telemetry disable`,
	}
	return cmd
}

// runTelemetry runs one of the telemetry commands, which returns the line text
// prints, and publishes the state it left.
func runTelemetry(out io.Writer, run func() (string, error)) error {
	format, err := cliout.ParseFormat(telemetryOutput)
	if err != nil {
		return err
	}
	line, err := run()
	if err != nil {
		return err
	}
	state := currentTelemetryState()
	return cliout.Renderer{Format: format, Out: out}.Emit(&state, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, line)
	}))
}

func telemetryEnable() (string, error) {
	if err := config.CFG.TelemetryEnabled.SetHomeString("true"); err != nil {
		return "", fmt.Errorf("failed to enable telemetry: %w", err)
	}
	return "Telemetry enabled", nil
}

func telemetryDisable() (string, error) {
	if err := config.CFG.TelemetryEnabled.SetHomeString("false"); err != nil {
		return "", fmt.Errorf("failed to disable telemetry: %w", err)
	}
	return "Telemetry disabled", nil
}

func telemetryStatus() (string, error) {
	if telemetry.IsEnabled() {
		return "Telemetry is enabled", nil
	}
	return "Telemetry is disabled", nil
}
