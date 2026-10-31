package pack

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// writeContext writes the build context of the generated image breq describes
// into req.ContextDir, and reports the commands that build it: the escape
// hatch for an engine, a builder or a CI the CLI does not drive. The context
// is the one the target builds, its lists already resolved, so a plain
// `docker build` of it makes the same image, with nothing from the CLI.
//
// For a project's own Dockerfile it writes the builds' inputs under the
// directory instead (imagebuild.WriteDeclaredContext): the project as the
// Dockerfile reads it in project/, and, for a Dockerfile built on the install
// base, that base's build in base/. Its commands build the base, then the
// project with the base's tag as ASTRO_BASE: two plain builds, which any
// builder runs.
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
		breq.Secrets = req.BuildSecrets
		base, err := imagebuild.WriteDeclaredContext(dir, breq)
		if err != nil {
			return Result{}, contextDirError(err)
		}
		baseTag := ""
		if base {
			baseTag = breq.InstallBaseTag()
		}
		rel, err := filepath.Rel(breq.Context, breq.Dockerfile)
		if err != nil {
			return Result{}, err
		}
		return Result{
			Target:     TargetAstro,
			Kind:       KindContext,
			ContextDir: dir,
			NextSteps: []string{
				declaredBuildCommand("docker", req.Platform, tag, baseTag, filepath.ToSlash(rel), req.BuildSecrets, dir),
				declaredBuildCommand("podman", req.Platform, tag, baseTag, filepath.ToSlash(rel), req.BuildSecrets, dir),
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

// declaredBuildCommand is the engine's commands that build what
// WriteDeclaredContext wrote at dir, as one line: the install base in base/,
// tagged baseTag, with the netrc secret when secrets carry it, when there is
// one (baseTag not ""), then the project in project/, its Dockerfile at
// dockerfile, with the base as ASTRO_BASE and every build secret, as a build
// of the project's own Dockerfile forwards them all. Each argument is quoted
// for a POSIX shell when it needs it, as in buildCommand.
func declaredBuildCommand(engine, platform, tag, baseTag, dockerfile string, secrets []string, dir string) string {
	project := []string{engine, "build"}
	if platform != "" {
		project = append(project, "--platform", platform)
	}
	project = append(project, "--tag", tag)
	if dockerfile != "Dockerfile" {
		project = append(project, "--file", filepath.Join(dir, imagebuild.DeclaredProjectDir, filepath.FromSlash(dockerfile)))
	}
	if baseTag != "" {
		project = append(project, "--build-arg", airflowrt.BaseArg+"="+baseTag)
	}
	for _, s := range secrets {
		project = append(project, "--secret", s)
	}
	project = append(project, filepath.Join(dir, imagebuild.DeclaredProjectDir))
	if baseTag == "" {
		return shellLine(project)
	}
	return buildCommand(engine, platform, baseTag, secrets, filepath.Join(dir, imagebuild.DeclaredBaseDir)) + " && " + shellLine(project)
}

// shellLine is args as one POSIX shell command line, each argument quoted when
// it needs it.
func shellLine(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = scaffold.ShellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// buildCommand is the engine's command that builds the context at dir for
// platform, tagged tag, with the netrc secret among secrets, the one the
// generated Dockerfile mounts (as Build forwards only it). Each argument is
// quoted for a POSIX shell when it needs it (a path with a space), so the
// line runs as printed.
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
	args = append(args, dir)
	return shellLine(args)
}
