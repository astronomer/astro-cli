package astro

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
)

// The transport astro deploy bundle calls, vars so a test can stand in for
// the upload and the delete.
var (
	DeployBundle = astrodeploy.DeployBundle
	DeleteBundle = astrodeploy.DeleteBundle
)

// waitForBundle waits for a Deployment to take a bundle deploy or delete. A
// var so a test need not sit through the poll's sleeps.
var waitForBundle = astrodeploy.WaitForBundle

// deployBundleOptions are astro deploy bundle's flags.
type deployBundleOptions struct {
	mountPath   string
	bundleType  string
	deployment  string
	workspace   string
	description string
	wait        bool
	waitTime    time.Duration
	delete      bool
	output      cliout.Format
}

func newDeployBundleCmd() *cobra.Command {
	o := &deployBundleOptions{}
	types := strings.Join(astrodeploy.BundleTypeNames(), ", ")
	cmd := &cobra.Command{
		Use:   manifestdeploy.BundleCommand + " [PATH]",
		Short: "Deploy a non-DAG bundle (for example a dbt project) to a Deployment",
		Long: "Deploy a non-DAG bundle (for example a dbt project) to a Deployment on Astro. " +
			"The bundle is a directory of its own, outside any Astro project: PATH, or the current directory. " +
			"It is uploaded as it is and mounted at --mount-path in Airflow, for your DAGs to read; it carries no DAGs. " +
			"With --delete it removes the bundle mounted at that path instead.\n\n" +
			"A bundle's type decides its defaults. With no --type, the type is the first one PATH looks like (" + types + "), else none. " +
			"A dbt project (a directory holding dbt_project.yml) mounts at " + astrodeploy.DbtDefaultMountPathPrefix + "<project name> unless --mount-path says otherwise. " +
			"Any other --type is recorded as a label, with no defaults, so it needs --mount-path.\n\n" +
			"This deploys the bundle's files; astro deploy deploys the project and its DAGs. " +
			"To list or change the bundles a Deployment has registered, use astro deployment bundle. " +
			"A project link called bundle is not reachable as astro deploy bundle: pass it as astro deploy --deployment bundle.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeployBundle(cmd, args, o)
		},
		Example: `  # Deploy the dbt project in the current directory, picking the Deployment from a list
  astro deploy bundle

  # Deploy a dbt project elsewhere to a given Deployment
  astro deploy bundle <PATH> --deployment <DEPLOYMENT_ID>

  # Deploy another kind of bundle, which needs a mount path
  astro deploy bundle <PATH> --type <TYPE> --mount-path /usr/local/airflow/<NAME> \
    --deployment <DEPLOYMENT_ID>

  # Remove the dbt project in the current directory from a Deployment
  astro deploy bundle --delete --deployment <DEPLOYMENT_ID>`,
	}
	f := cmd.Flags()
	f.StringVarP(&o.mountPath, "mount-path", "m", "", "Path the bundle is mounted at in Airflow, for reference by Dags. Default: the type's, "+astrodeploy.DbtDefaultMountPathPrefix+"<project name> for a dbt project")
	f.StringVar(&o.bundleType, "type", "", "Type of the bundle: "+types+", or any other label, which has no defaults. Default: detected from PATH, else none")
	f.StringVarP(&o.deployment, "deployment", "d", "", "Deployment to deploy to: a link name from the project here, or a Deployment id")
	f.StringVar(&o.workspace, "workspace", "", "Workspace whose Deployments are offered when no --deployment is given")
	f.StringVar(&o.description, "description", "", "Description to store on the deploy")
	f.BoolVarP(&o.wait, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	f.DurationVar(&o.waitTime, "wait-time", deployWaitTime, "Time to wait for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	f.BoolVar(&o.delete, "delete", false, "Remove the bundle mounted at --mount-path (by default the one PATH's type would mount at) instead of deploying")
	cliout.AddOutputFlag(cmd, &o.output)
	return cmd
}

func runDeployBundle(cmd *cobra.Command, args []string, o *deployBundleOptions) error {
	format := o.output
	// The result goes where stdout is now. What the platform code prints on
	// the way (the git note, the archive's) is output text mode has always
	// shown; under json it goes to stderr.
	out := cmd.OutOrStdout()
	defer strayStdoutToStderr(format)()

	if cmd.Flags().Changed("wait-time") && !o.wait {
		return cliout.Usage(errors.New("cannot use --wait-time with --wait=false"))
	}
	path := config.WorkingPath
	if len(args) > 0 {
		path = args[0]
	}
	if o.delete {
		return deleteBundle(cmd, out, path, o)
	}
	return deployBundle(cmd, out, path, o)
}

// deployBundle uploads the bundle at path.
func deployBundle(cmd *cobra.Command, out io.Writer, path string, o *deployBundleOptions) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle path %s does not exist", path)
		}
		return fmt.Errorf("failed to access bundle path %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path %s is not a directory", path)
	}
	// The walk every deploy decides by: a pyproject.toml project or a 1.x one
	// at or above the bundle path.
	if !utils.Locate(path).None() {
		return fmt.Errorf("bundle path %s is within an Astro project: a non-DAG bundle is a directory of its own. To deploy the project, run astro deploy", path)
	}
	bt := astrodeploy.ResolveBundleType(o.bundleType, path)
	if bt.Validate != nil {
		if err := bt.Validate(path); err != nil {
			return err
		}
	}
	mountPath, err := resolveBundleMountPath(cmd, path, bt, o.mountPath)
	if err != nil {
		return err
	}
	targetID, target, login, err := bundleTarget(cmd.Context(), o.deployment, o.workspace, o.output)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in := &astrodeploy.DeployBundleInput{
		BundlePath:    path,
		MountPath:     mountPath,
		DeploymentID:  targetID,
		Deployment:    target,
		BundleType:    bt.Name,
		Description:   o.description,
		AstroV1Client: login.client,
	}
	if !login.current {
		in.Login = &login.context
	}
	res, err := DeployBundle(in)
	if err != nil {
		return err
	}
	bundlePath, err := filepath.Abs(path)
	if err != nil {
		bundlePath = path
	}
	return publishThenWaitWith(cmd, o.output, o.wait, bundleWait(cmd, &login, res.DeploymentID, o.waitTime), func(waitErr error) error {
		r := cliout.Renderer{Format: o.output, Out: out}
		return r.Emit(newBundleDeployJSON(&res, bt.Name, bundlePath, o.wait, waitErr), renderBundleUploaded(res.BundleVersion))
	})
}

// deleteBundle removes the bundle mounted at --mount-path, or at the mount
// path the bundle at path would deploy to.
func deleteBundle(cmd *cobra.Command, out io.Writer, path string, o *deployBundleOptions) error {
	bt := astrodeploy.ResolveBundleType(o.bundleType, path)
	mountPath := o.mountPath
	if mountPath == "" {
		if bt.Validate != nil {
			if err := bt.Validate(path); err != nil {
				return err
			}
		}
		var err error
		if mountPath, err = resolveBundleMountPath(cmd, path, bt, ""); err != nil {
			return err
		}
	}
	targetID, target, login, err := bundleTarget(cmd.Context(), o.deployment, o.workspace, o.output)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in := &astrodeploy.DeleteBundleInput{
		MountPath:     mountPath,
		DeploymentID:  targetID,
		Deployment:    target,
		BundleType:    bt.Name,
		Description:   o.description,
		AstroV1Client: login.client,
	}
	if !login.current {
		in.Login = &login.context
	}
	res, err := DeleteBundle(in)
	if err != nil {
		return err
	}
	return publishThenWaitWith(cmd, o.output, o.wait, bundleWait(cmd, &login, res.DeploymentID, o.waitTime), func(waitErr error) error {
		r := cliout.Renderer{Format: o.output, Out: out}
		return r.Emit(newBundleDeleteJSON(&res, bt.Name, o.wait, waitErr), renderBundleDeleted(res.MountPath))
	})
}

// resolveBundleMountPath is given, or else the default of the bundle's type, which
// it says on stderr it chose. A type with no default needs one given.
func resolveBundleMountPath(cmd *cobra.Command, path string, bt astrodeploy.BundleType, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if bt.DefaultMountPath == nil {
		return "", cliout.Usage(fmt.Errorf("--mount-path is required for a bundle of type %s: only a %s bundle has a default", bt.Name, strings.Join(astrodeploy.BundleTypeNames(), " or ")))
	}
	mountPath, err := bt.DefaultMountPath(path)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "mount path %s: the default for a %s bundle\n", mountPath, bt.Name)
	return mountPath, nil
}

// bundleWait waits for the Deployment under the login the bundle went to.
func bundleWait(cmd *cobra.Command, login *deployLogin, deploymentID string, waitTime time.Duration) func() error {
	return func() error {
		if login.current {
			return waitForBundle(cmd.ErrOrStderr(), deploymentID, waitTime, login.client)
		}
		return astrodeploy.WaitForBundleIn(cmd.ErrOrStderr(), &login.context, deploymentID, waitTime, login.client)
	}
}

// bundleTarget is the Deployment a bundle deploys to or is deleted from, and
// the login it does so under, the way astro deploy from the same directory
// picks them.
//
// The login: in a pyproject.toml project here whose manifest loads, the
// project's (its Astro host's), for everything; anywhere else, or when the
// manifest does not load, the current context's.
//
// The target: name (--deployment), a link name of the project here or a
// Deployment id. Without one, the workspace's Deployments are offered
// (workspace, else the login's): only to a run that can be asked, and only on
// the current context's host, as astro deploy offers them. A run that cannot
// be asked, under --output json or without a terminal, is refused as astro
// deploy's is, with kind input_required. read is the Deployment when it was
// picked, so the deploy need not read it again.
func bundleTarget(ctx context.Context, name, workspace string, format cliout.Format) (id string, read *astrov1.Deployment, login deployLogin, err error) {
	m := manifestHere()
	domain := ""
	if m != nil {
		domain = m.Astro.LoginDomain()
	}
	login, err = loginForDeploy(ctx, domain)
	if err != nil {
		return "", nil, login, err
	}

	if name != "" {
		if m == nil || !hasLink(m, name) {
			return name, nil, login, nil
		}
		named, _, err := manifestdeploy.ResolveNamed(m, name, "", workspace, "")
		if err != nil {
			return "", nil, login, cliout.Usage(err)
		}
		return named.DeploymentID, nil, login, nil
	}

	if format != cliout.FormatText || !stdinIsTerminal() {
		return "", nil, login, input.Required(errBundleNoTarget)
	}
	if !login.current {
		return "", nil, login, errPickerOtherHost(login.context.Domain)
	}
	ws := workspace
	if ws == "" {
		if ws, err = coalesceWorkspace(); err != nil {
			return "", nil, login, fmt.Errorf("failed to find a valid workspace: %w", err)
		}
	}
	picked, err := deployment.GetDeployment(ws, "", "", false, nil, login.client)
	if err != nil {
		return "", nil, login, err
	}
	if picked.Id == "" {
		return "", nil, login, deployment.ErrNothingTo(ws, "", "", "")
	}
	return picked.Id, &picked, login, nil
}

// errBundleNoTarget is a bundle deploy or delete that names no Deployment and
// cannot be asked for one, in astro deploy's words for the same refusal.
var errBundleNoTarget = errors.New("this deploy names no Deployment and this run cannot be asked: pass the Deployment id or a link name with --deployment")

// bundleDeployJSON is what astro deploy bundle --output json publishes: the
// bundle deployed and where. Its deployment, workspace and git keys are the
// ones astro deploy's result uses for the same facts.
type bundleDeployJSON struct {
	Deployment     string `json:"deployment"`
	DeploymentName string `json:"deployment_name"`
	Workspace      string `json:"workspace"`
	// DeployID is the deploy record the upload created on the Deployment.
	DeployID string `json:"deploy_id"`
	// BundleType is the type the bundle deployed as (--type, or detected),
	// BundlePath the directory bundled, as an absolute path.
	BundleType string `json:"bundle_type"`
	BundlePath string `json:"bundle_path"`
	MountPath  string `json:"mount_path"`
	// BundleVersion is the version of the uploaded bundle.
	BundleVersion string `json:"bundle_version"`
	// Waited is true when the run waited for the Deployment to become
	// healthy (--wait), and WaitError says why that wait failed; it is
	// omitted when the wait succeeded or none was asked for.
	Waited    bool           `json:"waited"`
	WaitError string         `json:"wait_error,omitempty"`
	Git       *deployGitJSON `json:"git,omitempty"`
}

func newBundleDeployJSON(res *astrodeploy.BundleDeploy, bundleType, bundlePath string, waited bool, waitErr error) bundleDeployJSON {
	return bundleDeployJSON{
		Deployment:     res.DeploymentID,
		DeploymentName: res.DeploymentName,
		Workspace:      res.WorkspaceID,
		DeployID:       res.DeployID,
		BundleType:     bundleType,
		BundlePath:     bundlePath,
		MountPath:      res.MountPath,
		BundleVersion:  res.BundleVersion,
		Waited:         waited,
		WaitError:      errText(waitErr),
		Git:            bundleGitJSON(res.Git),
	}
}

// bundleDeleteJSON is what astro deploy bundle --delete --output json
// publishes: the bundle it removed from the Deployment. Astro removes the
// files as the Deployment takes the deploy, which a wait (waited, and
// wait_error when it failed) watches for. workspace is the Deployment's,
// omitted when the Deployment was named and so never read.
type bundleDeleteJSON struct {
	Action     string `json:"action"`
	Deployment string `json:"deployment"`
	Workspace  string `json:"workspace,omitempty"`
	DeployID   string `json:"deploy_id"`
	BundleType string `json:"bundle_type"`
	MountPath  string `json:"mount_path"`
	Waited     bool   `json:"waited"`
	WaitError  string `json:"wait_error,omitempty"`
}

func newBundleDeleteJSON(res *astrodeploy.BundleDelete, bundleType string, waited bool, waitErr error) bundleDeleteJSON {
	return bundleDeleteJSON{
		Action:     "deleted",
		Deployment: res.DeploymentID,
		Workspace:  res.WorkspaceID,
		DeployID:   res.DeployID,
		BundleType: bundleType,
		MountPath:  res.MountPath,
		Waited:     waited,
		WaitError:  errText(waitErr),
	}
}

// bundleGitJSON is the commit a bundle deploy recorded, as a deploy result
// publishes it; nil when it recorded none.
func bundleGitJSON(g *astrodeploy.BundleGit) *deployGitJSON {
	if g == nil {
		return nil
	}
	return &deployGitJSON{CommitSHA: g.CommitSHA, Branch: g.Branch, CommitURL: g.CommitURL}
}

// renderBundleUploaded is the text a finished bundle upload has always
// printed.
func renderBundleUploaded(version string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully uploaded bundle with version %s to Astro.\n", version)
	})
}

func renderBundleDeleted(mountPath string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully requested bundle delete for mount path %s from Astro.\n", mountPath)
	})
}

// publishThenWait publishes a bundle deploy's or delete's result and, with
// --wait, waits for the Deployment to take it (waitFor). publish is handed
// the wait's failure, nil when it succeeded or there was no wait.
//
// The order depends on the format. In text the result comes first, as it
// always has, so a person sees the upload succeed before the wait begins;
// the wait's progress goes to stderr, and its failure is the run's error.
// Under json the one object is the last thing the run writes, after the wait
// it reports on. A wait that fails there still publishes the result, saying
// why, because the upload happened and a script needs its deploy id and
// version, and then exits 1 (failedAfterResult).
func publishThenWaitWith(cmd *cobra.Command, format cliout.Format, wait bool, waitFor func() error, publish func(waitErr error) error) error {
	waitDone := func() error {
		if !wait {
			return nil
		}
		return waitFor()
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
