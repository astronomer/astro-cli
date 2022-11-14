package cmd

import (
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	dagID     string
	taskLogs  bool
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run DAG-ID",
		Short: "Run a local DAG withpython by running its tasks sequentially",
		Long:  "Run a local DAG by running its tasks sequentially. This command will spin up a docker airflow environment and execute your DAG code",
		Args:  cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    run,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().BoolVarP(&taskLogs, "task-logs", "", false, "Show task logs while the DAG is running")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings or env file export objects too")

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if len(args) > 0 {
		dagID = args[0]
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.RunDAG(dagID, settingsFile, noCache, taskLogs)
}
