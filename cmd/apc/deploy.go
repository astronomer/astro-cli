package apc

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/git"
)

var (
	forceDeploy  bool
	forcePrompt  bool
	deployOutput cliout.Format
	deployYes    bool

	// hasUncommittedChanges is a variable so a test does not depend on the
	// state of the checkout it runs in.
	hasUncommittedChanges = git.HasUncommittedChanges

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

var deployExample = `  # Deploy an image built on this machine, picking the Deployment from a list
  astro deploy --image-name <IMAGE_NAME>

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
	// noticeNoDagsDir: there is no dags directory to upload, so
	// DagsOnlyDeploy uploaded nothing. An empty upload would have deleted
	// the Deployment's DAGs. The %s after the path is the advice: where to
	// run the deploy from, or what the project lacks.
	noticeNoDagsDir = "no Dags were uploaded: there is no dags directory in %s, and the Deployment keeps the Dags it had. %s"
	// adviceRunFromProject: --image-name needs no project, so the deploy may
	// not have been run from one.
	adviceRunFromProject = "To upload them, run the deploy from the project directory."
	// adviceCreateDagsDir: the deploy ran from a project, which has none.
	adviceCreateDagsDir = "To upload Dags, create a dags directory in the project."
	// noticeDagsUndecided: the image was deployed, and the cluster config
	// that says whether the Deployment takes DAG uploads could not be read,
	// so an upload that may have been due did not happen.
	noticeDagsUndecided = "Dags were NOT updated: whether this Deployment takes Dag uploads could not be read (%v). The image was deployed, and the Deployment keeps the Dags it had. To upload them, run astro deploy %s --dags from the project directory."
)

// What a deploy that would build the project is told: v2 builds no project
// for APC. Astro CLI 1.x keeps deploying projects there, and a pyproject.toml
// project gets an APC deploy of its own later.
const (
	errBuildDeployNoProject = "Astro CLI v2 cannot build and deploy projects to Astro Private Cloud yet: use Astro CLI 1.x for now. " +
		"To deploy an image you built yourself, pass --image-name"
	errBuildDeployManifest = "Astro CLI v2 cannot build and deploy projects to Astro Private Cloud yet: use Astro CLI 1.x for now. " +
		"Support for pyproject.toml projects on Astro Private Cloud is coming. Until then, --dags uploads this project's DAGs, " +
		"and a project that declares dockerfile under [tool.astro] (a Dockerfile FROM an Astro Runtime image) " +
		"builds an image with astro package --tag <image> that astro deploy <deployment-id> --image-name <image> deploys"
)

var errUncommittedChanges = errors.New("project directory has uncommitted changes: commit them, or use astro deploy <deployment-id> --force to deploy anyway")

func NewDeployCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [DEPLOYMENT_ID]",
		Short: "Deploy an image or Dags to a Deployment",
		Long: "Deploy an image you built, or a project's Dags, to an APC Deployment. Astro CLI v2 does not build projects for Astro Private Cloud yet: " +
			"to build and deploy a project there, use Astro CLI 1.x. A project in the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml) deploys with Astro CLI 1.x only.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployAirflow(cmd, args, out)
		},
		Example: deployExample,
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace assigned to the Deployment")
	cmd.Flags().StringVar(&description, "description", "", "Description to attach to the deploy, for traceability (default: one based on the deploy type)")
	cmd.Flags().BoolVarP(&isImageOnlyDeploy, "image", "", false, "Push only an image to your Deployment; works only for Dag-only, Git-sync-based and NFS-based Deployments")
	cmd.Flags().StringVarP(&imageName, "image-name", "i", "", "Name of the custom image (present locally unless --remote is specified) to deploy")
	cmd.Flags().StringVar(&runtimeVersionForImageName, "runtime-version", "", "Runtime version of the image to deploy. Example - 12.1.1. Mandatory if --image-name --remote is provided")
	cmd.Flags().BoolVarP(&imagePresentOnRemote, "remote", "", false, "Custom image which is present on the remote registry. Can only be used with --image-name flag")
	cmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "Answer the deploy's confirmations yes: an image tag that is not recommended, and a DAGs folder with no DAGs")
	cliout.AddOutputFlag(cmd, &deployOutput)

	if !context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.34.0"}) {
		cmd.Flags().BoolVarP(&isDagOnlyDeploy, "dags", "d", false, "Push only the Dags of the pyproject.toml project here to your Deployment")
	}
	return cmd
}

// refuseUndeployable stops a deploy v2 does not make on APC, before anything
// is asked or sent:
//
//   - any deploy in or below a project in the Astro CLI 1.x layout, which
//     Astro CLI 1.x deploys (kind no_project);
//   - --dags anywhere but in a pyproject.toml project: it uploads that
//     project's dags directory (kind no_project);
//   - a deploy with no --image-name, which would build the project: v2 builds
//     none for APC yet. From a pyproject.toml project that is a mode v2 does
//     not have, a usage error; anywhere else there is no project to build,
//     kind no_project, as astro deploy on Astro says.
//
// --image-name (with or without --remote) needs no project: the image is
// built already, and the dags directory uploaded after it, where the
// Deployment takes one, is the working directory's when it has one.
func refuseUndeployable() error {
	inProject := project.HasManifest(config.WorkingPath)
	if !inProject {
		dir1x, err := utils.Project1xDir(config.WorkingPath)
		if err != nil {
			return err
		}
		if dir1x != "" {
			return utils.NoDeployableProject(utils.Deploy1xRefusedAPC)
		}
	}
	switch {
	case isDagOnlyDeploy:
		if !inProject {
			return utils.NoDeployableProject(utils.Deploy1xRefusedAPC)
		}
	case imagePresentOnRemote && imageName == "":
		return ErrImageNameNotPassedForRemoteFlag
	case imageName == "":
		// The value, not whether the flag was given: an empty --image-name=
		// names no image, so this would be a build.
		if inProject {
			return cliout.Usage(errors.New(errBuildDeployManifest))
		}
		return utils.NoProject(errBuildDeployNoProject)
	}
	return nil
}

// The kinds of deploy deployJSON.Type names.
const (
	deployTypeImageAndDags = "image_and_dags"
	deployTypeImage        = "image"
	deployTypeDags         = "dags"
)

// deployJSON is the one object `astro deploy --output json` publishes on APC,
// named as the Astro platform's deploy names the same things. Fields that do
// not apply to a deploy are omitted: a dags-only deploy pushes no image.
type deployJSON struct {
	Deployment string `json:"deployment"`
	Workspace  string `json:"workspace"`
	// Type is image_and_dags, image (--image, or a Deployment that takes no
	// DAG-only deploy), or dags (--dags).
	Type string `json:"type"`
	// Image is the image the Deployment now runs: the one pushed, or the one
	// --image-name --remote named.
	Image string `json:"image,omitempty"`
	// RuntimeVersion is --runtime-version, given with --image-name --remote.
	RuntimeVersion string `json:"runtime_version,omitempty"`
	// URL is the Deployment's Airflow UI, when Houston gives it.
	URL string `json:"url,omitempty"`
	// Warnings are what the deploy warned about without failing, as text
	// prints them (less the "Warning: " prefix): an --image-name image that
	// is all an image Deployment's DAGs (not when show_warnings is off), or
	// a DAG upload that did not happen (always): skipped for want of a dags
	// directory, to a Deployment that takes DAG uploads, or because whether
	// the Deployment takes them could not be read.
	Warnings []string `json:"warnings,omitempty"`
}

func deployAirflow(cmd *cobra.Command, args []string, out io.Writer) error {
	if err := refuseUndeployable(); err != nil {
		// Not a usage mistake to print the usage block under: what is in
		// the directory decided it.
		cmd.SilenceUsage = true
		return err
	}

	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	deploymentID := ""

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	// An error, not a printed note: returning nil here made a deploy that never
	// happened exit 0, so CI reported it as a success.
	if hasUncommittedChanges("") && !forceDeploy {
		// Not a usage mistake, so no usage block under the error.
		cmd.SilenceUsage = true
		return errUncommittedChanges
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if description == "" {
		description = utils.GetDefaultDeployDescription(isDagOnlyDeploy)
	}

	if isImageOnlyDeploy && isDagOnlyDeploy {
		return ErrBothDagsOnlyAndImageOnlySet
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

	if isDagOnlyDeploy {
		deployedTo, err := DagsOnlyDeploy(houstonClient, ws, deploymentID, config.WorkingPath, nil, true, description, opts)
		if err != nil {
			return err
		}
		result.Deployment, result.Type = deployedTo, deployTypeDags
		return emitDeploy(r, &result)
	}

	if imagePresentOnRemote {
		deployed, err := UpdateDeploymentImage(houstonClient, deploymentID, ws, runtimeVersionForImageName, imageName, opts)
		if err != nil {
			return err
		}
		deploymentID, dags = deployed.DeploymentID, deployed.Dags
		result.Image, result.RuntimeVersion = imageName, runtimeVersionForImageName
	} else {
		// Since we prompt the user to enter the deploymentID in come cases for DeployAirflowImage, reusing the same  deploymentID for DagsOnlyDeploy
		deployed, err := DeployAirflowImage(houstonClient, deploymentID, ws, forcePrompt, isImageOnlyDeploy, imageName, opts)
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

	after := &dagsAfterImage{client: houstonClient, workspace: ws, deployment: deploymentID, path: config.WorkingPath, imageName: imageName, description: description, dags: dags}
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
		// Uploaded below. A Deployment the deploy did not place is uploaded
		// to as a deploy always has, and DagsOnlyDeploy's refusals decide.
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
		// said whatever show_warnings is.
		advice := adviceRunFromProject
		if project.HasManifest(a.path) {
			advice = adviceCreateDagsDir
		}
		alwaysWarn(result, opts.Progress, fmt.Sprintf(noticeNoDagsDir, a.path, advice))
		return nil
	case a.dags == deploy.DagsFromUnknown && errors.Is(err, deploy.ErrAppConfigUnread):
		// Whether the Deployment takes uploads at all could not be read,
		// after its image was deployed: the deploy stands, and an upload
		// that may have been due did not happen, which is said whatever
		// show_warnings is.
		alwaysWarn(result, opts.Progress, fmt.Sprintf(noticeDagsUndecided, err, a.deployment))
		return nil
	case err != nil:
		return err
	}
	result.Type = deployTypeImageAndDags
	return nil
}

// warn prints a deploy warning with the deploy's progress (stdout in text,
// stderr under json) and records it in the json result, unless show_warnings
// is off, which silences the deploy's other warnings too.
func warn(result *deployJSON, progress io.Writer, msg string) {
	if !config.CFG.ShowWarnings.GetBool() {
		return
	}
	alwaysWarn(result, progress, msg)
}

// alwaysWarn prints a deploy warning and records it in the json result, as
// warn does, whatever show_warnings is: for a part of the deploy that was
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
