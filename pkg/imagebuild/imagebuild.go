// Package imagebuild builds a deployable Airflow image from a manifest's
// fields. It assembles a small build context (requirements.txt, packages.txt)
// over Astronomer's runtime base image and runs the container build, leaning
// on the runtime image's ONBUILD triggers to install the project's
// dependencies and OS packages — the same path `astro dev` builds through.
//
// It was lifted out of internal/localdocker so the v2 deploy path can build the
// same image the local Docker engine builds (docs/v2-deploy.md, decision 7 and
// section 1). Its inputs are manifest-shaped — Python dependencies, OS packages,
// a runtime base image, a tag — so it depends on neither localdocker nor a
// deploy package; each caller resolves the manifest to a Request and hands it
// over. Build itself takes an already-resolved BaseImage; RuntimeImage maps an
// Airflow version to that base and lives here now that both the local Docker
// engine and deploy resolve it the same way.
//
// Per the layer rules (docs/v2-architecture.md) it prints nothing and never
// exits: build output flows through rt.Callbacks and a failed install
// returns a named error.
package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// RuntimeImageRepo hosts Astro Runtime 3 images (Airflow 3). The tag is the
// runtime version, e.g. "3.1-2" or a floating "3.1" the registry serves.
const RuntimeImageRepo = "astrocrpublic.azurecr.io/runtime"

// RuntimeImage maps an Airflow version to the runtime base image a build starts
// FROM. The version arrives resolved by the caller — a pinned runtime tag
// ("3.1-2") or a floating one ("3.1") — and is used as the tag directly. Only
// Airflow 3 ships as Astro Runtime 3, so anything else is refused; the built
// image's io.astronomer label gives back the exact version. Both the local
// Docker engine and the deploy path resolve the base this way.
func RuntimeImage(airflowVersion string) (string, error) {
	v := strings.TrimSpace(airflowVersion)
	if v == "" {
		return "", errors.New("no Airflow version was given; one is needed to pick a runtime image")
	}
	if major, _, _ := strings.Cut(v, "."); major != "3" {
		return "", fmt.Errorf("only Airflow 3 is supported in this release, not %q", v)
	}
	return RuntimeImageRepo + ":" + v, nil
}

const (
	// dockerfileName is the one-line `FROM <base>` Dockerfile the build runs;
	// the runtime image's ONBUILD triggers do the install.
	dockerfileName = "Dockerfile.astro-local"
	// buildContextDir is the sub-directory of a Request's WorkDir that holds
	// the build context — the two files the ONBUILD triggers copy.
	buildContextDir = "deps-build"
	// requirementsName and packagesName are the fixed names the runtime image's
	// ONBUILD triggers copy from the build context.
	requirementsName = "requirements.txt"
	packagesName     = "packages.txt"
	// contextDirPerm is owner-only, matching internal/localstate.
	contextDirPerm = 0o700
)

// airflowDist is the one distribution the runtime image forbids in
// requirements: the base image already provides Airflow (its tag is the
// Airflow version), so the build installs every dependency except this one.
const airflowDist = "apache-airflow"

// Commander runs the container build. It is the only path to a container
// runtime, so callers inject one (localdocker's engine Commander satisfies it)
// and tests substitute a fake that never touches a real daemon. env is
// appended to the process environment — the DOCKER_HOST/CONTAINER_HOST pair
// that reaches a podman machine.
type Commander interface {
	// Run runs the command wired to the given stdio. Nil readers/writers are
	// allowed and mean "none"/discard.
	Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error
}

// Request describes an image to build from a manifest's fields. It carries no
// localdocker or deploy types, so either caller can fill it.
// filePermRW is the mode for the files written into the build context:
// owner read/write. Was pkg/proxy's FilePermRW, borrowed for the constant alone —
// a file mode is not a contract worth a module dependency, and taking one on the
// proxy from an image builder was coupling with nothing behind it.
const filePermRW = 0o600

type Request struct {
	// WorkDir is the directory the build context and Dockerfile are written
	// under; the caller owns its location and lifetime.
	WorkDir string
	// BaseImage is the resolved runtime image the build starts FROM
	// (astrocrpublic.azurecr.io/runtime:<version>).
	BaseImage string
	// Tag is the image reference the build produces.
	Tag string
	// Dependencies are the manifest's Python dependencies (PEP 508);
	// apache-airflow is dropped, since the runtime base already provides it.
	Dependencies []string
	// Packages are the manifest's OS (apt) package names.
	Packages []string
	// Platform is the build platform (e.g. "linux/amd64"). Empty builds for the
	// host — what local Docker mode wants; the deploy path pins linux/amd64.
	Platform string
	// Bin is the container CLI to shell out to; Env reaches its daemon.
	Bin string
	Env []string
}

// Builder builds images. cmd and now are seams: production values come from
// New, tests replace them so no build ever touches a real daemon.
type Builder struct {
	cmd Commander
	now func() time.Time
}

// New builds a Builder over the given command runner and clock.
func New(cmd Commander, now func() time.Time) *Builder {
	return &Builder{cmd: cmd, now: now}
}

// NewExecCommander returns the production Commander, backed by os/exec. A caller
// with no daemon seam of its own (the deploy path) uses it; localdocker keeps
// its own runner because it also needs command output.
func NewExecCommander() Commander { return execCommander{} }

type execCommander struct{}

func (execCommander) Run(ctx context.Context, extraEnv []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	// A nil reader/writer means "none": os/exec routes a nil Stdout/Stderr to the
	// null device, so a silent probe needs no extra plumbing.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.In, s.Out, s.Err
	return cmd.Run()
}

// Build installs the request's dependencies and OS packages into a layer over
// its base image through the runtime image's ONBUILD triggers, and returns the
// image to run. With nothing to install it builds nothing and returns the base
// image unchanged (the fast path). Build output streams to cb.OnLine
// (component "build"); a failed install returns a named error, never a hang.

func (b *Builder) Build(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	deps := runtimeDeps(req.Dependencies)
	// The base image already provides Airflow, so a project with nothing beyond
	// Airflow and no OS packages needs no build and runs the base as-is.
	if len(deps) == 0 && len(req.Packages) == 0 {
		return req.BaseImage, nil
	}

	contextDir := filepath.Join(req.WorkDir, buildContextDir)
	if err := os.MkdirAll(contextDir, contextDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", contextDir, err)
	}
	// requirements.txt carries the deps; packages.txt carries the OS packages,
	// one apt name per line. Both must exist even when empty, or the image's
	// `ONBUILD COPY <file> .` fails, so an empty list still writes the file.
	if err := os.WriteFile(filepath.Join(contextDir, requirementsName), []byte(strings.Join(deps, "\n")+"\n"), filePermRW); err != nil {
		return "", fmt.Errorf("writing the generated requirements file: %w", err)
	}
	var packagesData []byte
	if len(req.Packages) > 0 {
		packagesData = []byte(strings.Join(req.Packages, "\n") + "\n")
	}
	if err := os.WriteFile(filepath.Join(contextDir, packagesName), packagesData, filePermRW); err != nil {
		return "", fmt.Errorf("writing the generated packages file: %w", err)
	}
	// The Dockerfile is only `FROM <base>`; the ONBUILD triggers do the
	// install. It sits outside the context so the trailing `COPY . .` does not
	// bake it in.
	dfPath := filepath.Join(req.WorkDir, dockerfileName)
	if err := os.WriteFile(dfPath, []byte("FROM "+req.BaseImage+"\n"), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", dfPath, err)
	}

	w := &rt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(rt.LogLine{Component: "build", Time: b.now(), Text: line})
		}
	}}
	// --pull keeps the base fresh for a floating tag; the daemon still caches
	// the install layer when the requirements file is unchanged. A pinned
	// platform (deploy wants linux/amd64) is added only when set, so the
	// host-platform local build keeps its exact command.
	args := []string{"build", "--tag", req.Tag, "--file", dfPath, "--pull"}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	args = append(args, contextDir)
	err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: w, Err: w}, req.Bin, args...)
	w.Flush()
	if err != nil {
		return "", fmt.Errorf("installing the project's dependencies into the runtime image failed; see the build output above: %w", err)
	}
	return req.Tag, nil
}

// runtimeDeps drops the apache-airflow distribution from the manifest
// dependencies. The runtime base image is Airflow already, and its install
// script rejects apache-airflow in requirements.txt ("change the base image
// instead"). Every other dependency — providers, pandas, and the rest —
// installs normally.
func runtimeDeps(deps []string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if distName(d) == airflowDist {
			continue
		}
		out = append(out, d)
	}
	return out
}

// distName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
func distName(req string) string {
	s := strings.TrimSpace(req)
	if i := strings.IndexAny(s, "[ \t<>=!~;@("); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}
