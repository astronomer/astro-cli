package deploy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/containercfg"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// ImageDeployV2Input is the resolved input for a v2 project's image deploy. The
// deployment is already chosen (internal/deploy). The image is either built from
// the manifest fields or adopted from a prebuilt local image (ImageName); a
// "both" deploy (IncludeDags) also ships the dags/ tarball.
type ImageDeployV2Input struct {
	// Login is the Astro login the deploy runs under, whose host the
	// Deployment and its registry live on; nil is the current context.
	Login          *config.Context
	ProjectDir     string
	DeploymentID   string
	AirflowVersion string   // the manifest's Airflow requirement pin; the base resolves from it
	Runtime        string   // the manifest's [tool.astro] runtime build, "" for the series' newest
	Dependencies   []string // manifest [project] dependencies
	Packages       []string // manifest [tool.astro] packages
	// Dockerfile is the manifest's [tool.astro] dockerfile, slash-separated and
	// relative to ProjectDir. Set means the project's own file is the build.
	Dockerfile string
	// BuildSecrets are docker build --secret specs for that file's build.
	BuildSecrets []string
	ImageName    string // a prebuilt local image (--image-name); "" builds from the manifest
	// OnBuild runs once the deployment has cleared the deploy and just before
	// the image build starts, so a refused deploy says nothing about building.
	// A prebuilt image is not built and never calls it. nil skips it.
	OnBuild       func()
	IncludeDags   bool // also upload dags/ — a "both" deploy
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
}

// ImageDeployV2Result reports the outcome for cmd to render.
type ImageDeployV2Result struct {
	WorkspaceID       string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	URL               string
	Git               DeployGitV2
}

// errNoDocker is the plain, actionable message for the no-Docker user
// (docs/v2-deploy.md, section 5). An image deploy needs a container builder;
// dags-only does not.
var errNoDocker = errors.New("an image deploy needs Docker, but no running container engine was found. Start Docker and try again, or run 'astro deploy --dags' to deploy just your DAGs (no Docker needed). Server-side builds are coming")

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

// DeployImageV2 builds (or adopts) a v2 project's image, pushes it to the
// deployment's registry, and finalizes; a "both" deploy also uploads the dags/
// tarball. It reuses the v1 transport (createDeploy, the registry push in
// airflow.DockerImage.Push, deployDags, finalize) and, like DeployDagsV2,
// neither prints nor exits — it returns a result for cmd to render.
//
// Docker is required and checked before any transport work (docs/v2-deploy.md, section 5).
//
//nolint:gocritic // value input keeps this seam symmetric with DeployDagsV2
func DeployImageV2(in ImageDeployV2Input, astroV1Client astrov1.APIClient) (ImageDeployV2Result, error) {
	ctx := context.Background()

	cmd := newImageBuildCommander()
	bin, env, err := ensureContainerEngine(ctx, cmd)
	if err != nil {
		return ImageDeployV2Result{}, err
	}

	c, err := loginOrCurrent(in.Login)
	if err != nil {
		return ImageDeployV2Result{}, err
	}

	// A prebuilt image need not match the working tree, so it records no commit.
	var gitInfo DeployGitV2
	var commitMessage string
	if in.ImageName == "" {
		gitInfo, commitMessage = readDeployGitV2(in.ProjectDir)
	}

	var req imagebuild.Request
	if in.ImageName == "" {
		// Which image the manifest builds is imagebuild's rule, shared with
		// every other consumer that builds from a manifest.
		req, err = imagebuild.ForManifest(imagebuild.ManifestBuild{
			ProjectDir:     in.ProjectDir,
			AirflowVersion: in.AirflowVersion,
			Runtime:        in.Runtime,
			Dockerfile:     in.Dockerfile,
			Dependencies:   in.Dependencies,
			Packages:       in.Packages,
		})
		if err != nil {
			return ImageDeployV2Result{}, err
		}
	}
	planned := planRuntime(&in, &req)

	// The deployment's rules are checked before the build, which can take
	// minutes, and again on the built image's label, which is the authority.
	dep, allowed, err := checkDeployment(ctx, &c, &in, astroV1Client)
	if err != nil {
		return ImageDeployV2Result{}, err
	}
	if err := checkPlannedRuntime(dep.AstroRuntimeVersion, &planned, allowed); err != nil {
		return ImageDeployV2Result{}, err
	}

	if in.ImageName == "" && in.OnBuild != nil {
		in.OnBuild()
	}
	localImage, runtimeVersion, err := prepareDeployImage(ctx, &in, &req, cmd, bin, env)
	if err != nil {
		return ImageDeployV2Result{}, err
	}
	if err := checkRuntimeVersion(dep.AstroRuntimeVersion, runtimeVersion, allowed, planned.raise); err != nil {
		return ImageDeployV2Result{}, err
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
		return ImageDeployV2Result{}, explainHibernating(err, &dep)
	}
	if created.ImageRepository == "" || created.ImageTag == "" {
		return ImageDeployV2Result{}, errors.New("no image repository or tag received from Astro")
	}

	// Push the built/adopted image under the returned repository:tag with the
	// existing registry auth (CLI username + context token).
	remoteImage := fmt.Sprintf("%s:%s", created.ImageRepository, created.ImageTag)
	if _, err := airflowImageHandler(localImage).Push(remoteImage, registryUsername, c.Token, false); err != nil {
		return ImageDeployV2Result{}, err
	}

	// A "both" deploy also ships the dags tarball, fitting the image just pushed.
	var tarballVersion string
	if in.IncludeDags {
		tarballVersion, err = uploadDeployDags(&c, in.ProjectDir, in.DeploymentID, &dep, created, in.NoDagsBaseDir)
		if err != nil {
			return ImageDeployV2Result{}, err
		}
	}

	if err := finalizeDeployV2(dep.OrganizationId, dep.Id, created.Id, tarballVersion, astroV1Client); err != nil {
		return ImageDeployV2Result{}, err
	}

	if in.Wait {
		if err := deployment.HealthPollIn(dep.OrganizationId, c.Token, dep.Id, sleepTime, tickNum, int(in.WaitTime.Seconds()), astroV1Client); err != nil {
			return ImageDeployV2Result{}, err
		}
	}

	return ImageDeployV2Result{
		WorkspaceID:       dep.WorkspaceId,
		RuntimeVersion:    runtimeVersion,
		ImageTag:          created.ImageTag,
		DagTarballVersion: tarballVersion,
		URL:               dashboardURL(c.Domain, dep.Id, dep.WorkspaceId),
		Git:               gitInfo,
	}, nil
}

// checkDeployment fetches the deployment and confirms the deploy is allowed:
// cicd enforcement and dag-deploy enablement for a "both" deploy. It also
// returns the runtime versions the deployment offers, which the image's
// runtime is checked against.
func checkDeployment(ctx context.Context, c *config.Context, in *ImageDeployV2Input, astroV1Client astrov1.APIClient) (astrov1.Deployment, []string, error) {
	dep, err := deployment.GetDeploymentByID(c.Organization, in.DeploymentID, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, nil, err
	}
	if dep.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return astrov1.Deployment{}, nil, fmt.Errorf(errCiCdEnforcementUpdate, dep.Name)
	}
	if in.IncludeDags && !dep.IsDagDeployEnabled {
		return astrov1.Deployment{}, nil, fmt.Errorf(enableDagDeployMsg, in.DeploymentID)
	}
	allowed, err := offeredRuntimeVersions(ctx, dep.OrganizationId, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, nil, err
	}
	return dep, allowed, nil
}

// uploadDeployDags tars and uploads a v2 project's dags/ directory to the
// created deploy's upload URL and returns the tarball version. Both v2 paths use
// it: the dags-only deploy and a "both" image deploy, which ships the tarball to
// fit the image just pushed.
func uploadDeployDags(c *config.Context, projectDir, deploymentID string, dep *astrov1.Deployment, created *astrov1.Deploy, noDagsBaseDir bool) (string, error) {
	uploadURL := ""
	if created.DagsUploadUrl != nil {
		uploadURL = *created.DagsUploadUrl
	}
	if uploadURL == "" {
		return "", errors.New("no DAG upload URL received from Astro")
	}
	var deploymentType astrov1.DeploymentType
	if dep.Type != nil {
		deploymentType = *dep.Type
	}
	dagsPath := filepath.Join(projectDir, "dags")
	monitoringDag := includeMonitoringDag(c.OrganizationProduct == string(astrov1.OrganizationProductHOSTED), deploymentType)
	tarballVersion, err := uploadDags(projectDir, dagsPath, uploadURL, dep.AstroRuntimeVersion, monitoringDag, noDagsBaseDir)
	if err != nil {
		if strings.Contains(err.Error(), dagDeployDisabled) {
			return "", fmt.Errorf(enableDagDeployMsg, deploymentID)
		}
		return "", err
	}
	return tarballVersion, nil
}

// prepareDeployImage returns the local image to push and its runtime version.
// With a prebuilt image (--image-name) it validates the image exists locally and
// carries a runtime label; otherwise it builds req, ForManifest's request, at
// linux/amd64.
func prepareDeployImage(ctx context.Context, in *ImageDeployV2Input, req *imagebuild.Request, cmd imagebuild.Commander, bin string, env []string) (localImage, runtimeVersion string, err error) {
	if in.ImageName != "" {
		// A registry image name says nothing about its base, so read the label
		// off the local image: docker inspect fails when it is not present, which
		// is the "not found locally" signal.
		version, err := airflowImageHandler(in.ImageName).GetLabel("", runtimeImageLabel)
		if err != nil {
			return "", "", fmt.Errorf("image %q was not found locally or could not be inspected; build it or 'docker load' it first: %w", in.ImageName, err)
		}
		if version == "" {
			return "", "", fmt.Errorf("image %q is not based on Astro Runtime (the %s label is missing); build it FROM the Astro runtime", in.ImageName, runtimeImageLabel)
		}
		return in.ImageName, version, nil
	}

	workDir, err := os.MkdirTemp("", "astro-deploy-build-*")
	if err != nil {
		return "", "", fmt.Errorf("creating a build directory: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck // best-effort cleanup of a temp dir

	req.WorkDir = workDir
	req.Tag = deployImageTag(in.ProjectDir)
	req.Secrets = in.BuildSecrets
	req.Platform = deployImagePlatformSupport[0]
	req.Bin = bin
	req.Env = env
	// BuildLocal, not Build: the image is inspected, tagged and pushed next, so
	// it has to be a single-platform image in the local store even when there
	// is nothing to install. See BuildLocal for why a pulled base is not.
	built, err := imagebuild.New(cmd, buildNow).BuildLocal(ctx, *req, localrt.Callbacks{})
	if err != nil {
		return "", "", err
	}

	version, err := airflowImageHandler(built).GetLabel("", runtimeImageLabel)
	if err != nil {
		return "", "", fmt.Errorf("reading the runtime version off the built image: %w", err)
	}
	if version == "" {
		// Two different mistakes, so two different messages. A generated build
		// missing the label means the base we chose is wrong, which is ours. A
		// declared Dockerfile missing it means the user's own FROM is not an
		// Astro Runtime, and naming the base there would name an empty string
		// and send them at a decision they did not make.
		if req.FromDeclaredDockerfile() {
			return "", "", fmt.Errorf("the image built from %s is missing the %s label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image", in.Dockerfile, runtimeImageLabel)
		}
		return "", "", fmt.Errorf("the built image is missing the %s label; the runtime base %s should carry it", runtimeImageLabel, req.BaseImage)
	}
	return built, version, nil
}

// deployImageTag is the local tag the built deploy image carries. It appends a
// hash of the full project path (same idea as localdocker's composeProjectName),
// so two same-named projects in different directories never share a tag.
func deployImageTag(projectDir string) string {
	label := filepath.Base(projectDir)
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = "project"
	}
	if id, err := localrt.ProjectID(projectDir); err == nil && len(id) >= 6 {
		return "astro-deploy/" + label + "-" + id[:6]
	}
	return "astro-deploy/" + label
}

// offeredRuntimeVersions fetches the runtime versions the organization's
// deployments offer.
func offeredRuntimeVersions(ctx context.Context, organizationID string, astroV1Client astrov1.APIClient) ([]string, error) {
	resp, err := astroV1Client.GetDeploymentOptionsWithResponse(ctx, organizationID, &astrov1.GetDeploymentOptionsParams{})
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	allowed := make([]string, 0, len(resp.JSON200.RuntimeReleases))
	for _, r := range resp.JSON200.RuntimeReleases {
		allowed = append(allowed, r.Version)
	}
	return allowed, nil
}

// plannedRuntime is the Astro Runtime a deploy's image will carry, as far as
// the project says before anything is built, and the change that moves it.
type plannedRuntime struct {
	// version is an exact runtime ("3.3-8"), a series whose moving tag the
	// build resolves ("3.2", with series set), or "" when only the built
	// image can say.
	version string
	series  bool
	// raise names the change that gives the image runtime minimum or newer.
	raise func(minimum string) string
}

// planRuntime reads the runtime the image will carry from what decides it: the
// runtime base of a generated build, or the final FROM of a declared
// Dockerfile. A prebuilt image, an Airflow 2 base and a FROM this cannot read
// plan no version.
//
// A runtime build and a FROM have to agree with the apache-airflow pin's
// series, so a fix that moves either to another series names the pin too.
func planRuntime(in *ImageDeployV2Input, req *imagebuild.Request) plannedRuntime {
	switch {
	case in.ImageName != "":
		return plannedRuntime{raise: func(minimum string) string {
			return fmt.Sprintf("rebuild %s FROM Astro Runtime %s or newer", in.ImageName, minimum)
		}}
	case req.FromDeclaredDockerfile():
		v := airflowrt.ReadDeclaredBase(req.Dockerfile).RuntimeVersion()
		p := plannedRuntime{raise: func(minimum string) string {
			fix := fmt.Sprintf("change the FROM line in %s to Astro Runtime %s or newer", in.Dockerfile, minimum)
			if s := runtimeSeries(minimum); s != "" && s != runtimeSeries(v) {
				fix += fmt.Sprintf(", and pin apache-airflow to %s in pyproject.toml", s)
			}
			return fix
		}}
		if tag, ok := manifest.ParseRuntimeTag(v); ok && tag.Series != "" {
			p.version, p.series = v, !tag.Build
		}
		return p
	case in.Runtime != "":
		return plannedRuntime{version: in.Runtime, raise: func(minimum string) string {
			if s := runtimeSeries(minimum); s != "" && s != runtimeSeries(in.Runtime) {
				return fmt.Sprintf("pin apache-airflow to %s and set [tool.astro] runtime to %s or newer in pyproject.toml", s, minimum)
			}
			return fmt.Sprintf("set [tool.astro] runtime to %s or newer in pyproject.toml", minimum)
		}}
	default:
		_, tag, _ := strings.Cut(req.BaseImage, ":")
		return plannedRuntime{version: tag, series: true, raise: func(minimum string) string {
			s := runtimeSeries(minimum)
			if s == tag {
				// The pin is already the deployment's series; only its
				// moving tag served an older build.
				return fmt.Sprintf("set [tool.astro] runtime to %s or newer in pyproject.toml", minimum)
			}
			return fmt.Sprintf("pin apache-airflow to %s or newer in pyproject.toml", cmp.Or(s, minimum))
		}}
	}
}

// checkPlannedRuntime runs checkRuntimeVersion's rules on the runtime the image
// will carry, before it is built. An exact version takes the rules as they are.
// A series refuses only what no build of it can pass: a series older than the
// deployment's, or one the deployment offers no build of. Its patch is left to
// the check on the built image.
func checkPlannedRuntime(currentVersion string, p *plannedRuntime, allowed []string) error {
	if currentVersion == "" || p.version == "" {
		return nil
	}
	if !p.series {
		return checkRuntimeVersion(currentVersion, p.version, allowed, p.raise)
	}
	if current := runtimeSeries(currentVersion); current != "" && semver.Compare("v"+p.version, "v"+current) < 0 {
		return downgradeError(p.version, currentVersion, p.raise)
	}
	if !slices.ContainsFunc(allowed, func(v string) bool { return runtimeSeries(v) == p.version }) {
		return fmt.Errorf("cannot deploy unsupported Astro Runtime %s; supported versions: %s", p.version, strings.Join(allowed, ", "))
	}
	return checkAirflow3Floor(currentVersion, p.version+"-0")
}

// runtimeSeries is the Airflow series an Airflow 3 runtime version belongs to:
// "3.3" for "3.3-8" and for "3.3". It is "" for an Airflow 2 runtime, whose
// version does not show its Airflow series.
func runtimeSeries(version string) string {
	tag, _ := manifest.ParseRuntimeTag(version)
	return tag.Series
}

func downgradeError(tag, currentVersion string, raise func(string) string) error {
	msg := fmt.Sprintf("cannot deploy Astro Runtime %s: it is a downgrade from the deployment's current %s", tag, currentVersion)
	if raise != nil {
		msg += "; to deploy, " + raise(currentVersion)
	}
	return errors.New(msg)
}

// checkRuntimeVersion is v1's ValidRuntimeVersion without the prints: it returns
// a descriptive error instead of printing the reason (and leaving the caller to
// exit), so the v2 path stays print-free below cmd — the same move
// finalizeDeployV2 makes for finalize. The rules are identical: no downgrade,
// the version must be one the deployment allows, and an Airflow 2-to-3 jump
// needs the deployment at Runtime 12.0.0 or higher. raise, when set, names the
// fix for a downgrade.
func checkRuntimeVersion(currentVersion, tag string, allowed []string, raise func(string) string) error {
	// Old deployments carry no runtime version; nothing to check against.
	if currentVersion == "" {
		return nil
	}
	if airflowversions.CompareRuntimeVersions(tag, currentVersion) < 0 {
		return downgradeError(tag, currentVersion, raise)
	}
	supported := false
	for _, v := range allowed {
		if airflowversions.CompareRuntimeVersions(tag, v) == 0 {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("cannot deploy unsupported Astro Runtime %s; supported versions: %s", tag, strings.Join(allowed, ", "))
	}
	return checkAirflow3Floor(currentVersion, tag)
}

func checkAirflow3Floor(currentVersion, tag string) error {
	if airflowversions.AirflowMajorVersionForRuntimeVersion(currentVersion) == "2" &&
		airflowversions.AirflowMajorVersionForRuntimeVersion(tag) == "3" &&
		airflowversions.CompareRuntimeVersions(currentVersion, "12.0.0") < 0 {
		return fmt.Errorf("cannot upgrade from Airflow 2 to Airflow 3 unless the deployment is at Astro Runtime 12.0.0 or higher (currently %s)", currentVersion)
	}
	return nil
}

// finalizeDeployV2 marks a v2 deploy final. It carries the dag tarball version
// only when one exists (a "both" or dags-only deploy), so an image-only deploy
// finalizes with an empty request. It is the v1 finalizeDeploy without the
// prints, so the v2 path renders in cmd and stays ready for --output json.
func finalizeDeployV2(organizationID, deploymentID, deployID, dagTarballVersion string, astroV1Client astrov1.APIClient) error {
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

// ensureContainerEngine resolves the container CLI and confirms its daemon is
// reachable, so an image deploy fails with a plain message before any transport
// work when Docker is absent or down — or, for podman with no machine up, with
// the podman command that brings one up, which "Start Docker" would not be.
func ensureContainerEngine(ctx context.Context, cmd imagebuild.Commander) (bin string, env []string, err error) {
	bin, env, err = resolveContainerEngine()
	if errors.Is(err, container.ErrMachineNotRunning) {
		return "", nil, fmt.Errorf("an image deploy needs a running container engine, but %w. Or run 'astro deploy --dags' to deploy just your DAGs (no container engine needed)", err)
	}
	if err != nil {
		return "", nil, errNoDocker
	}
	// `info` needs the daemon, so it fails fast when the engine is installed but
	// not running.
	if err := cmd.Run(ctx, env, localrt.Stdio{}, bin, "info"); err != nil {
		return "", nil, errNoDocker
	}
	return bin, env, nil
}

// defaultResolveContainerEngine picks the engine the way local Docker mode and
// `astro package` do: the container.binary pin when set (the working
// directory's project config over global), otherwise pkg/container's
// PATH/OrbStack resolution, plus its connection env.
func defaultResolveContainerEngine() (bin string, env []string, err error) {
	return containercfg.Engine(config.WorkingPath)
}
