package cmd

import (
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	dagID    string
	dagFile  string
	taskLogs bool
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run DAG-ID",
		Short: "Run a local DAG with python by running its tasks sequentially",
		Long:  "Run a local DAG by running its tasks sequentially. This command will spin up a docker airflow environment and execute your DAG code. It will parse all the files in your dags folder if the --dag-file flag is not used. Use the --dag-file flag to only parse the DAG file where your DAG is defined.",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    run,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings file from which to import airflow objects")
	cmd.Flags().StringVarP(&dagFile, "dag-file", "d", "", "DAG file where your DAG is located(optional). Use this flag to parse only the DAG file that has the DAG you want to run. You may get parsing errors related to other DAGs if you don't specify a DAG file")

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

	return containerHandler.RunDAG(dagID, settingsFile, dagFile, noCache, taskLogs)
}
