package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
)

var (
	errNoWorkspaceID             = errors.New("no workspace id provided")
	errNoDomainSet               = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentName     = errors.New("please specify a valid deployment name")
	errDeploymentNotFound        = errors.New("no airflow deployments found")
	errInvalidDeploymentSelected = errors.New("invalid deployment selection\n") //nolint
)

const (
	houstonDeploymentHeader       = "Authenticated to %s \n\n"
	houstonSelectDeploymentPrompt = "Select which airflow deployment you want to deploy to:"
	houstonDeploymentPrompt       = "Deploying: %s\n"

	imageBuildingPrompt = "Building image..."

	warningInvalidImageName                   = "WARNING! The image in your Dockerfile is pulling from '%s', which is not supported. We strongly recommend that you use Astronomer Certified images that pull from 'astronomerinc/ap-airflow' or 'quay.io/astronomer/ap-airflow'. If you're running a custom image, you can override this. Are you sure you want to continue?\n"
	warningInvalidNameTag                     = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nPlease use one of the following tags: %s.\nAre you sure you want to continue?"
	warningInvalidNameTagEmptyRecommendations = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nAre you sure you want to continue?"
)

var tab = printutil.Table{
	Padding:        []int{5, 30, 30, 50},
	DynamicPadding: true,
	Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
}

func Airflow(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool) error {
	if wsID == "" {
		return errNoWorkspaceID
	}

	// Validate workspace
	currentWorkspace, err := houstonClient.GetWorkspace(wsID)
	if err != nil {
		return err
	}

	// Get Deployments from workspace ID
	request := houston.ListDeploymentsRequest{
		WorkspaceID: currentWorkspace.ID,
	}
	deployments, err := houstonClient.ListDeployments(request)
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
		return errInvalidDeploymentName
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

	fmt.Printf(houstonDeploymentPrompt, releaseName)

	// Build the image to deploy
	err = buildPushDockerImage(houstonClient, &c, releaseName, path, nextTag, cloudDomain, ignoreCacheDeploy)
	if err != nil {
		return err
	}

	deploymentLink := getAirflowUILink(houstonClient, deploymentID)
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

func buildPushDockerImage(houstonClient houston.ClientInterface, c *config.Context, name, path, nextTag, cloudDomain string, ignoreCacheDeploy bool) error {
	// Build our image
	fmt.Println(imageBuildingPrompt)

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(path, dockerfile), err)
	}

	image, tag := docker.GetImageTagFromParsedFile(cmds)
	if config.CFG.ShowWarnings.GetBool() && !validImageRepo(image) {
		i, _ := input.Confirm(fmt.Sprintf(warningInvalidImageName, image))
		if !i {
			fmt.Println("Canceling deploy...")
			os.Exit(1)
		}
	}
	// Get valid image tags for platform using Deployment Info request
	deploymentConfig, err := houstonClient.GetDeploymentConfig()
	if err != nil {
		return err
	}
	if config.CFG.ShowWarnings.GetBool() && !deploymentConfig.IsValidTag(tag) {
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
		Path:    config.WorkingPath,
		NoCache: ignoreCacheDeploy,
	}
	err = imageHandler.Build(buildConfig)
	if err != nil {
		return err
	}
	registry := "registry." + cloudDomain
	remoteImage := fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
	return imageHandler.Push(registry, "", c.Token, remoteImage)
}

func validImageRepo(image string) bool {
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

func getAirflowUILink(houstonClient houston.ClientInterface, deploymentID string) string {
	if deploymentID == "" {
		return ""
	}

	resp, err := houstonClient.GetDeployment(deploymentID)
	if err != nil || resp == nil {
		return ""
	}
	for _, url := range resp.Urls {
		if url.Type == houston.AirflowURLType {
			return url.URL
		}
	}
	return ""
}
