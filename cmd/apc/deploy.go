package apc

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

var (
	deployOutput     cliout.Format
	deployYes        bool
	deployDeployment string
	workspaceFlag    string
	deployForce      bool
	deployPrompt     bool
	deploySecrets    = []string{}

	DeployAirflowImage                 = deploy.Airflow
	DagsOnlyDeploy                     = deploy.DagsOnlyDeploy
	UpdateDeploymentImage              = deploy.UpdateDeploymentImage
	isDagOnlyDeploy                    bool
	description                        string
	isImageOnlyDeploy                  bool
	imageName                          string
	runtimeVersionForImageName         string
	imagePresentOnRemote               bool
	ErrBothDagsOnlyAndImageOnlySet     = errors.New("cannot use both --dags and --image together. Run 'astro deploy' to update both your image and dags")
	ErrImageNameNotPassedForRemoteFlag = errors.New("--image-name is mandatory when --remote flag is passed")
)

var deployExample = `  # Build this project and deploy it, picking the Deployment from a list
  astro deploy

  # Build this project and deploy it to a linked Deployment, or one by id
  astro deploy <LINK_OR_DEPLOYMENT_ID>

  # Deploy an image built on this machine to a given Deployment
  astro deploy <DEPLOYMENT_ID> --image-name <IMAGE_NAME>`

// The warnings a deploy can give about its DAGs.
const (
	// warningImageNameDagsInImage: the Deployment runs the DAGs inside its
	// image, and nothing here put any there. --image-name's image was not
	// built from the working directory, which need not be a project at all.
	// The image may well carry DAGs (one built from a Dockerfile that copies
	// them does); one astro package generated the build for does not.
	warningImageNameDagsInImage = "this Deployment runs the Dags inside the image %s; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."
	// noticeNoDagsDir: the project has no dags directory to upload, so
	// DagsOnlyDeploy uploaded nothing. An empty upload would have deleted
	// the Deployment's DAGs.
	noticeNoDagsDir = "no Dags were uploaded: there is no dags directory in %s, and the Deployment keeps the Dags it had. To upload Dags, create a dags directory in the project."
	// noticeImageAlone: an --image-name deploy outside a pyproject.toml
	// project ships the image alone, to a Deployment that takes DAG
	// uploads, so its DAGs are as they were.
	noticeImageAlone = "Dags were not updated: this deploy ran outside a pyproject.toml project, so it shipped the image alone, and the Deployment keeps the Dags it had. To upload them, run astro deploy %s --dags from the project directory."
	// noticeImageAloneUnplaced: the same, to a Deployment whose DAG source
	// could not be read.
	noticeImageAloneUnplaced = "this deploy ran outside a pyproject.toml project, so it shipped the image alone. If this Deployment takes Dag uploads, it keeps the Dags it had: to upload them, run astro deploy %s --dags from the project directory."
	// noticeDagsUndecided: the image was deployed, and the cluster config
	// that says whether the Deployment takes DAG uploads could not be read,
	// so an upload that may have been due did not happen.
	noticeDagsUndecided = "Dags were NOT updated: whether this Deployment takes Dag uploads could not be read (%v). The image was deployed, and the Deployment keeps the Dags it had. To upload them, run astro deploy %s --dags from the project directory."
)

func NewDeployCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [DEPLOYMENT_ID]",
		Short: "Deploy an image or Dags to a Deployment",
		Long: "Deploy a pyproject.toml project ([tool.astro]) to an Astro Private Cloud Deployment. Run it at the project's root: it builds the project into an image, Airflow 2 or Airflow 3, and pushes it to the Deployment, with the project's Dags inside the image for an image Deployment and uploaded beside it for a Dag-only one. " +
			"Name the Deployment by a link of the project (target = 'apc', written by astro link add) or by its id; an interactive deploy that names none asks, with the project's default link preselected. " +
			"With --image-name it deploys an image built on this machine, from any directory. A project in the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml) deploys with Astro CLI 1.x only.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployAirflow(cmd, args, out)
		},
		Example: deployExample,
	}
	// --force answered 1.x's uncommitted-changes check, which v2's deploy
	// does not make, and --prompt asked for the Deployment list with one
	// named, which v2 offers whenever none is. Both stay, accepted, hidden
	// and read by nothing on a project's deploy, because astronomer/deploy-
	// action passes --force on every deploy. --prompt still forces the list
	// on an --image-name deploy, as it always did.
	cmd.Flags().BoolVarP(&deployForce, "force", "f", false, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("force") //nolint:errcheck // the flag is defined just above
	cmd.Flags().BoolVarP(&deployPrompt, "prompt", "p", false, "Has no effect on a project's deploy; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("prompt") //nolint:errcheck // the flag is defined just above
	cmd.Flags().StringVar(&deployDeployment, "deployment", "", "Deployment to deploy to: a link name from the manifest, or a Deployment id")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace for the deploy, overriding the context")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace assigned to the Deployment")
	utils.MarkPreferredFlag(cmd.Flags(), "workspace", "workspace-id")
	cmd.Flags().StringVar(&description, "description", "", "Description to attach to the deploy, for traceability (default: one based on the deploy type)")
	cmd.Flags().BoolVarP(&isImageOnlyDeploy, "image", "", false, "Push only an image to your Deployment; works only for Dag-only, Git-sync-based and NFS-based Deployments")
	cmd.Flags().StringVarP(&imageName, "image-name", "i", "", "Name of the custom image (present locally unless --remote is specified) to deploy")
	cmd.Flags().StringVar(&runtimeVersionForImageName, "runtime-version", "", "Runtime version of the image to deploy. Example - 12.1.1. Mandatory if --image-name --remote is provided")
	cmd.Flags().BoolVarP(&imagePresentOnRemote, "remote", "", false, "Custom image which is present on the remote registry. Can only be used with --image-name flag")
	cmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "Answer the confirmations of an --image-name deploy yes: an image tag that is not recommended, and a Dags folder with no Dags")
	utils.AddBuildSecretFlag(cmd.Flags(), &deploySecrets)
	cliout.AddOutputFlag(cmd, &deployOutput)

	if !context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.34.0"}) {
		cmd.Flags().BoolVarP(&isDagOnlyDeploy, "dags", "d", false, "Push only the Dags of the pyproject.toml project here to your Deployment")
	}
	return cmd
}

// refuseUndeployable stops a deploy that is not a pyproject.toml project's
// build or --dags (deployManifest takes those) before anything is asked or
// sent:
//
//   - any deploy in or below a project in the Astro CLI 1.x layout, which
//     Astro CLI 1.x deploys, or below a pyproject.toml project's root
//     (kind no_project), --image-name included (utils.Locate's walk);
//   - --dags anywhere but in a pyproject.toml project: it uploads that
//     project's dags directory (kind no_project);
//   - a deploy with no --image-name outside a project, which has nothing to
//     build (kind no_project), as astro deploy on Astro says.
//
// --image-name (with or without --remote) runs at a pyproject.toml project's
// root, where the project's DAGs are uploaded after it to a Deployment that
// takes them, and outside any project, where the image ships alone.
func refuseUndeployable(where utils.Where) (inProject bool, err error) {
	inProject = where.AtRoot(config.WorkingPath)
	if !inProject && !where.None() {
		// In or below a 1.x project, or below a pyproject.toml project's
		// root: every mode is refused, --image-name included, as on Astro.
		return false, utils.NoDeployableProject(where)
	}
	switch {
	case isDagOnlyDeploy && (imageName != "" || imagePresentOnRemote):
		// As astro deploy says it: an image source makes no sense with a
		// deploy that ships only DAGs.
		return inProject, cliout.Usage(errDagsWithImageSource())
	case isDagOnlyDeploy:
		// --dags at a project's root is the manifest deploy's, so this is
		// --dags with no project here to upload the DAGs of.
		return false, utils.NoDeployableProject(where)
	case imagePresentOnRemote && imageName == "":
		return inProject, ErrImageNameNotPassedForRemoteFlag
	case imageName == "":
		// The value, not whether the flag was given: an empty --image-name=
		// names no image, so this would be a build, and there is no project
		// here to build.
		return false, utils.NoDeployableProject(where)
	}
	return inProject, nil
}

// errDagsWithImageSource is --dags with --image-name or --remote, in astro
// deploy's words, naming the ones given.
func errDagsWithImageSource() error {
	var passed []string
	if imageName != "" {
		passed = append(passed, "--image-name")
	}
	if imagePresentOnRemote {
		passed = append(passed, "--remote")
	}
	return fmt.Errorf("--dags deploys only your Dags; drop %s", strings.Join(passed, " and "))
}

// The kinds of deploy deployJSON.Type names, spelled as astro deploy on Astro
// spells them.
const (
	deployTypeImageAndDags = "image-and-dag"
	deployTypeImage        = "image-only"
	deployTypeDags         = "dag-only"
)

// deployJSON is the one object `astro deploy --output json` publishes on APC,
// named as the Astro platform's deploy names the same things. Fields that do
// not apply to a deploy are omitted: a dags-only deploy pushes no image.
type deployJSON struct {
	Deployment string `json:"deployment"`
	// Link is the manifest link the deploy resolved to, omitted when the
	// target was named by id.
	Link      string `json:"link,omitempty"`
	Workspace string `json:"workspace"`
	// Type is image-and-dag (the image, and the DAGs inside it or uploaded
	// beside it), image-only (--image, or a deploy whose Deployment took no
	// DAGs from it), or dag-only (--dags).
	Type string `json:"type"`
	// Image is the image the Deployment now runs: the one pushed, or the one
	// --image-name --remote named.
	Image string `json:"image,omitempty"`
	// DagsInImage is set on an image-and-dag deploy to an image Deployment,
	// which runs the DAGs inside the image: none were uploaded.
	DagsInImage bool `json:"dags_in_image,omitempty"`
	// RuntimeVersion is the runtime the pushed image carries, or
	// --runtime-version, given with --image-name --remote.
	RuntimeVersion string `json:"runtime_version,omitempty"`
	// URL is the Deployment's Airflow UI, when Houston gives it.
	URL string `json:"url,omitempty"`
	// Warnings are what the deploy warned about without failing, as text
	// prints them (less the "Warning: " prefix): an --image-name image that
	// is all an image Deployment's DAGs (not when apc.show_warnings is off), a
	// DAG upload that did not happen (always): skipped for want of a dags
	// directory, to a Deployment that takes DAG uploads, or because whether
	// the Deployment takes them could not be read; and a project's deploy's
	// own: an Airflow 2 to 3 upgrade, a Deployment that takes its DAGs from
	// elsewhere, and the checks before the build.
	Warnings []string `json:"warnings,omitempty"`
}

func deployAirflow(cmd *cobra.Command, args []string, out io.Writer) error {
	// --workspace and its older spelling --workspace-id must agree.
	if err := utils.ApplyPreferredFlags(cmd.Flags(), nil); err != nil {
		return err
	}
	if len(args) > 0 && deployDeployment != "" && args[0] != deployDeployment {
		return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], deployDeployment))
	}
	// One walk routes the deploy and words its refusal. A pyproject.toml
	// project's deploy from its root, a build or --dags, is the manifest
	// deploy. An image named on the command line (--image-name, local or
	// --remote) keeps APC's own route, from anywhere, as it was.
	where := utils.Locate(config.WorkingPath)
	if where.AtRoot(config.WorkingPath) && imageName == "" && !imagePresentOnRemote {
		cmd.SilenceUsage = true
		return deployManifest(cmd, args, &where, out)
	}

	inProject, err := refuseUndeployable(where)
	if err != nil {
		// Not a usage mistake to print the usage block under: what is in
		// the directory decided it.
		cmd.SilenceUsage = true
		return err
	}
	if cmd.Flags().Changed("build-secret") {
		cmd.SilenceUsage = true
		return errors.New("--build-secret has no effect with --image-name: the image is already built")
	}

	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	deploymentID := deployDeployment
	if len(args) > 0 {
		deploymentID = args[0]
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if description == "" {
		description = utils.GetDefaultDeployDescription(false)
	}

	// The deploy's progress (its notes, the image push, the DAG upload) goes
	// where it always has in text, and to stderr under json, where stdout
	// carries the result.
	opts := deploy.Options{Progress: cliout.NotesTo(cmd, deployOutput, out), Yes: deployYes}
	r := cliout.Renderer{Format: deployOutput, Out: out}
	result := deployJSON{Workspace: ws}
	// dags is where the Deployment takes its DAGs from, as the image deploy
	// found it.
	var dags deploy.DagsFrom

	if imagePresentOnRemote {
		deployed, err := UpdateDeploymentImage(houstonClient, deploymentID, ws, runtimeVersionForImageName, imageName, opts)
		if err != nil {
			return err
		}
		deploymentID, dags = deployed.DeploymentID, deployed.Dags
		result.Image, result.RuntimeVersion = imageName, runtimeVersionForImageName
	} else {
		// Since we prompt the user to enter the deploymentID in come cases for DeployAirflowImage, reusing the same  deploymentID for DagsOnlyDeploy
		deployed, err := DeployAirflowImage(houstonClient, deploymentID, ws, deployPrompt, isImageOnlyDeploy, imageName, opts)
		if err != nil {
			return err
		}
		deploymentID, dags = deployed.DeploymentID, deployed.Dags
		result.Image, result.URL = deployed.Image, deployed.URL
	}
	result.Deployment, result.Type = deploymentID, deployTypeImage

	// Don't deploy dags even for dags-only deployments --image is passed
	if isImageOnlyDeploy {
		fmt.Fprintln(opts.Progress, "Dags in the project will not be deployed since --image is passed.")
		return emitDeploy(r, &result)
	}

	after := &dagsAfterImage{client: houstonClient, workspace: ws, deployment: deploymentID, path: config.WorkingPath, imageName: imageName, description: description, dags: dags, inProject: inProject}
	if err := deployDagsAfterImage(after, opts, &result); err != nil {
		return err
	}
	return emitDeploy(r, &result)
}

// dagsAfterImage is what the DAG upload after an image deploy goes on.
type dagsAfterImage struct {
	client     houston.ClientInterface
	workspace  string
	deployment string
	// path is the directory whose dags directory is uploaded.
	path string
	// imageName is --image-name: an image not built from path, which need
	// not be a project at all.
	imageName   string
	description string
	// dags is where the Deployment takes its DAGs from, as the image deploy
	// found it.
	dags deploy.DagsFrom
	// inProject is whether the deploy ran at a pyproject.toml project's
	// root, whose dags directory follows the image.
	inProject bool
}

// deployDagsAfterImage uploads the working directory's DAGs to a Deployment
// whose image was just deployed, when it takes DAG uploads, and records in
// result what it did and what it warned about.
func deployDagsAfterImage(a *dagsAfterImage, opts deploy.Options, result *deployJSON) error {
	switch a.dags {
	case deploy.DagsFromImage:
		// The Deployment got its DAGs from the image just pushed, which was
		// built elsewhere and carries whatever it carries, possibly none, and
		// nothing else would say so.
		warn(result, opts.Progress, fmt.Sprintf(warningImageNameDagsInImage, a.imageName))
		return nil
	case deploy.DagsFromElsewhere:
		return nil
	case deploy.DagsFromUpload, deploy.DagsFromUnknown:
		// Uploaded below, from a pyproject.toml project. A Deployment the
		// deploy did not place is uploaded to as a deploy always has, and
		// DagsOnlyDeploy's refusals decide.
	}
	if !a.inProject {
		// No project here: the working directory's dags/ belongs to no
		// project this deploy knows, so the image ships alone, and an
		// upload that a Deployment taking them would want is said not to
		// have happened, whatever apc.show_warnings is.
		msg := noticeImageAlone
		if a.dags == deploy.DagsFromUnknown {
			msg = noticeImageAloneUnplaced
		}
		alwaysWarn(result, opts.Progress, fmt.Sprintf(msg, a.deployment))
		return nil
	}

	_, err := DagsOnlyDeploy(a.client, a.workspace, a.deployment, a.path, nil, true, a.description, opts)
	switch {
	case deploy.IsDagOnlyDeployDisabledInClusterConfig(err) || errors.Is(err, deploy.ErrDagOnlyDeployNotEnabledForDeployment):
		// A Deployment not placed, or changed since: it takes no upload, and
		// the image deploy stands, as it always has.
		return nil
	case errors.Is(err, deploy.ErrNoDagsDirectory):
		// DagsOnlyDeploy refuses before it looks for the directory, so the
		// Deployment takes uploads: one was due and did not happen, which is
		// said whatever apc.show_warnings is.
		alwaysWarn(result, opts.Progress, fmt.Sprintf(noticeNoDagsDir, a.path))
		return nil
	case a.dags == deploy.DagsFromUnknown && errors.Is(err, deploy.ErrAppConfigUnread):
		// Whether the Deployment takes uploads at all could not be read,
		// after its image was deployed: the deploy stands, and an upload
		// that may have been due did not happen, which is said whatever
		// apc.show_warnings is.
		alwaysWarn(result, opts.Progress, fmt.Sprintf(noticeDagsUndecided, err, a.deployment))
		return nil
	case err != nil:
		return err
	}
	result.Type = deployTypeImageAndDags
	return nil
}

// warn prints a deploy warning with the deploy's progress (stdout in text,
// stderr under json) and records it in the json result, unless apc.show_warnings
// is off, which silences the deploy's other warnings too.
func warn(result *deployJSON, progress io.Writer, msg string) {
	if !config.CFG.ShowWarnings.GetBool() {
		return
	}
	alwaysWarn(result, progress, msg)
}

// alwaysWarn prints a deploy warning and records it in the json result, as
// warn does, whatever apc.show_warnings is: for a part of the deploy that was
// asked for and did not happen.
func alwaysWarn(result *deployJSON, progress io.Writer, msg string) {
	fmt.Fprintln(progress, "Warning: "+msg)
	result.Warnings = append(result.Warnings, msg)
}

// emitDeploy publishes a finished deploy under json. In text the deploy has
// already said what it did, as it always has, and this prints nothing.
func emitDeploy(r cliout.Renderer, result *deployJSON) error {
	if r.Format != cliout.FormatJSON {
		return nil
	}
	return r.Emit(result, nil)
}
