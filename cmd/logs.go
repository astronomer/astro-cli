package cmd

import (
	"github.com/astronomer/astro-cli/logs"
	"github.com/spf13/cobra"
)

var (
	search string
	follow bool

	logsCmd = &cobra.Command{
		Use:     "logs",
		Aliases: []string{"log", "l"},
		Short:   "Stream logs from an Airflow deployment",
		Long: `Stream logs from an Airflow deployment. For example:

astro logs webserver YOU_DEPLOYMENT_ID -s string-to-find -f`,
	}

	webserverLogsCmd = &cobra.Command{
		Use:     "webserver",
		Aliases: []string{"web", "w"},
		Short:   "Stream logs from an Airflow webserver",
		Long: `Stream logs from an Airflow webserver. For example:

astro logs webserver YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: webserverRemoteLogs,
	}

	schedulerLogsCmd = &cobra.Command{
		Use:     "scheduler",
		Aliases: []string{"sch", "s"},
		Short:   "Stream logs from an Airflow scheduler",
		Long: `Stream logs from an Airflow scheduler. For example:

astro logs scheduler YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: schedulerRemoteLogs,
	}

	workersLogsCmd = &cobra.Command{
		Use:     "workers",
		Aliases: []string{"workers", "worker", "wrk"},
		Short:   "Stream logs from Airflow workers",
		Long: `Stream logs from Airflow workers. For example:

astro logs workers YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: workersRemoteLogs,
	}
)

func init() {
	RootCmd.AddCommand(logsCmd)
	webserverLogsCmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	webserverLogsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")

	// get airflow webserver logs
	logsCmd.AddCommand(webserverLogsCmd)

	workersLogsCmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	workersLogsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	// get airflow workers logs
	logsCmd.AddCommand(workersLogsCmd)

	schedulerLogsCmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	schedulerLogsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	// get airflow scheduler logs
	logsCmd.AddCommand(schedulerLogsCmd)
}

func webserverRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "webserver", search)
	}
	return logs.DeploymentLog(args[0], "webserver", search)
}

func schedulerRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "scheduler", search)
	}
	return logs.DeploymentLog(args[0], "scheduler", search)
}

func workersRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "workers", search)
	}
	return logs.DeploymentLog(args[0], "workers", search)
}
