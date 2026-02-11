package cmd

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/telemetry"
	"github.com/spf13/cobra"
)

func newTelemetryCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage anonymous telemetry settings",
		Long: `Manage anonymous telemetry settings for the Astro CLI.

Telemetry helps us understand how the CLI is used and improve it.
We collect anonymous usage data including:
- Commands used (not arguments or values)
- CLI version
- Operating system
- Invocation context (CI, interactive, etc.)

No personally identifiable information is collected.
You can opt out at any time using 'astro telemetry disable' or by setting
the ASTRO_TELEMETRY_DISABLED=1 environment variable.`,
	}
	cmd.AddCommand(
		newTelemetryEnableCmd(out),
		newTelemetryDisableCmd(out),
		newTelemetryStatusCmd(out),
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

func newTelemetryStatusCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current telemetry status",
		Long:  "Show whether anonymous telemetry is currently enabled or disabled.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return telemetryStatus(out)
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
