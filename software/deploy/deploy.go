package deploy

import (
	"errors"
	"fmt"
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
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	// this is used to monkey patch the function in order to write unit test cases
	imageHandlerInit = airflow.ImageHandlerInit

	dockerfile = "Dockerfile"

	deployImagePlatformSupport = []string{"linux/amd64"}
)

var (
	errNoWorkspaceID                        = errors.New("no workspace id provided")
	errNoDomainSet                          = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentID                  = errors.New("please specify a valid deployment ID")
	errDeploymentNotFound                   = errors.New("no airflow deployments found")
	errInvalidDeploymentSelected            = errors.New("invalid deployment selection\n") //nolint
	errDagOnlyDeployDisabledInConfig        = errors.New("to perform this operation, set both deployments.dagOnlyDeployment and deployments.configureDagDeployment to true in the Astronomer Platform")
	errDagOnlyDeployNotEnabledForDeployment = errors.New("to perform this operation, first set the deployment type to 'dag_only' via the UI or the API")
)

const (
	houstonDeploymentHeader       = "Authenticated to %s \n\n"
	houstonSelectDeploymentPrompt = "Select which airflow deployment you want to deploy to:"
	houstonDeploymentPrompt       = "Deploying: %s\n"

	imageBuildingPrompt = "Building image..."

	warningInvalidImageName                   = "WARNING! The image in your Dockerfile is pulling from '%s', which is not supported. We strongly recommend that you use Astronomer Certified or Runtime images that pull from 'astronomerinc/ap-airflow', 'quay.io/astronomer/ap-airflow' or 'quay.io/astronomer/astro-runtime'. If you're running a custom image, you can override this. Are you sure you want to continue?\n"
	warningInvalidNameTag                     = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nPlease use one of the following tags: %s.\nAre you sure you want to continue?"
	warningInvalidNameTagEmptyRecommendations = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nAre you sure you want to continue?"

	registryDomainPrefix = "registry."
	runtimeImageLabel    = "io.astronomer.docker.runtime.version"
	airflowImageLabel    = "io.astronomer.docker.airflow.version"
)

var tab = printutil.Table{
	Padding:        []int{5, 30, 30, 50},
	DynamicPadding: true,
	Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
}

func Airflow(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) error {
	if wsID == "" {
		return errNoWorkspaceID
	}

	// Validate workspace
	currentWorkspace, err := houston.Call(houstonClient.GetWorkspace)(wsID)
	if err != nil {
		return err
	}

	// Get Deployments from workspace ID
	request := houston.ListDeploymentsRequest{
		WorkspaceID: currentWorkspace.ID,
	}
	deployments, err := houston.Call(houstonClient.ListDeployments)(request)
	if err != nil {
		return err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	cloudDomain := c.Domain
	if cloudDomain == "" {
		return errNoDomainSet
	}

	// Use config deployment if provided
	if deploymentID == "" {
		deploymentID = config.CFG.ProjectDeployment.GetProjectString()
	}

	if deploymentID != "" && !deploymentExists(deploymentID, deployments) {
		return errInvalidDeploymentID
	}

	// Prompt user for deployment if no deployment passed in
	if deploymentID == "" || prompt {
		if len(deployments) == 0 {
			return errDeploymentNotFound
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
			return errInvalidDeploymentSelected
		}
		deploymentID = selected.ID
	}

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
		return fmt.Errorf("failed to get deployment info: %w", err)
	}

	fmt.Printf(houstonDeploymentPrompt, releaseName)

	// Build the image to deploy
	err = buildPushDockerImage(houstonClient, &c, deploymentInfo, releaseName, path, nextTag, cloudDomain, byoRegistryDomain, ignoreCacheDeploy, byoRegistryEnabled)
	if err != nil {
		return err
	}

	deploymentLink := getAirflowUILink(deploymentID, deploymentInfo.Urls)
	fmt.Printf("Successfully pushed Docker image to Astronomer registry, it can take a few minutes to update the deployment with the new image. Navigate to the Astronomer UI to confirm the state of your deployment (%s).\n", deploymentLink)

	return nil
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

func buildPushDockerImage(houstonClient houston.ClientInterface, c *config.Context, deploymentInfo *houston.Deployment, name, path, nextTag, cloudDomain, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled bool) error {
	// Build our image
	fmt.Println(imageBuildingPrompt)

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(path, dockerfile), err)
	}

	image, tag := docker.GetImageTagFromParsedFile(cmds)
	if config.CFG.ShowWarnings.GetBool() && !validAirflowImageRepo(image) && !validRuntimeImageRepo(image) {
		i, _ := input.Confirm(fmt.Sprintf(warningInvalidImageName, image))
		if !i {
			fmt.Println("Canceling deploy...")
			os.Exit(1)
		}
	}
	// Get valid image tags for platform using Deployment Info request
	deploymentConfig, err := houston.Call(houstonClient.GetDeploymentConfig)(nil)
	if err != nil {
		return err
	}
	// ignoring the error as user can be connected to platform where runtime is not enabled
	runtimeReleases, _ := houston.Call(houstonClient.GetRuntimeReleases)("")
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
	imageName := airflow.ImageName(name, "latest")

	imageHandler := imageHandlerInit(imageName)

	buildConfig := types.ImageBuildConfig{
		Path:            config.WorkingPath,
		NoCache:         ignoreCacheDeploy,
		TargetPlatforms: deployImagePlatformSupport,
		Output:          true,
	}
	err = imageHandler.Build("", "", buildConfig)
	if err != nil {
		return err
	}

	var registry, remoteImage, token string
	if byoRegistryEnabled {
		registry = byoRegistryDomain
		remoteImage = fmt.Sprintf("%s:%s", registry, fmt.Sprintf("%s-%s", name, nextTag))
	} else {
		registry = registryDomainPrefix + cloudDomain
		remoteImage = fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
		token = c.Token
	}

	err = imageHandler.Push(registry, "", token, remoteImage)
	if err != nil {
		return err
	}

	if byoRegistryEnabled {
		runtimeVersion, _ := imageHandler.GetLabel("", runtimeImageLabel)
		airflowVersion, _ := imageHandler.GetLabel("", airflowImageLabel)
		req := houston.UpdateDeploymentImageRequest{ReleaseName: name, Image: remoteImage, AirflowVersion: airflowVersion, RuntimeVersion: runtimeVersion}
		_, err := houston.Call(houstonClient.UpdateDeploymentImage)(req)
		return err
	}

	return nil
}

func validAirflowImageRepo(image string) bool {
	validDockerfileBaseImages := map[string]bool{
		"quay.io/astronomer/ap-airflow": true,
		"astronomerinc/ap-airflow":      true,
	}
	result, ok := validDockerfileBaseImages[image]
	if !ok {
		return false
	}
	return result
}

func validRuntimeImageRepo(image string) bool {
	validDockerfileBaseImages := map[string]bool{
		"quay.io/astronomer/astro-runtime": true,
	}
	result, ok := validDockerfileBaseImages[image]
	if !ok {
		return false
	}
	return result
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
func isDagOnlyDeploymentEnabled(appConfig *houston.AppConfig) bool {
	return appConfig != nil && appConfig.Flags.DagOnlyDeployment
}

func isDagOnlyDeploymentEnabledForDeployment(deploymentInfo *houston.Deployment) bool {
	return deploymentInfo != nil && deploymentInfo.DagDeployment.Type == houston.DagOnlyDeploymentType
}

func DeployDagsOnly(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, deploymentId string) error {

	// Throw error if the feature is disabled at Houston level
	if !isDagOnlyDeploymentEnabled(appConfig) {
		return errDagOnlyDeployDisabledInConfig
	}

	// Throw error if the feature is disabled at Deployment level
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentId)
	if err != nil {
		return fmt.Errorf("failed to get deployment info: %w", err)
	}
	if !isDagOnlyDeploymentEnabledForDeployment(deploymentInfo) {
		return errDagOnlyDeployNotEnabledForDeployment
	}

	return nil
}
