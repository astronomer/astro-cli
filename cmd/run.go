package cmd

import (
	"fmt"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	dagID         string
	dagFile       string
	executionDate string
	taskLogs      bool
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run DAG-ID",
		Short:   "Run a local DAG with Python by running its tasks sequentially",
		Long:    "Run a local DAG by running its tasks sequentially. This command spins up a single Airflow worker to execute your DAG code. It parses all files in your dags folder if the --dag-file flag is not used. Use the --dag-file flag to only parse the DAG file where your DAG is defined.",
		Args:    cobra.ExactArgs(1),
		PreRunE: utils.EnsureProjectDir,
		RunE:    run,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings file for importing Airflow objects")
	cmd.Flags().StringVarP(&dagFile, "dag-file", "d", "", "(Optional) The file where your DAG is located. Use this flag to parse only the DAG file that has the DAG you want to run. You may get parsing errors related to other DAGs if you don't specify a DAG file")
	cmd.Flags().StringVarP(&executionDate, "execution-date", "", "", "(Optional) Execution date for the dagrun. Defaults to now. Acceptable date formats: %Y-%m-%d, %Y-%m-%dT%H:%M:%S, %Y-%m-%d %H:%M:%S")
	cmd.Flags().BoolVarP(&taskLogs, "verbose", "", false, "(Optional) Print out the logs of the dag run")

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if config.CFG.DisableAstroRun.GetBool() {
		fmt.Println("The 'astro run' command is currently disabled. Run 'astro config set disable_astro_run false' to enable it")

		return nil
	}

	if len(args) > 0 {
		dagID = args[0]
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.RunDAG(dagID, settingsFile, dagFile, executionDate, noCache, taskLogs)
}
