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
	deployDeployment string
	workspaceFlag    string
	deployForce      bool
	deployPrompt     bool
	deploySecrets    = []string{}

	DagsOnlyDeploy                     = deploy.DagsOnlyDeploy
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

  # Deploy an image built on this machine to a linked Deployment, or one by id
  astro deploy <LINK_OR_DEPLOYMENT_ID> --image-name <IMAGE_NAME>

  # Point a Deployment at an image already in a registry, without pushing it
  astro deploy <LINK_OR_DEPLOYMENT_ID> --image-name <REGISTRY>/<IMAGE>:<TAG> \
    --remote --runtime-version <RUNTIME_VERSION>`

func NewDeployCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [DEPLOYMENT_ID]",
		Short: "Deploy an image or Dags to a Deployment",
		Long: "Deploy a pyproject.toml project ([tool.astro]) to an Astro Private Cloud Deployment. Run it at the project's root: it builds the project into an image, Airflow 2 or Airflow 3, and pushes it to the Deployment, with the project's Dags inside the image for an image Deployment and uploaded beside it for a Dag-only one. " +
			"Name the Deployment by a link of the project (target = 'apc', written by astro link add) or by its id; an interactive deploy that names none asks, with the project's default link preselected. " +
			"With --image-name it deploys an image built on this machine instead of building one, from the project's root or from any directory outside a project; with --remote as well, it points the Deployment at an image already in a registry, pushing nothing. " +
			"A project in the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml) deploys with Astro CLI 1.x only.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployAirflow(cmd, args, out)
		},
		Example: deployExample,
	}
	// --force answered 1.x's uncommitted-changes check, which v2's deploy
	// does not make, and --prompt asked for the Deployment list with one
	// named, which v2 offers whenever none is: to be asked, name none. Both
	// stay, accepted, hidden and read by nothing, as on Astro, because
	// astronomer/deploy-action passes --force on every deploy.
	cmd.Flags().BoolVarP(&deployForce, "force", "f", false, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("force") //nolint:errcheck // the flag is defined just above
	cmd.Flags().BoolVarP(&deployPrompt, "prompt", "p", false, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("prompt") //nolint:errcheck // the flag is defined just above
	cmd.Flags().StringVar(&deployDeployment, "deployment", "", "Deployment to deploy to: a link name from the manifest, or a Deployment id")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace for the deploy, overriding the context")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace assigned to the Deployment")
	utils.MarkPreferredFlag(cmd.Flags(), "workspace", "workspace-id")
	cmd.Flags().StringVar(&description, "description", "", "Description to attach to the deploy, for traceability (default: one based on the deploy type)")
	cmd.Flags().BoolVarP(&isImageOnlyDeploy, "image", "", false, "Push only an image to your Deployment; works only for Dag-only, Git-sync-based and NFS-based Deployments")
	cmd.Flags().StringVarP(&imageName, "image-name", "i", "", "Name of a prebuilt image to deploy instead of building one: an image on this machine, or with --remote one already in a registry. Outside a pyproject.toml project it deploys the image alone")
	cmd.Flags().StringVar(&runtimeVersionForImageName, "runtime-version", "", "Astro Runtime version of the --remote image, which this machine cannot read from it. Example - 12.1.1. Required with --remote")
	cmd.Flags().BoolVarP(&imagePresentOnRemote, "remote", "", false, "Deploy the --image-name image from the registry it is already in, without pulling or pushing it; needs --runtime-version")
	utils.AddBuildSecretFlag(cmd.Flags(), &deploySecrets)
	cliout.AddOutputFlag(cmd, &deployOutput)

	if !context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.34.0"}) {
		cmd.Flags().BoolVarP(&isDagOnlyDeploy, "dags", "d", false, "Push only the Dags of the pyproject.toml project here to your Deployment")
	}
	return cmd
}

// refuseImageFlags stops a deploy given image flags that cannot all apply,
// before anything is read or asked:
//
//   - --dags with --image-name or --remote, in astro deploy's words: an image
//     source makes no sense with a deploy that ships only DAGs;
//   - --remote without --image-name, which names the image it deploys;
//   - --remote without --runtime-version: Houston takes the image's runtime,
//     and an image in a registry has no label here to read it from.
func refuseImageFlags() error {
	switch {
	case isDagOnlyDeploy && (imageName != "" || imagePresentOnRemote):
		return cliout.Usage(errDagsWithImageSource())
	case imagePresentOnRemote && imageName == "":
		return ErrImageNameNotPassedForRemoteFlag
	case imagePresentOnRemote && runtimeVersionForImageName == "":
		return deploy.ErrRuntimeVersionNotPassedForRemoteImage
	}
	return nil
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
	// beside it), image-only (--image, an image deployed alone from outside
	// a project, or a deploy whose Deployment took no DAGs from it), or
	// dag-only (--dags).
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
	// prints them (less the "Warning: " prefix): a prebuilt image that is all
	// an image Deployment's DAGs (not when apc.show_warnings is off), a DAG
	// upload that did not happen (always): an image deployed alone, to a
	// Deployment that takes DAG uploads, skipped for want of a dags
	// directory, or because whether the Deployment takes them could not be
	// read; an Airflow 2 to 3 upgrade, a runtime the platform does not list,
	// a Deployment that takes its DAGs from elsewhere, and the checks before
	// a build.
	Warnings []string `json:"warnings,omitempty"`
}

// deployAirflow routes a deploy by where it runs (utils.Locate's walk):
//
//   - at a pyproject.toml project's root every mode is the project's deploy
//     (deployManifest): a build, --dags, or a prebuilt image (--image-name,
//     local or --remote), its Deployment resolved from the project's apc
//     links as on Astro;
//   - in or below a project in the Astro CLI 1.x layout, which Astro CLI 1.x
//     deploys, or below a pyproject.toml project's root, every mode is
//     refused (kind no_project), --image-name included;
//   - outside any project only a prebuilt image deploys, alone, with no DAGs
//     and no links to resolve: its Deployment is an id or the workspace's
//     pick. --dags there has no project to upload the DAGs of, and a build
//     has nothing to build (kind no_project), as astro deploy on Astro says.
func deployAirflow(cmd *cobra.Command, args []string, out io.Writer) error {
	// --workspace and its older spelling --workspace-id must agree.
	if err := utils.ApplyPreferredFlags(cmd.Flags(), nil); err != nil {
		return err
	}
	if len(args) > 0 && deployDeployment != "" && args[0] != deployDeployment {
		return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], deployDeployment))
	}
	where := utils.Locate(config.WorkingPath)
	inProject := where.AtRoot(config.WorkingPath)
	if !inProject && !where.None() {
		// Not a usage mistake to print the usage block under: what is in
		// the directory decided it.
		cmd.SilenceUsage = true
		return utils.NoDeployableProject(where)
	}
	if err := refuseImageFlags(); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	cmd.SilenceUsage = true
	if inProject {
		return deployManifest(cmd, args, &where, out)
	}
	// The value, not whether the flag was given: an empty --image-name=
	// names no image, so this would be a build, and there is no project
	// here to build.
	if isDagOnlyDeploy || imageName == "" {
		return utils.NoDeployableProject(where)
	}
	return deployManifest(cmd, args, nil, out)
}
