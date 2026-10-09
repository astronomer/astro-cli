package astro

// Rendering for `astro dbt deploy|delete|cleanup`, `astro deploy --non-dags`
// and `astro remote deploy`. The platform packages return what happened; this
// file decides how it looks, in text and in json.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/cosmosboost"
)

// dbtOutput is --output for the `dbt` family.
var dbtOutput cliout.Format

// waitForBundle waits for a Deployment to take a bundle deploy or delete. A
// var so a test need not sit through the poll's sleeps.
var waitForBundle = astrodeploy.WaitForBundle

// dbtDeployJSON is what `astro dbt deploy --output json` publishes: what was
// deployed where. Its deployment, workspace and git keys are the ones
// `astro deploy`'s result uses for the same facts.
type dbtDeployJSON struct {
	Deployment     string `json:"deployment"`
	DeploymentName string `json:"deployment_name"`
	Workspace      string `json:"workspace"`
	// DeployID is the deploy record the upload created on the Deployment.
	DeployID string `json:"deploy_id"`
	// Project is the dbt project's name, from its dbt_project.yml, and
	// ProjectPath the directory bundled.
	Project     string `json:"project"`
	ProjectPath string `json:"project_path"`
	MountPath   string `json:"mount_path"`
	// BundleVersion is the version of the uploaded bundle.
	BundleVersion string `json:"bundle_version"`
	// Waited is true when the run waited for the Deployment to become
	// healthy (--wait), and WaitError says why that wait failed; it is
	// omitted when the wait succeeded or none was asked for.
	Waited    bool           `json:"waited"`
	WaitError string         `json:"wait_error,omitempty"`
	Git       *deployGitJSON `json:"git,omitempty"`
}

// dbtDeleteJSON is what `astro dbt delete --output json` publishes: the
// bundle it removed from the Deployment. Astro removes the files as the
// Deployment takes the deploy, which a wait (waited, and wait_error when it
// failed) watches for. workspace is the Deployment's, omitted when the
// Deployment was named by id and so never read.
type dbtDeleteJSON struct {
	Action     string `json:"action"`
	Deployment string `json:"deployment"`
	Workspace  string `json:"workspace,omitempty"`
	DeployID   string `json:"deploy_id"`
	MountPath  string `json:"mount_path"`
	Waited     bool   `json:"waited"`
	WaitError  string `json:"wait_error,omitempty"`
}

// dbtCleanupJSON is what `astro dbt cleanup --output json` publishes: the
// paths it scanned, the artifacts it removed, and the ones it kept because
// something other than the CLI wrote them, each an absolute path.
type dbtCleanupJSON struct {
	Action  string   `json:"action"`
	Paths   []string `json:"paths"`
	Removed []string `json:"removed"`
	Kept    []string `json:"kept"`
}

// remoteDeployJSON is what `astro remote deploy --output json` publishes: the
// client image it pushed, which is what the agents' Helm values then name.
type remoteDeployJSON struct {
	Image    string `json:"image"`
	Registry string `json:"registry"`
	Tag      string `json:"tag"`
	// SourceImage is the local image pushed (--image-name), omitted when the
	// run built one.
	SourceImage string `json:"source_image,omitempty"`
	// Platforms are the --platform the image was built for, [] for the host's
	// or when nothing was built.
	Platforms []string `json:"platforms"`
	// RuntimeCheck is the check against --deployment, omitted without one.
	RuntimeCheck *remoteRuntimeCheckJSON `json:"runtime_check,omitempty"`
}

type remoteRuntimeCheckJSON struct {
	Deployment               string `json:"deployment"`
	ClientRuntimeVersion     string `json:"client_runtime_version"`
	DeploymentRuntimeVersion string `json:"deployment_runtime_version"`
}

func newDbtDeployJSON(res *astrodeploy.BundleDeploy, project, projectPath string, waited bool, waitErr error) dbtDeployJSON {
	obj := dbtDeployJSON{
		Deployment:     res.DeploymentID,
		DeploymentName: res.DeploymentName,
		Workspace:      res.WorkspaceID,
		DeployID:       res.DeployID,
		Project:        project,
		ProjectPath:    projectPath,
		MountPath:      res.MountPath,
		BundleVersion:  res.BundleVersion,
		Waited:         waited,
		WaitError:      errText(waitErr),
	}
	if g := res.Git; g != nil {
		obj.Git = &deployGitJSON{CommitSHA: g.CommitSHA, Branch: g.Branch, CommitURL: g.CommitURL}
	}
	return obj
}

// renderBundleUploaded is the text a finished bundle upload has always
// printed.
func renderBundleUploaded(version string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully uploaded bundle with version %s to Astro.\n", version)
	})
}

// publishThenWait publishes a bundle deploy's or delete's result and, with
// --wait, waits for the Deployment to take it. publish is handed the wait's
// failure, nil when it succeeded or there was no wait.
//
// The order depends on the format. In text the result comes first, as it
// always has, so a person sees the upload succeed before the wait begins;
// the wait's progress goes to stderr, and its failure is the run's error.
// Under json the one object is the last thing the run writes, after the wait
// it reports on. A wait that fails there still publishes the result, saying
// why, because the upload happened and a script needs its deploy id and
// version, and then exits 1 (failedAfterResult).
func publishThenWait(cmd *cobra.Command, format cliout.Format, wait bool, deploymentID string, waitTime time.Duration, publish func(waitErr error) error) error {
	waitDone := func() error {
		if !wait {
			return nil
		}
		return waitForBundle(cmd.ErrOrStderr(), deploymentID, waitTime, astroV1Client)
	}
	if format == cliout.FormatJSON {
		waitErr := waitDone()
		if err := publish(waitErr); err != nil {
			// The write failed, so stdout is what broke: the root must not
			// try to write the error object there too. failedAfterResult
			// marks it shown and puts the words on stderr, the write's
			// failure first and the wait's after it, so neither hides the
			// other.
			return failedAfterResult(cmd, format, errors.Join(err, waitErr))
		}
		return failedAfterResult(cmd, format, waitErr)
	}
	if err := publish(nil); err != nil {
		return err
	}
	return waitDone()
}

// errText is err's message, or "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func newDbtDeleteJSON(res *astrodeploy.BundleDelete, waited bool, waitErr error) dbtDeleteJSON {
	return dbtDeleteJSON{
		Action:     "deleted",
		Deployment: res.DeploymentID,
		Workspace:  res.WorkspaceID,
		DeployID:   res.DeployID,
		MountPath:  res.MountPath,
		Waited:     waited,
		WaitError:  errText(waitErr),
	}
}

func renderDbtDeleted(mountPath string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully requested bundle delete for mount path %s from Astro.\n", mountPath)
	})
}

func newDbtCleanupJSON(report *cosmosboost.CleanupReport) dbtCleanupJSON {
	return dbtCleanupJSON{
		Action:  "removed",
		Paths:   nonNil(report.Roots),
		Removed: nonNil(report.Removed),
		Kept:    nonNil(report.Kept),
	}
}

func renderDbtCleanup(b *bufio.Writer) {
	fmt.Fprintln(b, "Removed the Cosmos Boost artifacts")
}

func newRemoteDeployJSON(res *astrodeploy.ClientDeploy) remoteDeployJSON {
	obj := remoteDeployJSON{
		Image:       res.Image,
		Registry:    res.Registry,
		Tag:         res.Tag,
		SourceImage: res.SourceImage,
		Platforms:   nonNil(res.Platforms),
	}
	if c := res.RuntimeCheck; c != nil {
		obj.RuntimeCheck = &remoteRuntimeCheckJSON{
			Deployment:               c.DeploymentID,
			ClientRuntimeVersion:     c.ClientRuntimeVersion,
			DeploymentRuntimeVersion: c.DeploymentRuntimeVersion,
		}
	}
	return obj
}

// renderRemoteDeploy is the text a finished client image push has always
// printed: where it went, and what to do with it next.
func renderRemoteDeploy(image string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully pushed client image to %s\n", ansi.Bold(image))
		fmt.Fprintf(b, "\n--------------------------------\n")
		fmt.Fprintln(b, "The client image has been pushed to your private registry.")
		fmt.Fprintln(b, "Your next step would be to update the agent component to use the new client image.")
		fmt.Fprintln(b, "For that you would either need to update the helm chart values.yaml file or update your CI/CD pipeline to use the new client image.")
		fmt.Fprintf(b, "If you are using Astronomer provided Agent Helm chart, you would need to update the image field for each of the workers, dagProcessor, and triggerer component sections to the new image: %s\n", image)
		fmt.Fprintln(b, "Once you have updated the helm chart values.yaml file, you can run 'helm upgrade' or update via your CI/CD pipeline to update the agent components")
	})
}

// nonNil is s, or [] for nil, so a list publishes as [] and never null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
