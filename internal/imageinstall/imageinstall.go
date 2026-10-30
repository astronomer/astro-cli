// Package imageinstall decides what an Airflow image installs, on the host,
// before the build: it resolves the project with the host's uv and
// applies the image's policy to the lock (apply), handing the build its
// requirements.txt, uv-lock-constraints.txt and packages.txt ready-made
// (imagebuild.InstallLists). The build resolves nothing, so its context is
// self-contained and `docker build` runs it as written.
//
// Every generated build goes through Fill: local Docker mode, `astro package
// astro` and a deploy's build alike, so the three install the same versions
// from the same project. So does a project's own Dockerfile that astro init
// moved onto a -base tag, which reads the lists from the named build context
// astro-install (airflowrt.InstallDockerfile).
package imageinstall

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// UV is what the resolve asks of the host's uv (*uv.Client).
type UV interface {
	LockCheck(ctx context.Context, project string) error
	Lock(ctx context.Context, project string, stdio uv.Stdio) error
	ExportPylock(ctx context.Context, project string) ([]byte, error)
}

// Resolver resolves a project into the lists its image installs. Its fields
// are seams; Default fills them for production.
type Resolver struct {
	// UV finds the host's uv; an error (uv.ErrNotFound, most often) installs
	// the project unpinned, with a warning.
	UV func(ctx context.Context) (UV, error)
	// Catalog reads the runtime catalog, for the build and Python a floating
	// runtime tag serves; nil, or one returning nil, leaves those to the tag.
	Catalog func(ctx context.Context) *runtimeversions.Catalog
	// Constraints and Freeze read the runtime's constraints file and the
	// image's freeze, for a build and its Python (runtimecatalog's).
	Constraints, Freeze func(ctx context.Context, build, python string) ([]string, error)
}

// Default is the production Resolver: uv as a standalone start finds it, with
// the same cache, and the runtime's files from Astronomer's CDN (or their
// mirror), cached.
func Default() Resolver {
	return Resolver{
		UV: func(ctx context.Context) (UV, error) {
			root, err := localrt.CacheRoot()
			if err != nil {
				return nil, err
			}
			return uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
		},
		Catalog:     runtimecatalog.Catalog,
		Constraints: runtimecatalog.RuntimeConstraints,
		Freeze:      runtimecatalog.RuntimeFreeze,
	}
}

// Fill is Default().Fill.
func Fill(ctx context.Context, req *imagebuild.Request, warn func(string)) error {
	return Default().Fill(ctx, req, warn)
}

// Fill sets req.Install to what the generated build req describes installs,
// from its project's pyproject.toml and uv.lock (req.ManifestDir, else
// req.ProjectFiles), saying through warn what it could not do. A declared
// Dockerfile that reads the install lists from the named context
// (imagebuild.Request.ReadsInstallContext) is resolved the same way, against
// the runtime its final FROM names. Any other declared Dockerfile, a request
// naming no project, and a project with no pyproject.toml are left as they
// are, to install the request's own fields
// unpinned (imagebuild.UnpinnedLists), and so is a project uv cannot lock.
//
// A uv.lock that is missing or out of date with pyproject.toml, by `uv lock
// --check`, is written again with `uv lock`, in the project, with a warning
// to commit it: what is installed is then what the project would run. When
// uv cannot check or relock, a committed uv.lock is installed as it stands
// (settleLock). An error is returned for a lock that could not be listed, a
// pyproject.toml that cannot be read, and a lock made for another Airflow
// release than the image's base runs (*AirflowMismatchError), which refuses
// the build before it starts.
func (r Resolver) Fill(ctx context.Context, req *imagebuild.Request, warn func(string)) error {
	if warn == nil {
		warn = func(string) {}
	}
	// The runtime whose files the policy reads: a generated build's base, or
	// the image a declared Dockerfile's final stage names, an exact build or
	// a series tag alike (runtimeOf).
	base := req.BaseImage
	if req.FromDeclaredDockerfile() {
		if !req.ReadsInstallContext() {
			return nil
		}
		base = airflowrt.ReadDeclaredBase(req.Dockerfile).Ref()
	}
	project := req.ManifestDir
	if project == "" {
		project = req.ProjectFiles
	}
	if project == "" {
		return nil
	}
	pyproject, err := os.ReadFile(filepath.Join(project, "pyproject.toml"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("reading the project's pyproject.toml: %w", err)
	}

	client, err := r.UV(ctx)
	if err != nil {
		warn(fmt.Sprintf("the image installs pyproject.toml's dependencies without uv.lock's versions: uv is needed to read the lock, and %s. Install uv (https://docs.astral.sh/uv/) to build what the lock says", uvProblem(err)))
		return nil
	}
	settled, asCommitted := settleLock(ctx, client, project, warn)
	if !settled {
		return nil
	}
	exported, err := client.ExportPylock(ctx, project)
	switch {
	case err != nil && asCommitted:
		warn("uv could not read the committed uv.lock either, so the image installs pyproject.toml's dependencies without a lock's versions: " + err.Error())
		return nil
	case err != nil:
		return fmt.Errorf("listing what uv.lock installs: %w", err)
	}

	in := policyInput{pyproject: pyproject, pylock: exported, packages: req.Packages}
	build, python := r.runtimeOf(ctx, base)
	if build != "" && python != "" {
		if r.Constraints != nil {
			if pins, err := r.Constraints(ctx, build, python); err == nil {
				in.runtimePins = pins
			}
		}
		if r.Freeze != nil {
			if freeze, err := r.Freeze(ctx, build, python); err == nil {
				in.baseInstalled = freeze
			}
		}
	}
	recorded := ""
	if m, err := manifest.Parse(pyproject); err == nil {
		recorded = m.UV.RuntimeBuild
	}
	var catalog *runtimeversions.Catalog
	if r.Catalog != nil {
		catalog = r.Catalog(ctx)
	}
	dockerfile := ""
	if req.FromDeclaredDockerfile() {
		if rel, err := filepath.Rel(project, req.Dockerfile); err == nil {
			dockerfile = filepath.ToSlash(rel)
		}
	}
	if err := airflowMismatch(exported, in.baseInstalled, catalog, build, recorded, base, dockerfile); err != nil {
		return err
	}
	if in.baseInstalled == nil {
		warn(fmt.Sprintf("the package list of %s could not be read, so the image pins the project's own dependencies to uv.lock's versions and leaves the rest to the install", baseName(base)))
	}
	lists, warnings, err := apply(&in)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		warn(w)
	}
	req.Install = &lists
	return nil
}

// settleLock makes sure project has a uv.lock to install from: settled is
// false when it has none and uv could not write one, and asCommitted reports
// a lock uv did not vouch for, installed as it stands. Only uv's answer that the lock is
// missing or out of date (uv.ErrLockMissing, uv.ErrLockStale) runs uv lock in
// the project, with a warning to commit what it writes. A check uv could not
// make, offline or against an index it cannot reach, says nothing about the
// lock, so the committed one is installed as it stands; so is a stale one uv
// could not lock again, rather than nothing the project pinned.
func settleLock(ctx context.Context, client UV, project string, warn func(string)) (settled, asCommitted bool) {
	err := client.LockCheck(ctx, project)
	_, statErr := os.Stat(filepath.Join(project, "uv.lock"))
	hasLock := statErr == nil
	switch {
	case err == nil:
		return true, false
	case errors.Is(err, uv.ErrLockMissing), errors.Is(err, uv.ErrLockStale), !hasLock:
	default:
		warn("uv could not check uv.lock against pyproject.toml, so the image installs uv.lock as committed: " + err.Error())
		return true, true
	}
	warn("uv.lock is missing or out of date with pyproject.toml, so the build is running uv lock in the project; commit the uv.lock it writes")
	lockErr := client.Lock(ctx, project, uv.Stdio{})
	switch {
	case lockErr == nil:
		return true, false
	case hasLock:
		warn("uv could not lock the project again, so the image installs uv.lock as committed, without what pyproject.toml changed since: " + lockErr.Error())
		return true, true
	}
	warn("uv could not lock the project, so the image installs pyproject.toml's dependencies without a lock's versions: " + lockErr.Error())
	return false, false
}

// uvProblem says why uv could not be used, for a sentence.
func uvProblem(err error) string {
	if errors.Is(err, uv.ErrNotFound) {
		return "it was not found"
	}
	return "it could not be used: " + err.Error()
}

// baseName is the runtime an image is of, for a message: its tag.
func baseName(image string) string {
	if image == "" {
		return "the runtime"
	}
	return "runtime " + tagOf(image)
}

// tagOf is image's tag, without the -base suffix.
func tagOf(image string) string {
	ref := image[strings.LastIndex(image, "/")+1:]
	_, tag, ok := strings.Cut(ref, ":")
	if !ok {
		return ""
	}
	return strings.TrimSuffix(tag, "-base")
}

// runtimeOf is the runtime build an image is of and the Python it runs, from
// its tag: runtime:3.1-5-python-3.12 names both; runtime:3.1-5 and
// astro-runtime:12.6.0 the build, whose default Python the catalog (or, for
// Airflow 2, the runtime's major) names; a floating runtime:3.1 the series,
// whose newest build the catalog names. "" for what cannot be told.
func (r Resolver) runtimeOf(ctx context.Context, image string) (build, python string) {
	tag := tagOf(image)
	if tag == "" {
		return "", ""
	}
	if b, py, ok := strings.Cut(tag, "-python-"); ok {
		return b, py
	}
	var c *runtimeversions.Catalog
	if r.Catalog != nil {
		c = r.Catalog(ctx)
	}
	build = tag
	if c != nil {
		if _, ok := c.Runtime(tag); !ok {
			if newest, ok := c.NewestPublishedRuntimeTagged(tag); ok {
				build = newest
			}
		}
	}
	// An Airflow 3 series (3.1) with no catalog to name its build: the build
	// and its Python are not known. Airflow 2 runtimes start at 9.
	if major, _, _ := strings.Cut(build, "."); !strings.Contains(build, "-") && major == "3" {
		return "", ""
	}
	return build, runtimeversions.ImagePythonOf(c, build)
}
