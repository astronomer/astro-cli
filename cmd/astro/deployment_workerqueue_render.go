package astro

// How `astro deployment worker-queue create|update|delete` and `astro
// deployment logs` publish what they did, in text and in json.

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/workerqueue"
)

var (
	// deploymentWorkerQueueOutput is --output for the worker-queue family,
	// registered once on its root.
	deploymentWorkerQueueOutput cliout.Format
	// deploymentLogsOutput is `deployment logs`'s --output.
	deploymentLogsOutput cliout.Format
)

// emitWorkerQueue publishes a queue change: in text, the line it always
// printed, which names the Workspace the command was run against.
func emitWorkerQueue(r cliout.Renderer, res *workerqueue.Result, ws string) error {
	return r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "worker queue %s for %s in %s workspace %s\n", res.WorkerQueue.Name, res.DeploymentName, ws, res.Action)
	}))
}

// emitLogs publishes a Deployment's logs. They are a stream: under json each
// entry is one object on its own line (EmitEvent), and none is no lines,
// with the note that says so on stderr. In text each entry is the line and
// its source, as always.
func emitLogs(cmd *cobra.Command, r cliout.Renderer, res *deployment.LogsResult) error {
	if len(res.Entries) == 0 {
		w := r.Out
		if r.Format == cliout.FormatJSON {
			w = cmd.ErrOrStderr()
		}
		_, err := fmt.Fprintf(w, "No matching logs have been recorded in the past %d hours for Deployment %s\n", res.Hours, res.DeploymentName)
		return err
	}
	for i := range res.Entries {
		e := &res.Entries[i]
		if err := r.EmitEvent(e, cliout.Text(func(b *bufio.Writer) {
			fmt.Fprintf(b, "%s %s\n", e.Message, e.Source)
		})); err != nil {
			return err
		}
	}
	return nil
}
