package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/containercfg"
	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ManifestImageDeployInput is the resolved input for a project's image deploy. The
// deployment is already chosen (internal/deploy). The image is either built from
// the manifest fields or adopted from a prebuilt local image (ImageName); a
// "both" deploy (IncludeDags) also ships the dags/ tarball.
type ManifestImageDeployInput struct {
	// Login is the Astro login the deploy runs under, whose host the
	// Deployment and its registry live on; nil is the current context.
	Login *config.Context
	// Build is the project's root (Build.ProjectDir, absolute, which the
	// commit lookup and the dags upload use too) and the manifest's
	// image-deciding fields, as imagebuild.ManifestBuildOf reads them.
	Build imagebuild.ManifestBuild
	// Catalog reads the runtime catalog for the image's Python, and for an
	// Airflow 2 base when the Deployment offers no runtime for the pin
	// (astroBase). nil, or a nil catalog, builds on the runtime's default
	// Python, and then refuses an Airflow 2 project with no [tool.astro]
	// runtime and nothing offered.
	Catalog      func() *runtimeversions.Catalog
	DeploymentID string
	// BuildSecrets are docker build --secret specs for a declared Dockerfile's
	// build (Build.Dockerfile).
	BuildSecrets []string
	ImageName    string // a prebuilt local image (--image-name); "" builds from the manifest
	// OnBuild runs once the deployment has cleared the deploy and just before
	// the image build starts, so a refused deploy says nothing about building.
	// A prebuilt image is not built and never calls it. nil skips it.
	OnBuild func()
	// IncludeDags marks a "both" deploy: the image and the project's DAGs.
	// To a Deployment with DAG deploys on, the DAGs are uploaded beside the
	// image. To one with them off, the image is the only way DAGs reach it,
	// so the deploy ships the image alone and its DAGs are those inside it,
	// as the 1.x CLI deployed there.
	IncludeDags bool
	// ImageAlone marks a prebuilt image deployed from outside any project.
	// It ships no DAGs, and unlike --image it is not refused by a Deployment
	// with DAG deploys off: there is no project whose DAGs it would leave
	// stale, and the caller warns that the Deployment runs the image's.
	ImageAlone bool
	// Warn reports a finding the deploy goes ahead despite, as a sentence.
	// nil drops it.
	Warn func(string)
	// Survey is what the image built from the manifest would carry, as the
	// checks before the deploy listed it (imagebuild.SurveyProject). nil
	// surveys the project here when it is needed.
	Survey *imagebuild.ProjectSurvey
	// Engine resolves the container engine, memoized by the caller so the
	// checks before the deploy and the build share one resolution. nil is
	// ResolveContainerEngine.
	Engine        func() (bin string, env []string, err error)
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
	// Progress takes the image build's output and the Wait progress; nil is
	// stderr.
	Progress io.Writer
}

// ManifestImageDeployResult reports the outcome for cmd to render.
type ManifestImageDeployResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	// DagsInImage reports a "both" deploy to a Deployment with DAG deploys
	// off: no DAGs were uploaded, and the Deployment runs those the image
	// carries.
	DagsInImage bool
	// RemoteExecution reports a "both" deploy to a remote-execution
	// Deployment with DAG deploys off: no DAGs were uploaded, as the 1.x CLI
	// uploaded none there.
	RemoteExecution bool
	URL             string
	Git             ManifestDeployGit
	// DagDeployEnabled is whether the Deployment takes DAG deploys.
	DagDeployEnabled bool
	// RemoteExecutionEnabled is whether the Deployment runs remote
	// execution, whatever this deploy shipped (RemoteExecution is the narrower
	// "both" deploy that uploaded no DAGs because of it).
	RemoteExecutionEnabled bool
}

// Seams, replaced in tests so no deploy touches a real daemon or registry.
var (
	// resolveContainerEngine picks the container CLI binary and the env that
	// reaches its daemon. It errors when no supported engine is on PATH.
	resolveContainerEngine = defaultResolveContainerEngine
	// newImageBuildCommander returns the runner the build and the daemon probe
	// stream through.
	newImageBuildCommander = imagebuild.NewExecCommander
	// buildNow is the build clock; a var so tests keep log timestamps fixed.
	buildNow = time.Now
)

// DeployManifestImage builds (or adopts) a project's image, pushes it to the
// deployment's registry, and finalizes; a "both" deploy also uploads the dags/
// tarball. It uses the shared transport (createDeploy, the registry push
// through pkg/imagebuild on the engine the image was built on, uploadDags,
// finalize) and, like DeployManifestDags, neither prints nor exits — it
// returns a result for cmd to render, and writes only the image build's
// output, the push's progress and a --wait's progress, to in.Progress.
//
// Docker is required and checked before any transport work (docs/deploy.md, section 5).
//
//nolint:gocritic // value input keeps this seam symmetric with DeployManifestDags
func DeployManifestImage(in ManifestImageDeployInput, astroV1Client astrov1.APIClient) (ManifestImageDeployResult, error) {
	ctx := context.Background()

	cmd := newImageBuildCommander()
	resolve := in.Engine
	if resolve == nil {
		resolve = ResolveContainerEngine
	}
	bin, env, err := deploybuild.EnsureEngine(ctx, cmd, resolve)
	if err != nil {
		return ManifestImageDeployResult{}, err
	}

	c, err := loginOrCurrent(in.Login)
	if err != nil {
		return ManifestImageDeployResult{}, err
	}

	// A prebuilt image need not match the working tree, so it records no commit.
	var gitInfo ManifestDeployGit
	var commitMessage string
	if in.ImageName == "" {
		gitInfo, commitMessage = readManifestDeployGit(in.Build.ProjectDir)
	}

	// The deployment comes first: its rules are checked before the build,
	// which can take minutes, and the runtimes it offers decide a generated
	// build's base.
	dep, offer, err := checkDeployment(ctx, &c, &in, astroV1Client)
	if err != nil {
		return ManifestImageDeployResult{}, err
	}
	allowed := offer.versions()

	var req imagebuild.Request
	if in.ImageName == "" {
		// Which image the manifest builds is imagebuild's rule, shared with
		// every other consumer that builds from a manifest, over a base the
		// Deployment offers (deploybuild.OfferedBase).
		req, err = imagebuild.ForManifestWith(ctx, in.Build, astroBase(offer, in.Catalog))
		if err != nil {
			return ManifestImageDeployResult{}, err
		}
	}
	planned := deploybuild.PlanRuntime(in.ImageName, &req, &in.Build)

	// Checked again on the built image's label, which is the authority.
	if err := planned.Check(dep.AstroRuntimeVersion, allowed); err != nil {
		return ManifestImageDeployResult{}, err
	}
	dags, err := planDags(&in, &req, &dep, func() bool { return imagebuild.IsPodman(ctx, cmd, bin, env) })
	if err != nil {
		return ManifestImageDeployResult{}, err
	}

	if in.ImageName == "" && in.OnBuild != nil {
		in.OnBuild()
	}
	engine := imagebuild.Engine{Cmd: cmd, Bin: bin, Env: env}
	localImage, runtimeVersion, err := prepareDeployImage(ctx, &in, &req, engine)
	if err != nil {
		return ManifestImageDeployResult{}, err
	}
	if err := planned.CheckBuilt(dep.AstroRuntimeVersion, runtimeVersion, allowed); err != nil {
		return ManifestImageDeployResult{}, err
	}

	deployType := astrov1.CreateDeployRequestTypeIMAGEONLY
	if in.IncludeDags {
		deployType = astrov1.CreateDeployRequestTypeIMAGEANDDAG
	}
	description := descriptionOrCommitMessage(in.Description, commitMessage)
	created, err := createDeploy(dep.OrganizationId, dep.Id, astrov1.CreateDeployRequest{
		Description: &description,
		Type:        deployType,
		Git:         gitInfo.Commit,
	}, astroV1Client)
	if err != nil {
		return ManifestImageDeployResult{}, explainHibernating(err, &dep)
	}
	if created.ImageRepository == "" || created.ImageTag == "" {
		return ManifestImageDeployResult{}, errors.New("no image repository or tag received from Astro")
	}

	// Push the built/adopted image under the returned repository:tag with the
	// existing registry auth (CLI username + context token), on the engine it
	// was built on.
	// Its progress goes where the build's went, never to stdout, which under
	// --output json carries the result alone.
	remoteImage := fmt.Sprintf("%s:%s", created.ImageRepository, created.ImageTag)
	progress := in.Progress
	if progress == nil {
		progress = os.Stderr
	}
	if _, err := engine.Push(ctx, imagebuild.PushRequest{
		Source:   localImage,
		Image:    remoteImage,
		Username: registryUsername,
		Secret:   c.Token,
		Out:      progress,
	}); err != nil {
		return ManifestImageDeployResult{}, deploybuild.ExplainPush(err)
	}

	// A "both" deploy also ships the dags tarball, fitting the image just
	// pushed, unless the DAGs are inside the image.
	var tarballVersion string
	if dags.upload {
		tarballVersion, err = uploadDeployDags(&c, in.Build.ProjectDir, in.DeploymentID, &dep, created, in.NoDagsBaseDir)
		if err != nil {
			return ManifestImageDeployResult{}, err
		}
	}

	if err := finalizeManifestDeploy(dep.OrganizationId, dep.Id, created.Id, tarballVersion, astroV1Client); err != nil {
		return ManifestImageDeployResult{}, err
	}

	if in.Wait {
		if err := deployment.HealthPollIn(in.Progress, dep.OrganizationId, c.Token, dep.Id, sleepTime, tickNum, int(in.WaitTime.Seconds()), astroV1Client); err != nil {
			return ManifestImageDeployResult{}, err
		}
	}

	return ManifestImageDeployResult{
		WorkspaceID:            dep.WorkspaceId,
		RuntimeVersion:         runtimeVersion,
		ImageTag:               created.ImageTag,
		DagTarballVersion:      tarballVersion,
		DagsInImage:            dags.inImage,
		RemoteExecution:        dags.remote,
		URL:                    dashboardURL(c.Domain, dep.Id, dep.WorkspaceId),
		Git:                    gitInfo,
		DagDeployEnabled:       dep.IsDagDeployEnabled,
		RemoteExecutionEnabled: deployment.IsRemoteExecutionEnabled(&dep),
	}, nil
}

// checkDeployment fetches the deployment and confirms the deploy is allowed:
// cicd enforcement, DAG deploys enabled for an image-only (--image) deploy,
// which leaves the running DAGs in place and so needs them to live outside
// the image, and then that the Deployment is not hibernating. It also
// returns the runtimes the deployment offers, which a generated build starts
// from and the image's runtime is checked against.
func checkDeployment(ctx context.Context, c *config.Context, in *ManifestImageDeployInput, astroV1Client astrov1.APIClient) (astrov1.Deployment, offeredReleases, error) {
	dep, err := deployment.GetDeploymentByID(c.Organization, in.DeploymentID, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, nil, err
	}
	if dep.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return astrov1.Deployment{}, nil, fmt.Errorf(errCiCdEnforcementUpdate, dep.Name)
	}
	if !in.IncludeDags && !in.ImageAlone && !dep.IsDagDeployEnabled && !deployment.IsRemoteExecutionEnabled(&dep) {
		// As the 1.x CLI refused it: with DAG deploys off the DAGs live in
		// the image, so there are no running DAGs for --image to leave alone.
		// Under remote execution they live with the remote agents instead.
		return astrov1.Deployment{}, nil, fmt.Errorf(imageOnlyNeedsDagDeployMsg, in.DeploymentID)
	}
	// Astro refuses a hibernating Deployment's deploy when it is created, and
	// that comes after the build: refuse here, on the status Astro refuses
	// on, before the build's minutes are spent. After the two above, which
	// waking would not clear, so the wake-up step never hides them.
	if dep.Status == astrov1.DeploymentStatusHIBERNATING {
		return astrov1.Deployment{}, nil, &hibernatingError{deploymentID: dep.Id, name: dep.Name, refusal: instancelocate.ErrDeploymentHibernating}
	}
	offer, err := offeredRuntimes(ctx, dep.OrganizationId, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, nil, err
	}
	return dep, offer, nil
}

// imageOnlyNeedsDagDeployMsg refuses --image to a Deployment with DAG deploys
// off.
const imageOnlyNeedsDagDeployMsg = "--image deploys the image and leaves the running Dags in place, but astro deployment %[1]s has Dag deploys off, so its Dags are the ones inside the image. Run 'astro deploy' without --image to deploy the image with the project's Dags inside it, or enable Dag deploys with 'astro deployment update %[1]s --dag-deploy enable'"

// dagsPlan is how a deploy's DAGs reach the Deployment.
type dagsPlan struct {
	// upload sends the dags/ tarball beside the image: a "both" deploy to a
	// Deployment with DAG deploys on.
	upload bool
	// inImage leaves the DAGs to the image: a "both" deploy to one with them
	// off.
	inImage bool
	// remote uploads nothing and asks nothing of the image: a "both" deploy
	// to a remote-execution Deployment with DAG deploys off, whose DAGs live
	// with its remote agents.
	remote bool
}

// planDags works out how a deploy's DAGs reach the Deployment dep, and holds a
// deploy whose DAGs go inside the image to an image that carries them
// (checkDagsInImage).
func planDags(in *ManifestImageDeployInput, req *imagebuild.Request, dep *astrov1.Deployment, podman func() bool) (dagsPlan, error) {
	if !in.IncludeDags {
		return dagsPlan{}, nil
	}
	if dep.IsDagDeployEnabled {
		return dagsPlan{upload: true}, nil
	}
	if deployment.IsRemoteExecutionEnabled(dep) {
		return dagsPlan{remote: true}, nil
	}
	return dagsPlan{inImage: true}, checkDagsInImage(in, req, podman)
}

// checkDagsInImage holds a "both" deploy to a Deployment with DAG deploys off,
// where the image is the only way DAGs reach it, to an image that carries
// them. A generated image whose .dockerignore leaves out every DAG file under
// dags/ is refused: it would replace the running DAGs with none. A declared
// Dockerfile's is warned about instead, since its own steps may put DAGs in
// the image, and so is a prebuilt image, which cannot be looked into here.
func checkDagsInImage(in *ManifestImageDeployInput, req *imagebuild.Request, podman func() bool) error {
	warn := in.Warn
	if warn == nil {
		warn = func(string) {}
	}
	if in.ImageName != "" {
		warn(fmt.Sprintf("astro deployment %s has Dag deploys off, so it will run the Dags inside %s; no Dags are uploaded", in.DeploymentID, in.ImageName))
		return nil
	}
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
		warn(fmt.Sprintf("astro deployment %s has Dag deploys off, so it will run only the Dags inside the image, and the ignore file %s's build reads leaves out every Dag file under dags/. The Deployment will run whatever Dags %s itself puts in the image",
			in.DeploymentID, in.Build.Dockerfile, in.Build.Dockerfile))
		return nil
	}
	return fmt.Errorf("astro deployment %[1]s has Dag deploys off, so its Dags have to be inside the image, but the project's .dockerignore leaves out every Dag file under dags/. Remove the rule that drops them, or enable Dag deploys with 'astro deployment update %[1]s --dag-deploy enable'", in.DeploymentID)
}

// uploadDeployDags tars and uploads a project's dags/ directory to the
// created deploy's upload URL and returns the tarball version. Both manifest paths use
// it: the dags-only deploy and a "both" image deploy, which ships the tarball to
// fit the image just pushed.
func uploadDeployDags(c *config.Context, projectDir, deploymentID string, dep *astrov1.Deployment, created *astrov1.Deploy, noDagsBaseDir bool) (string, error) {
	uploadURL := ""
	if created.DagsUploadUrl != nil {
		uploadURL = *created.DagsUploadUrl
	}
	if uploadURL == "" {
		return "", errors.New("no Dag upload URL received from Astro")
	}
	var deploymentType astrov1.DeploymentType
	if dep.Type != nil {
		deploymentType = *dep.Type
	}
	dagsPath := filepath.Join(projectDir, "dags")
	monitoringDag := includeMonitoringDag(c.OrganizationProduct == string(astrov1.OrganizationProductHOSTED), deploymentType)
	tarballVersion, err := uploadDags(dagsPath, uploadURL, dep.AstroRuntimeVersion, monitoringDag, noDagsBaseDir)
	if err != nil {
		if strings.Contains(err.Error(), dagDeployDisabled) {
			return "", fmt.Errorf(enableDagDeployMsg, deploymentID)
		}
		return "", err
	}
	return tarballVersion, nil
}

// prepareDeployImage returns the local image to push and its runtime version,
// refusing a Remote Execution agent image: one is built FROM Runtime, so it
// carries the runtime label all the same.
func prepareDeployImage(ctx context.Context, in *ManifestImageDeployInput, req *imagebuild.Request, engine imagebuild.Engine) (localImage, runtimeVersion string, err error) {
	localImage, runtimeVersion, err = buildOrAdoptImage(ctx, in, req, engine)
	if err != nil {
		return "", "", err
	}
	if err := refuseAgentImage(ctx, engine, localImage); err != nil {
		return "", "", err
	}
	return localImage, runtimeVersion, nil
}

// buildOrAdoptImage returns the local image to push and its runtime version.
// With a prebuilt image (--image-name) it validates the image exists locally and
// carries a runtime label; otherwise it builds req, ForManifest's request, at
// linux/amd64 (deploybuild.BuildImage).
func buildOrAdoptImage(ctx context.Context, in *ManifestImageDeployInput, req *imagebuild.Request, engine imagebuild.Engine) (localImage, runtimeVersion string, err error) {
	readLabel := func(image, label string) (string, error) {
		return engine.Label(ctx, image, label)
	}
	if in.ImageName != "" {
		version, err := deploybuild.PrebuiltRuntime(in.ImageName, readLabel)
		if err != nil {
			return "", "", err
		}
		return in.ImageName, version, nil
	}
	req.Secrets = in.BuildSecrets
	req.Bin = engine.Bin
	req.Env = engine.Env
	return deploybuild.BuildImage(ctx, &deploybuild.Image{
		Request:    *req,
		ProjectDir: in.Build.ProjectDir,
		Dockerfile: in.Build.Dockerfile,
		Commander:  engine.Cmd,
		Now:        buildNow,
		Progress:   in.Progress,
		ReadLabel:  readLabel,
	})
}

// offeredRuntimes fetches the runtime releases the organization's
// deployments offer. Asked for the organization, as the runtime check always
// has: asked for one deployment's ID the options may list fewer runtimes (its
// upgrade targets, say), and that is not yet verified to keep the runtime it
// runs, which a redeploy has to pass.
func offeredRuntimes(ctx context.Context, organizationID string, astroV1Client astrov1.APIClient) (offeredReleases, error) {
	resp, err := astroV1Client.GetDeploymentOptionsWithResponse(ctx, organizationID, &astrov1.GetDeploymentOptionsParams{})
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return offeredReleases(resp.JSON200.RuntimeReleases), nil
}

// offeredReleases are the runtime releases an Astro deployment offers, and
// Astro's deploybuild.RuntimeOffer.
type offeredReleases []astrov1.RuntimeRelease

var _ deploybuild.RuntimeOffer = offeredReleases(nil)

// versions are the offered runtime versions, which the image's runtime is
// checked against.
func (o offeredReleases) versions() []string {
	out := make([]string, 0, len(o))
	for i := range o {
		out = append(out, o[i].Version)
	}
	return out
}

// NewestFor is the newest offered build whose Airflow release the pin covers:
// that exact release for a pin with a patch ("2.10.3"), any release of the
// series or generation for one without ("2.10", "2"). Astro names an Astronomer
// build of Airflow 3 with a local version ("3.1.8+astro.4"), which its release
// covers. "" when none is offered.
//
// Only a stable or deprecated build is a base nobody chose: a preview channel
// is the user's to opt into, by [tool.astro] runtime. Deprecated stays, as
// most Airflow 2 runtimes are. And a bare "3" offers nothing: every other
// path refuses that pin rather than pick an Airflow 3 series for the project,
// so the deploy refuses it the same way.
func (o offeredReleases) NewestFor(pin string) string {
	pin = strings.TrimSpace(pin)
	if pin == "" || pin == "3" {
		return ""
	}
	newest := ""
	for i := range o {
		if ch := o[i].Channel; ch != "" && ch != "stable" && ch != "deprecated" {
			continue
		}
		av := o[i].AirflowVersion
		if av != pin && !strings.HasPrefix(av, pin+".") && !strings.HasPrefix(av, pin+"+") {
			continue
		}
		if newest == "" || airflowversions.CompareRuntimeVersions(o[i].Version, newest) > 0 {
			newest = o[i].Version
		}
	}
	return newest
}

// astroBase resolves a generated build's runtime base for an Astro deploy:
// the newest runtime the deployment offers for the pin (deploybuild.
// OfferedBase), so the image carries a runtime the deployment takes, and with
// none offered imagebuild's rule over the public catalog: the series tag for
// Airflow 3, the catalog's newest released runtime for Airflow 2, refused
// when the catalog cannot be read.
//
// An offered Airflow 3 build that ships no Python requires-python admits is
// not the project's fault: a newer build of the series may ship one, as it
// did before the offer was read, so the catalog's rule decides instead.
func astroBase(offer offeredReleases, catalog func() *runtimeversions.Catalog) imagebuild.BaseImageFunc {
	fallback := imagebuild.CatalogBase(catalog)
	offered := deploybuild.OfferedBase(offer, func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		base, err := fallback(ctx, m)
		if err != nil && m.Runtime == "" && isAirflow2(m.AirflowVersion) && len(offer) > 0 {
			return "", fmt.Errorf("the Deployment offers no Astro Runtime for Airflow %s, so the base was looked up in the public runtime catalog, which gave none either: %w", m.AirflowVersion, err)
		}
		return base, err
	})
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		base, err := offered(ctx, m)
		var notShipped *runtimeversions.PythonNotShippedError
		if m.Runtime == "" && errors.As(err, &notShipped) {
			return fallback(ctx, m)
		}
		return base, err
	}
}

// isAirflow2 reports an Airflow 2 pin.
func isAirflow2(pin string) bool {
	pin = strings.TrimSpace(pin)
	return pin == "2" || strings.HasPrefix(pin, "2.")
}

// finalizeManifestDeploy marks a manifest deploy final. It carries the dag tarball version
// only when one exists (a "both" or dags-only deploy), so an image-only deploy
// finalizes with an empty request. It prints nothing, so the deploy renders in
// cmd and stays ready for --output json.
func finalizeManifestDeploy(organizationID, deploymentID, deployID, dagTarballVersion string, astroV1Client astrov1.APIClient) error {
	req := astrov1.FinalizeDeployRequest{}
	if dagTarballVersion != "" {
		req.DagTarballVersion = &dagTarballVersion
	}
	resp, err := astroV1Client.FinalizeDeployWithResponse(context.Background(), organizationID, deploymentID, deployID, req)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// ResolveContainerEngine resolves the container CLI and the env that reaches
// its daemon, through the seam tests replace (resolveContainerEngine). cmd
// memoizes it for a deploy's checks and its build to share.
func ResolveContainerEngine() (bin string, env []string, err error) {
	return resolveContainerEngine()
}

// defaultResolveContainerEngine picks the engine the way local Docker mode and
// `astro package` do: the container.binary pin when set (the working
// directory's project config over global), otherwise pkg/container's
// PATH/OrbStack resolution, plus its connection env.
func defaultResolveContainerEngine() (bin string, env []string, err error) {
	return containercfg.Engine(config.WorkingPath)
}
