package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	v2deploy "github.com/astronomer/astro-cli/internal/deploy"
)

var (
	dagID         string
	dagFile       string
	executionDate string
	taskLogs      bool
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run DAG-ID",
		Short: "Run a local DAG with Python by running its tasks sequentially",
		Long:  "Run a local DAG by running its tasks sequentially. This command spins up a single Airflow worker to execute your DAG code. It parses all files in your dags folder if the --dag-file flag is not used. Use the --dag-file flag to only parse the DAG file where your DAG is defined.",
		Args:  cobra.ExactArgs(1),
		// Not listed. astro local run is the v2 spelling, and this command
		// answers only for the v1 projects that still have it.
		Hidden: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// In a v2 project the v1 check below fails with advice to run
			// astro dev init, a command v2 removed, which left no way out.
			// Name the command that does this job instead.
			if v2deploy.IsV2Project(config.WorkingPath) {
				cmd.SilenceUsage = true
				dag := ""
				if len(args) == 1 {
					dag = " " + args[0]
				}
				return fmt.Errorf("`astro run` was removed in Astro CLI v2. Use `astro local run airflow dags test%s` instead", dag)
			}
			return utils.EnsureProjectDir(cmd, args)
		},
		RunE: run,
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
