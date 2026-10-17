package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/internal/imagepush"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/spinner"
)

// clientDockerfile is the remote-execution client image's Dockerfile, at the
// root of the project and so of the build context copied from it.
const clientDockerfile = "Dockerfile.client"

// errClientBuild is a client image build that ran and failed; the build's
// output is printed above it.
var errClientBuild = errors.New("an error was encountered while building the image, see the build logs for details")

// clientBuildRequest is the build of the client image: Dockerfile.client run
// against contextDir, the copy of the project prepareClientBuildContext made,
// tagged tag, for platforms (none is the host's), on the engine binary bin.
// Under podman it is built in docker format, as the 1.x image handler built it.
//
// A build secret's relative src is resolved against contextDir. The 1.x
// handler ran the build from inside the context, where docker resolved it, so
// `--build-secrets id=x,src=secret.txt` keeps naming the copy of the
// project's secret.txt.
func clientBuildRequest(contextDir, tag string, platforms, secrets []string, bin string) imagebuild.Request {
	req := imagebuild.Request{
		Dockerfile: filepath.Join(contextDir, clientDockerfile),
		Context:    contextDir,
		Tag:        tag,
		Platform:   strings.Join(platforms, ","),
		Secrets:    secretsFrom(contextDir, secrets),
		Bin:        bin,
	}
	if container.IsPodman(bin) {
		req.Format = "docker"
	}
	return req
}

// secretsFrom returns specs with each relative file source (src= or
// source=) made absolute under dir. Every other field, and a spec of
// type=env, whose src names an environment variable, is left as written.
func secretsFrom(dir string, specs []string) []string {
	if specs == nil {
		return nil
	}
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		fields := strings.Split(spec, ",")
		envType := false
		for _, f := range fields {
			k, v, _ := strings.Cut(f, "=")
			if strings.EqualFold(strings.TrimSpace(k), "type") && strings.EqualFold(strings.TrimSpace(v), "env") {
				envType = true
			}
		}
		for i, f := range fields {
			k, v, ok := strings.Cut(f, "=")
			key := strings.ToLower(strings.TrimSpace(k))
			if !ok || envType || (key != "src" && key != "source") {
				continue
			}
			if v = strings.TrimSpace(v); v != "" && !filepath.IsAbs(v) {
				fields[i] = k + "=" + filepath.Join(dir, v)
			}
		}
		out = append(out, strings.Join(fields, ","))
	}
	return out
}

// clientBuildStreams runs the build with the streams the 1.x image handler
// gave it, out and errOut, in place of pkg/imagebuild's line callbacks: raw
// output, blank lines kept, stdout and stderr apart when they are apart.
// Every other command passes through as asked.
type clientBuildStreams struct {
	imagebuild.Commander
	out, errOut io.Writer
}

func (c clientBuildStreams) Run(ctx context.Context, env []string, s localrt.Stdio, name string, args ...string) error {
	if len(args) > 0 && args[0] == "build" {
		s.Out, s.Err = c.out, c.errOut
	}
	return c.Commander.Run(ctx, env, s, name, args...)
}

// buildClientImage builds the client image of the project at projectDir from
// contextDir, its copy, with pkg/imagebuild. It shows the spinner the 1.x
// image handler showed and keeps the build's output for a failure; at debug
// level the output streams instead, stdout to stdout and stderr to stderr.
//
// The engine is imagepush's, not the containercfg.Engine resolution the
// manifest deploy builds on: the registry login before the build and the
// push after it run there, and an image built on one engine and pushed from
// another is not there to push.
// TODO: when the push moves into pkg/imagebuild (step 2 of retiring
// imagepush), move the build, the login and the push to containercfg.Engine
// together.
func buildClientImage(projectDir, contextDir, tag string, platforms, secrets []string) error {
	s := spinner.NewSpinner("Building project image…")
	debug := logger.IsLevelEnabled(logrus.DebugLevel)
	if !debug {
		s.Start()
		defer s.Stop()
	}

	bin, err := imagepush.ContainerRuntimeBinary()
	if err != nil {
		return err
	}
	if err := checkClientDockerfile(projectDir, contextDir); err != nil {
		return err
	}

	var buf bytes.Buffer
	streams := clientBuildStreams{Commander: newImageBuildCommander(), out: &buf, errOut: &buf}
	if debug {
		// os.Stdout as it is now: under --output json the command has
		// pointed it at stderr.
		streams.out, streams.errOut = os.Stdout, os.Stderr
	}
	req := clientBuildRequest(contextDir, tag, platforms, secrets, bin)
	if _, err := imagebuild.New(streams, buildNow).Build(context.Background(), req, localrt.Callbacks{}); err != nil {
		if !errors.Is(err, imagebuild.ErrDockerfileBuild) {
			return err
		}
		logger.Debugf("client image build: %v", err)
		s.FinalMSG = ""
		s.Stop()
		fmt.Println(strings.TrimSpace(buf.String()) + "\n")
		return errClientBuild
	}

	spinner.StopWithCheckmark(s, "Project image has been updated")
	return nil
}

// checkClientDockerfile refuses a build with no Dockerfile.client to run,
// naming the file in the project rather than its copy in the temporary
// context.
func checkClientDockerfile(projectDir, contextDir string) error {
	info, err := os.Stat(filepath.Join(contextDir, clientDockerfile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("the project at %s has no %s, which the client image is built from", projectDir, clientDockerfile)
	case err != nil:
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return fmt.Errorf("the %s in the project at %s, which the client image is built from, could not be read: %w", clientDockerfile, projectDir, err)
	case !info.Mode().IsRegular():
		return fmt.Errorf("the %s in the project at %s, which the client image is built from, is not a file", clientDockerfile, projectDir)
	}
	return nil
}
