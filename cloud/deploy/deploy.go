package deploy

import (
	"bufio"
	"bytes"
	httpContext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/azure"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/pkg/errors"
)

const (
	parse                  = "parse"
	astroDomain            = "astronomer.io"
	registryUsername       = "cli"
	runtimeImageLabel      = airflow.RuntimeImageLabel
	dagParseAllowedVersion = "4.1.0"

	composeImageBuildingPromptMsg     = "Building image..."
	composeSkipImageBuildingPromptMsg = "Skipping building image..."
	deploymentHeaderMsg               = "Authenticated to %s \n\n"

	warningInvalidImageNameMsg = "WARNING! The image in your Dockerfile '%s' is not based on Astro Runtime and is not supported. Change your Dockerfile with an image that pulls from 'quay.io/astronomer/astro-runtime' to proceed.\n"

	allTests                 = "all-tests"
	parseAndPytest           = "parse-and-all-tests"
	enableDagDeployMsg       = "DAG-only deploys are not enabled for this Deployment. Run 'astro deployment update %s --dag-deploy enable' to enable DAG-only deploys"
	dagDeployDisabled        = "dag deploy is not enabled for deployment"
	invalidWorkspaceID       = "Invalid workspace id %s was provided through the --workspace-id flag\n"
	errCiCdEnforcementUpdate = "cannot deploy since ci/cd enforcement is enabled for the deployment %s. Please use API Tokens instead"
)

var (
	pytestFile string
	dockerfile = "Dockerfile"

	deployImagePlatformSupport = []string{"linux/amd64"}

	// Monkey patched to write unit tests
	airflowImageHandler  = airflow.ImageHandlerInit
	containerHandlerInit = airflow.ContainerHandlerInit
	azureUploader        = azure.Upload
	canCiCdDeploy        = deployment.CanCiCdDeploy
	dagTarballVersion    = ""
	dagsUploadURL        = ""
	nextTag              = ""
)

var (
	errDagsParseFailed = errors.New("your local DAGs did not parse. Fix the listed errors or use `astro deploy [deployment-id] -f` to force deploy") //nolint:revive
	envFileMissing     = errors.New("Env file path is incorrect: ")                                                                                  //nolint:revive
)

var (
	sleepTime              = 90
	dagOnlyDeploySleepTime = 30
	tickNum                = 10
)

type deploymentInfo struct {
	deploymentID             string
	namespace                string
	deployImage              string
	currentVersion           string
	organizationID           string
	workspaceID              string
	webserverURL             string
	deploymentType           string
	desiredDagTarballVersion string
	dagDeployEnabled         bool
	cicdEnforcement          bool
	name                     string
	isRemoteExecutionEnabled bool
}

type InputDeploy struct {
	Path              string
	RuntimeID         string
	WsID              string
	Pytest            string
	EnvFile           string
	ImageName         string
	DeploymentName    string
	Prompt            bool
	Dags              bool
	Image             bool
	WaitForStatus     bool
	WaitTime          time.Duration
	DagsPath          string
	Description       string
	BuildSecretString string
}

// InputClientDeploy contains inputs for client image deployments
type InputClientDeploy struct {
	Path              string
	ImageName         string
	Platform          string
	BuildSecretString string
	DeploymentID      string
}

const accessYourDeploymentFmt = `

 Access your Deployment:

 Deployment View: %s
 Airflow UI: %s
`

func removeDagsFromDockerIgnore(fullpath string) error {
	f, err := os.Open(fullpath)
	if err != nil {
		return err
	}

	defer f.Close()

	var bs []byte
	buf := bytes.NewBuffer(bs)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if text != "dags/" {
			_, err = buf.WriteString(text + "\n")
			if err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	err = os.WriteFile(fullpath, bytes.Trim(buf.Bytes(), "\n"), 0o666) //nolint:gosec, mnd
	if err != nil {
		return err
	}

	return nil
}

func shouldIncludeMonitoringDag(deploymentType astroplatformcore.DeploymentType) bool {
	return !organization.IsOrgHosted() && !deployment.IsDeploymentDedicated(deploymentType) && !deployment.IsDeploymentStandard(deploymentType)
}

func deployDags(path, dagsPath, dagsUploadURL, currentRuntimeVersion string, deploymentType astroplatformcore.DeploymentType) (string, error) {
	if shouldIncludeMonitoringDag(deploymentType) {
		monitoringDagPath := filepath.Join(dagsPath, "astronomer_monitoring_dag.py")

		// Create monitoring dag file
		err := fileutil.WriteStringToFile(monitoringDagPath, airflow.Af2MonitoringDag)
		if err != nil {
			return "", err
		}

		// Remove the monitoring dag file after the upload
		defer os.Remove(monitoringDagPath)
	}

	versionID, err := UploadBundle(path, dagsPath, dagsUploadURL, true, currentRuntimeVersion)
	if err != nil {
		return "", err
	}

	return versionID, nil
}

// Deploy pushes a new docker image
func Deploy(deployInput InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error { //nolint
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	if c.Domain == astroDomain {
		fmt.Printf(deploymentHeaderMsg, "Astro")
	} else {
		fmt.Printf(deploymentHeaderMsg, c.Domain)
	}

	deployInfo, err := getDeploymentInfo(deployInput.RuntimeID, deployInput.WsID, deployInput.DeploymentName, deployInput.Prompt, platformCoreClient, coreClient)
	if err != nil {
		return err
	}

	var dagsPath string
	if deployInput.DagsPath != "" {
		dagsPath = deployInput.DagsPath
	} else {
		dagsPath = filepath.Join(deployInput.Path, "dags")
	}

	var dagFiles []string
	if !deployInfo.isRemoteExecutionEnabled {
		dagFiles = fileutil.GetFilesWithSpecificExtension(dagsPath, ".py")
	}

	if deployInfo.cicdEnforcement {
		if !canCiCdDeploy(c.Token) {
			return fmt.Errorf(errCiCdEnforcementUpdate, deployInfo.name) //nolint
		}
	}

	if deployInput.WsID != deployInfo.workspaceID {
		fmt.Printf(invalidWorkspaceID, deployInput.WsID)
		return nil
	}

	if deployInput.Image {
		if !deployInfo.dagDeployEnabled {
			return fmt.Errorf(enableDagDeployMsg, deployInfo.deploymentID) //nolint
		}
	}

	deploymentURL, err := deployment.GetDeploymentURL(deployInfo.deploymentID, deployInfo.workspaceID)
	if err != nil {
		return err
	}

	// Check if git metadata is enabled (default: true)
	var deployGit *astrocore.DeployGit
	var commitMessage string
	gitMetadataEnabled := config.CFG.DeployGitMetadata.GetBool()
	if envVal := os.Getenv("ASTRO_DEPLOY_GIT_METADATA"); envVal != "" {
		gitMetadataEnabled = util.CheckEnvBool(envVal)
	}
	if gitMetadataEnabled {
		deployGit, commitMessage = retrieveLocalGitMetadata(deployInput.Path)
	}

	// Use commit message as description fallback
	description := deployInput.Description
	if description == "" {
		description = commitMessage
	}

	// Build the deploy request with git metadata
	createDeployRequest := astroplatformcore.CreateDeployRequest{
		Description: &description,
	}

	// Set deploy type
	switch {
	case deployInput.Dags:
		createDeployRequest.Type = astroplatformcore.CreateDeployRequestTypeDAGONLY
	case deployInput.Image:
		createDeployRequest.Type = astroplatformcore.CreateDeployRequestTypeIMAGEONLY
	default:
		createDeployRequest.Type = astroplatformcore.CreateDeployRequestTypeIMAGEANDDAG
	}

	// Add git metadata if available
	if deployGit != nil {
		createDeployRequest.Git = &astroplatformcore.CreateDeployGitRequest{
			Provider:   astroplatformcore.GITHUB,
			Account:    deployGit.Account,
			Repo:       deployGit.Repo,
			Branch:     deployGit.Branch,
			CommitSha:  deployGit.CommitSha,
			CommitUrl:  deployGit.CommitUrl,
			AuthorName: deployGit.AuthorName,
			Path:       deployGit.Path,
		}
	}

	deploy, err := createDeploy(deployInfo.organizationID, deployInfo.deploymentID, createDeployRequest, platformCoreClient)
	if err != nil {
		return err
	}
	deployID := deploy.Id
	imageRepository := deploy.ImageRepository
	if deploy.DagsUploadUrl != nil {
		dagsUploadURL = *deploy.DagsUploadUrl
	} else {
		dagsUploadURL = ""
	}
	if deploy.ImageTag != "" {
		nextTag = deploy.ImageTag
	} else {
		nextTag = ""
	}

	if deployInput.Dags {
		if len(dagFiles) == 0 && config.CFG.ShowWarnings.GetBool() {
			i, _ := input.Confirm("Warning: No DAGs found. This will delete any existing DAGs. Are you sure you want to deploy?")

			if !i {
				fmt.Println("Canceling deploy...")
				return nil
			}
		}
		if deployInput.Pytest != "" {
			runtimeVersion, err := buildImage(deployInput.Path, deployInfo.currentVersion, deployInfo.deployImage, deployInput.ImageName, deployInfo.organizationID, deployInput.BuildSecretString, deployInfo.dagDeployEnabled, deployInfo.isRemoteExecutionEnabled, platformCoreClient)
			if err != nil {
				return err
			}

			err = parseOrPytestDAG(deployInput.Pytest, runtimeVersion, deployInput.EnvFile, deployInfo.deployImage, deployInfo.namespace, deployInput.BuildSecretString)
			if err != nil {
				return err
			}
		}

		if !deployInfo.dagDeployEnabled {
			return fmt.Errorf(enableDagDeployMsg, deployInfo.deploymentID) //nolint
		}

		fmt.Println("Initiating DAG deploy for: " + deployInfo.deploymentID)
		dagTarballVersion, err = deployDags(deployInput.Path, dagsPath, dagsUploadURL, deployInfo.currentVersion, astroplatformcore.DeploymentType(deployInfo.deploymentType))
		if err != nil {
			if strings.Contains(err.Error(), dagDeployDisabled) {
				return fmt.Errorf(enableDagDeployMsg, deployInfo.deploymentID) //nolint
			}

			return err
		}

		// finish deploy
		err = finalizeDeploy(deployID, deployInfo.deploymentID, deployInfo.organizationID, dagTarballVersion, deployInfo.dagDeployEnabled, platformCoreClient)
		if err != nil {
			return err
		}

		if deployInput.WaitForStatus {
			// Keeping wait timeout low since dag only deploy is faster
			err = deployment.HealthPoll(deployInfo.deploymentID, deployInfo.workspaceID, dagOnlyDeploySleepTime, tickNum, int(deployInput.WaitTime.Seconds()), platformCoreClient)
			if err != nil {
				return err
			}

			fmt.Println(
				"\nSuccessfully uploaded DAGs with version " + ansi.Bold(dagTarballVersion) + " to Astro. Navigate to the Airflow UI to confirm that your deploy was successful." +
					fmt.Sprintf(accessYourDeploymentFmt, ansi.Bold(deploymentURL), ansi.Bold(deployInfo.webserverURL)),
			)

			return nil
		}

		fmt.Println(
			"\nSuccessfully uploaded DAGs with version " + ansi.Bold(
				dagTarballVersion,
			) + " to Astro. Navigate to the Airflow UI to confirm that your deploy was successful. The Airflow UI takes about 1 minute to update." +
				fmt.Sprintf(
					accessYourDeploymentFmt,
					ansi.Bold(deploymentURL),
					ansi.Bold(deployInfo.webserverURL),
				),
		)
	} else {
		fullpath := filepath.Join(deployInput.Path, ".dockerignore")
		fileExist, _ := fileutil.Exists(fullpath, nil)
		if fileExist {
			err := removeDagsFromDockerIgnore(fullpath)
			if err != nil {
				return errors.Wrap(err, "Found dags entry in .dockerignore file. Remove this entry and try again")
			}
		}
		envFileExists, _ := fileutil.Exists(deployInput.EnvFile, nil)
		if !envFileExists && deployInput.EnvFile != ".env" {
			return fmt.Errorf("%w %s", envFileMissing, deployInput.EnvFile)
		}

		if deployInfo.dagDeployEnabled && len(dagFiles) == 0 && config.CFG.ShowWarnings.GetBool() && !deployInput.Image {
			i, _ := input.Confirm("Warning: No DAGs found. This will delete any existing DAGs. Are you sure you want to deploy?")

			if !i {
				fmt.Println("Canceling deploy...")
				return nil
			}
		}

		// Build our image
		runtimeVersion, err := buildImage(deployInput.Path, deployInfo.currentVersion, deployInfo.deployImage, deployInput.ImageName, deployInfo.organizationID, deployInput.BuildSecretString, deployInfo.dagDeployEnabled, deployInfo.isRemoteExecutionEnabled, platformCoreClient)
		if err != nil {
			return err
		}

		if len(dagFiles) > 0 {
			err = parseOrPytestDAG(deployInput.Pytest, runtimeVersion, deployInput.EnvFile, deployInfo.deployImage, deployInfo.namespace, deployInput.BuildSecretString)
			if err != nil {
				return err
			}
		} else {
			fmt.Println("No DAGs found. Skipping testing...")
		}

		repository := imageRepository
		// TODO: Resolve the edge case where two people push the same nextTag at the same time
		remoteImage := fmt.Sprintf("%s:%s", repository, nextTag)

		imageHandler := airflowImageHandler(deployInfo.deployImage)
		fmt.Println("Pushing image to Astronomer registry")
		_, err = imageHandler.Push(remoteImage, registryUsername, c.Token, false)
		if err != nil {
			return err
		}

		if deployInfo.dagDeployEnabled && len(dagFiles) > 0 {
			if !deployInput.Image {
				dagTarballVersion, err = deployDags(deployInput.Path, dagsPath, dagsUploadURL, deployInfo.currentVersion, astroplatformcore.DeploymentType(deployInfo.deploymentType))
				if err != nil {
					return err
				}
			} else {
				fmt.Println("Image Deploy only. Skipping deploying DAG...")
			}
		}
		// finish deploy
		err = finalizeDeploy(deployID, deployInfo.deploymentID, deployInfo.organizationID, dagTarballVersion, deployInfo.dagDeployEnabled, platformCoreClient)
		if err != nil {
			return err
		}

		if deployInput.WaitForStatus {
			err = deployment.HealthPoll(deployInfo.deploymentID, deployInfo.workspaceID, sleepTime, tickNum, int(deployInput.WaitTime.Seconds()), platformCoreClient)
			if err != nil {
				return err
			}
		}

		fmt.Println("Successfully pushed image to Astronomer registry. Navigate to the Astronomer UI for confirmation that your deploy was successful. To deploy dags only run astro deploy --dags." +
			fmt.Sprintf(accessYourDeploymentFmt, ansi.Bold("https://"+deploymentURL), ansi.Bold("https://"+deployInfo.webserverURL)))
	}

	return nil
}

func getDeploymentInfo(
	deploymentID, wsID, deploymentName string,
	prompt bool,
	platformCoreClient astroplatformcore.CoreClient,
	coreClient astrocore.CoreClient,
) (deploymentInfo, error) {
	// Use config deployment if provided
	if deploymentID == "" {
		deploymentID = config.CFG.ProjectDeployment.GetProjectString()
		if deploymentID != "" {
			fmt.Printf("Deployment ID found in the config file. This Deployment ID will be used for the deploy\n")
		}
	}

	if deploymentID != "" && deploymentName != "" {
		fmt.Printf("Both a Deployment ID and Deployment name have been supplied. The Deployment ID %s will be used for the Deploy\n", deploymentID)
	}

	// check if deploymentID or if force prompt was requested was given by user
	if deploymentID == "" || prompt {
		currentDeployment, err := deployment.GetDeployment(wsID, deploymentID, deploymentName, false, nil, platformCoreClient, coreClient)
		if err != nil {
			return deploymentInfo{}, err
		}
		coreDeployment, err := deployment.CoreGetDeployment(currentDeployment.OrganizationId, currentDeployment.Id, platformCoreClient)
		if err != nil {
			return deploymentInfo{}, err
		}
		var desiredDagTarballVersion string
		if coreDeployment.DesiredDagTarballVersion != nil {
			desiredDagTarballVersion = *coreDeployment.DesiredDagTarballVersion
		} else {
			desiredDagTarballVersion = ""
		}

		return deploymentInfo{
			currentDeployment.Id,
			currentDeployment.Namespace,
			airflow.ImageName(currentDeployment.Namespace, "latest"),
			currentDeployment.RuntimeVersion,
			currentDeployment.OrganizationId,
			currentDeployment.WorkspaceId,
			currentDeployment.WebServerUrl,
			string(*currentDeployment.Type),
			desiredDagTarballVersion,
			currentDeployment.IsDagDeployEnabled,
			currentDeployment.IsCicdEnforced,
			currentDeployment.Name,
			deployment.IsRemoteExecutionEnabled(&currentDeployment),
		}, nil
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return deploymentInfo{}, err
	}
	deployInfo, err := fetchDeploymentDetails(deploymentID, c.Organization, platformCoreClient)
	if err != nil {
		return deploymentInfo{}, err
	}
	deployInfo.deploymentID = deploymentID
	return deployInfo, nil
}

func parseOrPytestDAG(pytest, runtimeVersion, envFile, deployImage, namespace, buildSecretString string) error {
	validDAGParseVersion := airflowversions.CompareRuntimeVersions(runtimeVersion, dagParseAllowedVersion) >= 0
	if !validDAGParseVersion {
		fmt.Println("\nruntime image is earlier than 4.1.0, this deploy will skip DAG parse...")
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, "Dockerfile", namespace)
	if err != nil {
		return err
	}

	switch {
	case pytest == parse && validDAGParseVersion:
		// parse dags
		fmt.Println("Testing image...")
		err := parseDAGs(deployImage, buildSecretString, containerHandler)
		if err != nil {
			return err
		}
	case pytest != "" && pytest != parse && pytest != parseAndPytest:
		// check pytests
		fmt.Println("Testing image...")
		err := checkPytest(pytest, deployImage, buildSecretString, containerHandler)
		if err != nil {
			return err
		}
	case pytest == parseAndPytest:
		// parse dags and check pytests
		fmt.Println("Testing image...")
		err := parseDAGs(deployImage, buildSecretString, containerHandler)
		if err != nil {
			return err
		}

		err = checkPytest(pytest, deployImage, buildSecretString, containerHandler)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseDAGs(deployImage, buildSecretString string, containerHandler airflow.ContainerHandler) error {
	if !config.CFG.SkipParse.GetBool() && !util.CheckEnvBool(os.Getenv("ASTRONOMER_SKIP_PARSE")) {
		err := containerHandler.Parse("", deployImage, buildSecretString)
		if err != nil {
			fmt.Println(err)
			return errDagsParseFailed
		}
	} else {
		fmt.Println("Skipping parsing dags due to skip parse being set to true in either the config.yaml or local environment variables")
	}

	return nil
}

// Validate code with pytest
func checkPytest(pytest, deployImage, buildSecretString string, containerHandler airflow.ContainerHandler) error {
	if pytest != allTests && pytest != parseAndPytest {
		pytestFile = pytest
	}

	exitCode, err := containerHandler.Pytest(pytestFile, "", deployImage, "", buildSecretString)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
		}
		return errors.Wrap(err, "Something went wrong while Pytesting your DAGs,\nif the issue persists rerun the command without the '--pytest' flag to deploy")
	}

	fmt.Print("\nAll Pytests passed!\n")
	return err
}

func fetchDeploymentDetails(deploymentID, organizationID string, platformCoreClient astroplatformcore.CoreClient) (deploymentInfo, error) {
	resp, err := platformCoreClient.GetDeploymentWithResponse(httpContext.Background(), organizationID, deploymentID)
	if err != nil {
		return deploymentInfo{}, err
	}

	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return deploymentInfo{}, err
	}

	currentVersion := resp.JSON200.RuntimeVersion
	namespace := resp.JSON200.Namespace
	workspaceID := resp.JSON200.WorkspaceId
	webserverURL := resp.JSON200.WebServerUrl
	dagDeployEnabled := resp.JSON200.IsDagDeployEnabled
	cicdEnforcement := resp.JSON200.IsCicdEnforced
	isRemoteExecutionEnabled := deployment.IsRemoteExecutionEnabled(resp.JSON200)
	var desiredDagTarballVersion string
	if resp.JSON200.DesiredDagTarballVersion != nil {
		desiredDagTarballVersion = *resp.JSON200.DesiredDagTarballVersion
	} else {
		desiredDagTarballVersion = ""
	}

	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := airflow.ImageName(namespace, "latest")

	return deploymentInfo{
		namespace:                namespace,
		deployImage:              deployImage,
		currentVersion:           currentVersion,
		organizationID:           organizationID,
		workspaceID:              workspaceID,
		webserverURL:             webserverURL,
		dagDeployEnabled:         dagDeployEnabled,
		desiredDagTarballVersion: desiredDagTarballVersion,
		cicdEnforcement:          cicdEnforcement,
		isRemoteExecutionEnabled: isRemoteExecutionEnabled,
	}, nil
}

func buildImageWithoutDags(path, buildSecretString string, imageHandler airflow.ImageHandler) error {
	// flag to determine if we are setting the dags folder in dockerignore
	dagsIgnoreSet := false
	// flag to determine if dockerignore file was created on runtime
	dockerIgnoreCreate := false
	fullpath := filepath.Join(path, ".dockerignore")

	defer func() {
		// remove dags from .dockerignore file if we set it
		if dagsIgnoreSet {
			removeDagsFromDockerIgnore(fullpath) //nolint:errcheck
		}
		// remove created docker ignore file
		if dockerIgnoreCreate {
			os.Remove(fullpath)
		}
	}()

	fileExist, _ := fileutil.Exists(fullpath, nil)
	if !fileExist {
		// Create a dockerignore file and add the dags folder entry
		err := fileutil.WriteStringToFile(fullpath, "dags/")
		if err != nil {
			return err
		}
		dockerIgnoreCreate = true
	}
	lines, err := fileutil.Read(fullpath)
	if err != nil {
		return err
	}
	contains, _ := fileutil.Contains(lines, "dags/")
	if !contains {
		f, err := os.OpenFile(fullpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:mnd
		if err != nil {
			return err
		}

		defer f.Close()

		if _, err := f.WriteString("\ndags/"); err != nil {
			return err
		}

		dagsIgnoreSet = true
	}
	err = imageHandler.Build("", buildSecretString, types.ImageBuildConfig{Path: path, TargetPlatforms: deployImagePlatformSupport})
	if err != nil {
		return err
	}

	// remove dags from .dockerignore file if we set it
	if dagsIgnoreSet {
		err = removeDagsFromDockerIgnore(fullpath)
		if err != nil {
			return err
		}
	}

	return nil
}

func buildImage(path, currentVersion, deployImage, imageName, organizationID, buildSecretString string, dagDeployEnabled, isRemoteExecutionEnabled bool, platformCoreClient astroplatformcore.CoreClient) (version string, err error) {
	imageHandler := airflowImageHandler(deployImage)

	if imageName == "" {
		// Build our image
		fmt.Println(composeImageBuildingPromptMsg)

		if dagDeployEnabled || isRemoteExecutionEnabled {
			err := buildImageWithoutDags(path, buildSecretString, imageHandler)
			if err != nil {
				return "", err
			}
		} else {
			err := imageHandler.Build("", buildSecretString, types.ImageBuildConfig{Path: path, TargetPlatforms: deployImagePlatformSupport})
			if err != nil {
				return "", err
			}
		}
	} else {
		// skip build if an imageName is passed
		fmt.Println(composeSkipImageBuildingPromptMsg)

		err := imageHandler.TagLocalImage(imageName)
		if err != nil {
			return "", err
		}
	}

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse dockerfile: %s", filepath.Join(path, dockerfile))
	}

	DockerfileImage := docker.GetImageFromParsedFile(cmds)

	version, err = imageHandler.GetLabel("", runtimeImageLabel)
	if err != nil {
		fmt.Println("unable get runtime version from image")
	}

	if config.CFG.ShowWarnings.GetBool() && version == "" {
		fmt.Printf(warningInvalidImageNameMsg, DockerfileImage)
		fmt.Println("Canceling deploy...")
		os.Exit(1)
	}

	resp, err := platformCoreClient.GetDeploymentOptionsWithResponse(httpContext.Background(), organizationID, &astroplatformcore.GetDeploymentOptionsParams{})
	if err != nil {
		return "", err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return "", err
	}
	deploymentOptionsRuntimeVersions := []string{}
	for _, runtimeRelease := range resp.JSON200.RuntimeReleases {
		deploymentOptionsRuntimeVersions = append(deploymentOptionsRuntimeVersions, runtimeRelease.Version)
	}

	if !ValidRuntimeVersion(currentVersion, version, deploymentOptionsRuntimeVersions) {
		fmt.Println("Canceling deploy...")
		os.Exit(1)
	}

	WarnIfNonLatestVersion(version, httputil.NewHTTPClient())

	return version, nil
}

// finalize deploy
func finalizeDeploy(deployID, deploymentID, organizationID, dagTarballVersion string, dagDeploy bool, platformCoreClient astroplatformcore.CoreClient) error {
	finalizeDeployRequest := astroplatformcore.FinalizeDeployRequest{}
	if dagDeploy {
		finalizeDeployRequest.DagTarballVersion = &dagTarballVersion
	}
	resp, err := platformCoreClient.FinalizeDeployWithResponse(httpContext.Background(), organizationID, deploymentID, deployID, finalizeDeployRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	if resp.JSON200.DagTarballVersion != nil {
		fmt.Println("Deployed DAG bundle: ", *resp.JSON200.DagTarballVersion)
	}
	if resp.JSON200.ImageTag != "" {
		fmt.Println("Deployed Image Tag: ", resp.JSON200.ImageTag)
	}
	return nil
}

func createDeploy(organizationID, deploymentID string, request astroplatformcore.CreateDeployRequest, platformCoreClient astroplatformcore.CoreClient) (*astroplatformcore.Deploy, error) {
	resp, err := platformCoreClient.CreateDeployWithResponse(httpContext.Background(), organizationID, deploymentID, request)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, err
}

func ValidRuntimeVersion(currentVersion, tag string, deploymentOptionsRuntimeVersions []string) bool {
	// Allow old deployments which do not have runtimeVersion tag
	if currentVersion == "" {
		return true
	}

	// Check that the tag is not a downgrade
	if airflowversions.CompareRuntimeVersions(tag, currentVersion) < 0 {
		fmt.Printf("Cannot deploy a downgraded Astro Runtime version. Modify your Astro Runtime version to %s or higher in your Dockerfile\n", currentVersion)
		return false
	}

	// Check that the tag is supported by the deployment
	tagInDeploymentOptions := false
	for _, runtimeVersion := range deploymentOptionsRuntimeVersions {
		if airflowversions.CompareRuntimeVersions(tag, runtimeVersion) == 0 {
			tagInDeploymentOptions = true
			break
		}
	}
	if !tagInDeploymentOptions {
		fmt.Println("Cannot deploy an unsupported Astro Runtime version. Modify your Astro Runtime version to a supported version in your Dockerfile")
		fmt.Printf("Supported versions: %s\n", strings.Join(deploymentOptionsRuntimeVersions, ", "))
		return false
	}

	// If upgrading from Airflow 2 to Airflow 3, we require at least Runtime 12.0.0 (Airflow 2.10.0)
	currentVersionAirflowMajorVersion := airflowversions.AirflowMajorVersionForRuntimeVersion(currentVersion)
	tagAirflowMajorVersion := airflowversions.AirflowMajorVersionForRuntimeVersion(tag)
	if currentVersionAirflowMajorVersion == "2" && tagAirflowMajorVersion == "3" {
		if airflowversions.CompareRuntimeVersions(currentVersion, "12.0.0") < 0 {
			fmt.Println("Can only upgrade deployment from Airflow 2 to Airflow 3 with deployment at Astro Runtime 12.0.0 or higher")
			return false
		}
	}

	return true
}

func WarnIfNonLatestVersion(version string, httpClient *httputil.HTTPClient) {
	client := airflowversions.NewClient(httpClient, false, false)
	latestRuntimeVersion, err := airflowversions.GetDefaultImageTag(client, "", "", false)
	if err != nil {
		logger.Debugf("unable to get latest runtime version: %s", err)
		return
	}

	if airflowversions.CompareRuntimeVersions(version, latestRuntimeVersion) < 0 {
		fmt.Printf("WARNING! You are currently running Astro Runtime Version %s\nConsider upgrading to the latest version, Astro Runtime %s\n", version, latestRuntimeVersion)
	}
}

// ClientBuildContext represents a prepared build context for client deployment
type ClientBuildContext struct {
	// TempDir is the temporary directory containing the build context
	TempDir string
	// CleanupFunc should be called to clean up the temporary directory
	CleanupFunc func()
}

// prepareClientBuildContext creates a temporary build context with client dependency files
// This avoids modifying the original project files, preventing race conditions with concurrent deployments.
func prepareClientBuildContext(sourcePath string) (*ClientBuildContext, error) {
	// Create a temporary directory for the build context
	tempBuildDir, err := os.MkdirTemp("", "astro-client-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary build directory: %w", err)
	}

	// Cleanup function to be called by the caller
	cleanup := func() {
		os.RemoveAll(tempBuildDir)
	}

	// Always return cleanup function if we created a temp directory, even on error
	buildContext := &ClientBuildContext{
		TempDir:     tempBuildDir,
		CleanupFunc: cleanup,
	}

	// Check if source directory exists first
	if exists, err := fileutil.Exists(sourcePath, nil); err != nil {
		return buildContext, fmt.Errorf("failed to check if source directory exists: %w", err)
	} else if !exists {
		return buildContext, fmt.Errorf("source directory does not exist: %s", sourcePath)
	}

	// Copy all project files to the temporary directory
	err = fileutil.CopyDirectory(sourcePath, tempBuildDir)
	if err != nil {
		return buildContext, fmt.Errorf("failed to copy project files to temporary directory: %w", err)
	}

	// Process client dependency files
	err = setupClientDependencyFiles(tempBuildDir)
	if err != nil {
		return buildContext, fmt.Errorf("failed to setup client dependency files: %w", err)
	}

	return buildContext, nil
}

// setupClientDependencyFiles processes client-specific dependency files in the build context
func setupClientDependencyFiles(buildDir string) error {
	// Define dependency file pairs (client file -> regular file)
	dependencyFiles := map[string]string{
		"requirements-client.txt": "requirements.txt",
		"packages-client.txt":     "packages.txt",
	}

	// Process client dependency files in the build directory
	for clientFile, regularFile := range dependencyFiles {
		clientPath := filepath.Join(buildDir, clientFile)
		regularPath := filepath.Join(buildDir, regularFile)

		// Copy client file content to the regular file location (requires client file to exist)
		if err := fileutil.CopyFile(clientPath, regularPath); err != nil {
			return fmt.Errorf("failed to copy %s to %s in build context: %w", clientFile, regularFile, err)
		}
	}

	return nil
}

// DeployClientImage handles the client deploy functionality
func DeployClientImage(deployInput InputClientDeploy, platformCoreClient astroplatformcore.CoreClient) error { //nolint:gocritic
	c, err := config.GetCurrentContext()
	if err != nil {
		return errors.Wrap(err, "failed to get current context")
	}

	// Validate deployment runtime version if deployment ID is provided
	if err := validateClientImageRuntimeVersion(deployInput, platformCoreClient); err != nil {
		return err
	}

	// Get the remote client registry endpoint from config
	registryEndpoint := config.CFG.RemoteClientRegistry.GetString()
	if registryEndpoint == "" {
		fmt.Println("The Astro CLI is not configured to push client images to your private registry.")
		fmt.Println("For remote Deployments, client images must be stored in your private registry, not in Astronomer managed registries.")
		fmt.Println("Please provide your private registry information so the Astro CLI can push client images.")
		return errors.New("remote client registry is not configured. To configure it, run: 'astro config set remote.client_registry <endpoint>' and try again.")
	}

	// Use consistent deploy-<timestamp> tagging mechanism like regular deploys
	// The ImageName flag only specifies which local image to use, not the remote tag
	imageTag := "deploy-" + time.Now().UTC().Format("2006-01-02T15-04")

	// Build the full remote image name
	remoteImage := fmt.Sprintf("%s:%s", registryEndpoint, imageTag)

	// Create an image handler for building and pushing
	imageHandler := airflowImageHandler(remoteImage)

	if deployInput.ImageName != "" {
		// Use the provided local image (tag will be ignored, remote tag is always timestamp-based)
		fmt.Println("Using provided image:", deployInput.ImageName)
		err := imageHandler.TagLocalImage(deployInput.ImageName)
		if err != nil {
			return fmt.Errorf("failed to tag local image: %w", err)
		}
	} else {
		// Authenticate with the base image registry before building
		// This is needed because Dockerfile.client uses base images from a private registry

		// Skip registry login if the base image registry is not from astronomer, check the content of the Dockerfile.client file
		dockerfileClientContent, err := fileutil.ReadFileToString(filepath.Join(deployInput.Path, "Dockerfile.client"))
		if util.IsAstronomerRegistry(dockerfileClientContent) || err != nil {
			// login to the registry
			if err != nil {
				fmt.Println("WARNING: Failed to read Dockerfile.client, so will assume the base image is using images.astronomer.cloud and try to login to the registry")
			}
			baseImageRegistry := config.CFG.RemoteBaseImageRegistry.GetString()
			fmt.Printf("Authenticating with base image registry: %s\n", baseImageRegistry)
			err := airflow.DockerLogin(baseImageRegistry, registryUsername, c.Token)
			if err != nil {
				fmt.Println("Failed to authenticate with Astronomer registry that contains the base agent image used in the Dockerfile.client file.")
				fmt.Println("This could be because either your token has expired or you don't have permission to pull the base agent image.")
				fmt.Println("Please re-login via `astro login` to refresh the credentials or validate that `ASTRO_API_TOKEN` environment variable is set with the correct token and try again")
				return fmt.Errorf("failed to authenticate with registry %s: %w", baseImageRegistry, err)
			}
		}

		// Build the client image from the current directory
		// Determine target platforms for client deploy
		var targetPlatforms []string
		if deployInput.Platform != "" {
			// Parse comma-separated platforms from --platform flag
			targetPlatforms = strings.Split(deployInput.Platform, ",")
			// Trim whitespace from each platform
			for i, platform := range targetPlatforms {
				targetPlatforms[i] = strings.TrimSpace(platform)
			}
			fmt.Printf("Building client image for platforms: %s\n", strings.Join(targetPlatforms, ", "))
		} else {
			// Use empty slice to let Docker build for host platform by default
			targetPlatforms = []string{}
			fmt.Println("Building client image for host platform")
		}

		// Prepare build context with client dependency files
		buildContext, err := prepareClientBuildContext(deployInput.Path)
		if buildContext != nil && buildContext.CleanupFunc != nil {
			defer buildContext.CleanupFunc()
		}
		if err != nil {
			return fmt.Errorf("failed to prepare client build context: %w", err)
		}

		// Build the image from the prepared context
		buildConfig := types.ImageBuildConfig{
			Path:            buildContext.TempDir,
			TargetPlatforms: targetPlatforms,
		}

		err = imageHandler.Build("Dockerfile.client", deployInput.BuildSecretString, buildConfig)
		if err != nil {
			return fmt.Errorf("failed to build client image: %w", err)
		}
	}

	// Push the image to the remote registry (assumes docker login was done externally)
	fmt.Println("Pushing client image to configured remote registry")
	_, err = imageHandler.Push(remoteImage, "", "", false)
	if err != nil {
		if errors.Is(err, airflow.ErrImagePush403) {
			fmt.Printf("\n--------------------------------\n")
			fmt.Printf("Failed to push client image to %s\n", registryEndpoint)
			fmt.Println("It could be due to either your registry token has expired or you don't have permission to push the client image")
			fmt.Printf("Please ensure that you have logged in to `%s` via `docker login` and try again\n\n", registryEndpoint)
		}
		return fmt.Errorf("failed to push client image: %w", err)
	}

	fmt.Printf("Successfully pushed client image to %s\n", ansi.Bold(remoteImage))

	fmt.Printf("\n--------------------------------\n")
	fmt.Println("The client image has been pushed to your private registry.")
	fmt.Println("Your next step would be to update the agent component to use the new client image.")
	fmt.Println("For that you would either need to update the helm chart values.yaml file or update your CI/CD pipeline to use the new client image.")
	fmt.Printf("If you are using Astronomer provided Agent Helm chart, you would need to update the `image` field for each of the workers, dagProcessor, and triggerer component sections to the new image: %s\n", remoteImage)
	fmt.Println("Once you have updated the helm chart values.yaml file, you can run 'helm upgrade' or update via your CI/CD pipeline to update the agent components")

	return nil
}

// validateClientImageRuntimeVersion validates that the client image runtime version
// is not newer than the deployment runtime version
func validateClientImageRuntimeVersion(deployInput InputClientDeploy, platformCoreClient astroplatformcore.CoreClient) error { //nolint:gocritic
	// Skip validation if no deployment ID provided
	if deployInput.DeploymentID == "" {
		return nil
	}

	// Get current context for organization info
	c, err := config.GetCurrentContext()
	if err != nil {
		return errors.Wrap(err, "failed to get current context")
	}

	// Get deployment information
	deployInfo, err := fetchDeploymentDetails(deployInput.DeploymentID, c.Organization, platformCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to get deployment information")
	}

	// Parse Dockerfile.client to get client image runtime version
	dockerfileClientPath := filepath.Join(deployInput.Path, "Dockerfile.client")
	if _, err := os.Stat(dockerfileClientPath); os.IsNotExist(err) {
		return errors.New("Dockerfile.client is required for client image runtime version validation")
	}

	cmds, err := docker.ParseFile(dockerfileClientPath)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Dockerfile.client: %s", dockerfileClientPath)
	}

	baseImage := docker.GetImageFromParsedFile(cmds)
	if baseImage == "" {
		return errors.New("failed to find base image in Dockerfile.client")
	}

	// Extract runtime version from the base image tag
	clientRuntimeVersion, err := extractRuntimeVersionFromImage(baseImage)
	if err != nil {
		return errors.Wrapf(err, "failed to extract runtime version from client image %s", baseImage)
	}

	// Compare versions
	if airflowversions.CompareRuntimeVersions(clientRuntimeVersion, deployInfo.currentVersion) > 0 {
		return fmt.Errorf(`client image runtime version validation failed:

The client image is based on Astro Runtime version %s, which is newer than the deployment's runtime version %s.

To fix this issue, you can either:
1. Downgrade the client image version by updating the base image in Dockerfile.client to use runtime version %s or earlier
2. Upgrade the deployment's runtime version to %s or higher

This validation ensures compatibility between your client image and the deployment environment`,
			clientRuntimeVersion, deployInfo.currentVersion, deployInfo.currentVersion, clientRuntimeVersion)
	}

	fmt.Printf("✓ Client image runtime version %s is compatible with deployment runtime version %s\n",
		clientRuntimeVersion, deployInfo.currentVersion)

	return nil
}

// extractRuntimeVersionFromImage extracts the runtime version from an image tag
// Example: "images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.1-1-python-3.12-astro-agent-1.1.0"
// Returns: "3.1-1"
func extractRuntimeVersionFromImage(imageName string) (string, error) {
	// Split image name to get the tag part
	parts := strings.Split(imageName, ":")
	if len(parts) < 2 {
		return "", errors.New("image name does not contain a tag")
	}

	imageTag := parts[len(parts)-1] // Get the last part as the tag

	// Use the existing ParseImageTag function from airflow_versions package
	tagInfo, err := airflowversions.ParseImageTag(imageTag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse image tag: %s", imageTag)
	}

	return tagInfo.RuntimeVersion, nil
}
