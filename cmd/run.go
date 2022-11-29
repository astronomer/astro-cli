package cmd

import (
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	dagID    string
	taskLogs bool
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run DAG-ID",
		Short: "Run a local DAG with python by running its tasks sequentially",
		Long:  "Run a local DAG by running its tasks sequentially. This command will spin up a docker airflow environment and execute your DAG code. It will parse all the files in your dags folder. Remove files you do not wish to be parsed.",		
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    run,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings file from which to import airflow objects. If you create this file while airflow is running make sure restart airflow")

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
