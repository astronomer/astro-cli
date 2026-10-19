// Package deploy holds the manifest deploy logic that `astro deploy` routes into
// when the working directory is a project (a pyproject.toml with a
// [tool.astro] table, which internal/project's HasManifest checks). It settles which
// deployment to ship to — named on the command line, or asked for and answered
// — and drives the deploy through an injected transport: a dags-only deploy, an
// image deploy, or both.
//
// The package keeps to the core's layer rules (docs/architecture.md): it never
// prints, never exits, and never touches config, Docker, or the network
// directly. cmd owns flag parsing, config, and rendering; the deploy transport
// arrives as an interface, so the logic here stays unit-testable with a fake.
package deploy

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Request is the resolved input for a manifest deploy. cmd fills it from flags, args,
// the manifest, and the current context, then hands it to Run.
type Request struct {
	// ProjectDir is the project root (where dags/ and pyproject.toml live).
	ProjectDir string
	// Manifest is the loaded pyproject.toml: deployment links, the Airflow pin,
	// the project's dependencies and OS packages.
	Manifest *manifest.Manifest
	// LinkName is the positional argument, "" if none. It resolves exactly as
	// Deployment does.
	LinkName string
	// Deployment is --deployment. It names a manifest link, or — when the
	// project links nothing by that name — an Astro Deployment id, which is
	// what the flag meant before it learned link names and what Astro CLI 1.x
	// spelled --deployment-id. Either way it names the target on the command line,
	// which is the only thing a deploy will resolve from.
	Deployment string
	// WorkspaceID is the --workspace override, "" if unset.
	WorkspaceID string
	// ContextWorkspace is the workspace of the login the deploy runs under,
	// the fallback.
	ContextWorkspace string
	// DagsOnly is --dags: ship only the dags/ directory, no image, no Docker.
	DagsOnly bool
	// Image is --image: an image-only deploy, leaving the running dags in place.
	Image bool
	// ImageAlone ships the image with no DAGs whatever the flags say: an
	// --image-name deploy outside a project, which has no dags/ to take.
	ImageAlone bool
	// ImageName is --image-name: a prebuilt local image to deploy instead of
	// building from the manifest. "" means build.
	ImageName string
	// ImageInRegistry is an ImageName already in a registry (--image-name
	// --remote on Astro Private Cloud): nothing is built, read or pushed,
	// and the transport points the Deployment at the reference as it is.
	ImageInRegistry bool
	// ImageRuntime is the Astro Runtime version an ImageInRegistry image
	// carries (--runtime-version), which no local label can say.
	ImageRuntime string
	// BuildSecrets are docker build --secret specs forwarded to the image
	// build: --build-secret, BUILD_SECRET_INPUT or the manifest's
	// build-secrets, as util.ResolveProjectBuildSecrets picked them.
	//
	// Only a project that declared its own Dockerfile can use one: a generated
	// build's Dockerfile is `FROM <base>` and the install happens in the runtime
	// image's ONBUILD triggers, so there is no RUN of the project's for a secret
	// to be mounted into. cmd/astro refuses a --build-secret for any other
	// project rather than accepting secrets nothing could read.
	BuildSecrets []string
	// Description is recorded on the deploy.
	Description string
	// Wait polls the deployment until it reports healthy.
	Wait     bool
	WaitTime time.Duration
	// NoDagsBaseDir drops the dags/ prefix from the bundle (Airflow 3 layouts).
	NoDagsBaseDir bool
	// Interactive reports whether the run may ask which deployment to ship to
	// (stdin is a TTY, and the output is not json). A run that cannot be asked
	// must name its target on the command line.
	Interactive bool
	// Prompt asks which deployment to ship to even when one is named, with
	// the one named preselected (Astro Private Cloud's --prompt, as Astro CLI
	// 1.x had it). The one named resolves, and is refused, as without it,
	// before anything is asked; naming none asks as without it. It applies
	// only to a run that may ask: one that cannot ships to the one named, or
	// is refused for naming none, as without it.
	Prompt bool
	// Announce runs once the target is settled and before anything is built or
	// uploaded. It is how "building your project image" lands after the
	// question rather than before it: a deploy refused at the prompt says
	// nothing about building anything. nil skips it.
	Announce func(Target)
	// OnBuild runs just before an image build starts, after the deployment
	// has cleared the deploy. A dags-only deploy and a prebuilt --image-name
	// build nothing and never call it. nil skips it.
	OnBuild func()
	// CheckRuntime holds the manifest's [tool.astro] runtime build to its
	// Airflow pin with the runtime catalog, before an image is built FROM it
	// (runtimeversions.CheckRuntime's contract: warnings to report, and the
	// blocking finding as the error). nil checks nothing.
	CheckRuntime func(runtime, airflowPin string) ([]runtimeversions.Finding, error)
	// Warn reports a finding the deploy goes ahead despite, as a sentence.
	// nil drops it.
	Warn func(string)
	// Engine resolves the container engine that will build the image: its
	// CLI and the env that reaches its daemon. cmd hands the same memoized
	// resolver to the transport, so the engine is resolved once. It is
	// called only by a deploy that builds, to learn whether the engine is
	// podman, whose builds of a declared Dockerfile read a .containerignore
	// first. nil, or an engine it cannot resolve, counts as docker here; the
	// transport reports a missing engine.
	Engine func() (bin string, env []string, err error)
	// Link saves a link to a Deployment the prompt offered from the workspace
	// rather than from the manifest, once it is picked, so the next deploy
	// offers it by name. cmd wires it to scaffold.SaveLink. nil links nothing:
	// the deploy still ships to the picked Deployment, by id.
	Link func(scaffold.Link) error
	// Platform is the control plane the deploy ships to, as the kind of link
	// that names one of its Deployments: manifest.KindAstro (and "", which
	// reads as it) or manifest.KindAPC. Only links of that kind are offered,
	// preselected or resolved; a link saved for a picked Deployment is of it.
	Platform manifest.LinkKind
	// Domain is the Astro Private Cloud base domain the deploy runs under,
	// normalized as an apc link writes it (manifest.NormalizeAPCDomain). An
	// apc deploy ships only to apc links on it, and the link it saves for a
	// picked Deployment carries it. Unused by an astro deploy.
	Domain string
	// OtherDomain explains an apc link on a domain other than Domain, which
	// this deploy cannot reach: the login to switch to, or to make. nil says
	// only that the link is elsewhere.
	OtherDomain func(link, domain string) error
}

// platform is the kind of link the deploy ships to.
func (r *Request) platform() manifest.LinkKind {
	if r.Platform == "" {
		return manifest.KindAstro
	}
	return r.Platform
}

// deploysTo reports whether l is a link this deploy can ship to: one of its
// platform's, and for Astro Private Cloud one on the domain it runs under.
func (r *Request) deploysTo(l manifest.Link) bool {
	kind := l.Kind()
	if kind != r.platform() {
		return false
	}
	return kind != manifest.KindAPC || l.Domain == r.Domain
}

// shipsTo names what the deploy ships to, for messages that turn a link away.
func (r *Request) shipsTo() string {
	if r.platform() == manifest.KindAPC {
		return "Astro Private Cloud Deployments on " + r.Domain
	}
	return "Astro Deployments"
}

// Result is what a finished manifest deploy reports back to cmd for rendering. Text
// output ships now; the struct is what --output json will serialize later.
type Result struct {
	DeploymentID string
	WorkspaceID  string
	// Type is "dag-only", "image-only", or "image-and-dag".
	Type              string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	// DagsInImage reports an image-and-dag deploy to a Deployment with DAG
	// deploys off: no DAGs were uploaded, and the Deployment runs those
	// inside the image.
	DagsInImage bool
	// RemoteExecution reports an image-and-dag deploy to a remote-execution
	// Deployment with DAG deploys off, to which no DAGs were uploaded.
	RemoteExecution bool
	URL             string
	// LinkName is the manifest link the deploy resolved to, "" if unlinked.
	LinkName string
	Git      Git
	// DagDeployEnabled is whether the Deployment takes DAG deploys, as an
	// image deploy read it; false for a dags-only deploy.
	DagDeployEnabled bool
	// RemoteExecutionEnabled is whether the Deployment runs remote
	// execution, as an image deploy read it, whatever the deploy shipped.
	RemoteExecutionEnabled bool
}

// Git is what a deploy recorded about the commit it shipped.
type Git struct {
	// Commit is nil when the deploy recorded none.
	Commit *Commit
	// Uncommitted reports that no commit was recorded because the project had
	// uncommitted changes.
	Uncommitted bool
}

// Commit is the git commit a deploy recorded as its source.
type Commit struct {
	SHA    string
	Branch string
	// URL links to the commit on GitHub, "" for other remotes.
	URL string
}

// DagDeploy is the request the transport's dags-only deploy takes. The
// deployment is already resolved; the transport reads its runtime version and
// type from the server.
type DagDeploy struct {
	DeploymentID  string
	WorkspaceID   string
	ProjectDir    string
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
}

// DagResult is what the transport reports after a dags-only deploy.
type DagResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	DagTarballVersion string
	URL               string
	Git               Git
}

// ImageDeploy is the request the transport's image deploy takes. The deployment
// is already resolved. When ImageName is set the transport deploys that prebuilt
// local image; otherwise it builds from the manifest fields. IncludeDags marks a
// "both" deploy, which also ships the dags/ tarball.
type ImageDeploy struct {
	DeploymentID string
	WorkspaceID  string
	// Build is the project's root (Build.ProjectDir) and the manifest's
	// image-deciding fields, as imagebuild.ManifestBuildOf reads them.
	Build imagebuild.ManifestBuild
	// BuildSecrets are docker build --secret specs to expose to the declared
	// Dockerfile's (Build.Dockerfile) build. Only a declared Dockerfile has a
	// RUN of the project's own to consume one; a generated build drops them.
	BuildSecrets []string
	ImageName    string
	// ImageInRegistry and ImageRuntime are Request's: ImageName is already
	// in a registry, carrying Astro Runtime ImageRuntime.
	ImageInRegistry bool
	ImageRuntime    string
	// OnBuild is Request.OnBuild, for the transport to call.
	OnBuild func()
	// IncludeDags marks a "both" deploy. To a Deployment with DAG deploys
	// off, the transport ships the image alone, with the DAGs inside it.
	IncludeDags bool
	// ImageAlone is Request.ImageAlone: a prebuilt image deployed from
	// outside any project, which a Deployment with DAG deploys off takes too
	// (it runs whatever DAGs the image carries, which the caller warns of).
	ImageAlone bool
	// Warn is Request.Warn, for the transport to report with.
	Warn func(string)
	// Survey is what the image built from the manifest would carry, as the
	// checks before the deploy listed it; nil for a prebuilt image.
	Survey *imagebuild.ProjectSurvey
	// Engine is Request.Engine, for the transport to build with.
	Engine        func() (bin string, env []string, err error)
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
}

// ImageResult is what the transport reports after an image deploy. A "both"
// deploy also carries a DagTarballVersion, or DagsInImage when the
// Deployment's DAG deploys are off and the DAGs went inside the image.
type ImageResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	DagsInImage       bool
	RemoteExecution   bool
	URL               string
	Git               Git
	// DagDeployEnabled is whether the Deployment takes DAG deploys, read
	// while the image deploy checked it.
	DagDeployEnabled bool
	// RemoteExecutionEnabled is whether the Deployment runs remote
	// execution, read alongside it.
	RemoteExecutionEnabled bool
	// DagsNotShipped reports a "both" deploy that shipped the image and no
	// DAGs, by neither an upload nor the image: the Deployment takes its
	// DAGs from somewhere else (an Astro Private Cloud git-sync or volume
	// Deployment), or there was no dags directory to upload. The transport
	// warned why. The deploy is then reported as image-only.
	DagsNotShipped bool
}

// Deployer is the seam onto the deploy transport. The CLI wires it to the
// cloud/deploy transport (create deploy, build and push the image, upload the
// dag tarball, finalize); tests supply a fake. The interface keeps the manifest deploy's logic
// free of config, Docker, and the network so it stays unit-testable.
type Deployer interface {
	// ConfirmTarget presents the choices — the project's deployable links, then
	// the workspace's Deployments it does not link yet — and returns the name
	// of the one to ship to. defaultLink, when not "", is the project's default
	// link, which the prompt highlights. It is called on every interactive
	// deploy that did not name its target, and an answer of "no" comes back as
	// an error.
	//
	// It sits on the transport rather than on the Request because asking is a
	// cmd job — this package never prints — and ResolveUnlinked already asks
	// through the same seam.
	ConfirmTarget(choices []Choice, defaultLink string) (name string, err error)
	// ListDeployments lists the Astro Deployments in a workspace, for the
	// prompt to offer the ones the project does not link yet.
	ListDeployments(workspaceID string) ([]Deployment, error)
	// ResolveUnlinked runs the workspace-level pick/create flow and returns the
	// chosen Deployment. It is called only for a project that links nothing
	// at all, and only in an interactive run.
	ResolveUnlinked(workspaceID string) (Deployment, error)
	// DeployDags creates a DAG-only deploy, uploads the project's dags/
	// directory, and finalizes.
	DeployDags(*DagDeploy) (DagResult, error)
	// DeployImage builds (or adopts) the project image, pushes it, and
	// finalizes; a "both" deploy also uploads the dags. It requires Docker and
	// returns a plain error when it is unreachable.
	DeployImage(*ImageDeploy) (ImageResult, error)
}

// Run resolves the deployment and drives the deploy for a project: a
// dags-only deploy (--dags), an image-only deploy (--image), or the default
// "both" (image + dags).
//
// The target is settled first, and Request.Announce fires between the two
// halves, so nothing about building an image is printed before the deploy has
// something to ship to.
func Run(req Request, d Deployer) (Result, error) {
	// --dags ships only the dags/ directory, so an image source makes no sense
	// with it. Checked before anything else, so an impossible combination fails
	// rather than asking a question whose answer it will throw away.
	if req.DagsOnly && (req.Image || req.ImageName != "") {
		var passed []string
		if req.Image {
			passed = append(passed, "--image")
		}
		if req.ImageName != "" {
			passed = append(passed, "--image-name")
		}
		return Result{}, fmt.Errorf("--dags deploys only your Dags; drop %s", strings.Join(passed, " and "))
	}
	// Also before anything is asked: a project whose image source is refused
	// should not first make someone pick where to ship it.
	missingSecrets, survey, err := checkImageSource(req)
	if err != nil {
		return Result{}, err
	}

	target, err := resolveTarget(req, d)
	if err != nil {
		return Result{}, err
	}
	if req.Announce != nil {
		req.Announce(target)
	}

	var res Result
	if req.DagsOnly {
		res, err = runDagsOnly(req, target, d)
	} else {
		res, err = runImage(req, target, d, survey)
		err = missingSecrets.Explain(err)
	}
	if err != nil {
		return res, err
	}
	saveNewLink(req, target, &res)
	return res, nil
}

// saveNewLink writes the link to a Deployment picked from the workspace rather
// than from the manifest, once the deploy to it has succeeded, so a deploy
// that fails or is canceled leaves no link to a Deployment it never reached.
// A link that cannot be saved is warned about: the deploy itself succeeded.
func saveNewLink(req Request, target Target, res *Result) {
	if target.newLink == nil {
		return
	}
	if err := req.Link(*target.newLink); err != nil {
		res.LinkName = ""
		if req.Warn != nil {
			req.Warn(fmt.Sprintf("deployed, but could not save a link to %s deployment %s: %v. Link it with astro link add %s --deployment %s",
				req.platform(), target.DeploymentID, err, target.newLink.Name, target.DeploymentID))
		}
	}
}

// checkImageSource holds what an image deploy would build from to the checks
// before a build (deploybuild.CheckSource, which skips a dags-only deploy and
// a prebuilt --image-name). It returns the build secrets the declared
// Dockerfile mounts and nobody gave, for a failed build to name again, and the
// survey of what the image would carry, made once for every check of it (the
// transport's too).
func checkImageSource(req Request) (imagebuild.MissingSecrets, *imagebuild.ProjectSurvey, error) {
	return deploybuild.CheckSource(&deploybuild.Source{
		ProjectDir:   req.ProjectDir,
		Manifest:     req.Manifest,
		DagsOnly:     req.DagsOnly,
		ImageName:    req.ImageName,
		BuildSecrets: req.BuildSecrets,
		CheckRuntime: req.CheckRuntime,
		Warn:         req.Warn,
		Engine:       req.Engine,
	})
}

// runDagsOnly ships just the dags/ directory through the transport.
func runDagsOnly(req Request, target Target, d Deployer) (Result, error) {
	dag, err := d.DeployDags(&DagDeploy{
		DeploymentID:  target.DeploymentID,
		WorkspaceID:   target.WorkspaceID,
		ProjectDir:    req.ProjectDir,
		Description:   req.Description,
		NoDagsBaseDir: req.NoDagsBaseDir,
		Wait:          req.Wait,
		WaitTime:      req.WaitTime,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		DeploymentID:      target.DeploymentID,
		WorkspaceID:       firstNonEmpty(dag.WorkspaceID, target.WorkspaceID),
		Type:              "dag-only",
		RuntimeVersion:    dag.RuntimeVersion,
		DagTarballVersion: dag.DagTarballVersion,
		URL:               dag.URL,
		LinkName:          target.LinkName,
		Git:               dag.Git,
	}, nil
}

// runImage builds or adopts the project image and ships it, plus the dags for a
// default "both" deploy. --image drops the dags.
func runImage(req Request, target Target, d Deployer, survey *imagebuild.ProjectSurvey) (Result, error) {
	// Secrets are carried here; checkImageSource has already checked that
	// their variables are set. Whether asking for one is a mistake depends on
	// whether the USER asked — ResolveProjectBuildSecrets also reads
	// BUILD_SECRET_INPUT from the environment — and this package cannot see the
	// difference between a flag and an ambient variable. cmd/astro can, and
	// refuses there, before anything is resolved or prompted for.
	//
	// An earlier version refused here on `len(req.BuildSecrets) > 0 && dockerfile
	// == ""`, which turned an exported BUILD_SECRET_INPUT into a hard failure for
	// every project that generates its image. imagebuild passes a generated build
	// only the netrc secret, and checkImageSource has warned about the rest.
	includeDags := !req.Image && !req.ImageAlone
	img, err := d.DeployImage(&ImageDeploy{
		DeploymentID:    target.DeploymentID,
		WorkspaceID:     target.WorkspaceID,
		Build:           imagebuild.ManifestBuildOf(req.ProjectDir, req.Manifest),
		BuildSecrets:    req.BuildSecrets,
		ImageName:       req.ImageName,
		ImageInRegistry: req.ImageInRegistry,
		ImageRuntime:    req.ImageRuntime,
		OnBuild:         req.OnBuild,
		IncludeDags:     includeDags,
		ImageAlone:      req.ImageAlone,
		Warn:            req.Warn,
		Survey:          survey,
		Engine:          req.Engine,
		Description:     req.Description,
		NoDagsBaseDir:   req.NoDagsBaseDir,
		Wait:            req.Wait,
		WaitTime:        req.WaitTime,
	})
	if err != nil {
		return Result{}, err
	}
	kind := "image-only"
	if includeDags && !img.DagsNotShipped {
		kind = "image-and-dag"
	}
	return Result{
		DeploymentID:           target.DeploymentID,
		WorkspaceID:            firstNonEmpty(img.WorkspaceID, target.WorkspaceID),
		Type:                   kind,
		RuntimeVersion:         img.RuntimeVersion,
		ImageTag:               img.ImageTag,
		DagTarballVersion:      img.DagTarballVersion,
		DagsInImage:            img.DagsInImage,
		RemoteExecution:        img.RemoteExecution,
		URL:                    img.URL,
		LinkName:               target.LinkName,
		Git:                    img.Git,
		DagDeployEnabled:       img.DagDeployEnabled,
		RemoteExecutionEnabled: img.RemoteExecutionEnabled,
	}, nil
}

// Target is the deployment a run ships to, once it is settled.
type Target struct {
	DeploymentID string
	WorkspaceID  string
	// LinkName is the manifest link the deploy resolved to, or the one it
	// saves for a Deployment picked from the workspace; "" when the target was
	// named by id, or the run has no Request.Link to save one with.
	LinkName string
	// newLink is the link Run saves once the deploy succeeds, for a Deployment
	// picked from the workspace rather than from the manifest. nil otherwise.
	newLink *scaffold.Link
}

// Choice is one entry the prompt offers: a deployable link, or a workspace
// Deployment the project does not link yet.
type Choice struct {
	// Name is the link's name. For a Deployment not linked yet it is the name
	// the link will be saved under if it is picked, so every entry answers to
	// its name the same way.
	Name string
	// Where is the coordinate as the manifest writes it, so two links are told
	// apart by where they point and not only by what they are called.
	Where string
	// Unlinked marks a Deployment the project does not link yet: picking it
	// links it.
	Unlinked bool
	// Preselected marks the entry the prompt starts on, in place of the
	// default link: the one a --prompt deploy named.
	Preselected bool
}

// Deployment is an Astro Deployment as a workspace lists it.
type Deployment struct {
	ID          string
	Name        string
	WorkspaceID string
}

// ErrAborted reports a deploy prompt the user declined. It is exported so cmd
// can keep quiet about it: someone who was asked and said no does not need the
// answer read back to them as an error. The run still exits non-zero.
var ErrAborted = errors.New("no deployment selected")

// resolveTarget settles which deployment this run ships to.
//
// Deploy is the one command that never resolves from ambient state
// (docs/instances.md, "Deploy always asks"): shipping code is too consequential to
// decide from a pin, an exported variable, or a marker in a file nobody looked
// at. So there are exactly two ways here — the target is named on the command
// line, or an interactive run is asked and answers. A pin, ASTRO_DEPLOYMENT, or
// `default = true` only move the cursor in that prompt.
func resolveTarget(req Request, d Deployer) (Target, error) {
	var links map[string]manifest.Link
	if req.Manifest != nil {
		links = req.Manifest.Astro.Deployments
	}

	if target, named, err := resolveNamed(req, links); named || err != nil {
		if err != nil || !req.Prompt || !req.Interactive {
			return target, err
		}
		// --prompt: the target named resolves, and is refused, exactly as
		// without it; only then is the question asked, starting on it.
		return promptTarget(req, d, links, target)
	}

	deployable := linkNames(links, req.deploysTo)
	switch {
	case len(links) == 0:
		// A project that links nothing at all: the workspace-level flow, which
		// asks too.
		return unlinkedTarget(req, d)
	case len(deployable) == 0:
		// Links, but none of them anywhere this command can ship to. Falling
		// through to the workspace-level flow here would offer some unrelated
		// Deployment, which is how one project's DAGs land on another
		// project's Airflow.
		msg := fmt.Sprintf("astro deploy ships to %s, and this project links none%s", req.shipsTo(), describeLinks(links))
		if req.platform() == manifest.KindAPC {
			msg += ". Link one with astro link add, or name a Deployment id: astro deploy <deployment-id>"
		}
		return Target{}, errors.New(msg)
	case !req.Interactive:
		// The question an interactive run asks, refused: input_required under
		// --output json, in deploy's own words.
		return Target{}, input.Required(fmt.Errorf("a deploy must name the deployment it ships to: astro deploy <name> or --deployment <name>. "+
			"Deploy never picks for you — default = true only preselects the prompt, and astro use and %s are for the query commands. Deployable links: %s",
			deploymentEnvVar, strings.Join(deployable, ", ")))
	}

	pre := defaultLink(links, deployable)
	where := string(req.platform()) + " deployment "
	choices := make([]Choice, 0, len(deployable))
	for _, name := range linksFirst(deployable, links) {
		choices = append(choices, Choice{Name: name, Where: where + links[name].Deployment})
	}
	unlinked := unlinkedDeployments(req, d, links)
	for _, name := range unlinked.names {
		choices = append(choices, Choice{Name: name, Where: where + unlinked.byName[name].ID, Unlinked: true})
	}
	name, err := d.ConfirmTarget(choices, pre)
	if err != nil {
		return Target{}, err
	}
	if name == "" {
		return Target{}, ErrAborted
	}
	if link, ok := links[name]; ok {
		return linkTarget(req, name, link, links)
	}
	if dep, ok := unlinked.byName[name]; ok {
		return linkedTarget(req, name, dep), nil
	}
	return Target{}, fmt.Errorf("no deployment link %q in the manifest%s", name, knownLinks(links))
}

// promptTarget asks which deployment to ship to, as an interactive deploy
// that names none is asked, though one is named and resolved (--prompt,
// named): the project's deployable links, then the workspace's Deployments no
// link names, starting on the one named, never on the default link. A named
// target no entry is, an id the workspace list does not hold, is offered
// first. Answering with the target named deploys there as naming it without
// --prompt does, saving no link; picking another resolves as a pick does.
func promptTarget(req Request, d Deployer, links map[string]manifest.Link, named Target) (Target, error) {
	deployable := linkNames(links, req.deploysTo)
	where := string(req.platform()) + " deployment "
	choices := make([]Choice, 0, len(deployable)+1)
	for _, name := range linksFirst(deployable, links) {
		choices = append(choices, Choice{Name: name, Where: where + links[name].Deployment})
	}
	unlinked := unlinkedDeployments(req, d, links)
	for _, name := range unlinked.names {
		choices = append(choices, Choice{Name: name, Where: where + unlinked.byName[name].ID, Unlinked: true})
	}
	start := -1
	for i := range choices {
		if (named.LinkName != "" && choices[i].Name == named.LinkName) || (named.LinkName == "" && choices[i].Where == where+named.DeploymentID) {
			start = i
			break
		}
	}
	if start < 0 {
		choices = append([]Choice{{Name: firstNonEmpty(named.LinkName, named.DeploymentID), Where: where + named.DeploymentID}}, choices...)
		start = 0
	}
	choices[start].Preselected = true
	name, err := d.ConfirmTarget(choices, defaultLink(links, deployable))
	if err != nil {
		return Target{}, err
	}
	switch {
	case name == "":
		return Target{}, ErrAborted
	case name == choices[start].Name:
		return named, nil
	}
	if link, ok := links[name]; ok {
		return linkTarget(req, name, link, links)
	}
	if dep, ok := unlinked.byName[name]; ok {
		return linkedTarget(req, name, dep), nil
	}
	return Target{}, fmt.Errorf("no deployment link %q in the manifest%s", name, knownLinks(links))
}

// linksFirst orders the project's links for the prompt: the manifest's default
// link on top, then the rest by name. The default is committed with the
// project, so "1" means the same Deployment in every terminal.
func linksFirst(deployable []string, links map[string]manifest.Link) []string {
	top, _, ok := manifest.DefaultLink(links)
	if !ok || !slices.Contains(deployable, top) {
		return deployable
	}
	ordered := []string{top}
	for _, name := range deployable {
		if name != top {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

// unlinked is the workspace's Deployments the project links no link to, keyed
// by the link name each would be saved under.
type unlinked struct {
	names  []string
	byName map[string]Deployment
}

// unlinkedDeployments lists the Deployments in the deploy's workspace that no
// link points at, so the prompt can offer them after the links. The workspace
// is --workspace, else [tool.astro] workspace, else the login's. A list that
// fails costs only those entries: the links are still offered, with a warning.
func unlinkedDeployments(req Request, d Deployer, links map[string]manifest.Link) unlinked {
	out := unlinked{byName: map[string]Deployment{}}
	workspaceID := listWorkspace(req)
	if workspaceID == "" {
		return out
	}
	deps, err := d.ListDeployments(workspaceID)
	if err != nil {
		if req.Warn != nil {
			req.Warn(fmt.Sprintf("could not list the Deployments in workspace %s, so only the project's links are offered: %v", workspaceID, err))
		}
		return out
	}
	linked := map[string]bool{}
	for name := range links {
		if l := links[name]; req.deploysTo(l) {
			linked[l.Deployment] = true
		}
	}
	sort.SliceStable(deps, func(i, j int) bool {
		if deps[i].Name != deps[j].Name {
			return deps[i].Name < deps[j].Name
		}
		return deps[i].ID < deps[j].ID
	})
	taken := map[string]bool{manifest.ReservedLinkName: true, BundleCommand: true}
	for name := range links {
		taken[name] = true
	}
	for _, dep := range deps {
		if linked[dep.ID] {
			continue
		}
		if dep.WorkspaceID == "" {
			dep.WorkspaceID = workspaceID
		}
		name := newLinkName(dep, taken)
		taken[name] = true
		out.names = append(out.names, name)
		out.byName[name] = dep
	}
	// Names are given in the Deployments' order, so a collision's -2 goes to
	// the same one every run; the rows are then listed by the name they show.
	sort.Strings(out.names)
	return out
}

// newLinkName is the name a link to dep is saved under: the Deployment's name
// as `astro link add` spells it (scaffold.LinkNameFor), else its id, with -2,
// -3 and so on appended until it names no link already taken.
func newLinkName(dep Deployment, taken map[string]bool) string {
	base := scaffold.LinkNameFor(dep.Name)
	// A name of digits alone would shadow a row number: the prompt matches
	// names first, so a Deployment called "1" would take the answer "1" from
	// whatever row 1 is.
	if base == "" || base == manifest.ReservedLinkName || base == BundleCommand || isDigits(base) {
		base = dep.ID
	}
	name := base
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s-%d", base, n)
	}
	return name
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// linkedTarget is the target for a Deployment picked from the workspace rather
// than from the manifest, carrying the link Run saves once the deploy to it
// succeeds (saveNewLink), so the next deploy offers the Deployment by name.
func linkedTarget(req Request, name string, dep Deployment) Target {
	target := Target{
		DeploymentID: dep.ID,
		WorkspaceID:  firstNonEmpty(req.WorkspaceID, dep.WorkspaceID, req.ContextWorkspace),
	}
	// Outside a project (an --image-name deploy with no manifest) there is
	// no pyproject.toml to save a link into.
	if req.Link == nil || req.Manifest == nil {
		return target
	}
	target.LinkName = name
	target.newLink = &scaffold.Link{
		Name:       name,
		Kind:       req.platform(),
		Workspace:  dep.WorkspaceID,
		Deployment: dep.ID,
	}
	if req.platform() == manifest.KindAPC {
		// An apc link names its own domain and workspace, and inherits
		// neither: the workspace is the one the Deployment was listed from.
		target.newLink.Domain = req.Domain
		target.newLink.Workspace = target.WorkspaceID
	}
	return target
}

// listWorkspace is the workspace a deploy lists Deployments from, for the
// prompt's unlinked entries and for a project that links nothing alike:
// --workspace, else [tool.astro] workspace, else the login's. [tool.astro]
// workspace is an Astro workspace, so an Astro Private Cloud deploy skips it.
func listWorkspace(req Request) string {
	project := ""
	if req.Manifest != nil && req.platform() == manifest.KindAstro {
		project = req.Manifest.Astro.Workspace
	}
	return firstNonEmpty(req.WorkspaceID, project, req.ContextWorkspace)
}

// ResolveNamed settles a target named on the command line, as Run does: the
// argument (linkName) and --deployment, which must agree, each a link of m
// (nil for no manifest) or a Deployment id. named is false when neither names
// one. It is for a command that ships something other than the project, such
// as astro deploy bundle, to name its Deployment the way a deploy does.
func ResolveNamed(m *manifest.Manifest, linkName, deployment, workspaceID, contextWorkspace string) (target Target, named bool, err error) {
	req := Request{Manifest: m, LinkName: linkName, Deployment: deployment, WorkspaceID: workspaceID, ContextWorkspace: contextWorkspace}
	var links map[string]manifest.Link
	if m != nil {
		links = m.Astro.Deployments
	}
	return resolveNamed(req, links)
}

func resolveNamed(req Request, links map[string]manifest.Link) (Target, bool, error) {
	if req.LinkName != "" && req.Deployment != "" && req.LinkName != req.Deployment {
		return Target{}, true, fmt.Errorf("this deploy names two targets, %q and --deployment %q: name one", req.LinkName, req.Deployment)
	}
	name := firstNonEmpty(req.LinkName, req.Deployment)
	if name == "" {
		return Target{}, false, nil
	}
	target, err := namedTarget(req, name, links)
	return target, true, err
}

// namedTarget resolves a target named on the command line, by the positional
// argument or --deployment alike: a manifest link, else an Astro Deployment id.
// The id fall-through is what both meant before they learned link names — the
// usage line is `astro deploy DEPLOYMENT-ID`, and astronomer/deploy-action runs
// exactly that — so a CI job that passes an id keeps working.
func namedTarget(req Request, name string, links map[string]manifest.Link) (Target, error) {
	if link, ok := links[name]; ok {
		return linkTarget(req, name, link, links)
	}
	return Target{
		DeploymentID: name,
		WorkspaceID:  firstNonEmpty(req.WorkspaceID, req.ContextWorkspace),
	}, nil
}

// unlinkedTarget runs the workspace-level pick/create flow, for a project whose
// manifest declares no deployment links at all, and links what it picks.
func unlinkedTarget(req Request, d Deployer) (Target, error) {
	workspaceID := listWorkspace(req)
	if workspaceID == "" {
		return Target{}, errors.New("a workspace is required for a deploy: pass --workspace or set a current workspace")
	}
	if !req.Interactive {
		if req.Manifest == nil {
			// No project: there is no link to name, only an id.
			return Target{}, input.Required(errors.New("this deploy names no Deployment and this run cannot be asked: pass the Deployment id as the argument or with --deployment"))
		}
		return Target{}, input.Required(errors.New("this project links no deployment and this run cannot be asked: pass --deployment <name or id>"))
	}
	dep, err := d.ResolveUnlinked(workspaceID)
	if err != nil {
		return Target{}, err
	}
	if dep.ID == "" {
		return Target{}, ErrAborted
	}
	if dep.WorkspaceID == "" {
		dep.WorkspaceID = workspaceID
	}
	return linkedTarget(req, newLinkName(dep, map[string]bool{manifest.ReservedLinkName: true, BundleCommand: true}), dep), nil
}

// BundleCommand is the word astro deploy bundle is spelled with. astro
// deploy reads it as that subcommand, never as a link name, so no link is
// saved under it (scaffold.BundleLinkName): a link a deploy saves for a
// picked Deployment is named otherwise.
const BundleCommand = scaffold.BundleLinkName

// DefaultMark is the word that marks a project's default link in a list of
// links: the deploy prompt's "linked, default", and the `astro use` and `astro
// link` pickers. One word in every place, never the manifest's own
// `default = true`, which reads as an instruction to edit the file.
const DefaultMark = "default"

// deploymentEnvVar is the ephemeral layer of the query commands' rule, named
// so the non-interactive refusal tells the reader what deploy deliberately
// ignores. It is spelled out rather than imported because pkg/instances is the
// resolver deploy does not use.
const deploymentEnvVar = "ASTRO_DEPLOYMENT"

// defaultLink is the entry the prompt highlights: the project's default link
// (manifest.DefaultLink), "" when there is none or it is not one this command
// can ship to — a default that is an MWAA link must not move the cursor onto a
// neighbor.
//
// Only the committed default, never ASTRO_DEPLOYMENT or the `astro use`
// selection. Those are a person's choice of which Airflow to query; where code
// ships is the project's, and Astro Desktop reads the same marker, so the two
// apps highlight the same entry.
func defaultLink(links map[string]manifest.Link, deployable []string) string {
	if name, _, ok := manifest.DefaultLink(links); ok && slices.Contains(deployable, name) {
		return name
	}
	return ""
}

// linkTarget turns a resolved link into a target. Only a link of the deploy's
// platform can be deployed to: this command ships an image to an Astro
// Deployment, or under an Astro Private Cloud context to one of that
// platform's Deployments, and an mwaa or composer link has no deployment id
// to ship it to. An apc link on another domain than the one the deploy runs
// under is one this login does not reach.
func linkTarget(req Request, name string, link manifest.Link, links map[string]manifest.Link) (Target, error) {
	switch kind := link.Kind(); {
	case kind != req.platform():
		return Target{}, fmt.Errorf("link %q is %s, and astro deploy ships to %s%s", name, kind, req.shipsTo(), deployableLinks(req, links))
	case kind == manifest.KindAPC && link.Domain != req.Domain:
		if req.OtherDomain != nil {
			return Target{}, req.OtherDomain(name, link.Domain)
		}
		return Target{}, fmt.Errorf("link %q is on %s, and this deploy runs under %s", name, link.Domain, req.Domain)
	}
	return Target{
		DeploymentID: link.Deployment,
		WorkspaceID:  firstNonEmpty(req.WorkspaceID, link.Workspace, req.ContextWorkspace),
		LinkName:     name,
	}, nil
}

// DeployableLinks names the links astro deploy can ship to on Astro, sorted.
func DeployableLinks(links map[string]manifest.Link) []string {
	return linkNames(links, func(l manifest.Link) bool { return l.Kind() == manifest.KindAstro })
}

// describeLinks names what a project full of unshippable links actually
// declares, so the refusal above says what is there rather than only what is
// not. An apc link says which domain it is on.
func describeLinks(links map[string]manifest.Link) string {
	named := make([]string, 0, len(links))
	for _, name := range linkNames(links, nil) {
		l := links[name]
		if l.Kind() == manifest.KindAPC {
			named = append(named, fmt.Sprintf("%s is apc on %s", name, l.Domain))
			continue
		}
		named = append(named, fmt.Sprintf("%s is %s", name, l.Kind()))
	}
	return " (" + strings.Join(named, ", ") + ")"
}

func knownLinks(links map[string]manifest.Link) string {
	if len(links) == 0 {
		return " (the manifest declares no deployment links)"
	}
	return ", known links: " + strings.Join(linkNames(links, nil), ", ")
}

// deployableLinks lists the links this command can ship to, for the message
// that turns one away.
func deployableLinks(req Request, links map[string]manifest.Link) string {
	names := linkNames(links, req.deploysTo)
	if len(names) == 0 {
		return fmt.Sprintf(" (this project has no %s links%s)", req.platform(), onDomain(req))
	}
	return fmt.Sprintf(", %s links%s: %s", req.platform(), onDomain(req), strings.Join(names, ", "))
}

// onDomain is " on <domain>" for an Astro Private Cloud deploy, whose links
// count only on the domain it runs under, and "" for an astro one.
func onDomain(req Request) string {
	if req.platform() == manifest.KindAPC {
		return " on " + req.Domain
	}
	return ""
}

// linkNames lists the links keep accepts, sorted; a nil keep takes all of them.
func linkNames(links map[string]manifest.Link, keep func(manifest.Link) bool) []string {
	names := make([]string, 0, len(links))
	for name := range links {
		if keep == nil || keep(links[name]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
