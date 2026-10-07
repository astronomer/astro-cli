package astro

// `astro deployment create --clone`: a new Deployment copied from an existing
// one, with one create request built from what the API returns for it.

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/util"
)

// cloneSource is --clone: the Deployment id or name to copy.
var cloneSource string

const cloneFlagUsage = "Copy an existing Deployment, given by id or by name in the Workspace: its type, cluster or region, runtime version, executor, worker queues, resources, settings and plain environment variables. Not copied: secret environment variable values, a custom workload identity, Remote Execution agents, low latency, API server and event scheduler autoscaling, alerts, tokens, roles, and the image and DAGs. Takes --name, and optionally --workspace, --description and --wait"

const deploymentCreateExample = `  # Create a Deployment in the current Workspace
  astro deployment create --name etl

  # Copy a Deployment for a preview of a branch, and wait until it is healthy
  astro deployment create --clone <deployment-id> --name "etl-$BRANCH" --wait`

// cloneAllowedFlags are the flags --clone takes beside it. Everything else
// create takes describes the Deployment, which --clone copies instead.
var cloneAllowedFlags = []string{"clone", "name", "workspace", "workspace-id", "description", "wait", "wait-time", "output", "verbosity"}

var errCloneNeedsName = errors.New("--clone needs --name, the name of the new Deployment")

// deploymentClone is `astro deployment create --clone`.
func deploymentClone(cmd *cobra.Command, out io.Writer, format cliout.Format) error {
	var others []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if !slices.Contains(cloneAllowedFlags, f.Name) {
			others = append(others, "--"+f.Name)
		}
	})
	if len(others) > 0 {
		return cliout.Usage(fmt.Errorf("--clone copies the Deployment's configuration, so it cannot be used with %s. It takes --name, --workspace, --description and --wait", strings.Join(others, ", ")))
	}
	if cloneSource == "" {
		return cliout.Usage(errors.New("--clone needs a Deployment id or name"))
	}
	if label == "" {
		return cliout.Usage(errCloneNeedsName)
	}
	if cmd.Flags().Changed("wait-time") && !waitForStatus {
		return errors.New("cannot use --wait-time with --wait=false")
	}
	cmd.SilenceUsage = true

	// A name is looked up where the command is pointed, as for any other
	// command naming a Deployment; an id is read wherever it is.
	ws, err := coalesceWorkspace()
	if err != nil && !util.IsCUID(cloneSource) {
		return fmt.Errorf("failed to find a valid Workspace: %w", err)
	}
	src, err := deployment.CloneSource(cloneSource, ws, astroV1Client)
	if err != nil {
		return err
	}

	// The copy goes where its source is, unless --workspace says otherwise.
	target := ""
	if cmd.Flags().Changed("workspace-id") {
		target = workspaceID
	}
	var desc *string
	if cmd.Flags().Changed("description") {
		desc = &description
	}
	d, notes, err := deployment.Clone(&src, label, target, desc, waitForStatus, waitTimeForDeployment, astroV1Client)
	if d.Id == "" {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(cmd.ErrOrStderr(), "Note: "+string(n))
	}
	// The Deployment exists even when the --wait for it failed, so it is
	// published either way.
	r := cliout.Renderer{Format: format, Out: out}
	if emitErr := emitDeployment(r, &d, func(w io.Writer) error { return deployment.WriteCreated(w, d.WorkspaceId, &d) }); emitErr != nil {
		if err != nil {
			return err
		}
		return emitErr
	}
	return failedAfterResult(cmd, format, err)
}
