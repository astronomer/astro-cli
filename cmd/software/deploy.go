package software

import (
	"fmt"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/software/deploy"

	"github.com/spf13/cobra"
)

var (
	forceDeploy      bool
	forcePrompt      bool
	saveDeployConfig bool

	ignoreCacheDeploy = false

	ensureProjectDir   = utils.EnsureProjectDir
	deployAirflowImage = deploy.Airflow
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

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy [DEPLOYMENT ID]",
		Short:   "Deploy an Airflow project",
		Long:    "Deploy an Airflow project to an Astronomer Cluster",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    deployAirflow,
		Example: deployExample,
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().BoolVarP(&ignoreCacheDeploy, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
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

	return deployAirflowImage(houstonClient, config.WorkingPath, deploymentID, ws, ignoreCacheDeploy, forcePrompt)
}
