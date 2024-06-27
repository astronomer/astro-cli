package cloud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	mountPath      string
	dbtProjectPath string

	DeployBundle = cloud.DeployBundle
)

const (
	dbtDeployExample = `
Specify the ID of the Deployment on Astronomer you would like to deploy this dbt project to:

  $ astro dbt deploy <deployment ID>

Menu will be presented if you do not specify a deployment ID:

  $ astro dbt deploy
`

	dbtDefaultMountPathPrefix = "/usr/local/airflow/dbt/"
	dbtProjectYmlFilename     = "dbt_project.yml"
	dbtBundleType             = "dbt"
)

func newDbtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbt",
		Short: "Manage your dbt projects deployed to Deployments running on Astronomer",
	}
	cmd.AddCommand(
		newDbtDeployCmd(),
	)
	return cmd
}

func newDbtDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy DEPLOYMENT-ID",
		Short:   "Deploy your dbt project to a Deployment on Astro",
		Long:    "Deploy your dbt project to a Deployment on Astro. This command bundles your dbt project files and uploads it to your Deployment.",
		Args:    cobra.MaximumNArgs(1),
		RunE:    deployDbt,
		Example: dbtDeployExample,
	}

	cmd.Flags().StringVarP(&mountPath, "mount-path", "m", "", "Path to mount dbt project in Airflow, for reference by DAGs. Default /usr/local/dbt/{dbt project name}")
	cmd.Flags().StringVarP(&dbtProjectPath, "project-path", "p", "", "Path to the dbt project to deploy. Default current directory")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to deploy to")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Description to store on the deploy")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")

	return cmd
}

func deployDbt(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	// if the dbt project path is not provided, use the current directory
	if dbtProjectPath == "" {
		dbtProjectPath = config.WorkingPath
	}

	// check that there is a valid dbt project at the dbt project path
	err := validateDbtProjectExists(dbtProjectPath)
	if err != nil {
		return err
	}

	// extract the dbt project's name
	dbtProjectName, err := extractDbtProjectName(dbtProjectPath)
	if err != nil {
		return fmt.Errorf("dbt project name not found in %s: %w", dbtProjectPath, err)
	}

	// if the workspace ID is not provided, try to find a valid workspace
	if workspaceID == "" {
		var err error
		workspaceID, err = coalesceWorkspace()
		if err != nil {
			return fmt.Errorf("failed to find a valid workspace: %w", err)
		}
	}

	// get the deployment to deploy the dbt project to
	var deploymentID string
	if len(args) > 0 {
		// if provided, use the deployment ID from the command argument
		deploymentID = args[0]
	} else {
		// otherwise, prompt the user to select a deployment
		selectedDeployment, err := deployment.GetDeployment(workspaceID, "", deploymentName, false, nil, platformCoreClient, astroCoreClient)
		if err != nil {
			return err
		}
		deploymentID = selectedDeployment.Id
	}
	fmt.Println("Initiating dbt deploy for deployment ID: " + deploymentID)

	// if the mount path is not provided, derive it from the dbt project name
	if mountPath == "" {
		mountPath = dbtDefaultMountPathPrefix + dbtProjectName
		fmt.Printf("Generated mount path from dbt project name: %s\n", mountPath)
	}

	// deploy the dbt project as a bundle
	deployBundleInput := &cloud.DeployBundleInput{
		BundlePath:   dbtProjectPath,
		MountPath:    mountPath,
		DeploymentID: deploymentID,
		BundleType:   dbtBundleType,
		Description:  deployDescription,
		Wait:         waitForDeploy,
	}
	err = DeployBundle(deployBundleInput, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	return nil
}

func validateDbtProjectExists(dbtProjectPath string) error {
	dbtProjectYamlPath := filepath.Join(dbtProjectPath, dbtProjectYmlFilename)

	_, err := os.Stat(dbtProjectYamlPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("dbt project file not found at %s. Please run this command in the root of your dbt project, or use --project-path to specify the dbt project path", dbtProjectYamlPath)
	}
	return err
}

func extractDbtProjectName(dbtProjectPath string) (string, error) {
	dbtProjectYamlPath := filepath.Join(dbtProjectPath, dbtProjectYmlFilename)

	var dbtProject map[string]interface{}
	file, err := os.Open(dbtProjectYamlPath)
	if err != nil {
		return "", fmt.Errorf("could not open %s: %w", dbtProjectYamlPath, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&dbtProject)
	if err != nil {
		return "", fmt.Errorf("could not decode %s: %w", dbtProjectYamlPath, err)
	}

	dbtProjectName, ok := dbtProject["name"].(string)
	if !ok || dbtProjectName == "" {
		return "", errors.New("invalid dbt project name")
	}

	return dbtProjectName, nil
}
