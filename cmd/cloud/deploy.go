package cloud

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	forceDeploy       bool
	forcePrompt       bool
	saveDeployConfig  bool
	pytest            bool
	parse             bool
	dags              bool
	waitForDeploy     bool
	waitTime          time.Duration
	image             bool
	dagsPath          string
	pytestFile        string
	envFile           string
	imageName         string
	deploymentName    string
	deployDescription string
	noDagsBaseDir     bool
	dagBundleName     string
	nonDags           bool
	nonDagsMountPath  string
	nonDagsBundleType string
	nonDagsBundlePath string
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

	deployWaitTime = 300 * time.Second

	imageNameFlag = "image-name"
	nonDagsFlag   = "non-dags"
	dagsPathFlag  = "dags-path"
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy DEPLOYMENT-ID",
		Short: "Deploy your project to a Deployment on Astro",
		Long:  "Deploy your project to a Deployment on Astro. This command bundles your project files into a Docker image and pushes that Docker image to Astronomer. In Deployments with Remote Execution enabled, this only updates the Orchestration Plane components (the API Server and Scheduler). For all other components, use `astro remote deploy` instead. It does not include any metadata associated with your local Airflow environment.",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed(imageNameFlag) || cmd.Flags().Changed(nonDagsFlag) {
				return nil
			}
			// A DAG-only deploy sourcing its DAGs from --dags-path doesn't read anything else
			// from the working directory, unless --pytest/--parse is also used, which builds
			// and runs a local image to test-parse the DAGs and so still needs the project root.
			// Check the bound values (not cmd.Flags().Changed) so --dags=false or an explicit
			// empty --dags-path can't be misread as enabling the bypass.
			if dags && dagsPath != "" && !pytest && !parse {
				return nil
			}
			return EnsureProjectDir(cmd, args)
		},
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
	cmd.Flags().StringVarP(&imageName, imageNameFlag, "i", "", "Name of a custom image to deploy, or image name with custom tag when used with --client")
	cmd.Flags().BoolVarP(&dags, "dags", "d", false, "Push only DAGs to your Astro Deployment")
	cmd.Flags().BoolVar(&noDagsBaseDir, "no-dags-base-dir", false, "Exclude the dags directory prefix from the bundle. Use for Airflow 3.x deployments where sys.path includes the bundle root")
	cmd.Flags().StringVar(&dagBundleName, "dag-bundle-name", "", "Deploy DAGs to a named DAG bundle on the Deployment instead of the default bundle. Requires Airflow 3, and the bundle must already exist on the Deployment")
	cmd.Flags().MarkHidden("dag-bundle-name") //nolint:errcheck
	cmd.Flags().BoolVarP(&image, "image", "", false, "Push only an image to your Astro Deployment. If you have DAG Deploy enabled your DAGs will not be affected.")
	cmd.Flags().StringVar(&dagsPath, dagsPathFlag, "", "If set deploy dags from this path instead of the dags from working directory")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to deploy to")
	cmd.Flags().BoolVar(&parse, "parse", false, "Succeed only if all DAGs in your Astro project parse without errors")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTime, "wait-time", deployWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Add a description for more context on this deploy")
	utils.AddBuildSecretFlags(cmd.Flags(), &buildSecrets)
	cmd.Flags().Bool("force-upgrade-to-af3", false, "This flag is no longer required for Airflow 2 to Airflow 3 upgrades. Support will be removed in a future release.")
	cmd.Flags().MarkDeprecated("force-upgrade-to-af3", "this flag is no longer required for Airflow 2 to Airflow 3 upgrades. Support will be removed in a future release.") //nolint:errcheck
	cmd.Flags().BoolVar(&nonDags, nonDagsFlag, false, "Deploy a non-DAG bundle from a separate directory, instead of your Astro project. Requires --non-dags-mount-path")
	cmd.Flags().StringVar(&nonDagsMountPath, "non-dags-mount-path", "", "Path to mount the non-DAG bundle in Airflow, for reference by DAGs. Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundleType, "non-dags-bundle-type", "none", "Free-form label identifying the kind of non-DAG bundle (e.g. dbt). Any value is accepted. Defaults to \"none\". Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundlePath, "non-dags-local-path", "", "Path to the non-DAG bundle to deploy. Default current directory. Used with --non-dags")
	cmd.Flags().MarkHidden(nonDagsFlag)            //nolint:errcheck
	cmd.Flags().MarkHidden("non-dags-mount-path")  //nolint:errcheck
	cmd.Flags().MarkHidden("non-dags-bundle-type") //nolint:errcheck
	cmd.Flags().MarkHidden("non-dags-local-path")  //nolint:errcheck

	annotateDeployFlag(cmd, "image", "image")
	annotateDeployFlag(cmd, imageNameFlag, "image")
	annotateDeployFlag(cmd, "build-secret", "image")
	annotateDeployFlag(cmd, "dags", "dag")
	annotateDeployFlag(cmd, "no-dags-base-dir", "dag")
	annotateDeployFlag(cmd, "dag-bundle-name", "dag")
	annotateDeployFlag(cmd, dagsPathFlag, "dag")
	annotateDeployFlag(cmd, "pytest", "test")
	annotateDeployFlag(cmd, "test", "test")
	annotateDeployFlag(cmd, "env", "test")
	annotateDeployFlag(cmd, "parse", "test")
	annotateDeployFlag(cmd, nonDagsFlag, "non-dags")
	annotateDeployFlag(cmd, "non-dags-mount-path", "non-dags")
	annotateDeployFlag(cmd, "non-dags-bundle-type", "non-dags")
	annotateDeployFlag(cmd, "non-dags-local-path", "non-dags")
	cmd.SetUsageTemplate(deployFlagsUsageTemplate)
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

	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return errors.New("cannot use --wait-time with --wait=false")
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

	if dagBundleName != "" && image {
		return errors.New("cannot use --dag-bundle-name with --image; named DAG bundles apply only to deploys that include DAGs")
	}

	if cmd.Flags().Changed(imageNameFlag) {
		for _, f := range []string{"dags", "dags-path", "no-dags-base-dir", "pytest", "parse", "build-secret", "build-secrets", "dag-bundle-name"} {
			if cmd.Flags().Changed(f) {
				return fmt.Errorf("cannot use --%s with --image-name; --image-name implies an image-only deploy", f)
			}
		}
	}

	if nonDags {
		return deployNonDagsBundle(cmd, args)
	}

	// Save deploymentId in config if specified
	if deploymentID != "" && saveDeployConfig {
		err := config.CFG.ProjectDeployment.SetProjectString(deploymentID)
		if err != nil {
			return errors.Wrap(err, "failed to save deployment id in config")
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

	deployInput := cloud.InputDeploy{
		Path:           config.WorkingPath,
		RuntimeID:      deploymentID,
		WsID:           workspaceID,
		Pytest:         pytestFile,
		EnvFile:        envFile,
		ImageName:      imageName,
		DeploymentName: deploymentName,
		Prompt:         forcePrompt,
		Dags:           dags,
		NoDagsBaseDir:  noDagsBaseDir,
		Image:          image,
		WaitForStatus:  waitForDeploy,
		WaitTime:       waitTime,
		DagsPath:       dagsPath,
		Description:    deployDescription,
		BuildSecrets:   util.ResolveBuildSecrets(buildSecrets, os.Getenv("BUILD_SECRET_INPUT")),
		Force:          forceDeploy,
		DagBundleName:  dagBundleName,
	}

	return DeployImage(deployInput, astroV1Client, astroV1Alpha1Client)
}

func deployNonDagsBundle(cmd *cobra.Command, args []string) error {
	for _, f := range []string{"dags", "image", imageNameFlag, "dag-bundle-name", "pytest", "parse", "build-secret", "build-secrets", "dags-path", "no-dags-base-dir"} {
		if cmd.Flags().Changed(f) {
			return fmt.Errorf("cannot use --%s with --non-dags; --non-dags performs a non-DAG bundle deploy", f)
		}
	}

	if nonDagsMountPath == "" {
		return errors.New("--non-dags-mount-path is required with --non-dags")
	}

	if nonDagsBundlePath == "" {
		nonDagsBundlePath = config.WorkingPath
	}

	info, err := os.Stat(nonDagsBundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle path %s does not exist", nonDagsBundlePath)
		}
		return fmt.Errorf("failed to access bundle path %s: %w", nonDagsBundlePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path %s is not a directory", nonDagsBundlePath)
	}

	withinAstroProject, err := config.IsWithinProjectDir(nonDagsBundlePath)
	if err != nil {
		return fmt.Errorf("failed to verify bundle path is not within an Astro project: %w", err)
	}
	if withinAstroProject {
		return errors.New("bundle path is within an Astro project. Non-DAG bundles must be a separate directory")
	}

	targetDeploymentID, err := resolveDeploymentIDFromArgsFlags(args, workspaceID, deploymentName)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true

	deployBundleInput := &cloud.DeployBundleInput{
		BundlePath:    nonDagsBundlePath,
		MountPath:     nonDagsMountPath,
		DeploymentID:  targetDeploymentID,
		BundleType:    nonDagsBundleType,
		Description:   deployDescription,
		Wait:          waitForDeploy,
		WaitTime:      waitTime,
		AstroV1Client: astroV1Client,
	}
	return DeployBundle(deployBundleInput)
}
