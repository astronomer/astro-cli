package imagebuild

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// buildContextFlag is the build flag a named build context needs.
const buildContextFlag = "--build-context"

// NamedContextRequirement says what a build of a Dockerfile that reads the
// install lists from airflowrt.InstallContext needs, for the commands and
// errors that name it.
const NamedContextRequirement = "Docker with BuildKit (Docker Engine 23 or later, or the buildx plugin 0.8 or later) or Podman 4.2 or later"

// checkNamedContexts refuses a build that passes --build-context to a
// container CLI whose build does not take it: Docker's legacy builder (no
// buildx plugin, or DOCKER_BUILDKIT=0), or a Podman older than 4.2. Left to
// the build, that fails with an unknown-flag error the CLI would then blame on
// whatever else the build lacked. It asks the CLI itself, by its build help,
// rather than reading versions, because what decides it is the builder the CLI
// picks: a buildx plugin, an environment variable, a podman behind a docker
// shim. A help it cannot read, or one that is not a build's, passes: only a
// builder known not to take the flag is refused.
func (b *Builder) checkNamedContexts(ctx context.Context, req Request) error {
	var help bytes.Buffer
	if err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &help, Err: &help}, req.Bin, "build", "--help"); err != nil {
		return nil //nolint:nilerr // a help that does not run decides nothing; the build reports its own failure
	}
	text := help.String()
	if !strings.Contains(text, "Usage") || strings.Contains(text, buildContextFlag) {
		return nil
	}
	return fmt.Errorf("the project's Dockerfile reads the install lists from the %s build context, which needs %s; found %s",
		airflowrt.InstallContext, NamedContextRequirement, b.foundBuilder(ctx, req, text))
}

// foundBuilder says which builder the CLI picked and how to get one that takes
// a named context, for checkNamedContexts' error.
func (b *Builder) foundBuilder(ctx context.Context, req Request, help string) string {
	found := filepath.Base(req.Bin)
	var version bytes.Buffer
	if err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &version}, req.Bin, "--version"); err == nil {
		if v := strings.TrimSpace(version.String()); v != "" {
			found = v
		}
	}
	switch {
	case filepath.Base(req.Bin) == "podman":
		return found + ", whose build does not take " + buildContextFlag + ". Upgrade Podman to 4.2 or later"
	case buildKitOff(req.Env):
		return found + " with DOCKER_BUILDKIT=0, which selects Docker's legacy builder. Unset it to build with BuildKit"
	case strings.Contains(help, "buildx"):
		return found + " without the buildx plugin, so it builds with Docker's legacy builder. Install buildx (https://docs.docker.com/go/buildx/)"
	default:
		return found + ", whose build does not take " + buildContextFlag + ". Upgrade Docker, or install the buildx plugin"
	}
}

// buildKitOff reports whether the build's environment, the process's with env
// appended as the Commander appends it, turns BuildKit off.
func buildKitOff(env []string) bool {
	value := os.Getenv("DOCKER_BUILDKIT")
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "DOCKER_BUILDKIT="); ok {
			value = v
		}
	}
	return value == "0" || strings.EqualFold(value, "false")
}
