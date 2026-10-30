package imagebuild

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The help each builder prints for build, trimmed to what the check reads.
const (
	ncBuildxHelp    = "Usage:  docker buildx build [OPTIONS] PATH | URL | -\n\nStart a build\n\n      --build-context stringArray   Additional build contexts (e.g., name=path)\n"
	ncPodmanHelp    = "Build an image using instructions from Containerfiles\n\nUsage:\n  podman build [options] [CONTEXT]\n\n      --build-context stringArray   additional build context (e.g. name=path)\n"
	ncLegacyHelp    = "DEPRECATED: The legacy builder is deprecated and will be removed in a future release.\n            Install the buildx component to build images with BuildKit:\n            https://docs.docker.com/go/buildx/\n\nUsage:  docker build [OPTIONS] PATH | URL | -\n\nBuild an image from a Dockerfile\n"
	ncLegacyOffHelp = "DEPRECATED: The legacy builder is deprecated and will be removed in a future release.\n            BuildKit is currently disabled; enable it by removing the DOCKER_BUILDKIT=0\n            environment-variable.\n\nUsage:  docker build [OPTIONS] PATH | URL | -\n"
	ncOldPodmanHelp = "Build an image using instructions from Containerfiles\n\nUsage:\n  podman build [options] [CONTEXT]\n\n      --build-arg argument=value   argument=value to supply to the builder\n"
	ncDockerVersion = "Docker version 29.8.2, build 1a2b3c4\n"
	ncPodmanVersion = "podman version 4.1.1\n"
)

// namedContextBuild runs a build of a Dockerfile that reads the named context
// with bin, whose build --help prints help and whose --version prints
// version, and returns the calls made and the error.
func namedContextBuild(t *testing.T, bin, help, version string, env ...string) ([]string, error) {
	t.Helper()
	project := rewrittenProject(t)
	cmd := &fakeCmd{run: func(call string, s rt.Stdio) error {
		switch {
		case strings.HasSuffix(call, " build --help"):
			_, err := io.WriteString(s.Out, help)
			return err
		case strings.HasSuffix(call, " --version"):
			if version == "" {
				return errors.New("exit status 1")
			}
			_, err := io.WriteString(s.Out, version)
			return err
		}
		return nil
	}}
	req := Request{
		WorkDir: t.TempDir(), Dockerfile: filepath.Join(project, "Dockerfile"), Context: project,
		Tag: "img:1", Bin: bin, Env: env, Install: &InstallLists{Requirements: []string{"pandas==2.2.3"}},
	}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	return cmd.calls, err
}

func built(calls []string) bool {
	for _, c := range calls {
		if strings.Contains(c, " --build-context ") {
			return true
		}
	}
	return false
}

// BuildKit (buildx) and Podman 4.2 or later list --build-context in their
// build's help, and the build goes ahead.
func TestABuilderThatTakesNamedContextsBuilds(t *testing.T) {
	for _, tc := range []struct{ bin, help string }{{"docker", ncBuildxHelp}, {"podman", ncPodmanHelp}} {
		calls, err := namedContextBuild(t, tc.bin, tc.help, "")
		require.NoError(t, err, tc.bin)
		assert.True(t, built(calls), tc.bin)
	}
}

// Docker's legacy builder and Podman before 4.2 do not, and the build is
// refused before it starts, naming what is needed, what was found, and how to
// get there.
func TestABuilderWithoutNamedContextsIsRefusedBeforeTheBuild(t *testing.T) {
	for _, tc := range []struct {
		name, bin, help, version string
		env                      []string
		want                     string
	}{
		{
			"no buildx", "docker", ncLegacyHelp, ncDockerVersion, nil,
			"found Docker version 29.8.2, build 1a2b3c4 without the buildx plugin, so it builds with Docker's legacy builder. Install buildx (https://docs.docker.com/go/buildx/)",
		},
		{
			"BuildKit off", "docker", ncLegacyOffHelp, ncDockerVersion,
			[]string{"DOCKER_BUILDKIT=0"},
			"found Docker version 29.8.2, build 1a2b3c4 with DOCKER_BUILDKIT=0, which selects Docker's legacy builder. Unset it to build with BuildKit",
		},
		{
			"old podman", "podman", ncOldPodmanHelp, ncPodmanVersion, nil,
			"found podman version 4.1.1, whose build does not take --build-context. Upgrade Podman to 4.2 or later",
		},
		{
			"no version", "/usr/local/bin/podman", ncOldPodmanHelp, "", nil,
			"found podman, whose build does not take --build-context. Upgrade Podman to 4.2 or later",
		},
	} {
		calls, err := namedContextBuild(t, tc.bin, tc.help, tc.version, tc.env...)
		require.Error(t, err, tc.name)
		assert.Contains(t, err.Error(), "the project's Dockerfile reads the install lists from the astro-install build context, which needs "+NamedContextRequirement+"; ", tc.name)
		assert.Contains(t, err.Error(), strings.TrimPrefix(tc.want, "found "), tc.name)
		assert.False(t, built(calls), "%s: refused before the build", tc.name)
	}
}

// A help that does not run, or that is not a build's, decides nothing: the
// build goes ahead and reports its own failure, if any.
func TestAnUnreadableBuildHelpLetsTheBuildRun(t *testing.T) {
	for _, help := range []string{"", "something else entirely"} {
		calls, err := namedContextBuild(t, "docker", help, ncDockerVersion)
		require.NoError(t, err)
		assert.True(t, built(calls))
	}
	project := rewrittenProject(t)
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.HasSuffix(call, " build --help") {
			return errors.New("exit status 125")
		}
		return nil
	}}
	req := Request{WorkDir: t.TempDir(), Dockerfile: filepath.Join(project, "Dockerfile"), Context: project, Tag: "img:1", Bin: "docker"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, built(cmd.calls))
}

// A Dockerfile that reads no named context never asks.
func TestADockerfileWithoutTheNamedContextSkipsTheCheck(t *testing.T) {
	dir := declaredProject(t)
	cmd := &fakeCmd{}
	req := Request{WorkDir: t.TempDir(), Dockerfile: filepath.Join(dir, "docker", "Dockerfile"), Context: dir, Tag: "img:1", Bin: "docker"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Len(t, cmd.calls, 1)
	assert.NotContains(t, cmd.calls[0], "--help")
}

func TestBuildKitOff(t *testing.T) {
	t.Setenv("DOCKER_BUILDKIT", "")
	assert.False(t, buildKitOff(nil))
	assert.True(t, buildKitOff([]string{"DOCKER_BUILDKIT=0"}))
	assert.True(t, buildKitOff([]string{"DOCKER_BUILDKIT=false"}))
	assert.False(t, buildKitOff([]string{"DOCKER_BUILDKIT=1"}))
	t.Setenv("DOCKER_BUILDKIT", "0")
	assert.True(t, buildKitOff(nil))
	assert.False(t, buildKitOff([]string{"DOCKER_BUILDKIT=1"}), "the build's own env wins, as it is appended")
}
