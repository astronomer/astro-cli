package cmd

import (
	"io"
	"time"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/logs"
	"github.com/spf13/cobra"
)

var (
	search      string
	follow      bool
	since       time.Duration
	logsExample = `
  # Return logs for last 5 minutes of webserver logs and output them.
  astro deployment logs webserver example-deployment-uuid

  # Subscribe logs from airflow workers for last 5 min and specify search term, and subscribe to more.
  astro deployment logs workers example-deployment-uuid --follow --search "some search terms"
  
  # Return logs from airflow webserver for last 25 min.
  astro deployment logs webserver example-deployment-uuid --since 25m

  # Subscribe logs from airflow scheduler.
  astro deployment logs scheduler example-deployment-uuid -f
`
)

func newLogsCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Aliases: []string{"log", "l"},
		Short:   "Stream logs from an Airflow deployment",
		Long:    "Stream logs from an Airflow deployment",
		Example: logsExample,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			PersistentPreRunCheck(client, cmd, out)
		},
	}
	cmd.AddCommand(
		newWebserverLogsCmd(client, out),
		newSchedulerLogsCmd(client, out),
		newWorkersLogsCmd(client, out),
	)
	return cmd
}

func newLogsDeprecatedCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "logs",
		Aliases:    []string{"log", "l"},
		Short:      "Stream logs from an Airflow deployment",
		Long:       "Stream logs from an Airflow deployment",
		Example:    logsExample,
		Deprecated: "could please use new command instead `astro deployment logs [subcommands] [flags]`",
	}
	cmd.AddCommand(
		newWebserverLogsCmd(client, out),
		newSchedulerLogsCmd(client, out),
		newWorkersLogsCmd(client, out),
	)
	return cmd
}

func newWebserverLogsCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webserver",
		Aliases: []string{"web", "w"},
		Short:   "Stream logs from an Airflow webserver",
		Long: `Stream logs from an Airflow webserver. For example:

astro deployment logs webserver YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: webserverRemoteLogs,
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	return cmd
}

func newSchedulerLogsCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scheduler",
		Aliases: []string{"sch", "s"},
		Short:   "Stream logs from an Airflow scheduler",
		Long: `Stream logs from an Airflow scheduler. For example:

astro deployment logs scheduler YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: schedulerRemoteLogs,
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	return cmd
}

func newWorkersLogsCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workers",
		Aliases: []string{"workers", "worker", "wrk"},
		Short:   "Stream logs from Airflow workers",
		Long: `Stream logs from Airflow workers. For example:

astro deployment logs workers YOU_DEPLOYMENT_ID -s string-to-find
`,
		Args: cobra.ExactArgs(1),
		RunE: workersRemoteLogs,
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	// get airflow workers logs
	return cmd
}

func webserverRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "webserver", search, since)
	}
	return logs.DeploymentLog(args[0], "webserver", search, since)
}

func schedulerRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "scheduler", search, since)
	}
	return logs.DeploymentLog(args[0], "scheduler", search, since)
}

func workersRemoteLogs(cmd *cobra.Command, args []string) error {
	if follow {
		return logs.SubscribeDeploymentLog(args[0], "workers", search, since)
	}
	return logs.DeploymentLog(args[0], "workers", search, since)
}
