package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

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
// tagged tag, for platforms (none is the host's). Under podman it is built in
// docker format, as the 1.x image handler built it.
func clientBuildRequest(contextDir, tag string, platforms, secrets []string, bin string, env []string) imagebuild.Request {
	req := imagebuild.Request{
		Dockerfile: filepath.Join(contextDir, clientDockerfile),
		Context:    contextDir,
		Tag:        tag,
		Platform:   strings.Join(platforms, ","),
		Secrets:    secrets,
		Bin:        bin,
		Env:        env,
	}
	if container.IsPodman(bin) {
		req.Format = "docker"
	}
	return req
}

// buildClientImage builds the client image from contextDir with pkg/imagebuild,
// on the engine an image deploy resolves. It shows the spinner the 1.x image
// handler showed, and keeps the build's output for a failure; at debug level
// the output streams instead.
func buildClientImage(contextDir, tag string, platforms, secrets []string) error {
	s := spinner.NewSpinner("Building project image…")
	debug := logger.IsLevelEnabled(logrus.DebugLevel)
	if !debug {
		s.Start()
		defer s.Stop()
	}

	bin, env, err := ResolveContainerEngine()
	if err != nil {
		return err
	}
	// Checked here so a project without the file hears about Dockerfile.client
	// by name, rather than about a path in the temporary build context.
	f, err := os.DirFS(contextDir).Open(clientDockerfile)
	if err != nil {
		return fmt.Errorf("reading dockerfile: opening file: %w", err)
	}
	f.Close() //nolint:errcheck // opened only to check it is there

	var out strings.Builder
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) {
		if debug {
			fmt.Fprintln(os.Stderr, l.Text)
			return
		}
		out.WriteString(l.Text + "\n")
	}}
	req := clientBuildRequest(contextDir, tag, platforms, secrets, bin, env)
	if _, err := imagebuild.New(newImageBuildCommander(), buildNow).Build(context.Background(), req, cb); err != nil {
		if !errors.Is(err, imagebuild.ErrDockerfileBuild) {
			return err
		}
		logger.Debugf("client image build: %v", err)
		s.FinalMSG = ""
		s.Stop()
		fmt.Println(strings.TrimSpace(out.String()) + "\n")
		return errClientBuild
	}

	spinner.StopWithCheckmark(s, "Project image has been updated")
	return nil
}
