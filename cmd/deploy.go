package cmd

import (
	"fmt"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	deployExample = `
Deployment you would like to deploy to Airflow cluster:

  $ astro deploy <deployment name>

Deploy deployment using suggestion:

  $ astro deploy

  Select which airflow deployment you want to deploy to:
   #     LABEL                   DEPLOYMENT NAME            WORKSPACE     DEPLOYMENT ID
   1     new-deployment-name     physical-diameter-1566     w1            ck1ryz2jd00430f50a5dmu7g9
  
  >1
`
	deployCmd = &cobra.Command{
		Use:     "deploy DEPLOYMENT",
		Short:   "Deploy an airflow project",
		Long:    "Deploy an airflow project to a given deployment",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    deploy,
		Example: deployExample,
		Aliases: []string{"airflow deploy"},
	}
)

func init() {
	RootCmd.AddCommand(deployCmd)
	deployCmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	deployCmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	deployCmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	deployCmd.Flags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
}

func deploy(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	releaseName := ""

	// Get release name from args, if passed
	if len(args) > 0 {
		releaseName = args[0]
	}

	// Save release name in config if specified
	if len(releaseName) > 0 && saveDeployConfig {
		config.CFG.ProjectDeployment.SetProjectString(releaseName)
	}

	if git.HasUncommitedChanges() && !forceDeploy {
		fmt.Println(messages.REGISTRY_UNCOMMITTED_CHANGES)
		return nil
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.Deploy(config.WorkingPath, releaseName, ws, forcePrompt)
}
