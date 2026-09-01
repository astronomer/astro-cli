package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

func newTelemetryCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage telemetry settings",
		Long: `Manage telemetry settings for the Astro CLI.

Telemetry helps us understand how the CLI is used and improve it.
We collect usage data including:
- Commands used (not arguments or values)
- Commands and flags the CLI does not have, when one is typed
- CLI version
- Operating system
- Invocation context (CI, interactive, etc.)

While you are logged in, events are linked to your Astro organization so we can
see how the CLI is used across accounts. Logged-out usage stays anonymous.

No personally identifiable information is collected.
You can opt out at any time using 'astro telemetry disable' or by setting
the ASTRO_TELEMETRY_DISABLED=1 environment variable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return telemetryStatus(out)
		},
	}
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
			return telemetryEnable(out)
		},
	}
	return cmd
}

func newTelemetryDisableCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable anonymous telemetry",
		Long:  "Disable anonymous telemetry collection for the Astro CLI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return telemetryDisable(out)
		},
	}
	return cmd
}

func telemetryEnable(out io.Writer) error {
	if err := config.CFG.TelemetryEnabled.SetHomeString("true"); err != nil {
		return fmt.Errorf("failed to enable telemetry: %w", err)
	}
	fmt.Fprintln(out, "Telemetry enabled")
	return nil
}

func telemetryDisable(out io.Writer) error {
	if err := config.CFG.TelemetryEnabled.SetHomeString("false"); err != nil {
		return fmt.Errorf("failed to disable telemetry: %w", err)
	}
	fmt.Fprintln(out, "Telemetry disabled")
	return nil
}

func telemetryStatus(out io.Writer) error {
	enabled := telemetry.IsEnabled()
	if enabled {
		fmt.Fprintln(out, "Telemetry is enabled")
	} else {
		fmt.Fprintln(out, "Telemetry is disabled")
	}
	return nil
}
