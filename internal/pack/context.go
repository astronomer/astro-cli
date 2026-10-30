package pack

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// writeContext writes the build context of the generated image breq describes
// into req.ContextDir, and reports the commands that build it: the escape
// hatch for an engine, a builder or a CI the CLI does not drive. The context
// is the one the target builds, its lists already resolved, so a plain
// `docker build` of it makes the same image, with nothing from the CLI.
//
// For a project's own Dockerfile it writes the build's inputs under the
// directory instead (imagebuild.WriteDeclaredContext): the Dockerfile, the
// project as the build reads it in context/, and, when the Dockerfile reads
// them from the named context (one astro init moved onto a -base tag), the
// lists in astro-install/, which the commands pass with --build-context.
func writeContext(req Request, breq imagebuild.Request, name string) (Result, error) {
	dir, err := filepath.Abs(req.ContextDir)
	if err != nil {
		return Result{}, err
	}
	breq.Platform = req.Platform
	tag := req.Tag
	if tag == "" {
		tag = fmt.Sprintf("astro-package/%s:latest", name)
	}
	if breq.FromDeclaredDockerfile() {
		lists, err := imagebuild.WriteDeclaredContext(dir, breq)
		if err != nil {
			return Result{}, contextDirError(err)
		}
		return Result{
			Target:     TargetAstro,
			Kind:       KindContext,
			ContextDir: dir,
			NextSteps: []string{
				declaredBuildCommand("docker", req.Platform, tag, req.BuildSecrets, dir, lists),
				declaredBuildCommand("podman", req.Platform, tag, req.BuildSecrets, dir, lists),
			},
		}, nil
	}
	if err := imagebuild.WriteContext(dir, breq); err != nil {
		return Result{}, contextDirError(err)
	}
	return Result{
		Target:     TargetAstro,
		Kind:       KindContext,
		ContextDir: dir,
		NextSteps: []string{
			buildCommand("docker", req.Platform, tag, req.BuildSecrets, dir),
			buildCommand("podman", req.Platform, tag, req.BuildSecrets, dir),
		},
	}, nil
}

// contextDirError says how to use --context-dir when the directory is in use.
func contextDirError(err error) error {
	if errors.Is(err, imagebuild.ErrContextDirNotEmpty) {
		return fmt.Errorf("%w; --context-dir writes only into a new or empty directory", err)
	}
	return err
}

// declaredBuildCommand is the engine's command that builds what
// WriteDeclaredContext wrote at dir: its Dockerfile over its context/, with
// the lists as the named context when it wrote them (lists), and every build
// secret, as a build of the project's own Dockerfile forwards them all.
// Named contexts need BuildKit under Docker (Engine 23, or Buildx 0.8, and
// later), and Podman 4.2 (Buildah 1.26.4) or later.
func declaredBuildCommand(engine, platform, tag string, secrets []string, dir string, lists bool) string {
	args := []string{engine, "build"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, "--tag", tag, "--file", filepath.Join(dir, imagebuild.DeclaredDockerfileName))
	if lists {
		args = append(args, "--build-context", airflowrt.InstallContext+"="+filepath.Join(dir, imagebuild.DeclaredInstallDir))
	}
	for _, s := range secrets {
		args = append(args, "--secret", s)
	}
	return strings.Join(append(args, filepath.Join(dir, imagebuild.DeclaredContextDir)), " ")
}

// buildCommand is the engine's command that builds the context at dir for
// platform, tagged tag, with the netrc secret among secrets, the one the
// generated Dockerfile mounts (as Build forwards only it).
func buildCommand(engine, platform, tag string, secrets []string, dir string) string {
	args := []string{engine, "build"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, "--tag", tag)
	for _, s := range secrets {
		if parsed, err := manifest.ParseBuildSecret(s); err == nil && parsed.ID == manifest.RuntimeSecretID {
			args = append(args, "--secret", s)
		}
	}
	return strings.Join(append(args, dir), " ")
}
