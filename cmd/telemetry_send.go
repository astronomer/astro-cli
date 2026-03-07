package cmd

import (
	"github.com/astronomer/astro-cli/pkg/telemetry"
	"github.com/spf13/cobra"
)

func newTelemetrySendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "_telemetry-send",
		Short:         "Send telemetry data (internal use only)",
		Long:          "Internal command used to send telemetry data asynchronously. Reads JSON payload from stdin.",
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Annotations:   map[string]string{telemetry.SkipPreRunAnnotation: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return telemetry.SendEvent()
		},
	}
	return cmd
}
