// Package deploy holds the v2 deploy logic that `astro deploy` routes into
// when the working directory is a v2 project (a pyproject.toml with a
// [tool.astro] table). It classifies the project for routing, resolves which
// deployment to ship to from the manifest and flags, and drives the deploy
// through an injected transport — a dags-only deploy, an image deploy, or both.
//
// The package keeps to the v2 layer rules (docs/v2-architecture.md): it never
// prints, never exits, and never touches config, Docker, or the network
// directly. cmd owns flag parsing, config, and rendering; the deploy transport
// arrives as an interface, so the logic here stays unit-testable with a fake.
package deploy

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// IsV2Project reports whether dir holds a v2 project: a pyproject.toml carrying
// a [tool.astro] table. A pyproject that fails to parse, or one whose
// [tool.astro] fails validation, still counts as v2 — it is a v2 project with a
// manifest to fix, and the v2 path gives the clearer error. A pyproject without
// [tool.astro] (a plain Python project) and a missing pyproject are not v2.
func IsV2Project(dir string) bool {
	_, err := manifest.Load(filepath.Join(dir, project.Marker))
	switch {
	case err == nil:
		return true
	case errors.Is(err, manifest.ErrNotFound), errors.Is(err, manifest.ErrNoAstroSection):
		return false
	default:
		return true
	}
}

// Request is the resolved input for a v2 deploy. cmd fills it from flags, args,
// the manifest, and the current context, then hands it to Run.
type Request struct {
	// ProjectDir is the v2 project root (where dags/ and pyproject.toml live).
	ProjectDir string
	// Manifest is the loaded pyproject.toml: deployment links, the Airflow pin,
	// the project's dependencies and OS packages.
	Manifest *manifest.Manifest
	// LinkName is the positional argument naming a deployment link, "" if none.
	LinkName string
	// DeploymentID is the --deployment override; it always beats the manifest.
	DeploymentID string
	// WorkspaceID is the --workspace override, "" if unset.
	WorkspaceID string
	// ContextWorkspace is the workspace from the current context, the fallback.
	ContextWorkspace string
	// DagsOnly is --dags: ship only the dags/ directory, no image, no Docker.
	DagsOnly bool
	// Image is --image: an image-only deploy, leaving the running dags in place.
	Image bool
	// ImageName is --image-name: a prebuilt local image to deploy instead of
	// building from the manifest. "" means build.
	ImageName string
	// Description is recorded on the deploy.
	Description string
	// Wait polls the deployment until it reports healthy.
	Wait     bool
	WaitTime time.Duration
	// NoDagsBaseDir drops the dags/ prefix from the bundle (Airflow 3 layouts).
	NoDagsBaseDir bool
	// Interactive reports whether the run may prompt (stdin is a TTY). A
	// non-interactive run with no named or default link must pass --deployment.
	Interactive bool
}

// Result is what a finished v2 deploy reports back to cmd for rendering. Text
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
}

// ImageDeploy is the request the transport's image deploy takes. The deployment
// is already resolved. When ImageName is set the transport deploys that prebuilt
// local image; otherwise it builds from the manifest fields. IncludeDags marks a
// "both" deploy, which also ships the dags/ tarball.
type ImageDeploy struct {
	DeploymentID   string
	WorkspaceID    string
	ProjectDir     string
	AirflowVersion string
	Dependencies   []string
	Packages       []string
	ImageName      string
	IncludeDags    bool
	Description    string
	NoDagsBaseDir  bool
	Wait           bool
	WaitTime       time.Duration
}

// ImageResult is what the transport reports after an image deploy. A "both"
// deploy also carries a DagTarballVersion.
type ImageResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	ImageTag          string
	DagTarballVersion string
	URL               string
}

// Deployer is the seam onto the deploy transport. The CLI wires it to the
// cloud/deploy transport (create deploy, build and push the image, upload the
// dag tarball, finalize); tests supply a fake. The interface keeps the v2 logic
// free of config, Docker, and the network so it stays unit-testable.
type Deployer interface {
	// ResolveUnlinked runs the workspace-level pick/create flow and returns the
	// chosen deployment id. It is called only when no link is named or
	// defaulted, and only in an interactive run.
	ResolveUnlinked(workspaceID string) (deploymentID string, err error)
	// DeployDags creates a DAG-only deploy, uploads the project's dags/
	// directory, and finalizes.
	DeployDags(*DagDeploy) (DagResult, error)
	// DeployImage builds (or adopts) the project image, pushes it, and
	// finalizes; a "both" deploy also uploads the dags. It requires Docker and
	// returns a plain error when it is unreachable.
	DeployImage(*ImageDeploy) (ImageResult, error)
}

// Run resolves the deployment and drives the deploy for a v2 project: a
// dags-only deploy (--dags), an image-only deploy (--image), or the default
// "both" (image + dags).
func Run(req Request, d Deployer) (Result, error) {
	// --dags ships only the dags/ directory, so an image source makes no sense
	// with it.
	if req.DagsOnly && (req.Image || req.ImageName != "") {
		return Result{}, errors.New("--dags deploys only your DAGs; drop --image and --image-name")
	}

	sel, err := resolveSelection(req)
	if err != nil {
		return Result{}, err
	}

	if sel.deploymentID == "" {
		// The unlinked, workspace-level flow: no link named or defaulted.
		if sel.workspaceID == "" {
			return Result{}, errors.New("a workspace is required for a deploy: pass --workspace or set a current workspace")
		}
		if !req.Interactive {
			return Result{}, errors.New("no deployment link is named or defaulted, and this is a non-interactive run: pass --deployment <id>")
		}
		id, err := d.ResolveUnlinked(sel.workspaceID)
		if err != nil {
			return Result{}, err
		}
		if id == "" {
			return Result{}, errors.New("no deployment selected")
		}
		sel.deploymentID = id
	}

	if req.DagsOnly {
		return runDagsOnly(req, sel, d)
	}
	return runImage(req, sel, d)
}

// runDagsOnly ships just the dags/ directory through the transport.
func runDagsOnly(req Request, sel selection, d Deployer) (Result, error) {
	dag, err := d.DeployDags(&DagDeploy{
		DeploymentID:  sel.deploymentID,
		WorkspaceID:   sel.workspaceID,
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
		DeploymentID:      sel.deploymentID,
		WorkspaceID:       firstNonEmpty(dag.WorkspaceID, sel.workspaceID),
		Type:              "dag-only",
		RuntimeVersion:    dag.RuntimeVersion,
		DagTarballVersion: dag.DagTarballVersion,
		URL:               dag.URL,
		LinkName:          sel.linkName,
	}, nil
}

// runImage builds or adopts the project image and ships it, plus the dags for a
// default "both" deploy. --image drops the dags.
func runImage(req Request, sel selection, d Deployer) (Result, error) {
	var deps, packages []string
	airflowVersion := ""
	if req.Manifest != nil {
		deps = req.Manifest.Project.Dependencies
		packages = req.Manifest.Astro.Packages
		airflowVersion = req.Manifest.Astro.AirflowVersion
	}
	includeDags := !req.Image
	img, err := d.DeployImage(&ImageDeploy{
		DeploymentID:   sel.deploymentID,
		WorkspaceID:    sel.workspaceID,
		ProjectDir:     req.ProjectDir,
		AirflowVersion: airflowVersion,
		Dependencies:   deps,
		Packages:       packages,
		ImageName:      req.ImageName,
		IncludeDags:    includeDags,
		Description:    req.Description,
		NoDagsBaseDir:  req.NoDagsBaseDir,
		Wait:           req.Wait,
		WaitTime:       req.WaitTime,
	})
	if err != nil {
		return Result{}, err
	}
	kind := "image-only"
	if includeDags {
		kind = "image-and-dag"
	}
	return Result{
		DeploymentID:      sel.deploymentID,
		WorkspaceID:       firstNonEmpty(img.WorkspaceID, sel.workspaceID),
		Type:              kind,
		RuntimeVersion:    img.RuntimeVersion,
		ImageTag:          img.ImageTag,
		DagTarballVersion: img.DagTarballVersion,
		URL:               img.URL,
		LinkName:          sel.linkName,
	}, nil
}

// selection is the resolved deployment target. An empty deploymentID means the
// unlinked, workspace-level flow.
type selection struct {
	deploymentID string
	workspaceID  string
	linkName     string
}

// resolveSelection picks the deployment target from the flags and manifest, in
// the order the design doc lays out: an explicit --deployment wins, then a
// named link, then the link marked default = true, then the lone-link default,
// then the unlinked workspace-level flow.
func resolveSelection(req Request) (selection, error) {
	// --deployment always overrides the manifest, so CI can target a deployment
	// that is not in the file.
	if req.DeploymentID != "" {
		return selection{
			deploymentID: req.DeploymentID,
			workspaceID:  firstNonEmpty(req.WorkspaceID, req.ContextWorkspace),
		}, nil
	}

	var links map[string]manifest.Deployment
	if req.Manifest != nil {
		links = req.Manifest.Astro.Deployments
	}

	// A named argument wins over the default.
	if req.LinkName != "" {
		link, ok := links[req.LinkName]
		if !ok {
			return selection{}, fmt.Errorf("no deployment link %q in the manifest%s", req.LinkName, knownLinks(links))
		}
		return selection{
			deploymentID: link.Deployment,
			workspaceID:  firstNonEmpty(req.WorkspaceID, link.Workspace, req.ContextWorkspace),
			linkName:     req.LinkName,
		}, nil
	}

	// Otherwise the default link: a link marked default = true, or — the interim
	// rule — a project's one link when nothing is marked (design doc section 3).
	if name, link, ok := defaultLink(links); ok {
		return selection{
			deploymentID: link.Deployment,
			workspaceID:  firstNonEmpty(req.WorkspaceID, link.Workspace, req.ContextWorkspace),
			linkName:     name,
		}, nil
	}

	// Nothing named or defaulted: the unlinked, workspace-level flow.
	return selection{
		workspaceID: firstNonEmpty(req.WorkspaceID, req.ContextWorkspace),
	}, nil
}

// defaultLink returns the link `astro deploy` ships to with no argument. A link
// marked default = true wins; the manifest guarantees at most one. With none
// marked, the interim rule keeps its promise: a project's one link is its
// default. A one-link project behaves the same whether that link is marked.
func defaultLink(links map[string]manifest.Deployment) (string, manifest.Deployment, bool) {
	for name, link := range links {
		if link.Default {
			return name, link, true
		}
	}
	if len(links) == 1 {
		for name, link := range links {
			return name, link, true
		}
	}
	return "", manifest.Deployment{}, false
}

func knownLinks(links map[string]manifest.Deployment) string {
	if len(links) == 0 {
		return " (the manifest declares no deployment links)"
	}
	names := make([]string, 0, len(links))
	for name := range links {
		names = append(names, name)
	}
	sort.Strings(names)
	return ", known links: " + strings.Join(names, ", ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
