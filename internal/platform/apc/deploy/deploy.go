package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/mod/semver"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	gzipFile = fileutil.GzipFile

	workspaceDeploymentsVar = workspaceDeployments
)

var (
	ErrNoWorkspaceID       = errors.New("no workspace id provided")
	errNoDomainSet         = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentID = errors.New("please specify a valid deployment ID")
	// ErrDagOnlyDeployDisabledInConfigLegacy is returned for Houston before 2.0.0 (flat feature-flag paths).
	ErrDagOnlyDeployDisabledInConfigLegacy = errors.New("to perform this operation, set both deployments.dagOnlyDeployment and deployments.configureDagDeployment to true in your APC cluster")
	// ErrDagOnlyDeployDisabledInConfig is returned for Houston 2.0.0+ (deployMechanisms.*.enabled under merged deployments config).
	ErrDagOnlyDeployDisabledInConfig        = errors.New("to perform this operation, set both deployments.deployMechanisms.dagOnlyDeployment.enabled and deployments.deployMechanisms.configureDagDeployment.enabled to true in your APC cluster")
	ErrDagOnlyDeployNotEnabledForDeployment = errors.New("to perform this operation, first set the Deployment type to 'dag_deploy' via the UI or the API or the CLI")
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
	ErrBYORegistryDomainNotSet             = errors.New("Custom registry host is not set in config. It can be set at astronomer.houston.config.registry.protectedCustomRegistry.updateRegistry.host")
	ErrDeploymentTypeIncorrectForImageOnly = errors.New("--image only works for Dag-only, Git-sync-based and NFS-based deployments")
)

const (
	houstonDeploymentPrompt = "Deploying: %s\n"

	registryDomainPrefix = "registry."
	airflowImageLabel    = "io.astronomer.docker.airflow.version"
)

// registryUser is the username the APC registry takes the context's token
// under, as auth.RegistryAuth logs in with it.
const registryUser = "user"

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
	// WarnEmptyDags reports a DAGs folder with no DAGs in it, as a deploy
	// does on Astro: the upload goes ahead. nil says nothing.
	WarnEmptyDags func(string)
}

func (o Options) progress() io.Writer {
	if o.Progress != nil {
		return o.Progress
	}
	return os.Stdout
}

// imageEngine is what a deploy does to an image on the container engine:
// pkg/imagebuild's Engine, which tests replace.
type imageEngine interface {
	Tag(ctx context.Context, src, dst string) error
	Label(ctx context.Context, image, label string) (string, error)
	Push(ctx context.Context, req imagebuild.PushRequest) (string, error)
}

// airflowImage is the APC image reference of a release: <release>/airflow:<tag>.
func airflowImage(release, tag string) string {
	return release + "/airflow:" + tag
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

func getGetTagFromImageName(imageName string) string {
	parts := strings.Split(imageName, ":")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
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

// workspaceDeployments lists the Deployments of workspace wsID, which must
// hold deploymentID: a deploy's Deployment is settled before it gets here
// (internal/deploy's target resolution), and this holds it to the workspace
// the deploy runs in.
func workspaceDeployments(houstonClient houston.ClientInterface, wsID, deploymentID string) ([]houston.Deployment, error) {
	if wsID == "" {
		return nil, ErrNoWorkspaceID
	}
	if deploymentID == "" {
		return nil, errInvalidDeploymentID
	}

	// Validate workspace
	currentWorkspace, err := houston.Call(houstonClient.GetWorkspace)(wsID)
	if err != nil {
		return nil, err
	}

	deployments, err := houston.Call(houstonClient.ListDeployments)(houston.ListDeploymentsRequest{WorkspaceID: currentWorkspace.ID})
	if err != nil {
		return deployments, err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return deployments, err
	}
	if c.Domain == "" {
		return deployments, errNoDomainSet
	}

	if !deploymentExists(deploymentID, deployments) {
		return deployments, errInvalidDeploymentID
	}
	return deployments, nil
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

// DagsOnlyDeploy uploads the project's DAGs to a Deployment, settled before
// it is called and held to the workspace, and returns that Deployment, on a
// failure too.
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
	deployments, err := workspaceDeploymentsVar(houstonClient, wsID, deploymentID)
	if err != nil {
		return deploymentID, err
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
	if len(dagFiles) == 0 && opts.WarnEmptyDags != nil {
		opts.WarnEmptyDags("no Dags found in the dags folder: the upload replaces the Deployment's Dags with none")
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
