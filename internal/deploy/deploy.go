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

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/util"
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
	// what the flag meant before it learned link names and what the 1.x path spells
	// --deployment-id. Either way it names the target on the command line,
	// which is the only thing a deploy will resolve from.
	Deployment string
	// Preselect is what the ambient layers point at: ASTRO_DEPLOYMENT, then the
	// project's `astro use` pin. It moves the cursor in the prompt and does
	// nothing else — a deploy is never decided by state the user cannot see on
	// the command line (docs/instances.md, "Deploy always asks").
	Preselect string
	// PreselectFrom names where Preselect came from, for the prompt's label:
	// the env var's name, or "pinned". Ignored when Preselect is empty.
	PreselectFrom string
	// WorkspaceID is the --workspace override, "" if unset.
	WorkspaceID string
	// ContextWorkspace is the workspace of the login the deploy runs under,
	// the fallback.
	ContextWorkspace string
	// DagsOnly is --dags: ship only the dags/ directory, no image, no Docker.
	DagsOnly bool
	// Image is --image: an image-only deploy, leaving the running dags in place.
	Image bool
	// ImageName is --image-name: a prebuilt local image to deploy instead of
	// building from the manifest. "" means build.
	ImageName string
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
	URL               string
	// LinkName is the manifest link the deploy resolved to, "" if unlinked.
	LinkName string
	Git      Git
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
// "both" deploy, which also ships the project's DAGs: as the dags/ tarball to
// a Deployment that takes DAG uploads, inside the image to one that does not.
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
	// OnBuild is Request.OnBuild, for the transport to call.
	OnBuild       func()
	IncludeDags   bool
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
}

// ImageResult is what the transport reports after an image deploy. A "both"
// deploy also carries a DagTarballVersion.
type ImageResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	URL               string
	Git               Git
}

// Deployer is the seam onto the deploy transport. The CLI wires it to the
// cloud/deploy transport (create deploy, build and push the image, upload the
// dag tarball, finalize); tests supply a fake. The interface keeps the manifest deploy's logic
// free of config, Docker, and the network so it stays unit-testable.
type Deployer interface {
	// ConfirmTarget presents the project's deployable links and returns the
	// name of the one to ship to. It is called on every interactive deploy that
	// did not name its target, and an answer of "no" comes back as an error.
	//
	// It sits on the transport rather than on the Request because asking is a
	// cmd job — this package never prints — and ResolveUnlinked already asks
	// through the same seam.
	ConfirmTarget(choices []Choice, preselect Preselect) (name string, err error)
	// ResolveUnlinked runs the workspace-level pick/create flow and returns the
	// chosen deployment id. It is called only for a project that links nothing
	// at all, and only in an interactive run.
	ResolveUnlinked(workspaceID string) (deploymentID string, err error)
	// DeployDags creates a DAG-only deploy, uploads the project's dags/
	// directory, and finalizes.
	DeployDags(*DagDeploy) (DagResult, error)
	// DeployImage builds (or adopts) the project image, pushes it, and
	// finalizes; a "both" deploy also ships the dags. It requires Docker and
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
		return Result{}, errors.New("--dags deploys only your DAGs; drop --image and --image-name")
	}
	// Also before anything is asked: a project whose image source is refused
	// should not first make someone pick where to ship it.
	missingSecrets, err := checkImageSource(req)
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

	if req.DagsOnly {
		return runDagsOnly(req, target, d)
	}
	res, err := runImage(req, target, d)
	return res, missingSecrets.Explain(err)
}

// checkImageSource holds what an image deploy would build from to the Airflow
// requirement, for a deploy that builds one from the manifest: a declared
// Dockerfile's FROM line (scaffold.CheckDockerfileAirflow), and a
// [tool.astro] runtime build (Request.CheckRuntime). It also warns about the
// per-machine files a declared Dockerfile's build would copy into the image
// (scaffold.LocalFilesWarning). A dags-only deploy and one
// adopting a prebuilt --image-name build nothing from the manifest, and are not
// held to it. It returns the build secrets the declared Dockerfile mounts and
// nobody gave, for a failed build to name again.
func checkImageSource(req Request) (imagebuild.MissingSecrets, error) {
	var missing imagebuild.MissingSecrets
	if req.DagsOnly || req.ImageName != "" || req.Manifest == nil {
		return missing, nil
	}
	if err := scaffold.CheckDockerfileAirflow(req.ProjectDir, req.Manifest); err != nil {
		return missing, err
	}
	warn := req.Warn
	if warn == nil {
		warn = func(string) {}
	}
	var err error
	if missing, err = imagebuild.CheckBuildSecrets(req.ProjectDir, req.Manifest.Astro.Dockerfile, req.BuildSecrets); err != nil {
		return missing, err
	}
	missing.Hint = util.BuildSecretHint
	for _, w := range missing.Warnings() {
		warn(w)
	}
	if w := scaffold.LocalFilesWarning(req.ProjectDir, req.Manifest.Astro.Dockerfile); w != "" {
		warn(w)
	}
	airflow := req.Manifest.Airflow()
	if airflow.Runtime == "" || req.CheckRuntime == nil {
		return missing, nil
	}
	warnings, err := req.CheckRuntime(airflow.Runtime, airflow.Pin)
	for _, w := range warnings {
		warn(manifest.Marker + ": tool.astro.runtime: " + w.Message)
	}
	return missing, err
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
func runImage(req Request, target Target, d Deployer) (Result, error) {
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
	includeDags := !req.Image
	img, err := d.DeployImage(&ImageDeploy{
		DeploymentID:  target.DeploymentID,
		WorkspaceID:   target.WorkspaceID,
		Build:         imagebuild.ManifestBuildOf(req.ProjectDir, req.Manifest),
		BuildSecrets:  req.BuildSecrets,
		ImageName:     req.ImageName,
		OnBuild:       req.OnBuild,
		IncludeDags:   includeDags,
		Description:   req.Description,
		NoDagsBaseDir: req.NoDagsBaseDir,
		Wait:          req.Wait,
		WaitTime:      req.WaitTime,
	})
	if err != nil {
		return Result{}, err
	}
	kind := "image-only"
	if includeDags {
		kind = "image-and-dag"
	}
	return Result{
		DeploymentID:      target.DeploymentID,
		WorkspaceID:       firstNonEmpty(img.WorkspaceID, target.WorkspaceID),
		Type:              kind,
		RuntimeVersion:    img.RuntimeVersion,
		ImageTag:          img.ImageTag,
		DagTarballVersion: img.DagTarballVersion,
		URL:               img.URL,
		LinkName:          target.LinkName,
		Git:               img.Git,
	}, nil
}

// Target is the deployment a run ships to, once it is settled.
type Target struct {
	DeploymentID string
	WorkspaceID  string
	// LinkName is the manifest link the deploy resolved to, "" when the target
	// was named by id or picked through the workspace-level flow.
	LinkName string
}

// Choice is one deployable link as the prompt offers it.
type Choice struct {
	Name string
	// Where is the coordinate as the manifest writes it, so two links are told
	// apart by where they point and not only by what they are called.
	Where string
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

	if req.LinkName != "" && req.Deployment != "" && req.LinkName != req.Deployment {
		return Target{}, fmt.Errorf("this deploy names two targets, %q and --deployment %q: name one", req.LinkName, req.Deployment)
	}
	if name := firstNonEmpty(req.LinkName, req.Deployment); name != "" {
		return namedTarget(req, name, links)
	}

	deployable := DeployableLinks(links)
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
		return Target{}, fmt.Errorf("astro deploy ships to Astro Deployments, and this project links none%s", nonAstroLinks(links))
	case !req.Interactive:
		// The question an interactive run asks, refused: input_required under
		// --output json, in deploy's own words.
		return Target{}, input.Required(fmt.Errorf("a deploy must name the deployment it ships to: `astro deploy <name>` or --deployment <name>. "+
			"Deploy never picks for you — `astro use`, %s, or `default = true` only preselect the prompt. Deployable links: %s",
			deploymentEnvVar, strings.Join(deployable, ", ")))
	}

	choices := make([]Choice, 0, len(deployable))
	for _, name := range deployable {
		choices = append(choices, Choice{Name: name, Where: "astro deployment " + links[name].Deployment})
	}
	name, err := d.ConfirmTarget(choices, preselect(req, links, deployable))
	if err != nil {
		return Target{}, err
	}
	if name == "" {
		return Target{}, ErrAborted
	}
	link, ok := links[name]
	if !ok {
		return Target{}, fmt.Errorf("no deployment link %q in the manifest%s", name, knownLinks(links))
	}
	return astroTarget(req, name, link, links)
}

// namedTarget resolves a target named on the command line, by the positional
// argument or --deployment alike: a manifest link, else an Astro Deployment id.
// The id fall-through is what both meant before they learned link names — the
// usage line is `astro deploy DEPLOYMENT-ID`, and astronomer/deploy-action runs
// exactly that — so a CI job that passes an id keeps working.
func namedTarget(req Request, name string, links map[string]manifest.Link) (Target, error) {
	if link, ok := links[name]; ok {
		return astroTarget(req, name, link, links)
	}
	return Target{
		DeploymentID: name,
		WorkspaceID:  firstNonEmpty(req.WorkspaceID, req.ContextWorkspace),
	}, nil
}

// unlinkedTarget runs the workspace-level pick/create flow, for a project whose
// manifest declares no deployment links at all.
func unlinkedTarget(req Request, d Deployer) (Target, error) {
	workspaceID := firstNonEmpty(req.WorkspaceID, req.ContextWorkspace)
	if workspaceID == "" {
		return Target{}, errors.New("a workspace is required for a deploy: pass --workspace or set a current workspace")
	}
	if !req.Interactive {
		return Target{}, input.Required(errors.New("this project links no deployment and this run cannot be asked: pass --deployment <name or id>"))
	}
	id, err := d.ResolveUnlinked(workspaceID)
	if err != nil {
		return Target{}, err
	}
	if id == "" {
		return Target{}, ErrAborted
	}
	return Target{DeploymentID: id, WorkspaceID: workspaceID}, nil
}

// Preselect is the entry the prompt highlights, and why.
//
// The reason travels with the name because the label is a claim about where the
// highlight came from, and only one of the three sources is the manifest's
// marker. A prompt that says "← default" over an entry ASTRO_DEPLOYMENT put
// there is telling the reader their file says something it does not — which is
// the invisible-state surprise "deploy always asks" exists to prevent, reintroduced as a
// caption.
type Preselect struct {
	// Name is the deployment to highlight, "" when nothing points anywhere.
	Name string
	// From is what put it there, for the label: an env var's name, "pinned",
	// or "default = true".
	From string
}

// The three things that can move the prompt's cursor. DefaultMarker is spelled
// as the manifest spells it, so the label and the file agree.
const (
	PinnedBy      = "astro use"
	DefaultMarker = "default = true"
	// deploymentEnvVar is the ephemeral layer of the query commands' rule,
	// named here so the non-interactive refusal tells the reader what deploy is
	// deliberately ignoring. It is spelled out rather than imported because
	// pkg/instances is the resolver deploy does not use.
	deploymentEnvVar = "ASTRO_DEPLOYMENT"
)

// preselect works out which entry to highlight: whatever the ambient layers
// point at, else the manifest's default link. A value that names nothing this
// command can ship to highlights nothing — it must not silently move the cursor
// onto a neighbor.
//
// The order — env, then pin, then marker — is the query commands' resolution
// chain, so the cursor lands where `astro af dags list` would have gone. Only the
// deciding is different here; the ranking is the same one rule.
func preselect(req Request, links map[string]manifest.Link, deployable []string) Preselect {
	if req.Preselect != "" && slices.Contains(deployable, req.Preselect) {
		from := req.PreselectFrom
		if from == "" {
			// A caller that named no source still gets a label, because a bare
			// arrow tells the reader less than nothing.
			from = "selected"
		}
		return Preselect{Name: req.Preselect, From: from}
	}
	if name, _, ok := manifest.DefaultLink(links); ok && slices.Contains(deployable, name) {
		return Preselect{Name: name, From: DefaultMarker}
	}
	return Preselect{}
}

// astroTarget turns a resolved link into a target. Only an astro link can be
// deployed to today: this command ships an image to an Astro Deployment, and an
// mwaa or composer link has no deployment id to ship it to.
func astroTarget(req Request, name string, link manifest.Link, links map[string]manifest.Link) (Target, error) {
	if kind := link.Kind(); kind != manifest.KindAstro {
		return Target{}, fmt.Errorf("link %q is %s, and astro deploy ships to Astro Deployments%s", name, kind, deployableLinks(links))
	}
	return Target{
		DeploymentID: link.Deployment,
		WorkspaceID:  firstNonEmpty(req.WorkspaceID, link.Workspace, req.ContextWorkspace),
		LinkName:     name,
	}, nil
}

// DeployableLinks names the links astro deploy can ship to, sorted.
func DeployableLinks(links map[string]manifest.Link) []string {
	return linkNames(links, isAstroLink)
}

func isAstroLink(l manifest.Link) bool { return l.Kind() == manifest.KindAstro }

// nonAstroLinks names what a project full of unshippable links actually
// declares, so the refusal above says what is there rather than only what is
// not.
func nonAstroLinks(links map[string]manifest.Link) string {
	named := make([]string, 0, len(links))
	for _, name := range linkNames(links, nil) {
		named = append(named, fmt.Sprintf("%s is %s", name, links[name].Kind()))
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
func deployableLinks(links map[string]manifest.Link) string {
	astro := linkNames(links, func(l manifest.Link) bool { return l.Kind() == manifest.KindAstro })
	if len(astro) == 0 {
		return " (this project has no astro links)"
	}
	return ", astro links: " + strings.Join(astro, ", ")
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
