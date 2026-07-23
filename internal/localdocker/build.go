package localdocker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/internal/localshared"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Docker mode installs the project's dependencies into a thin layer over the
// runtime image, so a manifest's [project] dependencies (pandas, say) are
// present in the container just as they are in standalone's uv venv — the two
// modes must not diverge on what Airflow can import.
//
// The mechanism is a build layer, not a start-time `pip install` into the
// running containers: it is reproducible (baked into an image layer, so the
// same dependency set rebuilds from cache in seconds), every one of the five
// Airflow services runs the same built image, and a dependency that cannot
// install fails the build with a named error instead of leaving containers
// half-up. With no extra dependencies there is no build at all — the runtime
// image runs as-is (the fast path).
//
// It leans on the runtime image's own ONBUILD contract rather than a hand
// written `pip install`: `FROM astrocrpublic.azurecr.io/runtime` triggers
// `COPY packages.txt .` / `COPY requirements.txt .` and the image's
// install-python-dependencies (which applies the matching Airflow constraints),
// the same path `astro dev` builds through. The build context therefore holds
// exactly those two files; the code itself reaches the containers through the
// compose mounts, so nothing else needs baking in.

const (
	// builtImagePrefix namespaces the per-project image built over the
	// runtime base. The tag derives from the compose project name, so
	// clean-up can find and remove it from the record alone.
	builtImagePrefix = "astro-local/"
	dockerfileName   = "Dockerfile.astro-local"
	buildContextDir  = "deps-build"
	// requirementsName and packagesName are the fixed names the runtime
	// image's ONBUILD triggers copy from the build context.
	requirementsName = "requirements.txt"
	packagesName     = "packages.txt"
)

// airflowDist is the one distribution the runtime image forbids in
// requirements: the base image already provides Airflow (its tag is the
// Airflow version), so docker mode installs every dependency except this one.
const airflowDist = "apache-airflow"

// builtImageTag is the deterministic tag for a project's dependency layer.
func builtImageTag(projectName string) string {
	return builtImagePrefix + projectName
}

// runtimeDeps drops the apache-airflow distribution from the manifest
// dependencies. Standalone installs it into the venv, but docker mode's base
// image is Airflow already, and the runtime image's install script rejects
// apache-airflow in requirements.txt ("change the base image instead"). Every
// other dependency — providers, pandas, and the rest — installs normally.
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

// buildDepsImage builds a layer that installs deps over baseImage through the
// runtime image's ONBUILD triggers, returning the tag to run. Build output
// streams to cb.OnLine (component "build"); a failed install returns a named
// error, never a hang.
func (e *Engine) buildDepsImage(ctx context.Context, conn engineConn, stateDir, baseImage, projectName string, deps, packages []string, cb localrt.Callbacks) (string, error) {
	tag := builtImageTag(projectName)
	contextDir := filepath.Join(stateDir, buildContextDir)
	if err := os.MkdirAll(contextDir, stateDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", contextDir, err)
	}
	// requirements.txt carries the deps; packages.txt carries the manifest's
	// OS packages, one apt name per line. It must exist even when empty, or
	// the image's `ONBUILD COPY packages.txt .` fails, so an empty list still
	// writes the (empty) file.
	if err := os.WriteFile(filepath.Join(contextDir, requirementsName), []byte(strings.Join(deps, "\n")+"\n"), proxy.FilePermRW); err != nil {
		return "", fmt.Errorf("writing the generated requirements file: %w", err)
	}
	var packagesData []byte
	if len(packages) > 0 {
		packagesData = []byte(strings.Join(packages, "\n") + "\n")
	}
	if err := os.WriteFile(filepath.Join(contextDir, packagesName), packagesData, proxy.FilePermRW); err != nil {
		return "", fmt.Errorf("writing the generated packages file: %w", err)
	}
	// The Dockerfile is only `FROM <base>`; the ONBUILD triggers do the
	// install. It sits outside the context so the trailing `COPY . .` does not
	// bake it in.
	dfPath := filepath.Join(stateDir, dockerfileName)
	if err := os.WriteFile(dfPath, []byte("FROM "+baseImage+"\n"), proxy.FilePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", dfPath, err)
	}

	w := &localshared.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{Component: "build", Time: e.now(), Text: line})
		}
	}}
	// --pull keeps the base fresh for a floating tag; the daemon still caches
	// the install layer when the requirements file is unchanged.
	err := e.cmd.Run(ctx, conn.env, localrt.Stdio{Out: w, Err: w},
		conn.bin, "build", "--tag", tag, "--file", dfPath, "--pull", contextDir)
	w.Flush()
	if err != nil {
		return "", fmt.Errorf("installing the project's dependencies into the runtime image failed; see the build output above: %w", err)
	}
	return tag, nil
}

// removeBuiltImage deletes the project's dependency layer. It is best-effort:
// a project started with no extra dependencies never built one, and an image
// another running project shares must not block the clean.
func (e *Engine) removeBuiltImage(ctx context.Context, conn engineConn, projectName string) {
	//nolint:errcheck // best-effort clean-up; a missing or in-use image is fine
	_ = e.cmd.Run(ctx, conn.env, localrt.Stdio{}, conn.bin, "image", "rm", "--force", builtImageTag(projectName))
}
