package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

const (
	// projectDockerfileName is the Dockerfile of the step that copies the
	// project in. It sits in WorkDir, outside the project, beside its ignore
	// file.
	projectDockerfileName = "Dockerfile.astro-project"
	// ignoreFile is the project's own ignore file, at its root.
	ignoreFile = ".dockerignore"
)

// projectIgnoreRules are left out of every ProjectContext copy, after the
// project's own rules, so a "!" there cannot bring one back: the per-machine
// files Astro tools and Python write into a project, and the project's own
// requirements.txt and packages.txt, which a 1.x layout may still carry. The
// generated ones are what the image installed, and stay in AIRFLOW_HOME.
var projectIgnoreRules = []string{
	"**/.venv",
	"**/.env",
	"**/__pycache__",
	"**/*.pyc",
	".git",
	".astro",
	// The pickling fix the standalone engine drops into plugins/ for Airflow 2
	// on macOS (pkg/scaffold's pickleFixRule).
	"plugins/fix_local_executor_pickle.py",
	"/" + requirementsName,
	"/" + packagesName,
}

const projectIgnoreHeader = "# Added by the astro CLI for this build: per-machine files, and the generated dependency files.\n"

// ProjectIgnore is the ignore file a ProjectContext copy is built with: the
// project's own .dockerignore, then projectIgnoreRules, then excludes. It is
// written beside the generated Dockerfile as <Dockerfile>.dockerignore, which
// BuildKit reads instead of the context's .dockerignore, so the project's file
// is never edited. `astro package astro` reads it to address the image by
// what it copies.
func ProjectIgnore(projectDir string, excludes []string) (string, error) {
	own, err := os.ReadFile(filepath.Join(projectDir, ignoreFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading the project's %s: %w", ignoreFile, err)
	}
	var b strings.Builder
	b.Write(own)
	if len(own) > 0 && own[len(own)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(projectIgnoreHeader)
	for _, rule := range append(append([]string{}, projectIgnoreRules...), excludes...) {
		b.WriteString(rule + "\n")
	}
	return b.String(), nil
}

// buildProject is a generated build that ships req.ProjectContext.
//
// Two builds, because one cannot do it. The runtime image's ONBUILD triggers
// copy requirements.txt and packages.txt from the MAIN build context, so the
// first build is the usual one over the generated context, tagged depsTag.
// The second starts FROM it (a child's triggers have already run, so none fire
// again) and copies the project in with the project as its main context, as
// the 1.x CLI built it: Docker applies the ignore rules and its own symlink
// semantics, and nothing outside the project can be reached. The ignore file
// is <Dockerfile>.dockerignore beside the generated Dockerfile, so the
// project's .dockerignore is read, not rewritten.
//
// The intermediate tag is removed once the second build has run, whether or
// not it succeeded; its layers stay, as the final image's parent.
func (b *Builder) buildProject(ctx context.Context, req Request, dfPath, contextDir string, cb rt.Callbacks) (string, error) {
	deps := req
	deps.Tag = depsTag(req.Tag)
	if _, err := b.build(ctx, deps, dfPath, contextDir, cb); err != nil {
		return "", err
	}
	defer b.untag(ctx, req, deps.Tag)

	ignore, err := ProjectIgnore(req.ProjectContext, req.ProjectExcludes)
	if err != nil {
		return "", err
	}
	projectDF := filepath.Join(req.WorkDir, projectDockerfileName)
	if err := os.WriteFile(projectDF, []byte("FROM "+deps.Tag+"\nCOPY --chown=astro:0 . .\n"), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", projectDF, err)
	}
	ignorePath := projectDF + ignoreFile
	if err := os.WriteFile(ignorePath, []byte(ignore), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", ignorePath, err)
	}

	// No --pull: the base is the image just built, which no registry has. No
	// secrets: this step runs nothing.
	args := []string{"build", "--tag", req.Tag, "--file", projectDF}
	if isPodman(req.Bin) {
		// Podman reads the ignore file it is pointed at.
		args = append(args, "--ignorefile", ignorePath)
	}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	args = append(args, req.ProjectContext)
	if err := b.stream(ctx, req, args, cb); err != nil {
		return "", fmt.Errorf("copying the project into the image failed; see the build output above: %w", err)
	}
	return req.Tag, nil
}

// depsTag is the intermediate tag of a ProjectContext build's dependency
// step: the final tag with "-deps" on its tag part, in the same repository.
func depsTag(tag string) string {
	name, t := tag, "latest"
	if i := strings.LastIndex(tag, ":"); i > strings.LastIndex(tag, "/") {
		name, t = tag[:i], tag[i+1:]
	}
	return name + ":" + t + "-deps"
}

// untag drops ref, an intermediate tag, leaving the image it names in place
// as the parent of what was built over it. Best effort: a leftover tag
// changes nothing.
func (b *Builder) untag(ctx context.Context, req Request, ref string) {
	args := []string{"image", "rm", "--no-prune", ref}
	if isPodman(req.Bin) {
		// podman refuses to remove an image another one is built on, even by
		// name, and drops the name with untag instead.
		args = []string{"untag", ref, ref}
	}
	//nolint:errcheck // best effort; a leftover tag changes nothing
	b.cmd.Run(ctx, req.Env, rt.Stdio{}, req.Bin, args...)
}

// isPodman reports whether the container CLI is podman.
func isPodman(bin string) bool {
	return strings.HasPrefix(strings.TrimSuffix(filepath.Base(bin), ".exe"), "podman")
}
