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
// tagged tag, for platforms (none is the host's), on the engine binary bin,
// with the build's stdout and stderr raw to out and errOut. Under podman it is
// built in docker format, as the 1.x image handler built it.
//
// The build runs in contextDir, as the 1.x handler's did after its chdir, and
// the secrets go to it as given, so docker resolves a relative secret source
// against the copy of the project as it always has, whatever form the spec
// takes.
func clientBuildRequest(contextDir, tag string, platforms, secrets []string, bin string, out, errOut io.Writer) imagebuild.Request {
	req := imagebuild.Request{
		Dockerfile: filepath.Join(contextDir, clientDockerfile),
		Context:    contextDir,
		Dir:        contextDir,
		Tag:        tag,
		Platform:   strings.Join(platforms, ","),
		Secrets:    secrets,
		Bin:        bin,
		Stdout:     out,
		Stderr:     errOut,
	}
	if container.IsPodman(bin) {
		req.Format = "docker"
	}
	return req
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
	// Absolute, so the Dockerfile, the context and the directory the build
	// runs in name the same place however the caller spelled it.
	if contextDir, err = filepath.Abs(contextDir); err != nil {
		return err
	}

	var buf bytes.Buffer
	var out, errOut io.Writer = &buf, &buf
	if debug {
		// os.Stdout as it is now: under --output json the command has
		// pointed it at stderr.
		out, errOut = os.Stdout, os.Stderr
	}
	req := clientBuildRequest(contextDir, tag, platforms, secrets, bin, out, errOut)
	_, err = imagebuild.New(newImageBuildCommander(), buildNow).Build(context.Background(), req, localrt.Callbacks{})
	switch {
	case err == nil:
		spinner.StopWithCheckmark(s, "Project image has been updated")
		return nil
	case errors.Is(err, imagebuild.ErrDockerfileBuild):
		logger.Debugf("client image build: %v", err)
		s.FinalMSG = ""
		s.Stop()
		fmt.Println(strings.TrimSpace(buf.String()) + "\n")
		return errClientBuild
	default:
		return clientDockerfileError(err, projectDir, req.Dockerfile)
	}
}

// clientDockerfileError names a Dockerfile.client the build could not use by
// its place in the project, rather than by its copy in the temporary context
// that pkg/imagebuild's error names. Any other error is returned as it is.
func clientDockerfileError(err error, projectDir, dockerfile string) error {
	const builtFrom = "which the client image is built from"
	if errors.Is(err, imagebuild.ErrDockerfileNotAFile) {
		return fmt.Errorf("the %s in the project at %s, %s, is not a file", clientDockerfile, projectDir, builtFrom)
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Path != dockerfile {
		return err
	}
	if errors.Is(pe.Err, fs.ErrNotExist) {
		return fmt.Errorf("the project at %s has no %s, %s", projectDir, clientDockerfile, builtFrom)
	}
	return fmt.Errorf("the %s in the project at %s, %s, could not be read: %w", clientDockerfile, projectDir, builtFrom, pe.Err)
}
