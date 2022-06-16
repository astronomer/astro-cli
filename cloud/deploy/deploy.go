package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
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
	runtimeImageLabel      = "io.astronomer.docker.runtime.version"
	defaultRuntimeVersion  = "4.2.5"
	dagParseAllowedVersion = "4.1.0"

	composeImageBuildingPromptMsg = "Building image..."
	deploymentHeaderMsg           = "Authenticated to %s \n\n"

	warningInvaildImageNameMsg = "WARNING! The image in your Dockerfile '%s' is not based on Astro Runtime and is not supported. Change your Dockerfile with an image that pulls from 'quay.io/astronomer/astro-runtime' to proceed.\n"
	warningInvalidImageTagMsg  = "WARNING! You are about to push an image using the '%s' runtime tag. This is not supported.\nPlease use one of the following supported tags: %s"
)

var (
	splitNum   = 2
	pytestFile string
	dockerfile = "Dockerfile"

	// Monkey patched to write unit tests
	airflowImageHandler  = airflow.ImageHandlerInit
	containerHandlerInit = airflow.ContainerHandlerInit
)

var errDagsParseFailed = errors.New("your local DAGs did not parse. Fix the listed errors or use `astro deploy [deployment-id] -f` to force deploy") //nolint:revive

type deploymentInfo struct {
	deploymentID   string
	namespace      string
	deployImage    string
	currentVersion string
	organizationID string
	webserverURL   string
}

// Deploy pushes a new docker image
func Deploy(path, deploymentID, wsID, pytest, envFile string, prompt bool, client astro.Client) error {
	// Get cloud domain
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	cloudDomain := c.Domain
	if cloudDomain == "" {
		return errors.New("no domain set, re-authenticate")
	}

	deployInfo, err := getDeploymentInfo(deploymentID, wsID, prompt, cloudDomain, client)
	if err != nil {
		return err
	}

	// Build our image
	version, err := buildImage(&c, path, deployInfo.currentVersion, deployInfo.deployImage, client)
	if err != nil {
		return err
	}

	err = parseDAG(pytest, version, envFile, deployInfo.deployImage, deployInfo.namespace)
	if err != nil {
		return err
	}

	// Create the image
	imageCreateInput := astro.ImageCreateInput{
		Tag:          version,
		DeploymentID: deployInfo.deploymentID,
	}
	imageCreateRes, err := client.CreateImage(imageCreateInput)
	if err != nil {
		return err
	}

	domain := c.Domain
	if strings.Contains(domain, "cloud") {
		splitDomain := strings.SplitN(domain, ".", splitNum) // This splits out 'cloud' from the domain string
		domain = splitDomain[1]
	}

	nextTag := "deploy-" + time.Now().UTC().Format("2006-01-02T15-04")
	var registry string
	if domain == "localhost" {
		registry = config.CFG.LocalRegistry.GetString()
	} else {
		registry = "images." + strings.Split(domain, ".")[0] + ".cloud"
	}
	repository := registry + "/" + deployInfo.organizationID + "/" + deployInfo.deploymentID
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

	// Deploy the image
	err = imageDeploy(imageCreateRes.ID, repository, nextTag, client)
	if err != nil {
		return err
	}

	deploymentURL := "cloud." + domain + "/" + deployInfo.organizationID + "/deployments/" + deployInfo.deploymentID

	fmt.Println("Successfully pushed Docker image to Astronomer registry. Navigate to the Astronomer UI for confirmation that your deploy was successful." +
		"\n\n Deployment can be accessed at the following URLs: \n" +
		fmt.Sprintf("\n Deployment Dashboard: %s", ansi.Bold(deploymentURL)) +
		fmt.Sprintf("\n Airflow Dashboard: %s", ansi.Bold(deployInfo.webserverURL)))

	return nil
}

func getDeploymentInfo(deploymentID, wsID string, prompt bool, cloudDomain string, client astro.Client) (deploymentInfo, error) {
	// Use config deployment if provided
	if deploymentID == "" {
		deploymentID = config.CFG.ProjectDeployment.GetProjectString()
	}

	// check if deploymentID or if force prompt was requested was given by user
	if deploymentID == "" || prompt {
		currentDeployment, err := deployment.GetDeployment(wsID, deploymentID, client)
		if err != nil {
			return deploymentInfo{}, err
		}

		return deploymentInfo{
			currentDeployment.ID,
			currentDeployment.ReleaseName,
			airflow.ImageName(currentDeployment.ReleaseName, "latest"),
			currentDeployment.RuntimeRelease.Version,
			currentDeployment.Workspace.OrganizationID,
			currentDeployment.DeploymentSpec.Webserver.URL,
		}, nil
	}
	deployInfo, err := getImageName(cloudDomain, deploymentID, client)
	if err != nil {
		return deploymentInfo{}, err
	}
	deployInfo.deploymentID = deploymentID
	return deployInfo, nil
}

func parseDAG(pytest, version, envFile, deployImage, namespace string) error {
	dagParseVersionCheck := versions.GreaterThanOrEqualTo(version, dagParseAllowedVersion)
	if !dagParseVersionCheck {
		fmt.Println("\nruntime image is earlier than 4.1.0, this deploy will skip DAG parse...")
	}

	fmt.Println("testing", deployImage)
	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, "Dockerfile", namespace, true)
	if err != nil {
		return err
	}

	// parse dags
	if pytest == parse && dagParseVersionCheck {
		if !config.CFG.SkipParse.GetBool() && !util.CheckEnvBool(os.Getenv("ASTRONOMER_SKIP_PARSE")) {
			fmt.Println("Testing image...")
			err := containerHandler.Parse(deployImage)
			if err != nil {
				fmt.Println(err)
				return errDagsParseFailed
			}
		} else {
			fmt.Println("Skiping parsing dags due to skip parse being set to true in either the config.yaml or local environemnt variables")
		}
		// check pytests
	} else if pytest != "" && pytest != parse {
		fmt.Println("Testing image...")
		err := checkPytest(pytest, deployImage, containerHandler)
		if err != nil {
			return err
		}
	}
	return nil
}

// Validate code with pytest
func checkPytest(pytest, deployImage string, containerHandler airflow.ContainerHandler) error {
	if pytest != "all-tests" {
		pytestFile = pytest
	}

	exitCode, err := containerHandler.Pytest(pytestFile, deployImage)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
		}
		return errors.Wrap(err, "Something went wrong while Pytesting your local DAGs,\nif the issue persists rerun the command without the '--pytest' flag to deploy")
	}

	fmt.Print("\nAll Pytests passed!\n")
	return err
}

func getImageName(cloudDomain, deploymentID string, client astro.Client) (deploymentInfo, error) {
	if cloudDomain == astroDomain {
		fmt.Printf(deploymentHeaderMsg, "Astro")
	} else {
		fmt.Printf(deploymentHeaderMsg, cloudDomain)
	}

	// get current version and namespace
	deploymentsInput := astro.DeploymentsInput{
		DeploymentID: deploymentID,
	}
	deployments, err := client.ListDeployments(deploymentsInput)
	if err != nil {
		return deploymentInfo{}, err
	}

	if len(deployments) == 0 {
		return deploymentInfo{}, errors.New("invalid Deployment ID")
	}
	currentVersion := deployments[0].RuntimeRelease.Version
	namespace := deployments[0].ReleaseName
	organizationID := deployments[0].Workspace.OrganizationID
	webserverURL := deployments[0].DeploymentSpec.Webserver.URL

	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := airflow.ImageName(namespace, "latest")

	return deploymentInfo{namespace: namespace, deployImage: deployImage, currentVersion: currentVersion, organizationID: organizationID, webserverURL: webserverURL}, nil
}

func buildImage(c *config.Context, path, currentVersion, deployImage string, client astro.Client) (string, error) {
	// Build our image
	fmt.Println(composeImageBuildingPromptMsg)

	imageHandler := airflowImageHandler(deployImage)
	err := imageHandler.Build(types.ImageBuildConfig{Path: path, Output: true})
	if err != nil {
		return "", err
	}

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse dockerfile: %s", filepath.Join(path, dockerfile))
	}

	DockerfileImage := docker.GetImageFromParsedFile(cmds)

	version, err := imageHandler.GetLabel(runtimeImageLabel)
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

	// Allows System Admins to test with internal runtime releases
	admin, _ := c.GetSystemAdmin()
	var runtimeReleases []astro.RuntimeRelease
	if admin {
		runtimeReleases, err = client.ListInternalRuntimeReleases()
	} else {
		runtimeReleases, err = client.ListPublicRuntimeReleases()
	}

	if err != nil {
		return "", err
	}

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
		fmt.Println("Canceling deploy...")
		os.Exit(1)
	}

	return version, nil
}

// Deploy the image
func imageDeploy(imageCreateResID, repository, nextTag string, client astro.Client) error {
	imageDeployInput := astro.ImageDeployInput{
		ID:         imageCreateResID,
		Repository: repository,
		Tag:        nextTag,
	}
	resp, err := client.DeployImage(imageDeployInput)
	if err != nil {
		return err
	}

	fmt.Println("Deployed Image Tag: ", resp.Tag)
	return nil
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
