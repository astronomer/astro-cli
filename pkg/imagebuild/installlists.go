package imagebuild

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// What a generated build installs is decided before the build, by the caller,
// and handed to it ready-made as InstallLists: the CLI resolves the project
// with the host's uv and applies the image's policy to the lock
// (internal/imageinstall). The build itself resolves nothing. Its Dockerfile is
// the runtime's ONBUILD steps written out, reading the three lists from the
// context, so the context is self-contained: `docker build` runs it as written,
// on any machine, with nothing from the CLI (WriteContext).

// InstallLists are the three files a generated build's install steps read, one
// line per entry, as the runtime's install scripts read them from the image's
// working directory.
type InstallLists struct {
	// Requirements is requirements.txt: what the image installs over its
	// base, without Airflow. Constraints, when there are any, are named in it
	// with -c (RequirementsFile).
	Requirements []string
	// Constraints is uv-lock-constraints.txt: the versions the lock resolved
	// for packages the image installs only as a dependency of a requirement.
	// It installs nothing by itself.
	Constraints []string
	// Packages is packages.txt: the OS (apt) packages, installed first.
	Packages []string
}

// Empty reports lists that install nothing over the base: no requirement and
// no OS package. Constraints alone install nothing.
func (l InstallLists) Empty() bool {
	return len(l.Requirements) == 0 && len(l.Packages) == 0
}

// Files are the lists as the build context holds them, by file name. All three
// are always there, empty or not, so the Dockerfile's COPY lines never miss
// one.
func (l InstallLists) Files() map[string][]byte {
	reqs := l.Requirements
	if len(l.Constraints) > 0 {
		reqs = append([]string{"-c " + lockConstraintsName}, reqs...)
	}
	return map[string][]byte{
		requirementsName:    lines(reqs),
		lockConstraintsName: lines(l.Constraints),
		packagesName:        lines(l.Packages),
	}
}

// Digest is a digest of Files, for a caller that addresses the image by what
// builds it (`astro package astro`'s content tag).
func (l InstallLists) Digest() string {
	files := l.Files()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%s\x00%x\x00", name, sha256.Sum256(files[name]))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// lines is one entry per line, each ending in \n, or nothing for none.
func lines(entries []string) []byte {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// UnpinnedLists are the lists of a project with no lock to pin from: the
// dependencies as written, without Airflow (runtimeDeps), after a
// --find-links line for each page in findLinks, and the OS packages, plus git
// when a dependency is a git direct reference (WithGit). It is what
// a request that carries no Install installs, and what the CLI falls back to
// when it cannot lock the project.
func UnpinnedLists(deps, findLinks, packages []string) InstallLists {
	deps = runtimeDeps(deps)
	reqs := make([]string, 0, len(findLinks)+len(deps))
	for _, l := range findLinks {
		reqs = append(reqs, "--find-links "+l)
	}
	reqs = append(reqs, deps...)
	if slices.ContainsFunc(deps, isGitReference) {
		packages = WithGit(packages)
	}
	return InstallLists{Requirements: reqs, Packages: packages}
}

// gitPackage is the apt package uv needs to install a git direct reference.
const gitPackage = "git"

// WithGit is packages with git added, for lists that install a git direct
// reference (`name @ git+https://…`): the runtime image ships no git, so uv
// fails such an install with "Git executable not found", and the generated
// Dockerfile installs packages.txt before requirements.txt. packages already
// naming git is returned as it is, and is never modified.
//
// Only https sources are served by this: an ssh one also needs an ssh client
// and a key, which the build has neither of.
func WithGit(packages []string) []string {
	if slices.ContainsFunc(packages, isGitPackage) {
		return packages
	}
	return append(slices.Clone(packages), gitPackage)
}

// isGitPackage reports an apt entry naming git, pinned (git=1:2.39…) or with
// an architecture (git:amd64) or not.
func isGitPackage(pkg string) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(pkg), "=")
	name, _, _ = strings.Cut(name, ":")
	return name == gitPackage
}

// isGitReference reports a PEP 508 direct reference fetched with git. The
// scheme is matched as uv matches it, without regard to case.
func isGitReference(dep string) bool {
	_, ref, ok := strings.Cut(dep, "@")
	return ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "git+")
}

// Lists is what a generated build of r installs: Install when the caller
// resolved it, else UnpinnedLists of the request's own fields.
func (r *Request) Lists() InstallLists {
	if r.Install != nil {
		return *r.Install
	}
	return UnpinnedLists(r.Dependencies, r.FindLinks, r.Packages)
}

// installFindLinksName is the find-links file an older build left in a
// project's .astro directory, which a generated build leaves out of its
// context. Nothing reads it.
const installFindLinksName = "install-find-links.txt"

// generatedDockerfile is the Dockerfile a generated build runs over from, the
// -base image: the runtime's ONBUILD steps written out
// (airflowrt.InstallDockerfileWith), reading the lists from the context
// (InstallLists.Files). The netrc mount is written only when mountNetrc is
// set, which Build sets when the request carries that secret, so a build with
// no secret to mount keeps to a plain RUN, which the legacy builder and an
// older Podman also run.
func generatedDockerfile(from string, mountNetrc bool) string {
	return airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{MountNetrc: mountNetrc})
}

// GeneratedDockerfile is the Dockerfile a generated build over the runtime
// base baseImage runs (Request.BaseImage, before baseOf), with the netrc
// secret mounted into the Python install when mountNetrc is set, as Build
// sets it when the request carries that secret. It is for a caller that
// addresses the image by what builds it (`astro package astro`'s content tag
// hashes it, so a change to the recipe is a change to the tag).
func GeneratedDockerfile(baseImage string, mountNetrc bool) string {
	return generatedDockerfile(baseOf(baseImage), mountNetrc)
}

// writeContext fills contextDir, which exists and is empty, with what a
// generated build of req reads there: the project's files when the image
// ships them (ProjectFiles), then the lists, which replace any copy the
// project carries.
func writeContext(contextDir string, req Request) error {
	if req.ProjectFiles != "" {
		if err := copyProjectFiles(req.ProjectFiles, contextDir); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(contextDir, ".astro", installFindLinksName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing the project's %s from the build context: %w", installFindLinksName, err)
		}
	}
	for name, data := range req.Lists().Files() {
		if err := os.WriteFile(filepath.Join(contextDir, name), data, filePermRW); err != nil {
			return fmt.Errorf("writing %s into the build context: %w", name, err)
		}
	}
	return nil
}

// Names WriteContext gives the Dockerfile and the ignore file that keeps it
// out of the image.
const (
	contextDockerfileName = "Dockerfile"
	contextIgnore         = contextDockerfileName + "\n" + dockerIgnoreName + "\n"
)

// ErrContextDirNotEmpty reports a WriteContext into a directory that holds
// something: it writes only into a new or empty one, so it never mixes its
// files with another build's, or deletes anything.
var ErrContextDirNotEmpty = errors.New("is not empty")

// WriteContext writes the build context of the generated build req describes
// into dir, which must not exist or be empty, for `docker build dir` to run
// with nothing from the CLI: a Dockerfile (generatedDockerfile, mounting the
// netrc secret, required=false, so a build with or without one runs it), the
// lists, the project's files when req carries ProjectFiles, and a
// .dockerignore keeping the Dockerfile out of the image. The image is the one
// Build makes of req. A request for a declared Dockerfile is refused: its
// project already is its context.
func WriteContext(dir string, req Request) error {
	if req.FromDeclaredDockerfile() {
		return errors.New("the project declares its own Dockerfile, so the project is its build context")
	}
	switch entries, err := os.ReadDir(dir); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("reading %s: %w", dir, err)
	case len(entries) > 0:
		return fmt.Errorf("%s %w", dir, ErrContextDirNotEmpty)
	}
	if err := os.MkdirAll(dir, contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := writeContext(dir, req); err != nil {
		return err
	}
	for name, data := range map[string]string{
		contextDockerfileName: generatedDockerfile(baseOf(req.BaseImage), true),
		dockerIgnoreName:      contextIgnore,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), filePermRW); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}
