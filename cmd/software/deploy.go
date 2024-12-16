package software

import (
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/software/deploy"

	"github.com/spf13/cobra"
)

var (
	forceDeploy      bool
	forcePrompt      bool
	saveDeployConfig bool

	ignoreCacheDeploy = false

	EnsureProjectDir                   = utils.EnsureProjectDir
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

var deployExample = `
Deployment you would like to deploy to Airflow cluster:

$ astro deploy <deployment-id>

Menu will be presented if you do not specify a deployment name:

$ astro deploy
`

const (
	registryUncommittedChanges = "Project directory has uncommmited changes, use `astro deploy <deployment-id> -f` to force deploy."
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy [DEPLOYMENT ID]",
		Short:   "Deploy an Airflow project",
		Long:    "Deploy an Airflow project to an Astronomer Cluster",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureProjectDir,
		RunE:    deployAirflow,
		Example: deployExample,
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().BoolVarP(&ignoreCacheDeploy, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	cmd.Flags().StringVar(&description, "description", "", "Improve traceability by attaching a description to a code deploy. If you don't provide a description, the system automatically assigns a default description based on the deploy type.")
	cmd.Flags().BoolVarP(&isImageOnlyDeploy, "image", "", false, "Push only an image to your Astro Deployment. This only works for Dag-only, Git-sync-based and NFS-based deployments.")
	cmd.Flags().StringVarP(&imageName, "image-name", "i", "", "Name of the custom image(should be present locally unless --remote is specified) to deploy")
	cmd.Flags().StringVar(&runtimeVersionForImageName, "runtime-version", "", "Runtime version of the image to deploy. Example - 12.1.1. Mandatory if --image-name --remote is provided")
	cmd.Flags().BoolVarP(&imagePresentOnRemote, "remote", "", false, "Custom image which is present on the remote registry. Can only be used with --image-name flag")

	if !context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.34.0"}) {
		cmd.Flags().BoolVarP(&isDagOnlyDeploy, "dags", "d", false, "Push only DAGs to your Deployment")
	}
	return cmd
}

func deployAirflow(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	deploymentID := ""

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	// Save release name in config if specified
	if deploymentID != "" && saveDeployConfig {
		err = config.CFG.ProjectDeployment.SetProjectString(deploymentID)
		if err != nil {
			return err
		}
	}

	if git.HasUncommittedChanges("") && !forceDeploy {
		fmt.Println(registryUncommittedChanges)
		return nil
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var byoRegistryEnabled bool
	var byoRegistryDomain string
	if appConfig != nil && appConfig.Flags.BYORegistryEnabled {
		byoRegistryEnabled = true
		byoRegistryDomain = appConfig.BYORegistryDomain
		if byoRegistryDomain == "" {
			return deploy.ErrBYORegistryDomainNotSet
		}
	}

	if description == "" {
		description = utils.GetDefaultDeployDescription(isDagOnlyDeploy)
	}

	if isImageOnlyDeploy && isDagOnlyDeploy {
		return ErrBothDagsOnlyAndImageOnlySet
	}

	if isDagOnlyDeploy {
		return DagsOnlyDeploy(houstonClient, appConfig, ws, deploymentID, config.WorkingPath, nil, true, description)
	}

	if imagePresentOnRemote {
		if imageName == "" {
			return ErrImageNameNotPassedForRemoteFlag
		}
		deploymentID, err = UpdateDeploymentImage(houstonClient, deploymentID, ws, runtimeVersionForImageName, imageName)
		if err != nil {
			return err
		}
	} else {
		// Since we prompt the user to enter the deploymentID in come cases for DeployAirflowImage, reusing the same  deploymentID for DagsOnlyDeploy
		deploymentID, err = DeployAirflowImage(houstonClient, config.WorkingPath, deploymentID, ws, byoRegistryDomain, ignoreCacheDeploy, byoRegistryEnabled, forcePrompt, description, isImageOnlyDeploy, imageName)
		if err != nil {
			return err
		}
	}

	// Don't deploy dags even for dags-only deployments --image is passed
	if isImageOnlyDeploy {
		fmt.Println("Dags in the project will not be deployed since --image is passed.")
		return nil
	}

	err = DagsOnlyDeploy(houstonClient, appConfig, ws, deploymentID, config.WorkingPath, nil, true, description)
	// Don't throw the error if dag-deploy itself is disabled
	if errors.Is(err, deploy.ErrDagOnlyDeployDisabledInConfig) || errors.Is(err, deploy.ErrDagOnlyDeployNotEnabledForDeployment) {
		return nil
	}
	return err
}
