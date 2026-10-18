package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/versions"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// What an Astro Private Cloud manifest deploy does to an image, beyond what
// deploybuild does on every platform, lives here: which Deployment and which
// registry, Houston's tag scheme and runtime list, and how DAGs reach the
// Deployment. Building, labeling, tagging and pushing are pkg/imagebuild's,
// through internal/deploybuild, as they are for Astro.

// The labels Houston reads off a pushed image, beside the runtime version:
// skip.revision leaves a dag_deploy Deployment's DAG revision alone, as Astro
// CLI 1.x's build always asked, and the description names the deploy in the
// Deployment's history.
const (
	labelSkipRevision        = "io.astronomer.skip.revision=true"
	labelRevisionDescription = "io.astronomer.deploy.revision.description="
)

// Seams a test replaces, so no manifest deploy touches a real engine.
var (
	// newManifestCommander is the runner the engine check, the build, the
	// label reads and the push go through.
	newManifestCommander = imagebuild.NewExecCommander
	// manifestBuildNow is the build's clock.
	manifestBuildNow = time.Now
)

// ManifestImage is a pyproject.toml project's image deploy to an Astro
// Private Cloud Deployment, the Deployment already settled
// (internal/deploy's target resolution, with apc links).
type ManifestImage struct {
	DeploymentID string
	WorkspaceID  string
	// Build is the project's root and the manifest's image-deciding fields
	// (imagebuild.ManifestBuildOf).
	Build imagebuild.ManifestBuild
	// BuildSecrets are docker build --secret specs for a declared
	// Dockerfile's build.
	BuildSecrets []string
	// IncludeDags is a deploy of the image and the DAGs: the default. False
	// is --image, which leaves the Deployment's DAGs as they are and so is
	// refused for a Deployment whose DAGs are its image's.
	IncludeDags bool
	// OnBuild runs just before the build, once the Deployment has cleared the
	// deploy. nil skips it.
	OnBuild func()
	// Warn reports what the deploy goes ahead despite. nil drops it.
	Warn func(string)
	// Survey is what the generated image would carry, as the checks before
	// the deploy listed it; nil surveys the project here when it is needed.
	Survey *imagebuild.ProjectSurvey
	// Engine resolves the container engine, memoized by the caller so the
	// checks before the deploy and the build share one resolution.
	Engine      func() (bin string, env []string, err error)
	Description string
	// Progress takes the build's output, the registry push's and the DAG
	// upload's; nil is stderr.
	Progress io.Writer
	// Catalog is how the public runtime catalog is read for a generated
	// build: the Python a runtime build runs, and an Airflow 2 runtime the
	// platform lists none of.
	Catalog runtimeversions.Options
}

// ManifestImageResult is what a manifest image deploy shipped.
type ManifestImageResult struct {
	WorkspaceID string
	// RuntimeVersion is the runtime the pushed image carries (its label).
	RuntimeVersion string
	// Image is the reference pushed: <registry>/<release>/airflow:<tag>, or
	// the BYO registry's.
	Image string
	// URL is the Deployment's Airflow UI, "" when Houston gives none.
	URL string
	// DagsUploaded is a deploy that uploaded the project's DAGs beside the
	// image (a dag_deploy Deployment).
	DagsUploaded bool
	// DagsInImage is a deploy whose DAGs are the image's: an image
	// Deployment runs the DAGs the image carries.
	DagsInImage bool
	// DagsNotShipped is a deploy of the image and the DAGs that shipped no
	// DAGs: the Deployment takes them from git-sync or a volume, or turned
	// out to take no upload, or there was no dags directory to upload. Warn
	// said why.
	DagsNotShipped bool
	// Airflow3Upgrade is a deploy that moves the Deployment from Airflow 2
	// to Airflow 3.
	Airflow3Upgrade bool
}

// manifestTarget is the Deployment a manifest deploy ships to, as Houston
// describes it.
type manifestTarget struct {
	release   string
	nextTag   string
	info      *houston.Deployment
	appConfig *houston.AppConfig
	// workspace is the workspace Houston places the Deployment in, which its
	// app config and runtime list are scoped to.
	workspace string
	dags      DagsFrom
	// byo is a cluster whose Deployments take images from a registry of the
	// customer's own, through the updateDeploymentImage mutation.
	byo bool
}

// DeployManifestImage builds a project's image, holds its runtime to what
// Houston refuses of a pushed image, pushes it to the Deployment's registry,
// and delivers the DAGs the way the Deployment takes them: inside the image
// for an image Deployment, as an upload for a dag_deploy one, and not at all
// for git-sync or volume.
//
// What Houston would refuse after the push, where the deploy never sees its
// answer, is refused before it: a downgrade (Airflow 3 to 2 included) and a
// runtime newer than the newest it lists. A runtime it does not list
// otherwise deploys, with a warning. The image is built at linux/amd64, since
// Houston drops an arm image.
func DeployManifestImage(client houston.ClientInterface, in *ManifestImage) (ManifestImageResult, error) {
	ctx := context.Background()
	progress := in.Progress
	if progress == nil {
		progress = os.Stderr
	}
	warn := in.Warn
	if warn == nil {
		warn = func(string) {}
	}

	// The Deployment is read first: a deploy it refuses outright needs no
	// container engine to be told so.
	t, err := lookUpManifestTarget(client, in.WorkspaceID, in.DeploymentID)
	if err != nil {
		return ManifestImageResult{}, err
	}
	if !in.IncludeDags && t.dags == DagsFromImage {
		return ManifestImageResult{}, fmt.Errorf("%w: Deployment %s runs the Dags inside its image, so an image without them would leave it none. Run astro deploy without --image to deploy the image with the project's Dags inside it", ErrDeploymentTypeIncorrectForImageOnly, in.DeploymentID)
	}

	cmd := newManifestCommander()
	bin, env, err := deploybuild.EnsureEngine(ctx, cmd, in.Engine)
	if err != nil {
		return ManifestImageResult{}, err
	}

	listed := listedRuntimes(client, t.info.ClusterID, t.workspace, in.DeploymentID)
	req, err := imagebuild.ForManifestWith(ctx, in.Build, houstonBase(listed, in.Catalog))
	if err != nil {
		return ManifestImageResult{}, err
	}
	planned := deploybuild.PlanRuntime("", &req, &in.Build)
	allowed := listed.versions()
	current := t.info.RuntimeVersion
	if err := planned.CheckListed(current, allowed); err != nil {
		return ManifestImageResult{}, err
	}
	if in.IncludeDags && t.dags == DagsFromImage {
		if err := checkDagsInImage(in, &req, func() bool { return imagebuild.IsPodman(ctx, cmd, bin, env) }, warn); err != nil {
			return ManifestImageResult{}, err
		}
	}

	if in.OnBuild != nil {
		in.OnBuild()
	}
	engine := imagebuild.Engine{Cmd: cmd, Bin: bin, Env: env}
	localImage, runtimeVersion, err := buildManifestImage(ctx, in, &req, engine, progress)
	if err != nil {
		return ManifestImageResult{}, err
	}
	unlisted, err := planned.CheckBuiltListed(current, runtimeVersion, allowed)
	if err != nil {
		return ManifestImageResult{}, err
	}
	if unlisted != "" {
		warn(unlisted)
	}
	upgrade := deploybuild.Airflow3Upgrade(current, runtimeVersion)
	if upgrade {
		warn(fmt.Sprintf("this deploy upgrades Deployment %s from Airflow 2 to Airflow 3 (Astro Runtime %s to %s)", in.DeploymentID, current, runtimeVersion))
	}

	fmt.Fprintf(progress, houstonDeploymentPrompt, t.release)
	c, _ := config.GetCurrentContext() //nolint:errcheck // the target lookup has already read it
	pushed, err := pushManifestImage(ctx, client, &c, t, engine, localImage, runtimeVersion, in.Description, progress)
	if err != nil {
		return ManifestImageResult{}, err
	}

	res := ManifestImageResult{
		WorkspaceID:     in.WorkspaceID,
		RuntimeVersion:  runtimeVersion,
		Image:           pushed,
		URL:             getAirflowUILink(in.DeploymentID, t.info.Urls),
		Airflow3Upgrade: upgrade,
	}
	if !in.IncludeDags {
		return res, nil
	}
	err = deliverDags(client, in, t, &res, progress, warn)
	return res, err
}

// deliverDags gets the project's DAGs to a Deployment whose image was just
// pushed, the way it takes them, and records in res what it did.
func deliverDags(client houston.ClientInterface, in *ManifestImage, t *manifestTarget, res *ManifestImageResult, progress io.Writer, warn func(string)) error {
	switch t.dags {
	case DagsFromImage:
		res.DagsInImage = true
		return nil
	case DagsFromElsewhere:
		res.DagsNotShipped = true
		warn(fmt.Sprintf("Deployment %s takes its Dags from %s, so this deploy shipped the image alone and the Dags are as they were", in.DeploymentID, dagsSource(t.info, t.appConfig)))
		return nil
	case DagsFromUpload, DagsFromUnknown:
	}
	_, err := DagsOnlyDeploy(client, in.WorkspaceID, in.DeploymentID, in.Build.ProjectDir, nil, true, in.Description, Options{Progress: progress, WarnEmptyDags: warn})
	switch {
	case err == nil:
		res.DagsUploaded = true
		return nil
	case t.dags == DagsFromUnknown && (IsDagOnlyDeployDisabledInClusterConfig(err) || errors.Is(err, ErrDagOnlyDeployNotEnabledForDeployment)):
		// A Deployment whose DAG source could not be read, which turns out
		// to take no upload: the image shipped alone.
		res.DagsNotShipped = true
		warn(fmt.Sprintf("Deployment %s takes no Dag upload (%v), so this deploy shipped the image alone and the Dags are the ones it runs from elsewhere", in.DeploymentID, err))
		return nil
	case errors.Is(err, ErrNoDagsDirectory):
		res.DagsNotShipped = true
		warn(fmt.Sprintf("no Dags were uploaded: there is no dags directory in %s, and the Deployment keeps the Dags it had", in.Build.ProjectDir))
		return nil
	}
	return &DagsAfterImageError{Image: res.Image, DeploymentID: in.DeploymentID, Err: err}
}

// DagsAfterImageError is a DAG upload that failed after the image was
// pushed: the Deployment runs the new image with the DAGs it had.
type DagsAfterImageError struct {
	// Image is the image pushed.
	Image        string
	DeploymentID string
	Err          error
}

func (e *DagsAfterImageError) Error() string {
	return fmt.Sprintf("the image %s was pushed to Deployment %s, but uploading the Dags failed: %v. The Deployment runs the new image with the Dags it had; to upload them, run astro deploy %s --dags",
		e.Image, e.DeploymentID, e.Err, e.DeploymentID)
}

func (e *DagsAfterImageError) Unwrap() error { return e.Err }

// dagsSource names where a Deployment that takes no DAGs from a deploy gets
// them.
func dagsSource(info *houston.Deployment, appConfig *houston.AppConfig) string {
	switch info.DagDeployment.Type {
	case houston.GitSyncDeploymentType:
		return "a git repository (git-sync)"
	case houston.VolumeDeploymentType:
		return "a volume"
	case houston.DagOnlyDeploymentType:
		if !isDagOnlyDeploymentEnabled(appConfig) {
			return "Dag-only deploys, which its cluster has turned off"
		}
	}
	return "elsewhere"
}

// lookUpManifestTarget reads the Deployment a manifest deploy ships to: that
// it is in the workspace, its release name and next registry tag, its
// cluster config, and where it takes its DAGs from.
func lookUpManifestTarget(client houston.ClientInterface, wsID, deploymentID string) (*manifestTarget, error) {
	if deploymentID == "" {
		return nil, errInvalidDeploymentID
	}
	_, deployments, err := getDeploymentIDForCurrentCommand(client, wsID, deploymentID, false)
	if errors.Is(err, errInvalidDeploymentID) {
		return nil, fmt.Errorf("no Deployment %s in workspace %s: name a Deployment of the workspace, or the workspace it is in with --workspace", deploymentID, wsID)
	}
	if err != nil {
		return nil, err
	}
	t := &manifestTarget{}
	for i := range deployments {
		if deployments[i].ID == deploymentID {
			t.release, t.nextTag = deployments[i].ReleaseName, deployments[i].DeploymentInfo.NextCli
		}
	}
	t.info, err = houston.Call(client.GetDeployment)(deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment info: %w", err)
	}
	t.workspace = resolvedWorkspaceUUIDForAppConfig(wsID, deploymentID, deployments, t.info)
	t.appConfig, err = houston.Call(client.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: t.info.ClusterID, WorkspaceUUID: t.workspace, DeploymentUUID: deploymentID})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAppConfigUnread, err)
	}
	if t.appConfig != nil && t.appConfig.Flags.BYORegistryEnabled {
		if t.appConfig.BYORegistryDomain == "" {
			return nil, ErrBYORegistryDomainNotSet
		}
		// Houston keeps no next tag for a BYO registry.
		t.byo, t.nextTag = true, "deploy-"+time.Now().UTC().Format("2006-01-02T15-04")
	}
	t.dags = dagsFrom(t.info, t.appConfig)
	return t, nil
}

// listedReleases are the runtime builds the platform lists for a
// Deployment's cluster.
type listedReleases houston.RuntimeReleases

// versions are the runtime versions listed.
func (l listedReleases) versions() []string {
	out := make([]string, 0, len(l))
	for i := range l {
		out = append(out, l[i].Version)
	}
	return out
}

// listedRuntimes reads the runtime builds Houston lists for a Deployment
// (houston-api's runtimeReleases query): stable, not yanked, Airflow 3 only
// where it is on, at or above its minimum, and, unless the platform lists
// every runtime, the newest build of each Airflow version. From Houston
// 2.0.0 the query takes the workspace and the Deployment, so their overrides
// of those settings apply, as they do to the app config
// (houston.GetRuntimeReleases leaves them out before it). A Houston too old
// to list runtimes (before 0.29.0), and one whose list cannot be read, list
// none, and the deploy then holds the image to the downgrade rule alone.
func listedRuntimes(client houston.ClientInterface, clusterID, workspaceID, deploymentID string) listedReleases {
	vars := map[string]interface{}{}
	for k, v := range map[string]string{"clusterId": clusterID, "workspaceUuid": workspaceID, "deploymentUuid": deploymentID} {
		if v != "" {
			vars[k] = v
		}
	}
	releases, err := houston.Call(client.GetRuntimeReleases)(vars)
	if err != nil {
		return nil
	}
	return listedReleases(releases)
}

// newestFor is the newest runtime build listed that carries the Airflow the
// pin names: that exact version for a pin with a patch ("2.10.3"), any of its
// series for one without ("3.1"). "" when none is listed.
func (l listedReleases) newestFor(pin string) string {
	newest := ""
	for i := range l {
		av := l[i].AirflowVersion
		if av != pin && !strings.HasPrefix(av, pin+".") && !strings.HasPrefix(av, pin+"+") {
			continue
		}
		if newest == "" || airflowversions.CompareRuntimeVersions(l[i].Version, newest) > 0 {
			newest = l[i].Version
		}
	}
	return newest
}

// houstonBase resolves a generated build's runtime base by local Docker
// mode's rule (imagebuild.LocalRuntimeImageWith), so Airflow 2 from 2.7 and
// Airflow 3 both build. With no [tool.astro] runtime it builds on the newest
// runtime the platform lists for the pin, so the image carries a runtime
// Houston accepts, which an air-gapped cluster can answer and the public
// catalog may not. When the platform lists none, an Airflow 3 pin builds on
// its series tag and an Airflow 2 one on the newest runtime the public
// catalog has; with that unreadable too, the deploy is refused, naming
// [tool.astro] runtime.
func houstonBase(listed listedReleases, o runtimeversions.Options) imagebuild.BaseImageFunc {
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		runtime := m.Runtime
		if runtime == "" {
			runtime = listed.newestFor(m.AirflowVersion)
		}
		base, err := imagebuild.LocalRuntimeImageWith(ctx, m.AirflowVersion, runtime, m.RequiresPython, o)
		if err != nil && runtime == "" && strings.HasPrefix(m.AirflowVersion, "2") {
			return "", fmt.Errorf("the platform lists no Astro Runtime for Airflow %s, and the public runtime catalog gave none either: set [tool.astro] runtime in pyproject.toml to the runtime build to deploy: %w", m.AirflowVersion, err)
		}
		return base, err
	}
}

// buildManifestImage builds the image a manifest deploy pushes, at
// linux/amd64 with the labels Houston reads, and returns its local tag and
// the runtime its label carries.
func buildManifestImage(ctx context.Context, in *ManifestImage, req *imagebuild.Request, engine imagebuild.Engine, progress io.Writer) (localImage, runtimeVersion string, err error) {
	readLabel := func(image, label string) (string, error) {
		return engine.Label(ctx, image, label)
	}
	req.Secrets = in.BuildSecrets
	req.Bin, req.Env = engine.Bin, engine.Env
	req.Labels = append(req.Labels, labelSkipRevision)
	if in.Description != "" {
		req.Labels = append(req.Labels, labelRevisionDescription+in.Description)
	}
	return deploybuild.BuildImage(ctx, &deploybuild.Image{
		Request:    *req,
		ProjectDir: in.Build.ProjectDir,
		Dockerfile: in.Build.Dockerfile,
		Commander:  engine.Cmd,
		Now:        manifestBuildNow,
		Progress:   progress,
		ReadLabel:  readLabel,
	})
}

// checkDagsInImage holds a deploy to an image Deployment, whose DAGs are the
// image's, to an image that carries them. A generated image whose
// .dockerignore leaves out every DAG file under dags/ is refused: it would
// replace the running DAGs with none. A declared Dockerfile's is warned
// about, since its own steps may put DAGs in the image.
func checkDagsInImage(in *ManifestImage, req *imagebuild.Request, podman func() bool, warn func(string)) error {
	survey := in.Survey
	if survey == nil {
		s, err := imagebuild.SurveyProject(in.Build.ProjectDir, in.Build.Dockerfile, podman())
		if err != nil {
			return err
		}
		survey = &s
	}
	if survey.DagsOnDisk == 0 || survey.DagsCarried > 0 {
		return nil
	}
	if req.FromDeclaredDockerfile() {
		warn(fmt.Sprintf("Deployment %s runs only the Dags inside its image, and the ignore file %s's build reads leaves out every Dag file under dags/. The Deployment will run whatever Dags %s itself puts in the image",
			in.DeploymentID, in.Build.Dockerfile, in.Build.Dockerfile))
		return nil
	}
	return fmt.Errorf("Deployment %s runs the Dags inside its image, but the project's .dockerignore leaves out every Dag file under dags/. Remove the rule that drops them", in.DeploymentID)
}

// pushManifestImage pushes localImage to the registry the Deployment takes
// its image from, under Houston's tag scheme, and returns the reference
// pushed. The platform registry (the Deployment's own from 1.0.0, registry.
// <domain> before it) takes the push under the release's repository with
// the context's token, carried by the push alone: nothing is written to the
// user's container config. A BYO registry takes the push on the engine's own
// login, and the image by the updateDeploymentImage mutation, which reads no
// labels, so the runtime and the deploy's description go with it.
func pushManifestImage(ctx context.Context, client houston.ClientInterface, c *config.Context, t *manifestTarget, engine imageEngine, localImage, runtimeVersion, description string, progress io.Writer) (string, error) {
	var registry, remoteImage string
	push := imagebuild.PushRequest{Source: localImage, Digest: config.CFG.ShaAsTag.GetBool(), Out: progress}
	if t.byo {
		registry = t.appConfig.BYORegistryDomain
		remoteImage = fmt.Sprintf("%s:%s-%s", registry, t.release, t.nextTag)
	} else {
		platformVersion, _ := client.GetPlatformVersion(nil) //nolint:errcheck // an unread version is a pre-1.0 platform, as APC deploy has always taken it
		if versions.GreaterThanOrEqualTo(platformVersion, "1.0.0") {
			var err error
			if registry, err = getDeploymentRegistryURL(t.info.Urls); err != nil {
				return "", err
			}
		} else {
			registry = registryDomainPrefix + c.Domain
		}
		remoteImage = registry + "/" + airflowImage(t.release, t.nextTag)
		push.Username, push.Secret = registryUser, c.Token
	}
	push.Image = remoteImage
	fmt.Fprintln(progress, "Pushing image to configured registry")
	sha, err := engine.Push(ctx, push)
	if err != nil {
		return "", deploybuild.ExplainPush(err)
	}
	if !t.byo {
		return remoteImage, nil
	}
	if push.Digest {
		remoteImage = registry + "@" + sha
	}
	airflowVersion, _ := engine.Label(ctx, localImage, airflowImageLabel) //nolint:errcheck // the mutation takes an empty one, as APC deploy has always sent it
	req := houston.UpdateDeploymentImageRequest{ReleaseName: t.release, Image: remoteImage, AirflowVersion: airflowVersion, RuntimeVersion: runtimeVersion, DeployRevisionDescription: description}
	if _, err := houston.Call(client.UpdateDeploymentImage)(req); err != nil {
		return "", err
	}
	return remoteImage, nil
}
