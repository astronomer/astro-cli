package deploy

import (
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/software/auth"
	"github.com/docker/docker/api/types/versions"
)

var (
	// this is used to monkey patch the function in order to write unit test cases
	imageHandlerInit = airflow.ImageHandlerInit

	dockerfile = "Dockerfile"

	deployImagePlatformSupport = []string{"linux/amd64"}

	gzipFile = fileutil.GzipFile

	getDeploymentIDForCurrentCommandVar = getDeploymentIDForCurrentCommand
)

var (
	ErrNoWorkspaceID                         = errors.New("no workspace id provided")
	errNoDomainSet                           = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentID                   = errors.New("please specify a valid deployment ID")
	errDeploymentNotFound                    = errors.New("no airflow deployments found")
	errInvalidDeploymentSelected             = errors.New("invalid deployment selection\n") //nolint
	ErrDagOnlyDeployDisabledInConfig         = errors.New("to perform this operation, set both deployments.dagOnlyDeployment and deployments.configureDagDeployment to true in your Astronomer cluster")
	ErrDagOnlyDeployNotEnabledForDeployment  = errors.New("to perform this operation, first set the Deployment type to 'dag_deploy' via the UI or the API or the CLI")
	ErrEmptyDagFolderUserCancelledOperation  = errors.New("no DAGs found in the dags folder. User canceled the operation")
	ErrBYORegistryDomainNotSet               = errors.New("Custom registry host is not set in config. It can be set at astronomer.houston.config.deployments.registry.protectedCustomRegistry.updateRegistry.host") //nolint
	ErrDeploymentTypeIncorrectForImageOnly   = errors.New("--image only works for Dag-only, Git-sync-based and NFS-based deployments")
	WarningInvalidImageNameMsg               = "WARNING! The image in your Dockerfile '%s' is not based on Astro Runtime and is not supported. Change your Dockerfile with an image that pulls from 'quay.io/astronomer/astro-runtime' to proceed.\n"
	ErrNoRuntimeLabelOnCustomImage           = errors.New("the image should have label io.astronomer.docker.runtime.version")
	ErrRuntimeVersionNotPassedForRemoteImage = errors.New("if --image-name and --remote is passed, it's mandatory to pass --runtime-version")
)

const (
	houstonDeploymentHeader       = "Authenticated to %s \n\n"
	houstonSelectDeploymentPrompt = "Select which airflow deployment you want to deploy to:"
	houstonDeploymentPrompt       = "Deploying: %s\n"

	imageBuildingPrompt = "Building image..."

	warningInvalidImageName                   = "WARNING! The image in your Dockerfile is pulling from '%s', which is not supported. We strongly recommend that you use Astronomer Certified or Runtime images that pull from 'astronomerinc/ap-airflow', 'quay.io/astronomer/ap-airflow' or 'quay.io/astronomer/astro-runtime'. If you're running a custom image, you can override this. Are you sure you want to continue?\n"
	warningInvalidNameTag                     = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nPlease use one of the following tags: %s.\nAre you sure you want to continue?"
	warningInvalidNameTagEmptyRecommendations = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nAre you sure you want to continue?"

	registryDomainPrefix              = "registry."
	runtimeImageLabel                 = "io.astronomer.docker.runtime.version"
	airflowImageLabel                 = "io.astronomer.docker.airflow.version"
	composeSkipImageBuildingPromptMsg = "Skipping building image since --image-name flag is used..."
)

var tab = printutil.Table{
	Padding:        []int{5, 30, 30, 50},
	DynamicPadding: true,
	Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
}

func Airflow(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
	deploymentID, deployments, err := getDeploymentIDForCurrentCommand(houstonClient, wsID, deploymentID, prompt)
	if err != nil {
		return deploymentID, err
	}

	c, _ := config.GetCurrentContext()
	cloudDomain := c.Domain
	nextTag := ""
	releaseName := ""
	for i := range deployments {
		deployment := deployments[i]
		if deployment.ID == deploymentID {
			nextTag = deployment.DeploymentInfo.NextCli
			releaseName = deployment.ReleaseName
		}
	}

	if byoRegistryEnabled {
		nextTag = "deploy-" + time.Now().UTC().Format("2006-01-02T15-04") // updating nextTag logic for private registry, since houston won't maintain next tag in case of BYO registry
	}

	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return deploymentID, fmt.Errorf("failed to get deployment info: %w", err)
	}

	appConfig, err := houston.Call(houstonClient.GetAppConfig)(deploymentInfo.ClusterID)
	if err != nil {
		return deploymentID, fmt.Errorf("failed to get app config: %w", err)
	}

	if appConfig != nil && appConfig.Flags.BYORegistryEnabled {
		byoRegistryEnabled = true
		byoRegistryDomain = appConfig.BYORegistryDomain
		if byoRegistryDomain == "" {
			return deploymentID, ErrBYORegistryDomainNotSet
		}
	}

	// isImageOnlyDeploy is not valid for image-based deployments since image-based deployments inherently mean that the image itself contains dags.
	// If we deploy only the image, the deployment will not have any dags for image-based deployments.
	// Even on astro, image-based deployments are not allowed to be deployed with --image flag.
	if isImageOnlyDeploy && deploymentInfo.DagDeployment.Type == houston.ImageDeploymentType {
		return "", ErrDeploymentTypeIncorrectForImageOnly
	}
	// We don't need to exclude the dags from the image because the dags present in the image are not respected anyways for non-image based deployments

	fmt.Printf(houstonDeploymentPrompt, releaseName)

	// Build the image to deploy
	err = buildPushDockerImage(houstonClient, &c, deploymentInfo, releaseName, path, nextTag, cloudDomain, byoRegistryDomain, ignoreCacheDeploy, byoRegistryEnabled, description, imageName)
	if err != nil {
		return deploymentID, err
	}

	deploymentLink := getAirflowUILink(deploymentID, deploymentInfo.Urls)
	fmt.Printf("Successfully pushed Docker image to Astronomer registry, it can take a few minutes to update the deployment with the new image. Navigate to the Astronomer UI to confirm the state of your deployment (%s).\n", deploymentLink)

	return deploymentID, nil
}

// Find deployment ID in deployments slice
func deploymentExists(deploymentID string, deployments []houston.Deployment) bool {
	for idx := range deployments {
		deployment := deployments[idx]
		if deployment.ID == deploymentID {
			return true
		}
	}
	return false
}

func validateRuntimeVersion(houstonClient houston.ClientInterface, tag string, deploymentInfo *houston.Deployment) error {
	// Get valid image tags for platform using Deployment Info request
	deploymentConfig, err := houston.Call(houstonClient.GetDeploymentConfig)(nil)
	if err != nil {
		return err
	}
	vars := make(map[string]interface{})
	vars["clusterId"] = deploymentInfo.ClusterID
	// ignoring the error as user can be connected to platform where runtime is not enabled
	runtimeReleases, _ := houston.Call(houstonClient.GetRuntimeReleases)(vars)
	var validTags string
	if config.CFG.ShowWarnings.GetBool() && deploymentInfo.DesiredAirflowVersion != "" && !deploymentConfig.IsValidTag(tag) {
		validTags = strings.Join(deploymentConfig.GetValidTags(tag), ", ")
	}
	if config.CFG.ShowWarnings.GetBool() && deploymentInfo.DesiredRuntimeVersion != "" && !runtimeReleases.IsValidVersion(tag) {
		validTags = strings.Join(runtimeReleases.GreaterVersions(tag), ", ")
	}
	if validTags != "" {
		validTags := strings.Join(deploymentConfig.GetValidTags(tag), ", ")

		msg := fmt.Sprintf(warningInvalidNameTag, tag, validTags)
		if validTags == "" {
			msg = fmt.Sprintf(warningInvalidNameTagEmptyRecommendations, tag)
		}

		i, _ := input.Confirm(msg)
		if !i {
			fmt.Println("Canceling deploy...")
			os.Exit(1)
		}
	}
	return nil
}

func UpdateDeploymentImage(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string) (string, error) {
	if runtimeVersion == "" {
		return "", ErrRuntimeVersionNotPassedForRemoteImage
	}
	deploymentID, _, err := getDeploymentIDForCurrentCommandVar(houstonClient, wsID, deploymentID, deploymentID == "")
	if err != nil {
		return "", err
	}
	if deploymentID == "" {
		return "", errInvalidDeploymentID
	}
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return "", fmt.Errorf("failed to get deployment info: %w", err)
	}
	fmt.Println("Skipping building the image since --image-name flag is used...")
	req := houston.UpdateDeploymentImageRequest{ReleaseName: deploymentInfo.ReleaseName, Image: imageName, AirflowVersion: "", RuntimeVersion: runtimeVersion}
	_, err = houston.Call(houstonClient.UpdateDeploymentImage)(req)
	fmt.Println("Image successfully updated")
	return deploymentID, err
}

func pushDockerImage(byoRegistryEnabled bool, deploymentInfo *houston.Deployment, byoRegistryDomain, name, nextTag, cloudDomain string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, c *config.Context, customImageName string) error {
	var registry, remoteImage, token string
	if byoRegistryEnabled {
		registry = byoRegistryDomain
		remoteImage = fmt.Sprintf("%s:%s", registry, fmt.Sprintf("%s-%s", name, nextTag))
	} else {
		platformVersion, _ := houstonClient.GetPlatformVersion(nil)
		if versions.GreaterThanOrEqualTo(platformVersion, "1.0.0") {
			registry, err := getDeploymentRegistryURL(deploymentInfo.Urls)
			if err != nil {
				return err
			}
			// Switch to per deployment registry login
			err = auth.RegistryAuth(houstonClient, os.Stdout)
			if err != nil {
				logger.Debugf("There was an error logging into registry: %s", err.Error())
				return err
			}
			remoteImage = fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
			token = c.Token
		} else {
			registry = registryDomainPrefix + cloudDomain
			remoteImage = fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
			token = c.Token
		}
	}
	if customImageName != "" {
		if tagFromImageName := getGetTagFromImageName(customImageName); tagFromImageName != "" {
			remoteImage = fmt.Sprintf("%s:%s", registry, tagFromImageName)
		}
	}
	useShaAsTag := config.CFG.ShaAsTag.GetBool()
	sha, err := imageHandler.Push(remoteImage, "", token, useShaAsTag)
	if err != nil {
		return err
	}
	if byoRegistryEnabled {
		if useShaAsTag {
			remoteImage = fmt.Sprintf("%s@%s", registry, sha)
		}
		runtimeVersion, _ := imageHandler.GetLabel("", runtimeImageLabel)
		airflowVersion, _ := imageHandler.GetLabel("", airflowImageLabel)
		req := houston.UpdateDeploymentImageRequest{ReleaseName: name, Image: remoteImage, AirflowVersion: airflowVersion, RuntimeVersion: runtimeVersion}
		_, err = houston.Call(houstonClient.UpdateDeploymentImage)(req)
		return err
	}
	return nil
}

func buildDockerImageForCustomImage(imageHandler airflow.ImageHandler, customImageName string, deploymentInfo *houston.Deployment, houstonClient houston.ClientInterface) error {
	fmt.Println(composeSkipImageBuildingPromptMsg)
	err := imageHandler.TagLocalImage(customImageName)
	if err != nil {
		return err
	}
	runtimeLabel, err := imageHandler.GetLabel("", airflow.RuntimeImageLabel)
	if err != nil {
		fmt.Println("unable get runtime version from image")
		return err
	}
	if runtimeLabel == "" {
		return ErrNoRuntimeLabelOnCustomImage
	}
	err = validateRuntimeVersion(houstonClient, runtimeLabel, deploymentInfo)
	return err
}

func buildDockerImageFromWorkingDir(path string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, deploymentInfo *houston.Deployment, ignoreCacheDeploy bool, description string) error {
	// all these checks inside Dockerfile should happen only when no image-name is provided
	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(path, dockerfile), err)
	}

	_, tag := docker.GetImageTagFromParsedFile(cmds)

	// Get valid image tags for platform using Deployment Info request
	err = validateRuntimeVersion(houstonClient, tag, deploymentInfo)
	if err != nil {
		return err
	}
	// Build our image
	fmt.Println(imageBuildingPrompt)
	deployLabels := []string{"io.astronomer.skip.revision=true"}
	if description != "" {
		deployLabels = append(deployLabels, "io.astronomer.deploy.revision.description="+description)
	}
	buildConfig := types.ImageBuildConfig{
		Path:            config.WorkingPath,
		NoCache:         ignoreCacheDeploy,
		TargetPlatforms: deployImagePlatformSupport,
		Labels:          deployLabels,
	}

	err = imageHandler.Build("", "", buildConfig)
	return err
}

func buildDockerImage(ignoreCacheDeploy bool, deploymentInfo *houston.Deployment, customImageName, path string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, description string) error {
	if customImageName == "" {
		return buildDockerImageFromWorkingDir(path, imageHandler, houstonClient, deploymentInfo, ignoreCacheDeploy, description)
	}
	return buildDockerImageForCustomImage(imageHandler, customImageName, deploymentInfo, houstonClient)
}

func getGetTagFromImageName(imageName string) string {
	parts := strings.Split(imageName, ":")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func buildPushDockerImage(houstonClient houston.ClientInterface, c *config.Context, deploymentInfo *houston.Deployment, name, path, nextTag, cloudDomain, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled bool, description, customImageName string) error {
	imageName := airflow.ImageName(name, "latest")
	imageHandler := imageHandlerInit(imageName)
	err := buildDockerImage(ignoreCacheDeploy, deploymentInfo, customImageName, path, imageHandler, houstonClient, description)
	if err != nil {
		return err
	}
	return pushDockerImage(byoRegistryEnabled, deploymentInfo, byoRegistryDomain, name, nextTag, cloudDomain, imageHandler, houstonClient, c, customImageName)
}

func getAirflowUILink(deploymentID string, deploymentURLs []houston.DeploymentURL) string {
	if deploymentID == "" {
		return ""
	}

	for _, url := range deploymentURLs {
		if url.Type == houston.AirflowURLType {
			return url.URL
		}
	}
	return ""
}

func getDeploymentRegistryURL(deploymentURLs []houston.DeploymentURL) (string, error) {
	for _, url := range deploymentURLs {
		if url.Type == houston.RegistryURLType {
			return url.URL, nil
		}
	}
	return "", errors.New("no valid registry url found failed to push")
}

func getDeploymentIDForCurrentCommand(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
	if wsID == "" {
		return deploymentID, []houston.Deployment{}, ErrNoWorkspaceID
	}

	// Validate workspace
	currentWorkspace, err := houston.Call(houstonClient.GetWorkspace)(wsID)
	if err != nil {
		return deploymentID, []houston.Deployment{}, err
	}

	// Get Deployments from workspace ID
	request := houston.ListDeploymentsRequest{
		WorkspaceID: currentWorkspace.ID,
	}
	deployments, err := houston.Call(houstonClient.ListDeployments)(request)
	if err != nil {
		return deploymentID, deployments, err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return deploymentID, deployments, err
	}

	cloudDomain := c.Domain
	if cloudDomain == "" {
		return deploymentID, deployments, errNoDomainSet
	}

	// Use config deployment if provided
	if deploymentID == "" {
		deploymentID = config.CFG.ProjectDeployment.GetProjectString()
	}

	if deploymentID != "" && !deploymentExists(deploymentID, deployments) {
		return deploymentID, deployments, errInvalidDeploymentID
	}

	// Prompt user for deployment if no deployment passed in
	if deploymentID == "" || prompt {
		if len(deployments) == 0 {
			return deploymentID, deployments, errDeploymentNotFound
		}

		fmt.Printf(houstonDeploymentHeader, cloudDomain)
		fmt.Println(houstonSelectDeploymentPrompt)

		deployMap := map[string]houston.Deployment{}
		for i := range deployments {
			deployment := deployments[i]
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), deployment.Label, deployment.ReleaseName, currentWorkspace.Label, deployment.ID}, false)

			deployMap[strconv.Itoa(index)] = deployment
		}

		tab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := deployMap[choice]
		if !ok {
			return deploymentID, deployments, errInvalidDeploymentSelected
		}
		deploymentID = selected.ID
	}
	return deploymentID, deployments, nil
}

func isDagOnlyDeploymentEnabled(appConfig *houston.AppConfig) bool {
	return appConfig != nil && appConfig.Flags.DagOnlyDeployment
}

func isDagOnlyDeploymentEnabledForDeployment(deploymentInfo *houston.Deployment) bool {
	return deploymentInfo != nil && deploymentInfo.DagDeployment.Type == houston.DagOnlyDeploymentType
}

func validateIfDagDeployURLCanBeConstructed(deploymentInfo *houston.Deployment) error {
	_, err := config.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("could not get current context! Error: %w", err)
	}
	if deploymentInfo == nil || deploymentInfo.ReleaseName == "" {
		return errInvalidDeploymentID
	}
	return nil
}

func getDagDeployURL(deploymentInfo *houston.Deployment) string {
	// Checks if dagserver URL exists and returns the URL
	for _, url := range deploymentInfo.Urls {
		if url.Type == houston.DagServerURLType {
			logger.Infof("Using dag deploy URL from dagserver: %s", url.URL)
			return url.URL
		}
	}

	// If no dagserver URL is found, we look for airflow URL to detect upload url
	for _, url := range deploymentInfo.Urls {
		if url.Type != houston.AirflowURLType {
			continue
		}

		parsedAirflowURL, err := neturl.Parse(url.URL)
		if err != nil {
			logger.Infof("Error parsing airflow URL: %v", err)
			break
		}

		// Use URL scheme and host from the airflow URL
		dagUploadURL := fmt.Sprintf("https://%s/%s/dags/upload", parsedAirflowURL.Host, deploymentInfo.ReleaseName)
		logger.Infof("Generated Dag Upload URL from airflow base URL: %s", dagUploadURL)
		return dagUploadURL
	}
	return ""
}

func DagsOnlyDeploy(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
	deploymentID, _, err := getDeploymentIDForCurrentCommandVar(houstonClient, wsID, deploymentID, deploymentID == "")
	if err != nil {
		return err
	}

	if deploymentID == "" {
		return errInvalidDeploymentID
	}

	// Throw error if the feature is disabled at Deployment level
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment info: %w", err)
	}
	appConfig, err := houston.Call(houstonClient.GetAppConfig)(deploymentInfo.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get app config: %w", err)
	}
	// Throw error if the feature is disabled at Houston level
	if !isDagOnlyDeploymentEnabled(appConfig) {
		return ErrDagOnlyDeployDisabledInConfig
	}
	if !isDagOnlyDeploymentEnabledForDeployment(deploymentInfo) {
		return ErrDagOnlyDeployNotEnabledForDeployment
	}

	uploadURL := ""
	if dagDeployURL == nil {
		// Throw error if the upload URL can't be constructed
		err = validateIfDagDeployURLCanBeConstructed(deploymentInfo)
		if err != nil {
			return err
		}
		uploadURL = getDagDeployURL(deploymentInfo)
	} else {
		uploadURL = *dagDeployURL
	}

	dagsPath := filepath.Join(dagsParentPath, "dags")
	dagsTarPath := filepath.Join(dagsParentPath, "dags.tar")
	dagsTarGzPath := dagsTarPath + ".gz"
	dagFiles := fileutil.GetFilesWithSpecificExtension(dagsPath, ".py")

	// Alert the user if dags folder is empty
	if len(dagFiles) == 0 && config.CFG.ShowWarnings.GetBool() {
		i, _ := input.Confirm("Warning: No DAGs found. This will delete any existing DAGs. Are you sure you want to deploy?")
		if !i {
			return ErrEmptyDagFolderUserCancelledOperation
		}
	}

	// Generate the dags tar
	err = fileutil.Tar(dagsPath, dagsTarPath, true, nil)
	if err != nil {
		return err
	}
	if cleanUpFiles {
		defer os.Remove(dagsTarPath)
	}

	// Gzip the tar
	err = gzipFile(dagsTarPath, dagsTarGzPath)
	if err != nil {
		return err
	}
	if cleanUpFiles {
		defer os.Remove(dagsTarGzPath)
	}

	c, _ := config.GetCurrentContext()

	headers := map[string]string{
		"authorization": c.Token,
	}

	uploadFileArgs := fileutil.UploadFileArguments{
		FilePath:            dagsTarGzPath,
		TargetURL:           uploadURL,
		FormFileFieldName:   "file",
		Headers:             headers,
		Description:         description,
		MaxTries:            8,
		InitialDelayInMS:    1 * 1000,
		BackoffFactor:       2,
		RetryDisplayMessage: "please wait, attempting to upload the dags",
	}
	return fileutil.UploadFile(&uploadFileArgs)
}
