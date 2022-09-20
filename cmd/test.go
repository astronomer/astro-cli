package cmd

import (
	"github.com/spf13/cobra"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
)

var (
	dagID     string
	startDate string
)

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test DAG-ID",
		Short: "Test a local DAG by running its tasks sequentially",
		Long:  "Test a local DAG by running its tasks sequentially. This command will spin up a docker container with airflow environment and execute your DAG code",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    test,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings or env file export objects too")
	cmd.Flags().StringVarP(&startDate, "start-date", "d", "", "The start date for the DAG this is the current date and time by defualt")

	return cmd
}

func test(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if len(args) > 0 {
		dagID = args[0]
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.RunTest(dagID, settingsFile, startDate, noCache)
}
