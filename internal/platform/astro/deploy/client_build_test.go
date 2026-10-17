package deploy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/imagepush"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// withClientBuild pins container.binary to bin, which is how the client
// build, the login and the push all pick their engine, and gives the build a
// fake runner, for one test. The config must already be initialized.
func withClientBuild(t *testing.T, bin string) *fakeImageCmd {
	t.Helper()
	cmd := &fakeImageCmd{}
	origCmd, origBin := newImageBuildCommander, config.CFG.DockerCommand.GetString()
	require.NoError(t, config.CFG.DockerCommand.SetHomeString(bin))
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	t.Cleanup(func() {
		newImageBuildCommander = origCmd
		assert.NoError(t, config.CFG.DockerCommand.SetHomeString(origBin))
	})
	return cmd
}

// clientProject is a project and its client build context, the context
// holding a Dockerfile.client FROM base.
func clientProject(t *testing.T, base string) (project, ctx string) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ctx = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ctx, clientDockerfile), []byte("FROM "+base+"\n"), 0o600))
	return t.TempDir(), ctx
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
// the build secrets and the engine binary as given and no connection env, as
// the 1.x image handler built it. Podman builds it in docker format, as that
// handler did.
func TestClientBuildRequest(t *testing.T) {
	ctx := t.TempDir()
	creds := filepath.Join(t.TempDir(), "creds")
	req := clientBuildRequest(ctx, "registry.example.com/agents/client:deploy-1", []string{"linux/amd64", "linux/arm64"}, []string{"id=aws,src=" + creds}, "podman")
	assert.Equal(t, imagebuild.Request{
		Dockerfile: filepath.Join(ctx, "Dockerfile.client"),
		Context:    ctx,
		Tag:        "registry.example.com/agents/client:deploy-1",
		Platform:   "linux/amd64,linux/arm64",
		Secrets:    []string{"id=aws,src=" + creds},
		Format:     "docker",
		Bin:        "podman",
	}, req)
	assert.True(t, req.FromDeclaredDockerfile())

	docker := clientBuildRequest(ctx, "t", nil, nil, "docker")
	assert.Empty(t, docker.Format, "docker build takes no --format")
	assert.Empty(t, docker.Platform, "no platforms builds for the host")
	assert.Nil(t, docker.Secrets)
}

// The 1.x handler ran the build from inside the context, so docker resolved a
// relative src there. It still is, by making it absolute; an absolute src, an
// env secret and type=env's src (a variable name) are left as written.
func TestClientBuildSecretsResolveAgainstTheContext(t *testing.T) {
	ctx := t.TempDir()
	abs := filepath.Join(ctx, "elsewhere", "creds")
	got := secretsFrom(ctx, []string{
		"id=a,src=secret.txt",
		"id=b,source=sub/secret.txt",
		"id=c,src=" + abs,
		"id=d,env=PW",
		"type=env,id=e,src=PW",
		"id=f",
	})
	assert.Equal(t, []string{
		"id=a,src=" + filepath.Join(ctx, "secret.txt"),
		"id=b,source=" + filepath.Join(ctx, "sub", "secret.txt"),
		"id=c,src=" + abs,
		"id=d,env=PW",
		"type=env,id=e,src=PW",
		"id=f",
	}, got)
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
					" --secret id=aws,src=" + filepath.Join(ctx, "creds") + " --secret id=PW,env=PW --platform linux/amd64,linux/arm64 " + ctx
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
			project, ctx := clientProject(t, tc.base)
			cmd := withClientBuild(t, tc.bin)
			require.NoError(t, buildClientImage(project, ctx, "reg/client:deploy-1", tc.platforms, tc.secrets))
			assert.Equal(t, []string{tc.want(ctx)}, cmd.calls)
		})
	}
}

// The build runs on the engine binary imagepush logs in and pushes with, so
// all three reach the same engine whatever container.binary says.
func TestBuildClientImageRunsOnThePushEngine(t *testing.T) {
	for _, bin := range []string{"docker", "podman"} {
		t.Run(bin, func(t *testing.T) {
			project, ctx := clientProject(t, agentBase)
			cmd := withClientBuild(t, bin)
			pushBin, err := imagepush.ContainerRuntimeBinary()
			require.NoError(t, err)
			require.Equal(t, bin, pushBin)

			require.NoError(t, buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil))
			require.Len(t, cmd.calls, 1)
			assert.True(t, strings.HasPrefix(cmd.calls[0], pushBin+" build "), "built on %q, pushes on %q", cmd.calls[0], pushBin)
		})
	}
}

// A failed build prints the build's output as it came, blank lines and all,
// then the error the 1.x image handler returned.
func TestBuildClientImageFailurePrintsTheBuildOutput(t *testing.T) {
	project, ctx := clientProject(t, agentBase)
	cmd := withClientBuild(t, "docker")
	cmd.buildOut = "#5 RUN pip install\n\n#5 ERROR: pip install failed\n"
	cmd.buildErr = errors.New("exit status 1")

	var err error
	out := captureStdout(t, func() { err = buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil) })
	require.ErrorIs(t, err, errClientBuild)
	assert.Equal(t, "an error was encountered while building the image, see the build logs for details", err.Error())
	assert.Contains(t, out, "#5 RUN pip install\n\n#5 ERROR: pip install failed\n")
}

// At debug level the build streams, stdout to stdout and stderr to stderr,
// as the 1.x image handler streamed it.
func TestBuildClientImageStreamsAtDebug(t *testing.T) {
	project, ctx := clientProject(t, agentBase)
	cmd := withClientBuild(t, "docker")
	cmd.buildOut = "to stdout\n"
	prev := logger.GetLevel()
	logger.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() { logger.SetLevel(prev) })

	var err error
	out := captureStdout(t, func() { err = buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil) })
	require.NoError(t, err)
	assert.Contains(t, out, "to stdout\n")
	require.Len(t, cmd.stdios, 1)
	assert.Equal(t, os.Stderr, cmd.stdios[0].Err, "stderr goes to stderr")
}

// A Dockerfile.client that is missing, or is not a file, is named as the
// project's, not by its path in the temporary build context, and nothing is
// built.
func TestBuildClientImageNeedsDockerfileClient(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, ctx string)
		want  string
	}{
		{"missing", func(*testing.T, string) {}, "has no Dockerfile.client, which the client image is built from"},
		{"a directory", func(t *testing.T, ctx string) {
			require.NoError(t, os.Mkdir(filepath.Join(ctx, clientDockerfile), 0o700))
		}, "the Dockerfile.client in the project at %s, which the client image is built from, is not a file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			project, ctx := t.TempDir(), t.TempDir()
			tc.setup(t, ctx)
			cmd := withClientBuild(t, "docker")
			err := buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil)
			require.Error(t, err)
			want := tc.want
			if strings.Contains(want, "%s") {
				want = strings.ReplaceAll(want, "%s", project)
			}
			assert.Contains(t, err.Error(), want)
			assert.Contains(t, err.Error(), project)
			assert.NotContains(t, err.Error(), ctx, "the temporary context is not the user's to look for")
			assert.Empty(t, cmd.calls)
		})
	}
}
