// Package pack builds the artifact a given Airflow platform consumes from a v2
// project's manifest, without shipping it — the build stage of a CI pipeline
// (docs/v2-deploy.md, section 4). Each platform eats a different shape (Astro a
// container image, MWAA and Composer a directory laid out for a bucket, OSS a
// plain image or bundle), so a target is a first-class argument, not a hidden
// assumption. One small Target interface fronts them all, so adding a platform
// is adding a file, not editing a switch.
//
// It keeps to the v2 layer rules (docs/v2-architecture.md): nothing here prints
// or exits. A target validates the manifest for its platform, builds an
// artifact with a named Kind, and reports a Result the caller renders as text
// or json. Docker and the network reach it only through injected seams, so the
// logic stays unit-testable with a fake.
package pack

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Kind names the shape of a packaged artifact, so one json reader handles every
// target: an image (a local Docker tag, optionally saved to a tarball), a tree
// (a directory laid out for a bucket sync), or a bundle (a tarball).
type Kind string

const (
	KindImage  Kind = "image"
	KindTree   Kind = "tree"
	KindBundle Kind = "bundle"
)

// Target names the registered targets, in listing order.
const (
	TargetAstro    = "astro"
	TargetMWAA     = "mwaa"
	TargetComposer = "composer"
	TargetOSS      = "oss"
)

// Request is the resolved input for a package build. cmd fills it from the
// discovered project, the loaded manifest, and the flags, then hands it to a
// target.
type Request struct {
	// ProjectDir is the v2 project root (where dags/ and pyproject.toml live).
	ProjectDir string
	// Manifest is the loaded pyproject.toml: the Airflow pin, the project's
	// dependencies and OS packages. A target validates it for its platform.
	Manifest *manifest.Manifest
	// Save, when set, also writes the artifact to this path (a .tar for image
	// targets).
	Save string
	// Tag overrides the image reference for image targets; "" uses the default
	// content-addressed name.
	Tag string
	// Platform is the build platform for image targets (e.g. "linux/amd64").
	Platform string
	// WorkDir is a scratch directory for the build context. Empty means the
	// target makes and removes its own temp dir.
	WorkDir string
	// OutDir overrides where a tree target writes its artifact directory. Empty
	// means the default: <ProjectDir>/dist/<target>.
	OutDir string
}

// Result is the packaged artifact — the value both text and json render. The
// Target and Kind fields are always set; the rest depend on the artifact shape,
// so one reader handles every target.
type Result struct {
	Target string `json:"target"`
	Kind   Kind   `json:"kind"`
	// Image and RuntimeVersion carry an image target's tag and resolved runtime
	// version; TreePath and BundlePath a tree or bundle target's path.
	Image          string `json:"image,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`
	TreePath       string `json:"tree_path,omitempty"`
	BundlePath     string `json:"bundle_path,omitempty"`
	// SavedPath is set only with --save; Size is the saved artifact's size in
	// bytes when it is cheap to read (the saved tarball or zip), else 0.
	SavedPath string `json:"saved_path,omitempty"`
	Size      int64  `json:"size,omitempty"`
	// DepsFile is the requirements/dependency file a tree target wrote — the
	// file MWAA reads from its bucket, or the file the Composer environment
	// update consumes. Empty for image targets.
	DepsFile string `json:"deps_file,omitempty"`
	// Warnings are non-fatal findings the artifact is still valid despite: an
	// Airflow pin the platform does not list, OS packages it cannot install.
	Warnings []string `json:"warnings,omitempty"`
	// NextSteps are the exact commands to ship the artifact — the upload
	// hand-off a tree target cannot do itself (it never touches the network).
	NextSteps []string `json:"next_steps,omitempty"`
}

// Target packages a v2 project for one Airflow platform. Build validates the
// manifest for the platform, produces the artifact, and reports it; build
// output streams through cb.
type Target interface {
	Name() string
	Build(ctx context.Context, req Request, cb localrt.Callbacks) (Result, error)
}

// Registry looks a target up by name. It holds the fully-built astro target and
// staged stubs for the rest, so `astro package <target>` resolves a name to an
// implementation without a switch.
type Registry struct {
	byName map[string]Target
	order  []string
}

// NewRegistry builds the registry over a fully-built astro target. mwaa and
// composer are built here too — they touch only the filesystem, so they need no
// injected engine. oss stays a staged stub: named so the command lists it, but
// erroring "not built yet" until its stage lands (docs/v2-deploy.md, section 4).
func NewRegistry(astro Target) *Registry {
	targets := []Target{
		astro,
		NewMWAATarget(),
		NewComposerTarget(),
		stagedTarget{name: TargetOSS},
	}
	r := &Registry{byName: make(map[string]Target, len(targets))}
	for _, t := range targets {
		r.byName[t.Name()] = t
		r.order = append(r.order, t.Name())
	}
	return r
}

// Lookup returns the target with the given name. An unknown name errors and
// lists the known ones.
func (r *Registry) Lookup(name string) (Target, error) {
	if t, ok := r.byName[name]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("unknown package target %q (known targets: %s)", name, strings.Join(r.Names(), ", "))
}

// Names lists the registered target names in listing order.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// StagedError reports a target that is registered but not built yet. cmd renders
// it as the command's error; a test asserts on it with errors.As.
type StagedError struct {
	Target string
}

func (e *StagedError) Error() string {
	return fmt.Sprintf("the %q package target is registered but not built yet", e.Target)
}

// stagedTarget is a placeholder for a platform whose stage has not landed: it
// registers a name so the command lists it, and refuses to build.
type stagedTarget struct {
	name string
}

func (s stagedTarget) Name() string { return s.name }

func (s stagedTarget) Build(context.Context, Request, localrt.Callbacks) (Result, error) {
	return Result{}, &StagedError{Target: s.name}
}

// sortedCopy returns a sorted copy of ss, leaving the input untouched. The
// content hash sorts its inputs so reordering the manifest does not change the
// tag.
func sortedCopy(ss []string) []string {
	out := append([]string(nil), ss...)
	sort.Strings(out)
	return out
}
