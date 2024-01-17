package deploy

import (
	"bufio"
	"bytes"
	httpContext "context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/docker/docker/api/types/versions"
	"github.com/pkg/errors"
)

const (
	parse                  = "parse"
	astroDomain            = "astronomer.io"
	registryUsername       = "cli"
	runtimeImageLabel      = airflow.RuntimeImageLabel
	defaultRuntimeVersion  = "4.2.5"
	dagParseAllowedVersion = "4.1.0"

	composeImageBuildingPromptMsg     = "Building image..."
	composeSkipImageBuildingPromptMsg = "Skipping building image..."
	deploymentHeaderMsg               = "Authenticated to %s \n\n"

	warningInvaildImageNameMsg = "WARNING! The image in your Dockerfile '%s' is not based on Astro Runtime and is not supported. Change your Dockerfile with an image that pulls from 'quay.io/astronomer/astro-runtime' to proceed.\n"
	warningInvalidImageTagMsg  = "WARNING! You are about to push an image using the '%s' runtime tag. This is not supported.\nConsider using one of the following supported tags: %s"

	message            = "DAGs uploaded successfully"
	action             = "UPLOAD"
	allTests           = "all-tests"
	parseAndPytest     = "parse-and-all-tests"
	enableDagDeployMsg = "DAG-only deploys are not enabled for this Deployment. Run 'astro deployment update %s --dag-deploy enable' to enable DAG-only deploys."
	dagDeployDisabled  = "dag deploy is not enabled for deployment"
	invalidWorkspaceID = "Invalid workspace id %s was provided through the --workspace-id flag\n"
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
	errDagsParseFailed        = errors.New("your local DAGs did not parse. Fix the listed errors or use `astro deploy [deployment-id] -f` to force deploy") //nolint:revive
	envFileMissing            = errors.New("Env file path is incorrect: ")                                                                                  //nolint:revive
	errCiCdEnforcementUpdate  = errors.New("cannot update dag deploy since ci/cd enforcement is enabled for this deployment. Please use API Tokens or API Keys instead")
	errImageDeployNoPriorDags = errors.New("cannot do image only deploy with no prior DAGs deployed. Please deploy DAGs to your deployment first")
)

var (
	sleepTime              = 90
	dagOnlyDeploySleepTime = 30
	tickNum                = 10
	timeoutNum             = 180
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
	DagsPath          string
	Description       string
	BuildSecretString string
}

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
	err = os.WriteFile(fullpath, bytes.Trim(buf.Bytes(), "\n"), 0o666) //nolint:gosec, gomnd
	if err != nil {
		return err
	}

	return nil
}

func shouldIncludeMonitoringDag(deploymentType astroplatformcore.DeploymentType) bool {
	return !organization.IsOrgHosted() && !deployment.IsDeploymentDedicated(deploymentType) && !deployment.IsDeploymentStandard(deploymentType)
}

func deployDags(path, dagsPath, dagsUploadURL string, deploymentType astroplatformcore.DeploymentType) (string, error) {
	// Check the dags directory
	monitoringDagPath := filepath.Join(dagsPath, "astronomer_monitoring_dag.py")

	if shouldIncludeMonitoringDag(deploymentType) {
		// Create monitoring dag file
		err := fileutil.WriteStringToFile(monitoringDagPath, airflow.MonitoringDag)
		if err != nil {
			return "", err
		}
	}

	// Generate the dags tar
	err := fileutil.Tar(dagsPath, path)
	if err != nil {
		return "", err
	}

	dagsFilePath := filepath.Join(path, "dags.tar")
	dagFile, err := os.Open(dagsFilePath)
	if err != nil {
		return "", err
	}
	defer dagFile.Close()

	versionID, err := azureUploader(dagsUploadURL, dagFile)
	if err != nil {
		return "", err
	}

	// Delete the tar file
	defer func() {
		dagFile.Close()
		if shouldIncludeMonitoringDag(deploymentType) {
			os.Remove(monitoringDagPath)
		}
		err = os.Remove(dagFile.Name())
		if err != nil {
			fmt.Println("\nFailed to delete dags tar file: ", err.Error())
			fmt.Println("\nPlease delete the dags tar file manually from path: " + dagFile.Name())
		}
	}()

	return versionID, nil
}

// Deploy pushes a new docker image
func Deploy(deployInput InputDeploy, corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error { //nolint
	// Get cloud domain
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	domain := c.Domain
	if domain == "" {
		return errors.New("no domain set, re-authenticate")
	}

	var dagsPath string
	if deployInput.DagsPath != "" {
		dagsPath = deployInput.DagsPath
	} else {
		dagsPath = filepath.Join(deployInput.Path, "dags")
	}

	dagFiles := fileutil.GetFilesWithSpecificExtension(dagsPath, ".py")
	fmt.Println("deployment id:")
	fmt.Println(deployInput.RuntimeID)

	deployInfo, err := getDeploymentInfo(deployInput.RuntimeID, deployInput.WsID, deployInput.DeploymentName, deployInput.Prompt, domain, corePlatformClient, coreClient)
	if err != nil {
		return err
	}
	fmt.Println("CI/CD enforcement:")
	fmt.Println(deployInfo.cicdEnforcement)
	if deployInfo.cicdEnforcement {
		if !canCiCdDeploy(c.Token) {
			return errCiCdEnforcementUpdate
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
		if deployInfo.desiredDagTarballVersion == "" {
			return errImageDeployNoPriorDags
		}
	}

	deploymentURL, err := deployment.GetDeploymentURL(deployInfo.deploymentID, deployInfo.workspaceID)
	if err != nil {
		return err
	}
	resp, err := createDeploy(deployInfo.organizationID, deployInfo.deploymentID, deployInput.Description, "", deployInput.Dags, coreClient)
	if err != nil {
		return err
	}
	deployID := resp.JSON200.Id
	if resp.JSON200.DagsUploadUrl != nil {
		dagsUploadURL = *resp.JSON200.DagsUploadUrl
	} else {
		dagsUploadURL = ""
	}
	if resp.JSON200.ImageTag != "" {
		nextTag = resp.JSON200.ImageTag
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
			version, err := buildImage(deployInput.Path, deployInfo.currentVersion, deployInfo.deployImage, deployInput.ImageName, deployInfo.organizationID, deployInput.BuildSecretString, deployInfo.dagDeployEnabled, corePlatformClient)
			if err != nil {
				return err
			}

			err = parseOrPytestDAG(deployInput.Pytest, version, deployInput.EnvFile, deployInfo.deployImage, deployInfo.namespace, deployInput.BuildSecretString)
			if err != nil {
				return err
			}
		}

		if !deployInfo.dagDeployEnabled {
			fmt.Println("deployment id:")
			fmt.Println(deployInfo.deploymentID)
			return fmt.Errorf(enableDagDeployMsg, deployInfo.deploymentID) //nolint
		}

		fmt.Println("Initiating DAG deploy for: " + deployInfo.deploymentID)
		dagTarballVersion, err = deployDags(deployInput.Path, dagsPath, dagsUploadURL, astroplatformcore.DeploymentType(deployInfo.deploymentType))
		if err != nil {
			if strings.Contains(err.Error(), dagDeployDisabled) {
				fmt.Println("deployment id:")
				fmt.Println(deployInfo.deploymentID)
				return fmt.Errorf(enableDagDeployMsg, deployInfo.deploymentID) //nolint
			}

			return err
		}

		// finish deploy
		err = updateDeploy(deployID, deployInfo.deploymentID, deployInfo.organizationID, dagTarballVersion, deployInfo.dagDeployEnabled, coreClient)
		if err != nil {
			return err
		}

		if deployInput.WaitForStatus {
			// Keeping wait timeout low since dag only deploy is faster
			err = deployment.HealthPoll(deployInfo.deploymentID, deployInfo.workspaceID, dagOnlyDeploySleepTime, tickNum, timeoutNum, corePlatformClient)
			if err != nil {
				return err
			}

			fmt.Println("\nSuccessfully uploaded DAGs with version " + ansi.Bold(dagTarballVersion) + " to Astro. Navigate to the Airflow UI to confirm that your deploy was successful." +
				"\n\n Access your Deployment: \n" +
				fmt.Sprintf("\n Deployment View: %s", ansi.Bold(deploymentURL)) +
				fmt.Sprintf("\n Airflow UI: %s", ansi.Bold(deployInfo.webserverURL)))

			return nil
		}

		fmt.Println("\nSuccessfully uploaded DAGs with version " + ansi.Bold(dagTarballVersion) + " to Astro. Navigate to the Airflow UI to confirm that your deploy was successful. The Airflow UI takes about 1 minute to update." +
			"\n\n Access your Deployment: \n" +
			fmt.Sprintf("\n Deployment View: %s", ansi.Bold(deploymentURL)) +
			fmt.Sprintf("\n Airflow UI: %s", ansi.Bold(deployInfo.webserverURL)))
	} else {
		fullpath := filepath.Join(deployInput.Path, ".dockerignore")
		fileExist, _ := fileutil.Exists(fullpath, nil)
		if fileExist {
			err := removeDagsFromDockerIgnore(fullpath)
			if err != nil {
				return errors.New("Found dags entry in .dockerignore file. Remove this entry and try again")
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
		version, err := buildImage(deployInput.Path, deployInfo.currentVersion, deployInfo.deployImage, deployInput.ImageName, deployInfo.organizationID, deployInput.BuildSecretString, deployInfo.dagDeployEnabled, corePlatformClient)
		if err != nil {
			return err
		}

		if len(dagFiles) > 0 {
			err = parseOrPytestDAG(deployInput.Pytest, version, deployInput.EnvFile, deployInfo.deployImage, deployInfo.namespace, deployInput.BuildSecretString)
			if err != nil {
				return err
			}
		} else {
			fmt.Println("No DAGs found. Skipping testing...")
		}

		registry := airflow.GetRegistryURL(domain)
		repository := resp.JSON200.ImageRepository
		// TODO: Resolve the edge case where two people push the same nextTag at the same time
		remoteImage := fmt.Sprintf("%s:%s", repository, nextTag)

		token := c.Token
		// Splitting out the Bearer part from the token
		splittedToken := strings.Split(token, " ")[1]

		imageHandler := airflowImageHandler(deployInfo.deployImage)
		err = imageHandler.Push(registry, registryUsername, splittedToken, remoteImage)
		if err != nil {
			return err
		}

		if deployInfo.dagDeployEnabled && len(dagFiles) > 0 {
			if !deployInput.Image {
				dagTarballVersion, err = deployDags(deployInput.Path, dagsPath, dagsUploadURL, astroplatformcore.DeploymentType(deployInfo.deploymentType))
				if err != nil {
					return err
				}
			} else {
				fmt.Println("Image Deploy only. Skipping deploying DAG...")
			}
		}
		// finish deploy
		if deployInput.Image {
			dagTarballVersion = deployInfo.desiredDagTarballVersion
		}
		err = updateDeploy(deployID, deployInfo.deploymentID, deployInfo.organizationID, dagTarballVersion, deployInfo.dagDeployEnabled, coreClient)
		if err != nil {
			return err
		}

		if deployInput.WaitForStatus {
			err = deployment.HealthPoll(deployInfo.deploymentID, deployInfo.workspaceID, sleepTime, tickNum, timeoutNum, corePlatformClient)
			if err != nil {
				return err
			}
		}

		fmt.Println("Successfully pushed image to Astronomer registry. Navigate to the Astronomer UI for confirmation that your deploy was successful." +
			"\n\n Access your Deployment: \n" +
			fmt.Sprintf("\n Deployment View: %s", ansi.Bold("https://"+deploymentURL)) +
			fmt.Sprintf("\n Airflow UI: %s", ansi.Bold("https://"+deployInfo.webserverURL)))
	}

	return nil
}

func getDeploymentInfo(deploymentID, wsID, deploymentName string, prompt bool, cloudDomain string, corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) (deploymentInfo, error) {
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
		currentDeployment, err := deployment.GetDeployment(wsID, deploymentID, deploymentName, false, corePlatformClient, coreClient)
		if err != nil {
			return deploymentInfo{}, err
		}
		coreDeployment, err := deployment.CoreGetDeployment(currentDeployment.OrganizationId, currentDeployment.Id, corePlatformClient)
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
			currentDeployment.WebServerAirflowApiUrl,
			string(*currentDeployment.Type),
			desiredDagTarballVersion,
			currentDeployment.IsDagDeployEnabled,
			currentDeployment.IsCicdEnforced,
		}, nil
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return deploymentInfo{}, err
	}
	deployInfo, err := getImageName(cloudDomain, deploymentID, c.Organization, corePlatformClient)
	if err != nil {
		return deploymentInfo{}, err
	}
	deployInfo.deploymentID = deploymentID
	return deployInfo, nil
}

func parseOrPytestDAG(pytest, version, envFile, deployImage, namespace, buildSecretString string) error {
	dagParseVersionCheck := versions.GreaterThanOrEqualTo(version, dagParseAllowedVersion)
	if !dagParseVersionCheck {
		fmt.Println("\nruntime image is earlier than 4.1.0, this deploy will skip DAG parse...")
	}

	fmt.Println("testing", deployImage)
	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, "Dockerfile", namespace)
	if err != nil {
		return err
	}

	switch {
	case pytest == parse && dagParseVersionCheck:
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

func getImageName(cloudDomain, deploymentID, organizationID string, corePlatformClient astroplatformcore.CoreClient) (deploymentInfo, error) {
	if cloudDomain == astroDomain {
		fmt.Printf(deploymentHeaderMsg, "Astro")
	} else {
		fmt.Printf(deploymentHeaderMsg, cloudDomain)
	}

	resp, err := corePlatformClient.GetDeploymentWithResponse(httpContext.Background(), organizationID, deploymentID)
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
		f, err := os.OpenFile(fullpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
		if err != nil {
			return err
		}

		defer f.Close()

		if _, err := f.WriteString("\ndags/"); err != nil {
			return err
		}

		dagsIgnoreSet = true
	}
	err = imageHandler.Build("", buildSecretString, types.ImageBuildConfig{Path: path, Output: true, TargetPlatforms: deployImagePlatformSupport})
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

func buildImage(path, currentVersion, deployImage, imageName, organizationID, buildSecretString string, dagDeployEnabled bool, corePlatformClient astroplatformcore.CoreClient) (version string, err error) {
	imageHandler := airflowImageHandler(deployImage)

	if imageName == "" {
		// Build our image
		fmt.Println(composeImageBuildingPromptMsg)

		if dagDeployEnabled {
			err := buildImageWithoutDags(path, buildSecretString, imageHandler)
			if err != nil {
				return "", err
			}
		} else {
			err := imageHandler.Build("", buildSecretString, types.ImageBuildConfig{Path: path, Output: true, TargetPlatforms: deployImagePlatformSupport})
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
		fmt.Printf(warningInvaildImageNameMsg, DockerfileImage)
		fmt.Println("Canceling deploy...")
		os.Exit(1)
	}

	if version == "" {
		version = defaultRuntimeVersion
	}
	resp, err := corePlatformClient.GetDeploymentOptionsWithResponse(httpContext.Background(), organizationID, &astroplatformcore.GetDeploymentOptionsParams{})
	if err != nil {
		return "", err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return "", err
	}

	runtimeReleases := resp.JSON200.RuntimeReleases
	runtimeVersions := []string{}

	for _, runtimeRelease := range runtimeReleases {
		runtimeVersions = append(runtimeVersions, runtimeRelease.Version)
	}

	isValidRuntimeVersions := ValidTags(runtimeVersions, currentVersion)
	isUpgradeValid := IsValidUpgrade(currentVersion, version)

	if !isUpgradeValid {
		fmt.Printf("You pushed a version of Astro Runtime that is incompatible with your Deployment\nModify your Astro Runtime version to %s or higher in your Dockerfile and try again\n", currentVersion)
		fmt.Println("Canceling deploy...")
		os.Exit(1)
	}

	isTagValid := IsValidTag(isValidRuntimeVersions, version)

	CheckVersion(version, os.Stdout)

	if !isTagValid {
		fmt.Println(fmt.Sprintf(warningInvalidImageTagMsg, version, isValidRuntimeVersions))
	}

	return version, nil
}

// update deploy
func updateDeploy(deployID, deploymentID, organizationID, dagTarballVersion string, dagDeploy bool, coreClient astrocore.CoreClient) error {
	UpdateDeployRequest := astrocore.UpdateDeployRequest{}
	if dagDeploy {
		UpdateDeployRequest.DagTarballVersion = &dagTarballVersion
	}
	resp, err := coreClient.UpdateDeployWithResponse(httpContext.Background(), organizationID, deploymentID, deployID, UpdateDeployRequest)
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

// create deploy
func createDeploy(organizationID, deploymentID, description, tag string, dagDeploy bool, coreClient astrocore.CoreClient) (astrocore.CreateDeployResponse, error) {
	createDeployRequest := astrocore.CreateDeployRequest{
		Description: &description,
	}
	if dagDeploy {
		createDeployRequest.Type = astrocore.CreateDeployRequestTypeDAG
	} else {
		createDeployRequest.Type = astrocore.CreateDeployRequestTypeIMAGE
	}
	if tag != "" {
		createDeployRequest.ImageTag = &tag
	}

	resp, err := coreClient.CreateDeployWithResponse(httpContext.Background(), organizationID, deploymentID, createDeployRequest)
	if err != nil {
		return astrocore.CreateDeployResponse{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrocore.CreateDeployResponse{}, err
	}
	return *resp, err
}

func IsValidUpgrade(currentVersion, tag string) bool {
	// To allow old deployments which do not have runtimeVersion tag
	if currentVersion == "" {
		return true
	}

	tagVersion := util.Coerce(tag)
	currentTagVersion := util.Coerce(currentVersion)

	if i := tagVersion.Compare(currentTagVersion); i >= 0 {
		return true
	}

	return false
}

func IsValidTag(runtimeVersions []string, tag string) bool {
	tagVersion := util.Coerce(tag)
	for _, runtimeVersion := range runtimeVersions {
		supportedVersion := util.Coerce(runtimeVersion)
		// i = 1 means version greater than
		if i := supportedVersion.Compare(tagVersion); i == 0 {
			return true
		}
	}
	return false
}

func ValidTags(runtimeVersions []string, currentVersion string) []string {
	// For old deployments which do not have runtimeVersion tag
	if currentVersion == "" {
		return runtimeVersions
	}

	currentTagVersion := util.Coerce(currentVersion)
	validVersions := []string{}

	for _, runtimeVersion := range runtimeVersions {
		supportedVersion := util.Coerce(runtimeVersion)
		// i = 1 means version greater than
		if i := supportedVersion.Compare(currentTagVersion); i >= 0 {
			validVersions = append(validVersions, runtimeVersion)
		}
	}

	return validVersions
}

func CheckVersion(version string, out io.Writer) {
	httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
	latestRuntimeVersion, _ := airflowversions.GetDefaultImageTag(httpClient, "")
	switch {
	case versions.LessThan(version, latestRuntimeVersion):
		// if current runtime version is not greater than or equal to the latest runtime verion let the user know
		fmt.Fprintf(out, "WARNING! You are currently running Astro Runtime Version %s\nConsider upgrading to the latest version, Astro Runtime %s\n", version, latestRuntimeVersion)
	case versions.GreaterThan(version, latestRuntimeVersion):
		i, _ := input.Confirm("WARNING! The Astro Runtime image in your Dockerfile is classified as \"Beta\" and may not be fit for pipelines in production. Are you sure you want to continue?\n")

		if !i {
			fmt.Fprintf(out, "Canceling deploy...")
			os.Exit(1)
		}
	default:
		fmt.Fprintf(out, "Runtime Version: %s\n", version)
	}
}
