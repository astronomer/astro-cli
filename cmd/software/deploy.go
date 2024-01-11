package software

import (
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

	EnsureProjectDir   = utils.EnsureProjectDir
	DeployAirflowImage = deploy.Airflow
	isDagOnlyDeploy    bool
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
	if !context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.34.0"}) {
		cmd.Flags().BoolVarP(&isDagOnlyDeploy, "dags", "", false, "Push only DAGs to your Astronomer Deployment")
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
	if len(deploymentID) > 0 && saveDeployConfig {
		err = config.CFG.ProjectDeployment.SetProjectString(deploymentID)
		if err != nil {
			return err
		}
	}

	if git.HasUncommittedChanges() && !forceDeploy {
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
	}
	if isDagOnlyDeploy {
		return deploy.DagsOnlyDeploy(houstonClient, appConfig, deploymentID, config.WorkingPath, nil, true)
	}
	return DeployAirflowImage(houstonClient, config.WorkingPath, deploymentID, ws, byoRegistryDomain, ignoreCacheDeploy, byoRegistryEnabled, forcePrompt)
}
