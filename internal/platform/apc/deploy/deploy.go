package deploy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/versions"
	"golang.org/x/mod/semver"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/internal/platform/apc/auth"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	// this is used to monkey patch the function in order to write unit test cases
	imageHandlerInit = airflow.ImageHandlerInit

	dockerfile = "Dockerfile"

	deployImagePlatformSupport = []string{"linux/amd64"}

	gzipFile = fileutil.GzipFile

	getDeploymentIDForCurrentCommandVar = getDeploymentIDForCurrentCommand

	// confirmEmptyDags asks whether to upload a dags directory with no DAGs.
	confirmEmptyDags = input.Confirm
)

var (
	ErrNoWorkspaceID             = errors.New("no workspace id provided")
	errNoDomainSet               = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentID       = errors.New("please specify a valid deployment ID")
	errDeploymentNotFound        = errors.New("no airflow deployments found")
	errInvalidDeploymentSelected = errors.New("invalid deployment selection\n")
	// ErrDagOnlyDeployDisabledInConfigLegacy is returned for Houston before 2.0.0 (flat feature-flag paths).
	ErrDagOnlyDeployDisabledInConfigLegacy = errors.New("to perform this operation, set both deployments.dagOnlyDeployment and deployments.configureDagDeployment to true in your APC cluster")
	// ErrDagOnlyDeployDisabledInConfig is returned for Houston 2.0.0+ (deployMechanisms.*.enabled under merged deployments config).
	ErrDagOnlyDeployDisabledInConfig        = errors.New("to perform this operation, set both deployments.deployMechanisms.dagOnlyDeployment.enabled and deployments.deployMechanisms.configureDagDeployment.enabled to true in your APC cluster")
	ErrDagOnlyDeployNotEnabledForDeployment = errors.New("to perform this operation, first set the Deployment type to 'dag_deploy' via the UI or the API or the CLI")
	ErrEmptyDagFolderUserCancelledOperation = errors.New("no Dags found in the dags folder. User canceled the operation")
	// ErrNoDagsDirectory is DagsOnlyDeploy's refusal when there is no dags
	// directory to upload, to a Deployment that takes uploads (the refusals
	// come first). An upload of nothing would replace the Deployment's DAGs
	// with none, so nothing is sent.
	ErrNoDagsDirectory = errors.New("no dags directory to upload")
	// ErrAppConfigUnread is DagsOnlyDeploy failing to read the cluster
	// config its refusals are decided by.
	ErrAppConfigUnread = errors.New("failed to get app config")
	// ErrDagsDirHoldsProject is DagsOnlyDeploy refusing a dags directory
	// that resolves to the directory its tarball is written to, or above it.
	ErrDagsDirHoldsProject = errors.New("the dags directory is the project directory or one above it")
	// Houston reads the host from its registry.protectedCustomRegistry.updateRegistry.host,
	// under astronomer.houston.config in the platform's values.
	ErrBYORegistryDomainNotSet               = errors.New("Custom registry host is not set in config. It can be set at astronomer.houston.config.registry.protectedCustomRegistry.updateRegistry.host")
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

// Deployed is what an image deploy pushed.
type Deployed struct {
	// DeploymentID is the Deployment deployed to: the one named, the
	// project's saved one, or the one picked.
	DeploymentID string
	// Image is the image reference pushed to the registry.
	Image string
	// URL is the Deployment's Airflow UI, "" when Houston gives none.
	URL string
	// Dags is where the Deployment takes its DAGs from, read from the
	// Deployment and the cluster config the deploy fetched.
	Dags DagsFrom
}

// DagsFrom is where a Deployment takes its DAGs from, as far as a deploy is
// concerned: what a DAG upload after the image push would do to it.
type DagsFrom int

const (
	// DagsFromUnknown is a Deployment the deploy did not place. It is the
	// zero value, so a path that never sets it uploads the DAGs as a deploy
	// always has, and DagsOnlyDeploy's own checks decide.
	DagsFromUnknown DagsFrom = iota
	// DagsFromImage is a Deployment that runs the DAGs inside its image: DAG
	// deployment type image, or no type at all where the type was read,
	// which Houston deploys as an image Deployment. The image just deployed
	// is all the DAGs it has.
	DagsFromImage
	// DagsFromUpload is a Deployment that takes DAG-only deploys: an upload
	// replaces its DAGs, and an empty one leaves it none.
	DagsFromUpload
	// DagsFromElsewhere is a git-sync or volume Deployment, or a DAG-only
	// one on a cluster without DAG-only deploys: a DAG upload is refused,
	// and the image does not decide the DAGs either.
	DagsFromElsewhere
)

// dagsFrom places a Deployment's DAGs from the Deployment and its merged
// cluster config (nil when it could not be read), by the same tests
// DagsOnlyDeploy refuses on. A Deployment with no type runs its image's DAGs
// on any cluster: git-sync, volume and DAG-only Deployments all carry their
// type. That holds only where the query that read the Deployment asked for
// its type: GetDeployment does not before 0.29.0, and every Deployment then
// reads as one with none.
func dagsFrom(deploymentInfo *houston.Deployment, appConfig *houston.AppConfig) DagsFrom {
	if deploymentInfo == nil {
		return DagsFromUnknown
	}
	switch deploymentInfo.DagDeployment.Type {
	case houston.ImageDeploymentType:
		return DagsFromImage
	case "":
		if deploymentInfo.DagDeploymentRead {
			return DagsFromImage
		}
	case houston.DagOnlyDeploymentType:
		if appConfig == nil {
			// Whether the cluster takes the upload is not known.
			return DagsFromUnknown
		}
		if isDagOnlyDeploymentEnabled(appConfig) {
			return DagsFromUpload
		}
		return DagsFromElsewhere
	case houston.GitSyncDeploymentType, houston.VolumeDeploymentType:
		return DagsFromElsewhere
	}
	return DagsFromUnknown
}

// Options is what a deploy's caller decides about its output and its
// questions.
type Options struct {
	// Progress is where the deploy's progress goes: its notes, the image
	// push and the DAG upload. nil is os.Stdout, where it has always gone; a
	// command whose stdout carries a result (--output json) sends it to
	// stderr.
	Progress io.Writer
	// Yes answers the deploy's confirmations yes: an image tag that is not
	// recommended, and a DAGs folder with no DAGs in it.
	Yes bool
}

func (o Options) progress() io.Writer {
	if o.Progress != nil {
		return o.Progress
	}
	return os.Stdout
}

// progressSetter is an image handler that can draw its progress somewhere
// other than stdout (airflow.DockerImage).
type progressSetter interface{ ProgressTo(io.Writer) }

// Airflow builds the project's image, or tags the one named, pushes it to
// the Deployment's registry and returns what it pushed. Its DeploymentID is
// set on a failure too, once the Deployment is known.
func Airflow(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, opts Options) (Deployed, error) {
	deploymentID, deployments, err := getDeploymentIDForCurrentCommand(houstonClient, wsID, deploymentID, prompt)
	if err != nil {
		return Deployed{DeploymentID: deploymentID}, err
	}

	c, _ := config.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code
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

	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return Deployed{DeploymentID: deploymentID}, fmt.Errorf("failed to get deployment info: %w", err)
	}

	appCfgWs := resolvedWorkspaceUUIDForAppConfig(wsID, deploymentID, deployments, deploymentInfo)
	appConfig, err := houston.Call(houstonClient.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: deploymentInfo.ClusterID, WorkspaceUUID: appCfgWs, DeploymentUUID: deploymentID})
	if err != nil {
		return Deployed{DeploymentID: deploymentID}, fmt.Errorf("failed to get app config: %w", err)
	}

	byoRegistryDomain := ""
	byoRegistryEnabled := appConfig != nil && appConfig.Flags.BYORegistryEnabled
	if byoRegistryEnabled {
		// updating nextTag logic for private registry, since houston won't maintain next tag in case of BYO registry
		nextTag = "deploy-" + time.Now().UTC().Format("2006-01-02T15-04")
		byoRegistryDomain = appConfig.BYORegistryDomain
		if byoRegistryDomain == "" {
			return Deployed{DeploymentID: deploymentID}, ErrBYORegistryDomainNotSet
		}
	}

	// isImageOnlyDeploy is not valid for image-based deployments since image-based deployments inherently mean that the image itself contains dags.
	// If we deploy only the image, the deployment will not have any dags for image-based deployments.
	// Even on astro, image-based deployments are not allowed to be deployed with --image flag.
	dags := dagsFrom(deploymentInfo, appConfig)
	if isImageOnlyDeploy && dags == DagsFromImage {
		return Deployed{DeploymentID: deploymentID}, ErrDeploymentTypeIncorrectForImageOnly
	}
	// We don't need to exclude the dags from the image because the dags present in the image are not respected anyways for non-image based deployments

	fmt.Fprintf(opts.progress(), houstonDeploymentPrompt, releaseName)

	// Build the image to deploy
	pushed, err := buildPushDockerImage(houstonClient, &c, deploymentInfo, releaseName, path, nextTag, cloudDomain, byoRegistryDomain, ignoreCacheDeploy, byoRegistryEnabled, description, imageName, opts)
	if err != nil {
		return Deployed{DeploymentID: deploymentID}, err
	}

	deploymentLink := getAirflowUILink(deploymentID, deploymentInfo.Urls)
	fmt.Fprintf(opts.progress(), "Successfully pushed Docker image to the APC registry, it can take a few minutes to update the deployment with the new image. Navigate to the APC UI to confirm the state of your deployment (%s).\n", deploymentLink)

	return Deployed{DeploymentID: deploymentID, Image: pushed, URL: deploymentLink, Dags: dags}, nil
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

// resolvedWorkspaceUUIDForAppConfig prefers the workspace ID Houston associates with the
// deployment so appConfig merges deployments config at the same tier as GetDeployment /
// workspaceDeployments. Falls back to wsID when the API response omits workspace (older queries).
func resolvedWorkspaceUUIDForAppConfig(wsID, deploymentID string, deployments []houston.Deployment, deploymentInfo *houston.Deployment) string {
	if deploymentInfo != nil && deploymentInfo.Workspace.ID != "" {
		return deploymentInfo.Workspace.ID
	}
	for i := range deployments {
		if deployments[i].ID == deploymentID && deployments[i].Workspace.ID != "" {
			return deployments[i].Workspace.ID
		}
	}
	return wsID
}

func validateRuntimeVersion(houstonClient houston.ClientInterface, tag string, deploymentInfo *houston.Deployment, opts Options) error {
	// Get valid image tags for platform using Deployment Info request
	deploymentConfig, err := houston.Call(houstonClient.GetDeploymentConfig)(nil)
	if err != nil {
		return err
	}
	vars := make(map[string]interface{})
	vars["clusterId"] = deploymentInfo.ClusterID
	// ignoring the error as user can be connected to platform where runtime is not enabled
	runtimeReleases, _ := houston.Call(houstonClient.GetRuntimeReleases)(vars) //nolint:errcheck // error deliberately ignored in this shell code
	var validTags string
	if config.CFG.ShowWarnings.GetBool() && deploymentInfo.DesiredAirflowVersion != "" && !deploymentConfig.IsValidTag(tag) {
		validTags = strings.Join(deploymentConfig.GetValidTags(tag), ", ")
	}
	if config.CFG.ShowWarnings.GetBool() && deploymentInfo.DesiredRuntimeVersion != "" && !runtimeReleases.IsValidVersion(tag) {
		validTags = strings.Join(runtimeReleases.GreaterVersions(tag), ", ")
	}
	if validTags != "" && !opts.Yes {
		validTags := strings.Join(deploymentConfig.GetValidTags(tag), ", ")

		msg := fmt.Sprintf(warningInvalidNameTag, tag, validTags)
		if validTags == "" {
			msg = fmt.Sprintf(warningInvalidNameTagEmptyRecommendations, tag)
		}

		i, err := input.Confirm(msg, input.AnsweredBy("--yes"))
		if err != nil {
			return err
		}
		if !i {
			fmt.Fprintln(opts.progress(), "Canceling deploy...")
			os.Exit(1)
		}
	}
	return nil
}

// UpdateDeploymentImage points a Deployment at an image already in a
// registry (--image-name --remote) and returns what it deployed. Its
// DeploymentID is set on a failed update too.
func UpdateDeploymentImage(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, opts Options) (Deployed, error) {
	if runtimeVersion == "" {
		return Deployed{}, ErrRuntimeVersionNotPassedForRemoteImage
	}
	deploymentID, deployments, err := getDeploymentIDForCurrentCommandVar(houstonClient, wsID, deploymentID, deploymentID == "")
	if err != nil {
		return Deployed{}, err
	}
	if deploymentID == "" {
		return Deployed{}, errInvalidDeploymentID
	}
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return Deployed{}, fmt.Errorf("failed to get deployment info: %w", err)
	}
	fmt.Fprintln(opts.progress(), "Skipping building the image since --image-name flag is used...")
	req := houston.UpdateDeploymentImageRequest{ReleaseName: deploymentInfo.ReleaseName, Image: imageName, AirflowVersion: "", RuntimeVersion: runtimeVersion}
	// It used to say the image was updated whether or not it was.
	if _, err = houston.Call(houstonClient.UpdateDeploymentImage)(req); err != nil {
		return Deployed{DeploymentID: deploymentID}, err
	}
	fmt.Fprintln(opts.progress(), "Image successfully updated")
	return Deployed{DeploymentID: deploymentID, Image: imageName, Dags: remoteDagsFrom(houstonClient, wsID, deploymentID, deployments, deploymentInfo)}, nil
}

// remoteDagsFrom places the DAGs of a Deployment whose image was just
// updated. The update has happened, so failing to read the cluster config
// does not fail it: the Deployment's own type is then all there is to go on,
// and a DAG-only Deployment is not placed.
func remoteDagsFrom(houstonClient houston.ClientInterface, wsID, deploymentID string, deployments []houston.Deployment, deploymentInfo *houston.Deployment) DagsFrom {
	appCfgWs := resolvedWorkspaceUUIDForAppConfig(wsID, deploymentID, deployments, deploymentInfo)
	appConfig, err := houston.Call(houstonClient.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: deploymentInfo.ClusterID, WorkspaceUUID: appCfgWs, DeploymentUUID: deploymentID})
	if err != nil {
		logger.Debugf("could not read the cluster config to place the Deployment's DAGs: %s", err.Error())
		return dagsFrom(deploymentInfo, nil)
	}
	return dagsFrom(deploymentInfo, appConfig)
}

func pushDockerImage(byoRegistryEnabled bool, deploymentInfo *houston.Deployment, byoRegistryDomain, name, nextTag, cloudDomain string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, c *config.Context, customImageName string, opts Options) (string, error) {
	var registry, remoteImage, token string
	if byoRegistryEnabled {
		registry = byoRegistryDomain
		remoteImage = fmt.Sprintf("%s:%s", registry, fmt.Sprintf("%s-%s", name, nextTag))
	} else {
		token = c.Token
		platformVersion, _ := houstonClient.GetPlatformVersion(nil) //nolint:errcheck // error deliberately ignored in this shell code
		if versions.GreaterThanOrEqualTo(platformVersion, "1.0.0") {
			var err error
			registry, err = getDeploymentRegistryURL(deploymentInfo.Urls)
			if err != nil {
				return "", err
			}
			// Switch to per deployment registry login
			err = auth.RegistryAuth(houstonClient, opts.progress(), registry, houston.GetAppConfigRequest{
				ClusterID:      deploymentInfo.ClusterID,
				WorkspaceUUID:  deploymentInfo.Workspace.ID,
				DeploymentUUID: deploymentInfo.ID,
			})
			if err != nil {
				logger.Debugf("There was an error logging into registry: %s", err.Error())
				return "", err
			}
			remoteImage = fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
		} else {
			registry = registryDomainPrefix + cloudDomain
			remoteImage = fmt.Sprintf("%s/%s", registry, airflow.ImageName(name, nextTag))
			token = c.Token
		}
	}
	// A custom image's own tag names only the source. To the APC registry it
	// is pushed as <release>/airflow:<nextTag>, as a build is: Houston deploys
	// only a push to that repository, skips a push tagged "latest", and does
	// not roll out a tag it has deployed before, so the user's tag would deploy
	// nothing for "latest" or a reused tag. It used to be pushed as
	// <registry>:<tag>, which Houston never deployed at all. A BYO registry
	// takes the image by the mutation instead, under its tag.
	if customImageName != "" && byoRegistryEnabled {
		if tagFromImageName := getGetTagFromImageName(customImageName); tagFromImageName != "" {
			remoteImage = fmt.Sprintf("%s:%s", registry, tagFromImageName)
		}
	}
	useShaAsTag := config.CFG.ShaAsTag.GetBool()
	fmt.Fprintln(opts.progress(), "Pushing image to configured registry")
	sha, err := imageHandler.Push(remoteImage, "", token, useShaAsTag)
	if err != nil {
		return "", err
	}
	if byoRegistryEnabled {
		if useShaAsTag {
			remoteImage = fmt.Sprintf("%s@%s", registry, sha)
		}
		runtimeVersion, _ := imageHandler.GetLabel("", runtimeImageLabel) //nolint:errcheck // error deliberately ignored in this shell code
		airflowVersion, _ := imageHandler.GetLabel("", airflowImageLabel) //nolint:errcheck // error deliberately ignored in this shell code
		req := houston.UpdateDeploymentImageRequest{ReleaseName: name, Image: remoteImage, AirflowVersion: airflowVersion, RuntimeVersion: runtimeVersion}
		if _, err = houston.Call(houstonClient.UpdateDeploymentImage)(req); err != nil {
			return "", err
		}
	}
	return remoteImage, nil
}

func buildDockerImageForCustomImage(imageHandler airflow.ImageHandler, customImageName string, deploymentInfo *houston.Deployment, houstonClient houston.ClientInterface, opts Options) error {
	fmt.Fprintln(opts.progress(), composeSkipImageBuildingPromptMsg)
	err := imageHandler.TagLocalImage(customImageName)
	if err != nil {
		return err
	}
	runtimeLabel, err := imageHandler.GetLabel("", airflow.RuntimeImageLabel)
	if err != nil {
		fmt.Fprintln(opts.progress(), "unable get runtime version from image")
		return err
	}
	if runtimeLabel == "" {
		return ErrNoRuntimeLabelOnCustomImage
	}
	err = validateRuntimeVersion(houstonClient, runtimeLabel, deploymentInfo, opts)
	return err
}

func buildDockerImageFromWorkingDir(path string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, deploymentInfo *houston.Deployment, ignoreCacheDeploy bool, description string, opts Options) error {
	// all these checks inside Dockerfile should happen only when no image-name is provided
	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, dockerfile))
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(path, dockerfile), err)
	}

	_, tag := docker.GetImageTagFromParsedFile(cmds)

	// Get valid image tags for platform using Deployment Info request
	err = validateRuntimeVersion(houstonClient, tag, deploymentInfo, opts)
	if err != nil {
		return err
	}
	// Build our image
	fmt.Fprintln(opts.progress(), imageBuildingPrompt)
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

	err = imageHandler.Build("", nil, buildConfig)
	return err
}

func buildDockerImage(ignoreCacheDeploy bool, deploymentInfo *houston.Deployment, customImageName, path string, imageHandler airflow.ImageHandler, houstonClient houston.ClientInterface, description string, opts Options) error {
	if customImageName == "" {
		return buildDockerImageFromWorkingDir(path, imageHandler, houstonClient, deploymentInfo, ignoreCacheDeploy, description, opts)
	}
	return buildDockerImageForCustomImage(imageHandler, customImageName, deploymentInfo, houstonClient, opts)
}

func getGetTagFromImageName(imageName string) string {
	parts := strings.Split(imageName, ":")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func buildPushDockerImage(houstonClient houston.ClientInterface, c *config.Context, deploymentInfo *houston.Deployment, name, path, nextTag, cloudDomain, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled bool, description, customImageName string, opts Options) (string, error) {
	imageName := airflow.ImageName(name, "latest")
	imageHandler := imageHandlerInit(imageName)
	if p, ok := imageHandler.(progressSetter); ok {
		p.ProgressTo(opts.progress())
	}
	err := buildDockerImage(ignoreCacheDeploy, deploymentInfo, customImageName, path, imageHandler, houstonClient, description, opts)
	if err != nil {
		return "", err
	}
	return pushDockerImage(byoRegistryEnabled, deploymentInfo, byoRegistryDomain, name, nextTag, cloudDomain, imageHandler, houstonClient, c, customImageName, opts)
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

		list := picker.List{
			Title:   fmt.Sprintf(houstonDeploymentHeader, cloudDomain) + houstonSelectDeploymentPrompt,
			Header:  []string{"LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
			Ask:     []input.Option{input.About("a deployment"), input.AnsweredBy("the deployment ID as an argument")},
			Invalid: errInvalidDeploymentSelected,
		}
		for i := range deployments {
			list.AddRow(false, deployments[i].Label, deployments[i].ReleaseName, currentWorkspace.Label, deployments[i].ID)
		}
		i, err := list.Pick(os.Stderr, os.Stdin)
		if err != nil {
			return deploymentID, deployments, err
		}
		deploymentID = deployments[i].ID
	}
	return deploymentID, deployments, nil
}

func isDagOnlyDeploymentEnabled(appConfig *houston.AppConfig) bool {
	return appConfig != nil && appConfig.Flags.DagOnlyDeployment
}

// IsDagOnlyDeployDisabledInClusterConfig reports whether err is the sentinel for DAG-only deploy
// disabled at cluster / merged-config level (either legacy or 2.0+ wording).
func IsDagOnlyDeployDisabledInClusterConfig(err error) bool {
	return errors.Is(err, ErrDagOnlyDeployDisabledInConfig) || errors.Is(err, ErrDagOnlyDeployDisabledInConfigLegacy)
}

func isHoustonPlatformVersionGTE200(version string) bool {
	if version == "" {
		return false
	}
	v := version
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if pr := semver.Prerelease(v); pr != "" {
		v = strings.TrimSuffix(v, pr)
	}
	if !semver.IsValid(v) {
		return false
	}
	return semver.Compare(v, "v2.0.0") >= 0
}

func errDagOnlyDeployDisabledAtCluster(appConfig *houston.AppConfig) error {
	if appConfig != nil && isHoustonPlatformVersionGTE200(appConfig.Version) {
		return ErrDagOnlyDeployDisabledInConfig
	}
	return ErrDagOnlyDeployDisabledInConfigLegacy
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

// resolveDagsDir returns the directory dagsPath is, its symlinks resolved:
// a dags symlink is uploaded as the directory it points at, not as a link.
// It returns ErrNoDagsDirectory when dagsPath is not there (a dangling link
// included) or not a directory, and any other failure to look at it as
// itself: a dags directory that cannot be read is not one that is missing,
// nor one with no DAGs in it.
func resolveDagsDir(dagsPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(dagsPath)
	var info fs.FileInfo
	if err == nil {
		info, err = os.Stat(resolved)
	}
	if err == nil && info.IsDir() {
		// Finding it takes only search permission; listing it takes read.
		if err = canList(resolved); err == nil {
			return resolved, nil
		}
		return "", fmt.Errorf("reading the dags directory: %w", err)
	}
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return "", fmt.Errorf("%w: %s is not a directory. Nothing was uploaded, and the Deployment keeps the Dags it had", ErrNoDagsDirectory, dagsPath)
	}
	return "", fmt.Errorf("reading the dags directory: %w", err)
}

// canList fails on a directory whose entries cannot be read. An empty one
// can be.
func canList(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// refuseDagsHoldingTarball fails when the dags directory is the directory
// the tarball is written to, or above it (a dags symlink to . or ..): the
// tarball would be archived into itself, with the whole project.
func refuseDagsHoldingTarball(dagsDir, dagsPath, tarballDir string) error {
	resolvedTarballDir, err := filepath.EvalSymlinks(tarballDir)
	if err != nil {
		return fmt.Errorf("reading the dags directory: %w", err)
	}
	rel, err := filepath.Rel(dagsPath, resolvedTarballDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("%w: %s resolves to %s, so the upload would archive the project into itself. Nothing was uploaded, and the Deployment keeps the Dags it had", ErrDagsDirHoldsProject, dagsDir, dagsPath)
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

// DagsOnlyDeploy uploads the project's DAGs to a Deployment and returns the
// Deployment it deployed to: the one named, or the one picked when none was.
// It returns that Deployment on a failure too, once it is known.
//
// Its refusals come first: a Deployment or cluster that takes no DAG upload
// is refused as such, whatever is on disk. Then, with no dags directory
// under dagsParentPath, it returns ErrNoDagsDirectory and uploads nothing:
// the empty bundle it would otherwise send deletes every DAG the Deployment
// has. A dags directory it cannot list, or that resolves to the project
// directory or above it, fails it. The bundle is made from the directory
// found, and making it fails if
// that directory has gone, so one removed meanwhile is not uploaded as no
// DAGs either.
func DagsOnlyDeploy(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, opts Options) (string, error) {
	deploymentID, deployments, err := getDeploymentIDForCurrentCommandVar(houstonClient, wsID, deploymentID, deploymentID == "")
	if err != nil {
		return deploymentID, err
	}

	if deploymentID == "" {
		return deploymentID, errInvalidDeploymentID
	}

	// Throw error if the feature is disabled at Deployment level
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return deploymentID, fmt.Errorf("failed to get deployment info: %w", err)
	}
	appCfgWs := resolvedWorkspaceUUIDForAppConfig(wsID, deploymentID, deployments, deploymentInfo)
	appConfig, err := houston.Call(houstonClient.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: deploymentInfo.ClusterID, WorkspaceUUID: appCfgWs, DeploymentUUID: deploymentID})
	if err != nil {
		return deploymentID, fmt.Errorf("%w: %w", ErrAppConfigUnread, err)
	}
	// Throw error if the feature is disabled at Houston level
	if !isDagOnlyDeploymentEnabled(appConfig) {
		return deploymentID, errDagOnlyDeployDisabledAtCluster(appConfig)
	}
	if !isDagOnlyDeploymentEnabledForDeployment(deploymentInfo) {
		return deploymentID, ErrDagOnlyDeployNotEnabledForDeployment
	}

	uploadURL := ""
	if dagDeployURL == nil {
		// Throw error if the upload URL can't be constructed
		err = validateIfDagDeployURLCanBeConstructed(deploymentInfo)
		if err != nil {
			return deploymentID, err
		}
		uploadURL = getDagDeployURL(deploymentInfo)
	} else {
		uploadURL = *dagDeployURL
	}

	dagsDir := filepath.Join(dagsParentPath, "dags")
	dagsPath, err := resolveDagsDir(dagsDir)
	if err != nil {
		return deploymentID, err
	}
	if err := refuseDagsHoldingTarball(dagsDir, dagsPath, dagsParentPath); err != nil {
		return deploymentID, err
	}
	dagsTarPath := filepath.Join(dagsParentPath, "dags.tar")
	dagsTarGzPath := dagsTarPath + ".gz"
	dagFiles, err := fileutil.FilesWithExtension(dagsPath, ".py")
	if err != nil {
		return deploymentID, fmt.Errorf("reading the dags directory: %w", err)
	}

	// Alert the user if dags folder is empty
	if len(dagFiles) == 0 && config.CFG.ShowWarnings.GetBool() && !opts.Yes {
		i, err := confirmEmptyDags("Warning: No Dags found. This will delete any existing Dags. Are you sure you want to deploy?", input.AnsweredBy("--yes"))
		if err != nil {
			return deploymentID, err
		}
		if !i {
			return deploymentID, ErrEmptyDagFolderUserCancelledOperation
		}
	}

	// Generate the dags tar, its paths under dags/ as the upload expects,
	// whatever the directory resolved to is called. It fails on a directory
	// gone since it was found, the question above included, rather than
	// making a bundle without it.
	err = fileutil.TarDir(dagsPath, dagsTarPath, "dags")
	if cleanUpFiles {
		defer os.Remove(dagsTarPath) //nolint:errcheck // best-effort cleanup
	}
	if err != nil {
		if _, gone := resolveDagsDir(dagsDir); gone != nil {
			return deploymentID, gone
		}
		return deploymentID, err
	}

	// Gzip the tar
	err = gzipFile(dagsTarPath, dagsTarGzPath)
	if err != nil {
		return deploymentID, err
	}
	if cleanUpFiles {
		defer os.Remove(dagsTarGzPath) //nolint:errcheck // best-effort cleanup
	}

	c, _ := config.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code

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
		Out:                 opts.progress(),
	}
	return deploymentID, fileutil.UploadFile(&uploadFileArgs)
}
