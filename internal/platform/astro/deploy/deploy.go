package deploy

import (
	httpContext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/azure"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	registryUsername         = "cli"
	runtimeImageLabel        = airflow.RuntimeImageLabel
	enableDagDeployMsg       = "Dag-only deploys are not enabled for this Deployment. Run 'astro deployment update %s --dag-deploy enable' to enable Dag-only deploys"
	dagDeployDisabled        = "dag deploy is not enabled for deployment"
	errCiCdEnforcementUpdate = "cannot deploy since ci/cd enforcement is enabled for the deployment %s. Please use API Tokens instead"
)

var (
	deployImagePlatformSupport = []string{"linux/amd64"}

	// Monkey patched to write unit tests
	airflowImageHandler = airflow.ImageHandlerInit
	azureUploader       = azure.Upload
	canCiCdDeploy       = deployment.CanCiCdDeploy
)

var (
	sleepTime              = 90
	dagOnlyDeploySleepTime = 30
	tickNum                = 10
)

type deploymentInfo struct {
	namespace                string
	deployImage              string
	currentVersion           string
	organizationID           string
	workspaceID              string
	webserverURL             string
	desiredDagTarballVersion string
	dagDeployEnabled         bool
	cicdEnforcement          bool
	isRemoteExecutionEnabled bool
}

// InputClientDeploy contains inputs for client image deployments
type InputClientDeploy struct {
	Path         string
	ImageName    string
	Platform     string
	BuildSecrets []string
	DeploymentID string
}

// includeMonitoringDag reports whether a DAG upload carries the monitoring DAG,
// for an org whose product is known, which need not be the current context's.
func includeMonitoringDag(orgHosted bool, deploymentType astrov1.DeploymentType) bool {
	return !orgHosted && !deployment.IsDeploymentDedicated(deploymentType) && !deployment.IsDeploymentStandard(deploymentType)
}

func uploadDags(path, dagsPath, dagsUploadURL, currentRuntimeVersion string, monitoringDag, noDagsBaseDir bool) (string, error) {
	if monitoringDag {
		monitoringDagPath := filepath.Join(dagsPath, "astronomer_monitoring_dag.py")

		var monitoringDag string
		switch airflowversions.AirflowMajorVersionForRuntimeVersion(currentRuntimeVersion) {
		case "2":
			monitoringDag = airflow.Af2MonitoringDag
		case "3":
			monitoringDag = airflow.Af3MonitoringDag
		default:
			return "", errors.New("unsupported Airflow major version for runtime version " + currentRuntimeVersion)
		}

		// Create monitoring dag file
		err := fileutil.WriteStringToFile(monitoringDagPath, monitoringDag)
		if err != nil {
			return "", err
		}

		// Remove the monitoring dag file after the upload
		defer os.Remove(monitoringDagPath) //nolint:errcheck // best-effort cleanup
	}

	// By default, prepend dags/ directory prefix. Use --no-dags-base-dir to place files at bundle root
	// (needed for some Airflow 3.x deployments where sys.path includes the bundle root, not dags/).
	prependBaseDir := !noDagsBaseDir
	versionID, err := UploadBundle(path, dagsPath, dagsUploadURL, prependBaseDir, currentRuntimeVersion)
	if err != nil {
		return "", err
	}

	return versionID, nil
}

func fetchDeploymentDetails(deploymentID, organizationID string, astroV1Client astrov1.APIClient) (deploymentInfo, error) {
	resp, err := astroV1Client.GetDeploymentWithResponse(httpContext.Background(), organizationID, deploymentID)
	if err != nil {
		return deploymentInfo{}, err
	}

	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return deploymentInfo{}, err
	}

	currentVersion := resp.JSON200.AstroRuntimeVersion
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

func createDeploy(organizationID, deploymentID string, request astrov1.CreateDeployRequest, astroV1Client astrov1.APIClient) (*astrov1.Deploy, error) {
	resp, err := astroV1Client.CreateDeployWithResponse(httpContext.Background(), organizationID, deploymentID, request)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, err
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
		os.RemoveAll(tempBuildDir) //nolint:errcheck // best-effort cleanup
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

	// Build a skip predicate from the project's .dockerignore so excluded
	// paths (e.g. infra/ with terragrunt provider-cache symlinks) are not
	// copied into the build context. This mirrors what the Docker builder does
	// for in-place builds; the client deploy copies the context first, so it
	// must honor .dockerignore itself.
	skip, err := dockerignoreSkipFunc(sourcePath)
	if err != nil {
		return buildContext, fmt.Errorf("failed to read .dockerignore: %w", err)
	}

	// Copy all project files to the temporary directory
	err = fileutil.CopyDirectoryFiltered(sourcePath, tempBuildDir, skip)
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

// alwaysIncludedBuildFiles are never excluded from the client build context,
// even if a user's .dockerignore would match them. The Docker builder applies
// the same special-casing to the Dockerfile and .dockerignore, and the client
// deploy additionally needs its client dependency files.
var alwaysIncludedBuildFiles = map[string]bool{
	"Dockerfile.client":       true,
	".dockerignore":           true,
	"requirements-client.txt": true,
	"packages-client.txt":     true,
}

// dockerignoreSkipFunc parses the .dockerignore at sourcePath (if any) and
// returns a predicate, suitable for fileutil.CopyDirectoryFiltered, that
// reports whether a path should be excluded from the build context. It returns
// nil (copy everything) when there is no .dockerignore file.
func dockerignoreSkipFunc(sourcePath string) (func(relPath string, isDir bool) bool, error) {
	f, err := os.Open(filepath.Join(sourcePath, ".dockerignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	patterns, err := ignorefile.ReadAll(f)
	if err != nil {
		return nil, err
	}

	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return nil, err
	}

	return func(relPath string, isDir bool) bool {
		if alwaysIncludedBuildFiles[relPath] {
			return false
		}
		matched, err := pm.MatchesOrParentMatches(relPath)
		if err != nil || !matched {
			return false
		}
		// When exclusion ("!") patterns exist, a child of a matched directory
		// may be re-included, so we must descend rather than prune the dir.
		if isDir && pm.Exclusions() {
			return false
		}
		return true
	}, nil
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

// ClientDeploy is what a client image deploy pushed, and where.
type ClientDeploy struct {
	// Image is the full reference pushed, Registry:Tag.
	Image    string
	Registry string
	Tag      string
	// SourceImage is the local image pushed in place of a build
	// (InputClientDeploy.ImageName); empty when the deploy built one.
	SourceImage string
	// Platforms are the platforms the image was built for; empty for the
	// host's, or when nothing was built.
	Platforms []string
	// RuntimeCheck is the runtime version check against a Deployment, nil
	// when no Deployment was named.
	RuntimeCheck *ClientRuntimeCheck
}

// ClientRuntimeCheck is the client image's runtime version checked against a
// Deployment's: the client's is not newer.
type ClientRuntimeCheck struct {
	DeploymentID             string
	ClientRuntimeVersion     string
	DeploymentRuntimeVersion string
}

// DeployClientImage builds the client image (or tags the local one named)
// and pushes it to the remote client registry. It does not report the result:
// its caller renders the ClientDeploy it returns.
func DeployClientImage(deployInput InputClientDeploy, astroV1Client astrov1.APIClient) (ClientDeploy, error) { //nolint:gocritic // intentional in this shell code
	c, err := config.GetCurrentContext()
	if err != nil {
		return ClientDeploy{}, errors.Wrap(err, "failed to get current context")
	}

	// Validate deployment runtime version if deployment ID is provided
	check, err := validateClientImageRuntimeVersion(deployInput, astroV1Client)
	if err != nil {
		return ClientDeploy{}, err
	}

	// Get the remote client registry endpoint from config
	registryEndpoint := config.CFG.RemoteClientRegistry.GetString()
	if registryEndpoint == "" {
		fmt.Println("The Astro CLI is not configured to push client images to your private registry.")
		fmt.Println("For remote Deployments, client images must be stored in your private registry, not in Astronomer managed registries.")
		fmt.Println("Please provide your private registry information so the Astro CLI can push client images.")
		return ClientDeploy{}, errors.New("remote client registry is not configured. To configure it, run: 'astro config set remote.client_registry <endpoint>' and try again.")
	}

	// Use consistent deploy-<timestamp> tagging mechanism like regular deploys
	// The ImageName flag only specifies which local image to use, not the remote tag
	imageTag := "deploy-" + time.Now().UTC().Format("2006-01-02T15-04")

	// Build the full remote image name
	remoteImage := fmt.Sprintf("%s:%s", registryEndpoint, imageTag)

	// Create an image handler for building and pushing
	imageHandler := airflowImageHandler(remoteImage)

	// Use empty slice to let Docker build for host platform by default
	targetPlatforms := []string{}
	if deployInput.ImageName != "" {
		// Use the provided local image (tag will be ignored, remote tag is always timestamp-based)
		fmt.Println("Using provided image:", deployInput.ImageName)
		err := imageHandler.TagLocalImage(deployInput.ImageName)
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("failed to tag local image: %w", err)
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
				return ClientDeploy{}, fmt.Errorf("failed to authenticate with registry %s: %w", baseImageRegistry, err)
			}
		}

		// Build the client image from the current directory
		// Determine target platforms for client deploy
		if deployInput.Platform != "" {
			// Parse comma-separated platforms from --platform flag
			targetPlatforms = strings.Split(deployInput.Platform, ",")
			// Trim whitespace from each platform
			for i, platform := range targetPlatforms {
				targetPlatforms[i] = strings.TrimSpace(platform)
			}
			fmt.Printf("Building client image for platforms: %s\n", strings.Join(targetPlatforms, ", "))
		} else {
			fmt.Println("Building client image for host platform")
		}

		// Prepare build context with client dependency files
		buildContext, err := prepareClientBuildContext(deployInput.Path)
		if buildContext != nil && buildContext.CleanupFunc != nil {
			defer buildContext.CleanupFunc()
		}
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("failed to prepare client build context: %w", err)
		}

		// Build the image from the prepared context
		buildConfig := types.ImageBuildConfig{
			Path:            buildContext.TempDir,
			TargetPlatforms: targetPlatforms,
		}

		err = imageHandler.Build("Dockerfile.client", deployInput.BuildSecrets, buildConfig)
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("failed to build client image: %w", err)
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
		return ClientDeploy{}, fmt.Errorf("failed to push client image: %w", err)
	}

	return ClientDeploy{
		Image:        remoteImage,
		Registry:     registryEndpoint,
		Tag:          imageTag,
		SourceImage:  deployInput.ImageName,
		Platforms:    targetPlatforms,
		RuntimeCheck: check,
	}, nil
}

// validateClientImageRuntimeVersion validates that the client image runtime version
// is not newer than the deployment runtime version
func validateClientImageRuntimeVersion(deployInput InputClientDeploy, astroV1Client astrov1.APIClient) (*ClientRuntimeCheck, error) { //nolint:gocritic // intentional in this shell code
	// Skip validation if no deployment ID provided
	if deployInput.DeploymentID == "" {
		return nil, nil
	}

	// Get current context for organization info
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current context")
	}

	// Get deployment information
	deployInfo, err := fetchDeploymentDetails(deployInput.DeploymentID, c.Organization, astroV1Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get deployment information")
	}

	// Parse Dockerfile.client to get client image runtime version
	dockerfileClientPath := filepath.Join(deployInput.Path, "Dockerfile.client")
	if _, err := os.Stat(dockerfileClientPath); os.IsNotExist(err) {
		return nil, errors.New("Dockerfile.client is required for client image runtime version validation")
	}

	cmds, err := docker.ParseFile(dockerfileClientPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse Dockerfile.client: %s", dockerfileClientPath)
	}

	baseImage := docker.GetImageFromParsedFile(cmds)
	if baseImage == "" {
		return nil, errors.New("failed to find base image in Dockerfile.client")
	}

	// Extract runtime version from the base image tag
	clientRuntimeVersion, err := extractRuntimeVersionFromImage(baseImage)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract runtime version from client image %s", baseImage)
	}

	// Compare versions
	if airflowversions.CompareRuntimeVersions(clientRuntimeVersion, deployInfo.currentVersion) > 0 {
		return nil, fmt.Errorf(`client image runtime version validation failed:

The client image is based on Astro Runtime version %s, which is newer than the deployment's runtime version %s.

To fix this issue, you can either:
1. Downgrade the client image version by updating the base image in Dockerfile.client to use runtime version %s or earlier
2. Upgrade the deployment's runtime version to %s or higher

This validation ensures compatibility between your client image and the deployment environment`,
			clientRuntimeVersion, deployInfo.currentVersion, deployInfo.currentVersion, clientRuntimeVersion)
	}

	fmt.Printf("✓ Client image runtime version %s is compatible with deployment runtime version %s\n",
		clientRuntimeVersion, deployInfo.currentVersion)

	return &ClientRuntimeCheck{
		DeploymentID:             deployInput.DeploymentID,
		ClientRuntimeVersion:     clientRuntimeVersion,
		DeploymentRuntimeVersion: deployInfo.currentVersion,
	}, nil
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
