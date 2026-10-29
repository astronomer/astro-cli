package apc

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// Houston's names for the components a Deployment logs, one per request.
const (
	logWebserver = "webserver"
	logScheduler = "scheduler"
	logWorker    = "worker"
	logTriggerer = "triggerer"

	// logWorkers is the workers' flag, and 1.x's subcommand.
	logWorkers = "workers"

	// maxLogsSince is the longest window Houston searches (2 days).
	maxLogsSince = 48 * time.Hour
)

var (
	search     string
	follow     bool
	since      time.Duration
	logsOutput cliout.Format

	// The components astro deployment logs picks, as Astro's does: a flag
	// each, or --component by name. None is every one.
	logsWebserver  bool
	logsScheduler  bool
	logsWorkers    bool
	logsTriggerer  bool
	logsComponents []string

	// subscribeLogs is a variable so a test can follow a stream without a
	// websocket server.
	subscribeLogs = deployment.SubscribeDeploymentLog
)

// The --component names, as Houston spells them, with workers and
// triggerers too, as the flag and an earlier v2's subcommand spelled them.
var logComponentNames = map[string]string{
	logWebserver: logWebserver,
	logScheduler: logScheduler,
	logWorker:    logWorker,
	logWorkers:   logWorker,
	logTriggerer: logTriggerer,
	"triggerers": logTriggerer,
}

// logComponentNamesProse is the --component names the platform takes, for
// help and refusals: Houston's worker beside the flag's workers.
func logComponentNamesProse(triggerer bool) string {
	return strings.Replace(strings.Join(allLogComponents(triggerer), ", "), logWorker, logWorker+" or "+logWorkers, 1)
}

// logComponentFlag is the flag that picks component.
func logComponentFlag(component string) string {
	if component == logWorker {
		return logWorkers
	}
	return component
}

// newLogsCmd is astro deployment logs [DEPLOYMENT_ID], shaped as Astro's:
// flags pick the components, every one when none does. 1.x's layout, a
// subcommand per component, still runs, hidden (newComponentLogsCmd).
func newLogsCmd(out io.Writer) *cobra.Command {
	triggerer := appConfig != nil && appConfig.Flags.TriggererEnabled
	cmd := &cobra.Command{
		Use:     "logs [DEPLOYMENT_ID]",
		Aliases: []string{"log", "l"},
		Short:   "Show an Airflow Deployment's logs",
		Long:    "Show an Airflow Deployment's logs, from every component unless flags such as --scheduler or --component pick some, for the Deployment named by its id or with --deployment, which takes a link name from the project's manifest too. --follow streams one component's logs as they come.",
		Example: `  # Show a Deployment's logs from every component, today
  astro deployment logs <DEPLOYMENT_ID>

  # Show the scheduler's and the workers' logs from the last 25 minutes
  astro deployment logs <DEPLOYMENT_ID> --scheduler --workers --since 25m

  # Follow the webserver's logs that contain a phrase
  astro deployment logs <DEPLOYMENT_ID> --webserver --follow --keyword "task failed"

  # Show the logs of the Deployment a link of this project names
  astro deployment logs --deployment <LINK_NAME>`,
		Args: logsArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			components, err := pickLogComponents(triggerer)
			if err != nil {
				return err
			}
			return showLogs(cmd, components, out)
		},
	}
	addDeploymentArgFlag(cmd)
	addLogsFlags(cmd, false)
	cmd.Flags().BoolVar(&logsWebserver, "webserver", false, "Show logs from the webserver")
	cmd.Flags().BoolVar(&logsScheduler, "scheduler", false, "Show logs from the scheduler")
	cmd.Flags().BoolVar(&logsWorkers, logWorkers, false, "Show logs from the workers")
	if triggerer {
		cmd.Flags().BoolVar(&logsTriggerer, "triggerer", false, "Show logs from the triggerer")
	}
	cmd.Flags().StringSliceVar(&logsComponents, "component", nil, "Show logs from a component by name ("+logComponentNamesProse(triggerer)+"), repeatable or comma-separated, instead of a flag such as --scheduler")

	cmd.AddCommand(
		newComponentLogsCmd(out, "webserver", []string{"web", "w"}, logWebserver),
		newComponentLogsCmd(out, "scheduler", []string{"sch", "s"}, logScheduler),
		newComponentLogsCmd(out, logWorkers, []string{logWorker, "wrk"}, logWorker),
	)
	return cmd
}

// newComponentLogsCmd is 1.x's astro deployment logs <component>
// [DEPLOYMENT_ID], hidden: a 1.x script still runs as it did, with
// --search, while help shows astro deployment logs, whose flags pick the
// component.
func newComponentLogsCmd(out io.Writer, use string, aliases []string, component string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use + " [DEPLOYMENT_ID]",
		Aliases: aliases,
		Hidden:  true,
		Short:   "Stream logs from an Airflow " + use,
		Long:    "1.x's spelling of astro deployment logs --" + use + ", kept for scripts written for Astro CLI 1.x",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showLogs(cmd, []string{component}, out)
		},
	}
	addDeploymentArgFlag(cmd)
	addLogsFlags(cmd, true)
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	return cmd
}

// addLogsFlags registers the flags every logs command takes, with oneX 1.x's
// --search as --keyword's hidden alias.
func addLogsFlags(cmd *cobra.Command, oneX bool) {
	cmd.Flags().StringVarP(&search, "keyword", "k", "", "Show only the log lines that contain this keyword")
	if oneX {
		utils.AddAlias(cmd.Flags(), "search", "s", "keyword")
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream the logs as they come, of one component")
	cmd.Flags().DurationVarP(&since, "since", "t", 0, "Only return logs newer than a relative duration like 5m, 1h, or 24h, up to 48h. With --follow, Houston streams only logs from the moment it starts")
	cliout.AddOutputFlag(cmd, &logsOutput)
}

// logsArgs refuses a component named where the Deployment goes, as 1.x's
// layout and an earlier v2's logs triggerer named one: taken for a Deployment
// id, it would ask Houston about a Deployment called triggerer. Otherwise it
// takes at most the Deployment.
func logsArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		if c, ok := logComponentNames[strings.ToLower(args[0])]; ok {
			flag := logComponentFlag(c)
			return cliout.Usage(fmt.Errorf("%q names a component, not a Deployment: pick it with --%s, as in %s <DEPLOYMENT_ID> --%s", args[0], flag, cmd.CommandPath(), flag))
		}
	}
	return cobra.MaximumNArgs(1)(cmd, args)
}

// pickLogComponents is the components the flags name, in Houston's
// spelling, once each, or every one the platform logs when none does.
// triggerer says whether it has a triggerer.
func pickLogComponents(triggerer bool) ([]string, error) {
	on := map[string]bool{logWebserver: logsWebserver, logScheduler: logsScheduler, logWorker: logsWorkers, logTriggerer: logsTriggerer}
	for _, typed := range logsComponents {
		typed = strings.TrimSpace(typed)
		if typed == "" {
			continue
		}
		c, ok := logComponentNames[strings.ToLower(typed)]
		if !ok || (c == logTriggerer && !triggerer) {
			return nil, cliout.Usage(fmt.Errorf("--component %q is not a component this platform logs: use %s", typed, logComponentNamesProse(triggerer)))
		}
		on[c] = true
	}
	var picked []string
	for _, c := range allLogComponents(triggerer) {
		if on[c] {
			picked = append(picked, c)
		}
	}
	if len(picked) == 0 {
		return allLogComponents(triggerer), nil
	}
	return picked, nil
}

// allLogComponents is every component the platform logs.
func allLogComponents(triggerer bool) []string {
	all := []string{logWebserver, logScheduler, logWorker}
	if triggerer {
		all = append(all, logTriggerer)
	}
	return all
}

// showLogs prints the log records of components, one per line, or under json
// one object per line, oldest first: a stream, which --follow keeps open, of
// one component. Records of several components name theirs in text, as
// Astro's logs name their source.
func showLogs(cmd *cobra.Command, components []string, out io.Writer) error {
	if err := requireDeployment(cmd); err != nil {
		return err
	}
	// Houston searches at most 2 days of logs and refuses a longer window. A
	// follow ignores --since, so only a search is held to it.
	if !follow && since > maxLogsSince {
		return cliout.Usage(fmt.Errorf("--since %s is longer than the %s of logs APC searches at most", since, maxLogsSince))
	}
	// Houston streams one component's logs per subscription.
	if follow && len(components) != 1 {
		return cliout.Usage(fmt.Errorf("--follow streams one component's logs: name one, with a flag such as --scheduler, or --component"))
	}
	cmd.SilenceUsage = true
	r := cliout.Renderer{Format: logsOutput, Out: out}
	if follow {
		component := components[0]
		return subscribeLogs(deploymentID, component, search, since, cliout.NotesTo(cmd, logsOutput, out),
			func(l houston.DeploymentLog) error { return emitLogEntry(r, component, l, true, false) })
	}
	logs, err := deployment.Logs(deploymentID, components, search, since, houstonClient)
	if err != nil {
		return err
	}
	for i := range logs {
		if err := emitLogEntry(r, logs[i].Component, logs[i], false, len(components) > 1); err != nil {
			return err
		}
	}
	return nil
}
