package cloud

import (
	"fmt"

	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/util"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	forceDeploy       bool
	forcePrompt       bool
	saveDeployConfig  bool
	pytest            bool
	parse             bool
	dags              bool
	waitForDeploy     bool
	image             bool
	dagsPath          string
	pytestFile        string
	envFile           string
	imageName         string
	deploymentName    string
	deployDescription string
	deployExample     = `
Specify the ID of the Deployment on Astronomer you would like to deploy this project to:

  $ astro deploy <deployment ID>

Menu will be presented if you do not specify a deployment ID:

  $ astro deploy
`

	DeployImage      = cloud.Deploy
	EnsureProjectDir = utils.EnsureProjectDir
	buildSecrets     = []string{}
)

const (
	registryUncommitedChangesMsg = "Project directory has uncommitted changes, use `astro deploy [deployment-id] -f` to force deploy."
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy DEPLOYMENT-ID",
		Short:   "Deploy your project to a Deployment on Astro",
		Long:    "Deploy your project to a Deployment on Astro. This command bundles your project files into a Docker image and pushes that Docker image to Astronomer. It does not include any metadata associated with your local Airflow environment.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureProjectDir,
		RunE:    deploy,
		Example: deployExample,
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy even if project contains errors or uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().BoolVar(&pytest, "pytest", false, "Deploy code to Astro only if the specified Pytests are passed")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables for Pytests")
	cmd.Flags().StringVarP(&pytestFile, "test", "t", "", "Location of Pytests or specific Pytest file. All Pytest files must be located in the tests directory")
	cmd.Flags().StringVarP(&imageName, "image-name", "i", "", "Name of a custom image to deploy")
	cmd.Flags().BoolVarP(&dags, "dags", "d", false, "Push only DAGs to your Astro Deployment")
	cmd.Flags().BoolVarP(&image, "image", "", false, "Push only an image to your Astro Deployment. If you have DAG Deploy enabled your DAGs will not be affected.")
	cmd.Flags().StringVar(&dagsPath, "dags-path", "", "If set deploy dags from this path instead of the dags from working directory")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to deploy to")
	cmd.Flags().BoolVar(&parse, "parse", false, "Succeed only if all DAGs in your Astro project parse without errors")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().MarkHidden("dags-path") //nolint:errcheck
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Add a description for more context on this deploy")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")
	return cmd
}

func deployTests(parse, pytest, forceDeploy bool, pytestFile string) string {
	if pytest && pytestFile == "" {
		pytestFile = "all-tests"
	}

	if !parse && !pytest && !forceDeploy || parse && !pytest && !forceDeploy || parse && !pytest && forceDeploy {
		pytestFile = "parse"
	}

	if parse && pytest {
		pytestFile = "parse-and-all-tests"
	}

	return pytestFile
}

func deploy(cmd *cobra.Command, args []string) error {
	deploymentID = ""

	// Get deploymentId from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	if deploymentID == "" || forcePrompt || workspaceID == "" {
		var err error
		workspaceID, err = coalesceWorkspace()
		if err != nil {
			return errors.Wrap(err, "failed to find a valid workspace")
		}
	}

	if dags && image {
		return errors.New("cannot use both --dags and --image together. Run 'astro deploy' to update both your image and dags")
	}

	// Save deploymentId in config if specified
	if len(deploymentID) > 0 && saveDeployConfig {
		err := config.CFG.ProjectDeployment.SetProjectString(deploymentID)
		if err != nil {
			return nil
		}
	}

	if git.HasUncommittedChanges("") && !forceDeploy {
		fmt.Println(registryUncommitedChangesMsg)
		return nil
	}

	// case for astro deploy --dags whose default operation should be not running any tests
	if dags && !parse && !pytest {
		pytestFile = ""
	} else {
		pytestFile = deployTests(parse, pytest, forceDeploy, pytestFile)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	BuildSecretString := util.GetbuildSecretString(buildSecrets)

	deployInput := cloud.InputDeploy{
		Path:              config.WorkingPath,
		RuntimeID:         deploymentID,
		WsID:              workspaceID,
		Pytest:            pytestFile,
		EnvFile:           envFile,
		ImageName:         imageName,
		DeploymentName:    deploymentName,
		Prompt:            forcePrompt,
		Dags:              dags,
		Image:             image,
		WaitForStatus:     waitForDeploy,
		DagsPath:          dagsPath,
		Description:       deployDescription,
		BuildSecretString: BuildSecretString,
	}

	return DeployImage(deployInput, platformCoreClient, astroCoreClient)
}
