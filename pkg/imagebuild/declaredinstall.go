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

// A project's own Dockerfile that builds its final stage FROM ${ASTRO_BASE}
// (airflowrt.BaseArg), as astro init writes a converted one, installs the
// same lists a generated build does, resolved by the CLI before the build. The
// CLI builds them into an install base first: the runtime's -base image the
// manifest picks (Request.BaseImage), with the install alone
// (airflowrt.InstallBaseDockerfile), tagged locally (InstallBaseTag). It then
// builds the project's Dockerfile with the project as its context and
// `--build-arg ASTRO_BASE=<that tag>`. Nothing is written into the project,
// the project's file states nothing about the install, and both builds are
// plain ones that every builder runs: Docker's legacy builder and any Podman
// take a build argument in FROM. A Dockerfile that does not build on the base
// is built as before.

// installBaseDir is the sub-directory of a Request's WorkDir an install base
// is built in: its Dockerfile and the lists, its context.
const installBaseDir = "install-base"

// UsesInstallBase reports whether the request builds a declared Dockerfile
// whose final stage is FROM the install base (airflowrt.UsesInstallBase):
// only then does the build need one. A Dockerfile that cannot be read builds
// on nothing; Build reports it.
func (r *Request) UsesInstallBase() bool {
	if !r.FromDeclaredDockerfile() {
		return false
	}
	data, err := os.ReadFile(r.Dockerfile)
	return err == nil && airflowrt.UsesInstallBase(data)
}

// installBaseDockerfile is the install base's Dockerfile for r, over the -base
// tag of its runtime, mounting the netrc secret when r carries one.
func (r *Request) installBaseDockerfile() string {
	return airflowrt.InstallBaseDockerfile(baseOf(r.BaseImage), hasRuntimeSecret(r.Secrets))
}

// hasRuntimeSecret reports a manifest.RuntimeSecretID spec among secrets, the
// one secret an install mounts.
func hasRuntimeSecret(secrets []string) bool {
	for _, spec := range secrets {
		if s, err := manifest.ParseBuildSecret(spec); err == nil && s.ID == manifest.RuntimeSecretID {
			return true
		}
	}
	return false
}

// InstallBaseTag is the local tag the install base of r is built under:
// localhost/<project>-base:<digest>, the digest covering what decides the
// image (its Dockerfile, the lists and the platform), so an unchanged base is
// the same tag and the engine's cache, and a changed one a new tag. localhost/
// names no registry a pull could reach, so a base missing from the engine
// fails rather than resolving to someone else's image.
func (r *Request) InstallBaseTag() string {
	h := sha256.New()
	fmt.Fprintf(h, "dockerfile\x00%s\x00lists\x00%s\x00platform\x00%s\x00", r.installBaseDockerfile(), r.Lists().Digest(), r.Platform)
	return fmt.Sprintf("localhost/%s-base:%s", tagName(r.Context), fmt.Sprintf("%x", h.Sum(nil))[:12])
}

// tagName is a repository name for the project at dir: its directory name,
// lowercased, with anything a repository name cannot hold replaced.
func tagName(dir string) string {
	name := strings.ToLower(filepath.Base(dir))
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			b.WriteRune(c)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return "astro"
	}
	return out
}

// buildInstallBase builds the install base of req (InstallBaseTag) in a
// directory of WorkDir, started empty, and returns its tag. It is a build of
// the CLI's own: pulled fresh like a generated build, given the netrc secret
// alone, with no labels.
func (b *Builder) buildInstallBase(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	if req.BaseImage == "" {
		return "", fmt.Errorf("the Dockerfile this project declares (%s) builds FROM %s, the install base, and no runtime was resolved to build it on", req.Dockerfile, airflowrt.BaseArgFrom)
	}
	if req.WorkDir == "" {
		return "", fmt.Errorf("the Dockerfile this project declares (%s) builds FROM %s, the install base, and the build has no directory to build it in", req.Dockerfile, airflowrt.BaseArgFrom)
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
	base.Dockerfile, base.Context, base.Labels = "", "", nil
	base.Tag = req.InstallBaseTag()
	return b.build(ctx, base, dfPath, contextDir, nil, cb)
}

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

// The layout WriteDeclaredContext writes under its directory.
const (
	// DeclaredBaseDir holds the install base's build: its Dockerfile and the
	// lists.
	DeclaredBaseDir = "base"
	// DeclaredProjectDir holds the project as the build of its own
	// Dockerfile reads it, that Dockerfile included.
	DeclaredProjectDir = "project"
)

// WriteDeclaredContext writes what the builds of the project's own Dockerfile
// read into dir, which must not exist or be empty, for two plain
// `docker build` runs with nothing from the CLI. DeclaredProjectDir is the
// project as the build's context, its ignore file already applied as docker
// applies it (declaredIgnoreFile), links kept as links, with the Dockerfile at
// its own path. When the Dockerfile builds on the install base
// (UsesInstallBase), DeclaredBaseDir is that base's build: its Dockerfile,
// mounting the netrc secret when req carries one (as Build does, so without
// one Docker's legacy builder runs it too), and the lists. It reports whether
// it wrote the base.
func WriteDeclaredContext(dir string, req Request) (base bool, err error) {
	if !req.FromDeclaredDockerfile() {
		return false, errors.New("the project declares no Dockerfile of its own")
	}
	switch entries, err := os.ReadDir(dir); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", dir, err)
	case len(entries) > 0:
		return false, fmt.Errorf("%s %w", dir, ErrContextDirNotEmpty)
	}
	data, err := os.ReadFile(req.Dockerfile)
	if err != nil {
		return false, fmt.Errorf("the Dockerfile this project declares (%s) could not be read: %w", req.Dockerfile, err)
	}
	rel, err := filepath.Rel(req.Context, req.Dockerfile)
	if err != nil {
		return false, fmt.Errorf("reading the project's files: %w", err)
	}
	projectDir := filepath.Join(dir, DeclaredProjectDir)
	if err := os.MkdirAll(projectDir, contextDirPerm); err != nil {
		return false, fmt.Errorf("creating %s: %w", projectDir, err)
	}
	if err := copyDeclaredContext(req.Context, req.Dockerfile, dir, projectDir); err != nil {
		return false, err
	}
	// docker reads the Dockerfile whatever the ignore file says, so the copy
	// carries it whatever the ignore file says too.
	target := filepath.Join(projectDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), ownerAll); err != nil {
		return false, fmt.Errorf("creating %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, data, filePermRW); err != nil { //nolint:gosec // G703: target is under the caller's new or empty --context-dir
		return false, fmt.Errorf("writing %s: %w", rel, err)
	}
	if !airflowrt.UsesInstallBase(data) {
		return false, nil
	}
	baseDir := filepath.Join(dir, DeclaredBaseDir)
	if err := writeLists(baseDir, req.Lists()); err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(baseDir, contextDockerfileName), []byte(req.installBaseDockerfile()), filePermRW); err != nil {
		return false, fmt.Errorf("writing %s: %w", contextDockerfileName, err)
	}
	return true, nil
}

// copyDeclaredContext copies the project at src into dst as a build of its
// Dockerfile at dockerfile reads it: under docker's ignore file, version
// control and links included, each link as itself. out, the directory being
// written, which holds dst, is left out when it is inside the project
// (`--context-dir ./build`), so the context never carries a copy of itself.
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
