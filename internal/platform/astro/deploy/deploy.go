package deploy

import (
	"cmp"
	httpContext "context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"github.com/pkg/errors"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/azure"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	registryUsername         = "cli"
	enableDagDeployMsg       = "Dag-only deploys are not enabled for this Deployment; run 'astro deployment update %s --dag-deploy enable'"
	dagDeployDisabled        = "dag deploy is not enabled for deployment"
	errCiCdEnforcementUpdate = "cannot deploy: CI/CD enforcement is enabled for Deployment %s; use an API token instead"
)

var (
	// Monkey patched to write unit tests
	azureUploader = azure.Upload
	canCiCdDeploy = deployment.CanCiCdDeploy
)

var (
	sleepTime              = 90
	dagOnlyDeploySleepTime = 30
	tickNum                = 10
)

type deploymentInfo struct {
	namespace                string
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
	// Registry is the repository (registry[:port]/repository, no tag) the
	// client image is pushed to: the one the Deployment's remote agents pull
	// from. The command resolves it (--registry, the link's client-registry,
	// [tool.astro.deploy] client-registry), and RegistrySource names where it
	// came from, for an error about the value.
	Registry       string
	RegistrySource string
	// ErrOut is the command's error writer, which the deploy's warnings go
	// to; nil is stderr.
	ErrOut io.Writer
	// Context ends the build, the registry login, the tag and the push when
	// the run is interrupted; nil is context.Background().
	Context httpContext.Context
}

// includeMonitoringDag reports whether a DAG upload carries the monitoring DAG,
// for an org whose product is known, which need not be the current context's.
func includeMonitoringDag(orgHosted bool, deploymentType astrov1.DeploymentType) bool {
	return !orgHosted && !deployment.IsDeploymentDedicated(deploymentType) && !deployment.IsDeploymentStandard(deploymentType)
}

// dagsUpload is how a project's dags/ directory is uploaded.
type dagsUpload struct {
	// MonitoringDag adds Astronomer's monitoring Dag to the upload
	// (includeMonitoringDag).
	MonitoringDag bool
	// NoDagsBaseDir is --no-dags-base-dir: the files go at the bundle's root
	// rather than under dags/.
	NoDagsBaseDir bool
	// CosmosBoost is [tool.astro.deploy] cosmos-boost: the upload carries
	// Cosmos Boost files for the dbt projects among the Dags.
	CosmosBoost bool
}

// monitoringDagPerm and monitoringDagDirPerm are the modes writeMonitoringDag
// creates the Dag and, when missing, its directory with.
const (
	monitoringDagPerm    = 0o644
	monitoringDagDirPerm = 0o755
)

// writeMonitoringDag writes the monitoring Dag at path, a name the CLI picks in
// the project's dags/ directory, for the upload to carry and the deploy to
// remove after. It is published through a temp file and a rename, which
// replaces whatever is at path, a symlink included, rather than writing through
// it: a project's dags/astronomer_monitoring_dag.py held as a link elsewhere
// would otherwise have the file it points at overwritten, and the cleanup
// would remove only the link.
func writeMonitoringDag(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), monitoringDagDirPerm); err != nil {
		return err
	}
	return fsatomic.WriteFile(path, []byte(content), monitoringDagPerm)
}

func uploadDags(dagsPath, dagsUploadURL, currentRuntimeVersion string, opts dagsUpload) (string, error) {
	if opts.MonitoringDag {
		monitoringDagPath := filepath.Join(dagsPath, "astronomer_monitoring_dag.py")

		var monitoringDag string
		switch airflowversions.AirflowMajorVersionForRuntimeVersion(currentRuntimeVersion) {
		case "2":
			monitoringDag = af2MonitoringDag
		case "3":
			monitoringDag = af3MonitoringDag
		default:
			return "", errors.New("unsupported Airflow major version for runtime version " + currentRuntimeVersion)
		}

		err := writeMonitoringDag(monitoringDagPath, monitoringDag)
		if err != nil {
			return "", err
		}

		// Remove the monitoring dag file after the upload
		defer os.Remove(monitoringDagPath) //nolint:errcheck // best-effort cleanup
	}

	// By default, prepend dags/ directory prefix. Use --no-dags-base-dir to place files at bundle root
	// (needed for some Airflow 3.x deployments where sys.path includes the bundle root, not dags/).
	versionID, err := UploadBundle(dagsPath, dagsUploadURL, BundleUpload{
		PrependBaseDir: !opts.NoDagsBaseDir,
		CosmosBoost:    opts.CosmosBoost,
		RuntimeVersion: currentRuntimeVersion,
	})
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

	return deploymentInfo{
		namespace:                namespace,
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
		return nil, fmt.Errorf("could not create temporary build directory: %w", err)
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
		return buildContext, fmt.Errorf("could not check source directory: %w", err)
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
		return buildContext, fmt.Errorf("could not read .dockerignore: %w", err)
	}

	// Copy all project files to the temporary directory
	err = fileutil.CopyDirectoryFiltered(sourcePath, tempBuildDir, skip)
	if err != nil {
		return buildContext, fmt.Errorf("could not copy project files to temporary directory: %w", err)
	}

	// Process client dependency files
	err = setupClientDependencyFiles(tempBuildDir)
	if err != nil {
		return buildContext, fmt.Errorf("could not set up client dependency files: %w", err)
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

		// The project's own requirements.txt or packages.txt was copied here
		// as it is, symlink included, and CopyFile writes through a symlink.
		// Remove it first so the write lands in the build context, not on
		// the file a symlink points at (a monorepo's shared requirements.txt,
		// say), which the client deps would otherwise overwrite.
		if err := os.Remove(regularPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not replace %s in build context: %w", regularFile, err)
		}
		// Copy client file content to the regular file location (requires client file to exist)
		if err := fileutil.CopyFile(clientPath, regularPath); err != nil {
			return fmt.Errorf("could not copy %s to %s in build context: %w", clientFile, regularFile, err)
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
		return ClientDeploy{}, errors.Wrap(err, "could not get current context")
	}

	// The registry is checked before anything calls the API, so a value with
	// a tag is a clear error naming where it was written, rather than Docker's
	// "invalid reference format", and is not hidden behind an auth or network
	// error.
	registryEndpoint := deployInput.Registry
	if registryEndpoint == "" {
		return ClientDeploy{}, errors.New("no client registry to push to: pass --registry <registry>/<repository>")
	}
	if err := config.ValidateRegistryEndpoint(registryEndpoint); err != nil {
		return ClientDeploy{}, fmt.Errorf("%s is not a usable registry endpoint: %w", deployInput.RegistrySource, err)
	}

	// Validate deployment runtime version if deployment ID is provided
	check, err := validateClientImageRuntimeVersion(deployInput, astroV1Client)
	if err != nil {
		return ClientDeploy{}, err
	}

	// Use consistent deploy-<timestamp> tagging mechanism like regular deploys
	// The ImageName flag only specifies which local image to use, not the remote tag
	imageTag := "deploy-" + time.Now().UTC().Format("2006-01-02T15-04")

	// Build the full remote image name
	remoteImage := fmt.Sprintf("%s:%s", registryEndpoint, imageTag)

	// One engine tags or builds the image, logs in and pushes it: an image
	// built on one engine is not there for another to push.
	ctx := cmp.Or(deployInput.Context, httpContext.Background())
	engine, err := clientImageEngine()
	if err != nil {
		return ClientDeploy{}, err
	}

	// What the deploy is doing (its steps, what it could not do) goes to
	// stderr: stdout is the deploy's result.
	progress := deployInput.progress()

	// Use empty slice to let Docker build for host platform by default
	targetPlatforms := []string{}
	if deployInput.ImageName != "" {
		// Use the provided local image (tag will be ignored, remote tag is always timestamp-based)
		fmt.Fprintln(progress, "Using provided image:", deployInput.ImageName)
		err := engine.Tag(ctx, deployInput.ImageName, remoteImage)
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("could not tag local image: %w", err)
		}
	} else {
		// Authenticate with the base image registry before building
		// This is needed because Dockerfile.client uses base images from a private registry

		// Skip registry login if the base image registry is not from astronomer, check the content of the Dockerfile.client file
		errOut := deployInput.ErrOut
		if errOut == nil {
			errOut = ansi.Messages
		}
		dockerfileClientContent, err := fileutil.ReadFileToString(filepath.Join(deployInput.Path, "Dockerfile.client"))
		needsLogin := util.IsAstronomerRegistry(dockerfileClientContent) || err != nil
		baseImageRegistry := config.CFG.RemoteBaseImageRegistry.GetString()
		switch {
		case !needsLogin:
		case !util.IsAstronomerRegistryHost(baseImageRegistry):
			// The login sends the Astro token, so it goes to an Astronomer
			// registry or nowhere: the setting can come from a project's
			// .astro/config.yaml, and any other host would just collect it.
			ansi.Warn(errOut, fmt.Sprintf("the base image registry %q is not an Astronomer registry, so astro does not log in to it with your Astro token; the build pulls with the credentials the container engine already has", baseImageRegistry))
		default:
			if err != nil {
				ansi.Warn(errOut, "could not read Dockerfile.client, so astro assumes its base image is on images.astronomer.cloud and logs in to that registry")
			}
			fmt.Fprintf(progress, "Authenticating with base image registry: %s\n", baseImageRegistry)
			err := loginBaseImageRegistry(ctx, engine, baseImageRegistry, c.Token)
			if err != nil {
				fmt.Fprintln(progress, "Could not authenticate with the Astronomer registry for the base agent image in Dockerfile.client.")
				fmt.Fprintln(progress, "Your token may have expired, or you may lack permission to pull the base agent image.")
				fmt.Fprintln(progress, "Run astro login to refresh your credentials, or check that ASTRO_API_TOKEN holds a valid token, then try again.")
				return ClientDeploy{}, fmt.Errorf("could not authenticate with registry %s: %w", baseImageRegistry, err)
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
			fmt.Fprintf(progress, "Building client image for platforms: %s\n", strings.Join(targetPlatforms, ", "))
		} else {
			fmt.Fprintln(progress, "Building client image for host platform")
		}

		// Prepare build context with client dependency files
		buildContext, err := prepareClientBuildContext(deployInput.Path)
		if buildContext != nil && buildContext.CleanupFunc != nil {
			defer buildContext.CleanupFunc()
		}
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("could not prepare client build context: %w", err)
		}

		// Build the image from the prepared context, tagged as it is pushed
		err = buildClientImage(ctx, deployInput.Path, buildContext.TempDir, remoteImage, targetPlatforms, deployInput.BuildSecrets, engine)
		if err != nil {
			return ClientDeploy{}, fmt.Errorf("could not build client image: %w", err)
		}
	}

	// Push the image to the remote registry (assumes docker login was done externally)
	fmt.Fprintln(progress, "Pushing client image to configured remote registry")
	_, err = engine.Push(ctx, imagebuild.PushRequest{Image: remoteImage, Out: progress})
	if err != nil {
		if errors.Is(err, imagebuild.ErrPushForbidden) {
			fmt.Fprintf(progress, "\n--------------------------------\n")
			fmt.Fprintf(progress, "Could not push client image to %s\n", registryEndpoint)
			fmt.Fprintln(progress, "Your registry token may have expired, or you may lack permission to push the client image.")
			fmt.Fprintf(progress, "Run docker login %s, then try again\n\n", registryEndpoint)
		}
		return ClientDeploy{}, fmt.Errorf("could not push client image: %w", deploybuild.ExplainPush(err))
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

// progress is where the deploy says what it is doing: the command's error
// writer, stderr when none was given.
func (i *InputClientDeploy) progress() io.Writer {
	if i.ErrOut != nil {
		return i.ErrOut
	}
	return os.Stderr
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
		return nil, errors.Wrap(err, "could not get current context")
	}

	// Get deployment information
	deployInfo, err := fetchDeploymentDetails(deployInput.DeploymentID, c.Organization, astroV1Client)
	if err != nil {
		return nil, errors.Wrap(err, "could not get Deployment information")
	}

	// Parse Dockerfile.client to get client image runtime version
	dockerfileClientPath := filepath.Join(deployInput.Path, "Dockerfile.client")
	if _, err := os.Stat(dockerfileClientPath); os.IsNotExist(err) {
		return nil, errors.New("Dockerfile.client is required for client image runtime version validation")
	}

	cmds, err := docker.ParseFile(dockerfileClientPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not parse Dockerfile.client: %s", dockerfileClientPath)
	}

	baseImage := docker.GetImageFromParsedFile(cmds)
	if baseImage == "" {
		return nil, errors.New("no base image found in Dockerfile.client")
	}

	// Extract runtime version from the base image tag
	clientRuntimeVersion, err := extractRuntimeVersionFromImage(baseImage)
	if err != nil {
		return nil, errors.Wrapf(err, "could not extract runtime version from client image %s", baseImage)
	}

	// Compare versions
	if airflowversions.CompareRuntimeVersions(clientRuntimeVersion, deployInfo.currentVersion) > 0 {
		return nil, fmt.Errorf(`client image runtime version validation failed: the client image is based on Astro Runtime %s, which is newer than the Deployment's runtime version %s

Either set the base image in Dockerfile.client to runtime version %s or earlier, or upgrade the Deployment to runtime version %s or later`,
			clientRuntimeVersion, deployInfo.currentVersion, deployInfo.currentVersion, clientRuntimeVersion)
	}

	fmt.Fprintf(deployInput.progress(), "Client image runtime version %s is compatible with Deployment runtime version %s\n",
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
		return "", errors.Wrapf(err, "could not parse image tag: %s", imageTag)
	}

	return tagInfo.RuntimeVersion, nil
}
