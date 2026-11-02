package imagebuild

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A project's own Dockerfile whose final stage builds FROM localhost/astro-base
// (airflowrt.InstallBaseRef, Request.InstallBase) installs what a generated
// build installs, resolved by the CLI before the build. The CLI builds that
// install into an install base first: the runtime's -base image the manifest
// picks (Request.BaseImage), the generated build's install steps over it,
// without the project's files (installSteps), tagged with the digest of what
// it installs (InstallBaseTag). It then builds the project's Dockerfile with
// the project as its context, from a copy of the file in its own work
// directory whose FROM names that exact tag (pinnedDockerfile): the project's
// file is never modified, and two builds of two copies of the project, with
// different packages, each build on their own base. Both are plain builds
// every builder runs.

// installBaseDir is the sub-directory of a Request's WorkDir an install base
// is built in: its Dockerfile, and the lists as its context.
const installBaseDir = "install-base"

// UsesInstallBase reports whether the request builds a declared Dockerfile
// whose final stage is FROM the install base (InstallBase): only then does
// the build need one.
func (r *Request) UsesInstallBase() bool {
	return r.FromDeclaredDockerfile() && r.InstallBase != ""
}

// installBaseDockerfile is the install base's Dockerfile for r: the install
// steps over the -base tag of its runtime, mounting the netrc secret when r
// carries one, as a generated build does.
func (r *Request) installBaseDockerfile() string {
	return installSteps(baseOf(r.BaseImage), r.hasRuntimeSecret())
}

// hasRuntimeSecret reports a manifest.RuntimeSecretID spec among the
// request's secrets, the one secret an install mounts.
func (r *Request) hasRuntimeSecret() bool {
	for _, spec := range r.Secrets {
		if s, err := manifest.ParseBuildSecret(spec); err == nil && s.ID == manifest.RuntimeSecretID {
			return true
		}
	}
	return false
}

// InstallDigest is the digest of what an install base of r installs: the
// runtime image it is built over (BaseImage, which names the build, its
// Python and its registry) and the lists. The base carries it as a label
// (airflowrt.InstallBaseDigestLabel), so a deploy of an image built on it can
// tell whether the project still installs the same.
func (r *Request) InstallDigest() string {
	h := sha256.New()
	fmt.Fprintf(h, "from\x00%s\x00lists\x00%s\x00", r.BaseImage, r.Lists().Digest())
	return fmt.Sprintf("%x", h.Sum(nil)[:6])
}

// InstallBaseTag is the exact tag the install base of r is built under:
// InstallBase with a digest of what decides the image (what it installs, its
// Dockerfile and the platform), so an unchanged base keeps its tag and the
// engine's cache, and a changed one gets a new tag.
func (r *Request) InstallBaseTag() string {
	h := sha256.New()
	fmt.Fprintf(h, "install\x00%s\x00dockerfile\x00%s\x00platform\x00%s\x00project\x00%s\x00", r.InstallDigest(), r.installBaseDockerfile(), r.Platform, r.Project)
	return fmt.Sprintf("%s:%x", r.InstallBase, h.Sum(nil)[:6])
}

// installBaseLabels are the labels the install base of r carries
// (airflowrt.InstallBaseProjectLabel and the rest).
func (r *Request) installBaseLabels() []string {
	return []string{
		airflowrt.InstallBaseProjectLabel + "=" + r.Project,
		airflowrt.InstallBaseDigestLabel + "=" + r.InstallDigest(),
		airflowrt.InstallBaseFromLabel + "=" + r.BaseImage,
	}
}

// buildInstallBase builds the install base of req (InstallBaseTag) in a
// directory of WorkDir, started empty, and returns its tag. It is a build of
// the CLI's own, as a generated one is: its base pulled fresh, given the netrc
// secret alone, with the install base's own labels (installBaseLabels), which
// an image built on it inherits. For InstallBaseOnly it is also tagged
// InstallBase, the plain tag a hand-run `docker build` of the project reads,
// and that is the tag returned.
func (b *Builder) buildInstallBase(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	if req.BaseImage == "" || req.WorkDir == "" {
		return "", fmt.Errorf("the Dockerfile this project declares (%s) builds FROM %s, its install base, and no runtime or build directory was given to build it", req.Dockerfile, req.InstallBase)
	}
	dir := filepath.Join(req.WorkDir, installBaseDir)
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clearing %s: %w", dir, err)
	}
	contextDir := filepath.Join(dir, buildContextDir)
	if err := writeLists(contextDir, req.Lists()); err != nil {
		return "", err
	}
	dfPath := filepath.Join(dir, dockerfileName)
	if err := os.WriteFile(dfPath, []byte(req.installBaseDockerfile()), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", dfPath, err)
	}
	base := req
	base.Dockerfile, base.Context, base.Labels = "", "", req.installBaseLabels()
	base.Tag = req.InstallBaseTag()
	var also []string
	if req.InstallBaseOnly {
		also = []string{req.InstallBase}
	}
	tag, err := b.build(ctx, base, dfPath, contextDir, also, cb)
	if err != nil || !req.InstallBaseOnly {
		return tag, err
	}
	return req.InstallBase, nil
}

// pinnedDockerfile writes a copy of req's Dockerfile into dir whose FROM line
// names base, the exact install base built for this build, in place of the
// one the file names, and returns its path. A file-specific ignore file
// (<Dockerfile>.dockerignore) beside the Dockerfile is copied beside the copy,
// where BuildKit looks for it, so the build reads the project as it would.
func pinnedDockerfile(req Request, dir, base string) (string, error) {
	data, err := os.ReadFile(req.Dockerfile)
	if err != nil {
		return "", fmt.Errorf("the Dockerfile this project declares (%s) could not be read: %w", req.Dockerfile, err)
	}
	ref, _ := airflowrt.InstallBaseRef(data)
	if err := os.MkdirAll(dir, contextDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	out := filepath.Join(dir, filepath.Base(req.Dockerfile))
	if err := os.WriteFile(out, airflowrt.ReplaceFrom(data, ref, base), filePermRW); err != nil { //nolint:gosec // G703: out is under the build's own work directory, or the caller's new or empty --context-dir
		return "", fmt.Errorf("writing %s: %w", out, err)
	}
	ignore := req.Dockerfile + dockerIgnoreSuffix
	switch _, err := os.Stat(ignore); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return "", fmt.Errorf("reading %s: %w", ignore, err)
	default:
		if err := copyFile(ignore, out+dockerIgnoreSuffix, filePermRW); err != nil {
			return "", fmt.Errorf("copying %s: %w", ignore, err)
		}
	}
	return out, nil
}

// dockerIgnoreSuffix names a Dockerfile's own ignore file:
// <Dockerfile>.dockerignore.
const dockerIgnoreSuffix = ".dockerignore"

// writeLists writes the lists' files into dir, creating it.
func writeLists(dir string, lists InstallLists) error {
	if err := os.MkdirAll(dir, contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	for name, data := range lists.Files() {
		if err := os.WriteFile(filepath.Join(dir, name), data, filePermRW); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

// WithRegistry is ref, a runtime image reference on one of Astronomer's
// repositories (astrocrpublic.azurecr.io/runtime:3.3-8), pulled from registry
// instead ([tool.astro] registry): registry.example.com/astronomer/runtime:3.3-8,
// the repository's last path segment and the tag kept. An empty registry
// returns ref as it is.
func WithRegistry(ref, registry string) string {
	if registry == "" {
		return ref
	}
	return registry + "/" + ref[strings.LastIndex(ref, "/")+1:]
}

// The layout WriteDeclaredContext writes under its directory.
const (
	// DeclaredBaseDir holds the install base's build: its Dockerfile and the
	// lists.
	DeclaredBaseDir = "base"
	// DeclaredProjectDir holds the project as the build of its own
	// Dockerfile reads it, that Dockerfile included, its FROM naming the
	// exact base.
	DeclaredProjectDir = "project"
)

// WriteDeclaredContext writes what the two builds of a Dockerfile built on the
// install base (UsesInstallBase) read into dir, which must not exist or be
// empty, for two plain `docker build` runs with nothing from the CLI:
// DeclaredBaseDir, the install base's Dockerfile (mounting the netrc secret
// only when req carries one, as Build does, so without one Docker's legacy
// builder runs it too) and the lists; and DeclaredProjectDir, the project as
// the build of its own Dockerfile reads it, under that file's ignore file as
// docker applies it, links kept as links, with the Dockerfile at its own path,
// its FROM naming the exact base, InstallBaseTag.
func WriteDeclaredContext(dir string, req Request) error {
	if !req.UsesInstallBase() {
		return errors.New("the project's Dockerfile does not build FROM its install base, so the project is its build context")
	}
	switch entries, err := os.ReadDir(dir); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("reading %s: %w", dir, err)
	case len(entries) > 0:
		return fmt.Errorf("%s %w", dir, ErrContextDirNotEmpty)
	}
	projectDir := filepath.Join(dir, DeclaredProjectDir)
	if err := os.MkdirAll(projectDir, contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", projectDir, err)
	}
	if err := copyDeclaredContext(req.Context, req.Dockerfile, dir, projectDir); err != nil {
		return err
	}
	rel, err := filepath.Rel(req.Context, req.Dockerfile)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	if _, err := pinnedDockerfile(req, filepath.Dir(filepath.Join(projectDir, rel)), req.InstallBaseTag()); err != nil {
		return err
	}
	baseDir := filepath.Join(dir, DeclaredBaseDir)
	if err := writeLists(baseDir, req.Lists()); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(baseDir, contextDockerfileName), []byte(req.installBaseDockerfile()), filePermRW); err != nil {
		return fmt.Errorf("writing %s: %w", contextDockerfileName, err)
	}
	return nil
}

// copyDeclaredContext copies the project at src into dst as a build of its
// Dockerfile at dockerfile reads it: under docker's ignore file, version
// control and links included, each link as itself. out, the directory being
// written, is left out when it is inside the project (`--context-dir
// ./build`), so the context never carries a copy of itself.
func copyDeclaredContext(src, dockerfile, out, dst string) error {
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	outInfo, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	rel, err := filepath.Rel(src, dockerfile)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	filter, err := newDeclaredFilter(resolved, declaredIgnoreFile(resolved, filepath.ToSlash(rel), false))
	if err != nil {
		return err
	}
	w := walker{filter: filter, worktrees: projectWorktrees(resolved), exclude: outInfo, visit: func(rel, from string, mode fs.FileMode) error {
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), ownerAll); err != nil {
			return err
		}
		if mode&fs.ModeSymlink != 0 {
			link, err := os.Readlink(from)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(from, target, mode.Perm())
	}}
	if err := w.walkDir(resolved, "", map[string]bool{resolved: true}); err != nil {
		return fmt.Errorf("copying the project's files: %w", err)
	}
	return nil
}
