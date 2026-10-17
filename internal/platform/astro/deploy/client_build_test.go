package deploy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// withClientBuild points the client image build at a fake engine named bin
// for one test, and restores the seams after.
func withClientBuild(t *testing.T, bin string) *fakeImageCmd {
	t.Helper()
	cmd := &fakeImageCmd{}
	origResolve, origCmd := resolveContainerEngine, newImageBuildCommander
	resolveContainerEngine = func() (string, []string, error) { return bin, nil, nil }
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	t.Cleanup(func() { resolveContainerEngine, newImageBuildCommander = origResolve, origCmd })
	return cmd
}

// clientContext is a client build context holding a Dockerfile.client FROM
// base.
func clientContext(t *testing.T, base string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, clientDockerfile), []byte("FROM "+base+"\n"), 0o600))
	return dir
}

// captureStdout runs f with os.Stdout sent to a pipe and returns what it wrote.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	prev := os.Stdout
	os.Stdout = w
	f()
	os.Stdout = prev
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

const agentBase = "images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.1-1-python-3.12-astro-agent-1.1.0"

// The request is a declared-Dockerfile build of Dockerfile.client over the
// copied project, tagged as it is pushed, at the platforms asked for, with
// the build secrets and the engine as given. Podman builds it in docker
// format, as the 1.x image handler did.
func TestClientBuildRequest(t *testing.T) {
	env := []string{"CONTAINER_HOST=unix:///podman.sock"}
	ctx := t.TempDir()
	req := clientBuildRequest(ctx, "registry.example.com/agents/client:deploy-1", []string{"linux/amd64", "linux/arm64"}, []string{"id=aws,src=creds"}, "podman", env)
	assert.Equal(t, imagebuild.Request{
		Dockerfile: filepath.Join(ctx, "Dockerfile.client"),
		Context:    ctx,
		Tag:        "registry.example.com/agents/client:deploy-1",
		Platform:   "linux/amd64,linux/arm64",
		Secrets:    []string{"id=aws,src=creds"},
		Format:     "docker",
		Bin:        "podman",
		Env:        env,
	}, req)
	assert.True(t, req.FromDeclaredDockerfile())

	docker := clientBuildRequest(ctx, "t", nil, nil, "docker", nil)
	assert.Empty(t, docker.Format, "docker build takes no --format")
	assert.Empty(t, docker.Platform, "no platforms builds for the host")
}

// The command the build runs. The 1.x image handler ran, from inside the
// build context,
//
//	docker build -t <tag> -f Dockerfile.client . --platform=<platforms> --secret <spec>
//
// and this is the same build: the same file, context, tag, platforms and
// secrets, spelled with absolute paths instead of a chdir.
func TestBuildClientImageCommand(t *testing.T) {
	for _, tc := range []struct {
		name, bin, base string
		platforms       []string
		secrets         []string
		want            func(ctx string) string
	}{
		{
			name: "docker, the agent base, several platforms and secrets",
			bin:  "docker", base: agentBase,
			platforms: []string{"linux/amd64", "linux/arm64"},
			secrets:   []string{"id=aws,src=creds", "id=PW,env=PW"},
			want: func(ctx string) string {
				return "docker build --tag reg/client:deploy-1 --file " + filepath.Join(ctx, "Dockerfile.client") +
					" --secret id=aws,src=creds --secret id=PW,env=PW --platform linux/amd64,linux/arm64 " + ctx
			},
		},
		{
			name: "podman builds docker format",
			bin:  "podman", base: agentBase,
			want: func(ctx string) string {
				return "podman build --tag reg/client:deploy-1 --file " + filepath.Join(ctx, "Dockerfile.client") + " --format docker " + ctx
			},
		},
		{
			name: "an Astro Runtime base is pulled",
			bin:  "docker", base: "quay.io/astronomer/astro-runtime:12.0.0",
			want: func(ctx string) string {
				return "docker build --tag reg/client:deploy-1 --file " + filepath.Join(ctx, "Dockerfile.client") + " --pull " + ctx
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := withClientBuild(t, tc.bin)
			ctx := clientContext(t, tc.base)
			require.NoError(t, buildClientImage(ctx, "reg/client:deploy-1", tc.platforms, tc.secrets))
			assert.Equal(t, []string{tc.want(ctx)}, cmd.calls)
		})
	}
}

// A failed build prints the build's output, then the error the 1.x image
// handler returned.
func TestBuildClientImageFailurePrintsTheBuildOutput(t *testing.T) {
	cmd := withClientBuild(t, "docker")
	cmd.buildOut = "#5 ERROR: pip install failed\n"
	cmd.buildErr = errors.New("exit status 1")
	ctx := clientContext(t, agentBase)

	var err error
	out := captureStdout(t, func() { err = buildClientImage(ctx, "reg/client:deploy-1", nil, nil) })
	require.ErrorIs(t, err, errClientBuild)
	assert.Equal(t, "an error was encountered while building the image, see the build logs for details", err.Error())
	assert.Contains(t, out, "#5 ERROR: pip install failed\n")
}

// A project without Dockerfile.client hears its name, not a path in the
// temporary build context, and nothing is built.
func TestBuildClientImageNeedsDockerfileClient(t *testing.T) {
	cmd := withClientBuild(t, "docker")
	err := buildClientImage(t.TempDir(), "reg/client:deploy-1", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading dockerfile: opening file: open Dockerfile.client:")
	assert.Empty(t, cmd.calls)
}

// No container engine is the engine resolution's error, before any build.
func TestBuildClientImageWithoutAnEngine(t *testing.T) {
	cmd := withClientBuild(t, "docker")
	resolveContainerEngine = func() (string, []string, error) { return "", nil, errors.New("no container engine found") }
	err := buildClientImage(clientContext(t, agentBase), "reg/client:deploy-1", nil, nil)
	require.EqualError(t, err, "no container engine found")
	assert.Empty(t, cmd.calls)
}
