package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/api/types/versions"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
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

// ManifestImage is an image deploy to an Astro Private Cloud Deployment,
// the Deployment already settled (internal/deploy's target resolution, with
// apc links): a pyproject.toml project's image, built from its manifest, or
// a prebuilt one (ImageName).
type ManifestImage struct {
	DeploymentID string
	WorkspaceID  string
	// ImageName is a prebuilt image to deploy instead of building one
	// (--image-name): a local image, pushed as a built one is, or, with
	// InRegistry, one already in a registry, which the Deployment is pointed
	// at as it is. "" builds from Build.
	ImageName string
	// InRegistry is an ImageName already in a registry (--remote): nothing
	// is built, read or pushed, and Houston's updateDeploymentImage points
	// the Deployment at it.
	InRegistry bool
	// RuntimeVersion is the Astro Runtime an InRegistry image carries
	// (--runtime-version): no local label can say.
	RuntimeVersion string
	// ImageAlone is a prebuilt image deployed from outside any project: no
	// DAGs go with it, and the deploy says what that leaves the Deployment.
	ImageAlone bool
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

	if in.InRegistry {
		return deployRegistryImage(client, in, progress, warn)
	}

	// The Deployment is read first: a deploy it refuses outright needs no
	// container engine to be told so.
	t, err := lookUpManifestTarget(client, in.WorkspaceID, in.DeploymentID)
	if err != nil {
		return ManifestImageResult{}, hintLinkOutsideAProject(in, err)
	}
	if err := refuseImageOnly(in, t); err != nil {
		return ManifestImageResult{}, err
	}

	cmd := newManifestCommander()
	bin, env, err := deploybuild.EnsureEngine(ctx, cmd, in.Engine)
	if err != nil {
		return ManifestImageResult{}, err
	}

	listed := listedRuntimes(client, t.info.ClusterID, t.workspace, in.DeploymentID)
	// A prebuilt image is not built, so it has no request: its runtime is
	// its label's, read below.
	var req imagebuild.Request
	if in.ImageName == "" {
		if req, err = imagebuild.ForManifestWith(ctx, in.Build, houstonBase(listed, in.Catalog, warn)); err != nil {
			return ManifestImageResult{}, err
		}
	}
	planned := deploybuild.PlanRuntime(in.ImageName, &req, &in.Build)
	allowed := listed.versions()
	current := t.info.RuntimeVersion
	if err := planned.CheckListed(current, allowed); err != nil {
		return ManifestImageResult{}, err
	}
	if in.ImageName == "" && in.IncludeDags && t.dags == DagsFromImage {
		if err := checkDagsInImage(in, &req, func() bool { return imagebuild.IsPodman(ctx, cmd, bin, env) }, warn); err != nil {
			return ManifestImageResult{}, err
		}
	}

	engine := imagebuild.Engine{Cmd: cmd, Bin: bin, Env: env}
	localImage, runtimeVersion, err := manifestDeployImage(ctx, in, &req, engine, progress)
	if err != nil {
		return ManifestImageResult{}, err
	}
	upgrade, err := checkRuntime(in, &planned, current, runtimeVersion, allowed, warn)
	if err != nil {
		return ManifestImageResult{}, err
	}

	fmt.Fprintf(progress, houstonDeploymentPrompt, t.release)
	c, _ := config.GetCurrentContext() //nolint:errcheck // the target lookup has already read it
	pushed, err := pushManifestImage(ctx, client, &c, t, engine, &pushedImage{local: localImage, named: in.ImageName, runtime: runtimeVersion}, in.Description, progress)
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
	err = dagsAfterImage(client, in, t, &res, progress, warn)
	return res, err
}

// refuseImageOnly refuses --image to a Deployment whose DAGs are its
// image's: the image is all the DAGs it would have. An image deployed alone,
// from outside any project, is not refused, as on Astro: no project's DAGs
// are left stale, and the deploy warns that the Deployment runs the image's.
func refuseImageOnly(in *ManifestImage, t *manifestTarget) error {
	if in.IncludeDags || in.ImageAlone || t.dags != DagsFromImage {
		return nil
	}
	return fmt.Errorf("%w: Deployment %s runs the Dags inside its image, so an image without them would leave it none. Run astro deploy without --image to deploy the image with the project's Dags inside it", ErrDeploymentTypeIncorrectForImageOnly, in.DeploymentID)
}

// manifestDeployImage is the local image a deploy pushes, and the runtime
// its label carries: the prebuilt one named, or the one built from the
// manifest, OnBuild called first.
func manifestDeployImage(ctx context.Context, in *ManifestImage, req *imagebuild.Request, engine imagebuild.Engine, progress io.Writer) (localImage, runtimeVersion string, err error) {
	if in.ImageName != "" {
		runtimeVersion, err = deploybuild.PrebuiltRuntime(in.ImageName, func(image, label string) (string, error) {
			return engine.Label(ctx, image, label)
		})
		return in.ImageName, runtimeVersion, err
	}
	if in.OnBuild != nil {
		in.OnBuild()
	}
	return buildManifestImage(ctx, in, req, engine, progress)
}

// checkRuntime holds the runtime the image carries to what Houston refuses
// of it after the push, and reports an Airflow 2 to 3 upgrade, which it warns
// of. A build is held to deploybuild's CheckBuiltListed: a downgrade, Airflow
// 3 where the platform lists none, and a runtime newer than the newest
// listed are refused, and any other runtime the platform does not list is
// warned of. A prebuilt image (ImageName, local or in a registry) is held to
// CheckPrebuiltListed, which refuses only the downgrade and warns of the
// rest: its runtime is what it is, and the platform may take it.
func checkRuntime(in *ManifestImage, planned *deploybuild.Plan, current, runtimeVersion string, allowed []string, warn func(string)) (upgrade bool, err error) {
	check := planned.CheckBuiltListed
	if in.ImageName != "" {
		check = planned.CheckPrebuiltListed
	}
	unlisted, err := check(current, runtimeVersion, allowed)
	if err != nil {
		return false, err
	}
	if unlisted != "" {
		warn(unlisted)
	}
	upgrade = deploybuild.Airflow3Upgrade(current, runtimeVersion)
	if upgrade {
		warn(fmt.Sprintf("this deploy upgrades Deployment %s from Airflow 2 to Airflow 3 (Astro Runtime %s to %s)", in.DeploymentID, current, runtimeVersion))
	}
	return upgrade, nil
}

// dagsAfterImage gets DAGs to a Deployment whose image was just deployed:
// the project's, as deliverDags delivers them, for a deploy of the image and
// the DAGs; none for --image; and none for an image deployed alone, which
// says what that leaves the Deployment.
func dagsAfterImage(client houston.ClientInterface, in *ManifestImage, t *manifestTarget, res *ManifestImageResult, progress io.Writer, warn func(string)) error {
	switch {
	case in.ImageAlone:
		warnImageAlone(in, t, warn)
		return nil
	case !in.IncludeDags:
		return nil
	}
	return deliverDags(client, in, t, res, progress, warn)
}

// The warnings a prebuilt image's deploy gives about the Deployment's DAGs.
const (
	// warningPrebuiltDagsInImage: the Deployment runs the DAGs inside its
	// image, and nothing here put any there: a prebuilt image was not built
	// from the project, if there is one. The image may well carry DAGs (one
	// built from a Dockerfile that copies them does); one astro package
	// generated the build for does not.
	warningPrebuiltDagsInImage = "this Deployment runs the Dags inside the image %s; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."
	// noticeImageAlone: an image deployed from outside any project ships
	// alone, to a Deployment that takes DAG uploads, so its DAGs are as they
	// were.
	noticeImageAlone = "Dags were not updated: this deploy ran outside a pyproject.toml project, so it shipped the image alone, and the Deployment keeps the Dags it had. To upload them, run astro deploy %s --dags from the project directory."
	// noticeImageAloneUnplaced: the same, to a Deployment whose DAG source
	// could not be read.
	noticeImageAloneUnplaced = "this deploy ran outside a pyproject.toml project, so it shipped the image alone. If this Deployment takes Dag uploads, it keeps the Dags it had: to upload them, run astro deploy %s --dags from the project directory."
	// noticeDagsUndecided: the image was deployed, and the cluster config
	// that says whether the Deployment takes DAG uploads could not be read,
	// so an upload that may have been due did not happen.
	noticeDagsUndecided = "Dags were NOT updated: whether this Deployment takes Dag uploads could not be read (%v). The image was deployed, and the Deployment keeps the Dags it had. To upload them, run astro deploy %s --dags from the project directory."
)

// warnImageAlone says what an image deployed from outside any project leaves
// the Deployment's DAGs: those inside the image, for an image Deployment
// (unless show_warnings is off), and the ones it had, for one that takes
// DAG uploads or may, which is said whatever show_warnings is: an upload
// the Deployment would take did not happen.
func warnImageAlone(in *ManifestImage, t *manifestTarget, warn func(string)) {
	switch t.dags {
	case DagsFromImage:
		warnPrebuiltDagsInImage(in, warn)
	case DagsFromUpload:
		warn(fmt.Sprintf(noticeImageAlone, in.DeploymentID))
	case DagsFromUnknown:
		warn(fmt.Sprintf(noticeImageAloneUnplaced, in.DeploymentID))
	case DagsFromElsewhere:
	}
}

// warnPrebuiltDagsInImage warns that an image Deployment runs only the DAGs
// a prebuilt image carries, unless show_warnings is off.
func warnPrebuiltDagsInImage(in *ManifestImage, warn func(string)) {
	if config.CFG.ShowWarnings.GetBool() {
		warn(fmt.Sprintf(warningPrebuiltDagsInImage, in.ImageName))
	}
}

// deliverDags gets the project's DAGs to a Deployment whose image was just
// pushed, the way it takes them, and records in res what it did.
func deliverDags(client houston.ClientInterface, in *ManifestImage, t *manifestTarget, res *ManifestImageResult, progress io.Writer, warn func(string)) error {
	switch t.dags {
	case DagsFromImage:
		res.DagsInImage = true
		if in.ImageName != "" {
			// A built image carries the project's DAGs; a prebuilt one
			// carries whatever it carries.
			warnPrebuiltDagsInImage(in, warn)
		}
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
	case t.dags == DagsFromUnknown && errors.Is(err, ErrAppConfigUnread):
		// Whether the Deployment takes uploads at all could not be read,
		// after its image was deployed: the deploy stands, and an upload
		// that may have been due did not happen.
		res.DagsNotShipped = true
		warn(fmt.Sprintf(noticeDagsUndecided, err, in.DeploymentID))
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
	t, err := lookUpDeployment(client, wsID, deploymentID)
	if err != nil {
		return nil, err
	}
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

// notInWorkspaceError is a deploy to a Deployment the workspace does not
// hold.
type notInWorkspaceError struct{ deployment, workspace string }

func (e *notInWorkspaceError) Error() string {
	return fmt.Sprintf("no Deployment %s in workspace %s: name a Deployment of the workspace, or the workspace it is in with --workspace", e.deployment, e.workspace)
}

// houstonID is the shape of a Houston id: a cuid, "c" and 24 more lowercase
// letters and digits.
var houstonID = regexp.MustCompile(`^c[a-z0-9]{24}$`)

// hintLinkOutsideAProject adds to a Deployment the workspace does not hold,
// named from outside any project in a shape no Houston id has, that it was
// read as an id: there are no links to resolve without a project, so a link
// name works only from the project's directory.
func hintLinkOutsideAProject(in *ManifestImage, err error) error {
	var notIn *notInWorkspaceError
	if !in.ImageAlone || !errors.As(err, &notIn) || houstonID.MatchString(in.DeploymentID) {
		return err
	}
	return fmt.Errorf("%w. %q is not shaped like a Deployment id: outside a pyproject.toml project there are no links, so a link name resolves only when the deploy runs from the project's directory", err, in.DeploymentID)
}

// lookUpDeployment reads what every image deploy needs of its Deployment:
// that it is in the workspace, its release name and next registry tag, and
// the Deployment itself, with the workspace Houston places it in.
func lookUpDeployment(client houston.ClientInterface, wsID, deploymentID string) (*manifestTarget, error) {
	if deploymentID == "" {
		return nil, errInvalidDeploymentID
	}
	deployments, err := workspaceDeployments(client, wsID, deploymentID)
	if errors.Is(err, errInvalidDeploymentID) {
		return nil, &notInWorkspaceError{deployment: deploymentID, workspace: wsID}
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

// listedReleases is APC's deploybuild.RuntimeOffer.
var _ deploybuild.RuntimeOffer = listedReleases(nil)

// BuildsFor are the runtime builds listed that carry the Airflow the pin
// names, newest first: that exact version for a pin with a patch ("2.10.3"),
// any of its series for one without ("3.1"). None for a bare "3", which every
// other path refuses to resolve.
func (l listedReleases) BuildsFor(pin string) []string {
	pin = strings.TrimSpace(pin)
	if pin == "" || pin == "3" {
		return nil
	}
	var out []string
	for i := range l {
		if manifest.PinCovers(pin, l[i].AirflowVersion) {
			out = append(out, l[i].Version)
		}
	}
	slices.SortFunc(out, func(a, b string) int { return airflowversions.CompareRuntimeVersions(b, a) })
	return out
}

// Offers reports whether the platform lists build.
func (l listedReleases) Offers(build string) bool {
	return slices.ContainsFunc(l, func(r houston.RuntimeRelease) bool { return r.Version == build })
}

// houstonBase resolves a generated build's runtime base by local Docker
// mode's rule (imagebuild.LocalRuntimeImageWith), so Airflow 2 from 2.7 and
// Airflow 3 both build. With no [tool.astro] runtime it builds on the newest
// runtime the platform lists for the pin, so the image carries a runtime
// Houston accepts, which an air-gapped cluster can answer and the public
// catalog may not. When the platform lists none, an Airflow 3 pin builds on
// its series tag and an Airflow 2 one on the newest runtime the public
// catalog has; with that unreadable too, the deploy is refused, naming
// [tool.astro] runtime. The offer half is deploybuild.OfferedBase, the rule
// Astro's deploy follows over its own offer.
func houstonBase(listed listedReleases, o runtimeversions.Options, warn func(string)) imagebuild.BaseImageFunc {
	return deploybuild.OfferedBase(listed, func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		base, err := imagebuild.LocalRuntimeImageWith(ctx, m.AirflowVersion, m.Runtime, m.RequiresPython, o)
		if err != nil && m.Runtime == "" && strings.HasPrefix(m.AirflowVersion, "2") {
			return "", fmt.Errorf("the platform lists no Astro Runtime for Airflow %s, and the public runtime catalog gave none either: set [tool.astro] runtime in pyproject.toml to the runtime build to deploy: %w", m.AirflowVersion, err)
		}
		return base, err
	}, warn)
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
func pushManifestImage(ctx context.Context, client houston.ClientInterface, c *config.Context, t *manifestTarget, engine imageEngine, img *pushedImage, description string, progress io.Writer) (string, error) {
	var registry, remoteImage string
	push := imagebuild.PushRequest{Source: img.local, Digest: config.CFG.ShaAsTag.GetBool(), Out: progress}
	if t.byo {
		registry = t.appConfig.BYORegistryDomain
		remoteImage = fmt.Sprintf("%s:%s-%s", registry, t.release, t.nextTag)
		// A prebuilt image keeps its own tag in a BYO registry, as Astro CLI
		// 1.x pushed it: the mutation names the image, so the tag need not
		// be Houston's.
		if tag := getGetTagFromImageName(img.named); tag != "" {
			remoteImage = registry + ":" + tag
		}
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
	airflowVersion, _ := engine.Label(ctx, img.local, airflowImageLabel) //nolint:errcheck // the mutation takes an empty one, as APC deploy has always sent it
	req := houston.UpdateDeploymentImageRequest{ReleaseName: t.release, Image: remoteImage, AirflowVersion: airflowVersion, RuntimeVersion: img.runtime, DeployRevisionDescription: description}
	if _, err := houston.Call(client.UpdateDeploymentImage)(req); err != nil {
		return "", explainUpdateImageErr(err, t.appConfig, false)
	}
	return remoteImage, nil
}

// pushedImage is the image pushManifestImage pushes: the local image, the
// prebuilt image it is when one was named ("" for one built), and the
// runtime its label carries.
type pushedImage struct {
	local, named, runtime string
}
