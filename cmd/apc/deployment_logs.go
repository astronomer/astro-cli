package apc

import (
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
)

const (
	logWebserver = "webserver"
	logScheduler = "scheduler"
	logWorker    = "worker"
	logTriggerer = "triggerer"
)

var (
	search      string
	follow      bool
	since       time.Duration
	logsExample = `
  # Return logs for last 5 minutes of webserver logs and output them.
  astro deployment logs webserver example-deployment-uuid

  # Follow worker logs from the last 5 minutes that match a search term
  astro deployment logs workers example-deployment-uuid --follow --search "some search terms"

  # Return logs from airflow webserver for last 25 min.
  astro deployment logs webserver example-deployment-uuid --since 25m

  # Subscribe logs from airflow scheduler.
  astro deployment logs scheduler example-deployment-uuid -f
`
)

func newLogsCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Aliases: []string{"log", "l"},
		Short:   "Stream logs from an Airflow Deployment",
		Long:    "Stream logs from an Airflow Deployment",
		Example: logsExample,
	}
	cmd.AddCommand(
		newWebserverLogsCmd(out),
		newSchedulerLogsCmd(out),
		newWorkersLogsCmd(out),
	)

	if appConfig != nil && appConfig.Flags.TriggererEnabled {
		cmd.AddCommand(newTriggererLogsCmd(out))
	}

	return cmd
}

func newWebserverLogsCmd(out io.Writer) *cobra.Command { //nolint:dupl // the duplication is acceptable here
	cmd := &cobra.Command{
		Use:     "webserver <DEPLOYMENT_ID>",
		Aliases: []string{"web", "w"},
		Short:   "Stream logs from an Airflow webserver",
		Long:    "Stream logs from an Airflow webserver",
		Example: `  # Show a Deployment's webserver logs from the last 25 minutes
  astro deployment logs webserver <DEPLOYMENT_ID> --since 25m

  # Follow new ones that contain a search term
  astro deployment logs webserver <DEPLOYMENT_ID> --follow --search "some search terms"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fetchRemoteLogs(logWebserver, args, out)
		},
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	return cmd
}

func newSchedulerLogsCmd(out io.Writer) *cobra.Command { //nolint:dupl // the duplication is acceptable here
	cmd := &cobra.Command{
		Use:     "scheduler <DEPLOYMENT_ID>",
		Aliases: []string{"sch", "s"},
		Short:   "Stream logs from an Airflow scheduler",
		Long:    "Stream logs from an Airflow scheduler",
		Example: `  # Show a Deployment's scheduler logs from the last 25 minutes
  astro deployment logs scheduler <DEPLOYMENT_ID> --since 25m

  # Follow new ones that contain a search term
  astro deployment logs scheduler <DEPLOYMENT_ID> --follow --search "some search terms"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fetchRemoteLogs(logScheduler, args, out)
		},
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	return cmd
}

func newWorkersLogsCmd(out io.Writer) *cobra.Command { //nolint:dupl // the duplication is acceptable here
	cmd := &cobra.Command{
		Use:     "workers <DEPLOYMENT_ID>",
		Aliases: []string{"worker", "wrk"},
		Short:   "Stream logs from Airflow workers",
		Long:    "Stream logs from Airflow workers",
		Example: `  # Show a Deployment's worker logs from the last 25 minutes
  astro deployment logs workers <DEPLOYMENT_ID> --since 25m

  # Follow new ones that contain a search term
  astro deployment logs workers <DEPLOYMENT_ID> --follow --search "some search terms"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fetchRemoteLogs(logWorker, args, out)
		},
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	// get airflow workers logs
	return cmd
}

func newTriggererLogsCmd(out io.Writer) *cobra.Command { //nolint:dupl // the duplication is acceptable here
	cmd := &cobra.Command{
		Use:     "triggerer <DEPLOYMENT_ID>",
		Aliases: []string{"triggerers", "trg"},
		Short:   "Stream logs from Airflow triggerer",
		Long:    "Stream logs from Airflow triggerer",
		Example: `  # Show a Deployment's triggerer logs from the last 25 minutes
  astro deployment logs triggerer <DEPLOYMENT_ID> --since 25m

  # Follow new ones that contain a search term
  astro deployment logs triggerer <DEPLOYMENT_ID> --follow --search "some search terms"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fetchRemoteLogs(logTriggerer, args, out)
		},
	}
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term inside logs")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Subscribe to watch more logs")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h")
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	// get airflow workers logs
	return cmd
}

func fetchRemoteLogs(component string, args []string, out io.Writer) error {
	if follow {
		return deployment.SubscribeDeploymentLog(args[0], component, search, since)
	}
	return deployment.Log(args[0], component, search, since, houstonClient, out)
}
