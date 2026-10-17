package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// withClientBuild resolves the container engine to bin, with a podman
// connection env for podman, and gives every image action a fake runner, for
// one test: the client build, the login and the push all run on it.
func withClientBuild(t *testing.T, bin string) *fakeImageCmd {
	t.Helper()
	var env []string
	if bin == "podman" {
		env = podmanEnv
	}
	return withEngine(t, bin, env, "")
}

// podmanEnv is the connection env a podman engine resolves with in tests.
var podmanEnv = []string{"CONTAINER_HOST=unix:///run/podman/podman.sock"}

// clientEngine is the engine the client deploy resolves, which withClientBuild
// has faked.
func clientEngine(t *testing.T) imagebuild.Engine {
	t.Helper()
	engine, err := clientImageEngine()
	require.NoError(t, err)
	return engine
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
// copied project, run in it, tagged as it is pushed, at the platforms asked
// for, with the build secrets and the engine binary and connection env as
// given, as the 1.x image handler built it, and the build's output raw to the
// writers given. Podman builds it in docker format, as that handler did.
func TestClientBuildRequest(t *testing.T) {
	ctx := t.TempDir()
	var out, errOut bytes.Buffer
	req := clientBuildRequest(ctx, "registry.example.com/agents/client:deploy-1", []string{"linux/amd64", "linux/arm64"}, []string{"id=aws,src=creds"}, "podman", podmanEnv, &out, &errOut)
	assert.Equal(t, imagebuild.Request{
		Dockerfile: filepath.Join(ctx, "Dockerfile.client"),
		Context:    ctx,
		Dir:        ctx,
		Tag:        "registry.example.com/agents/client:deploy-1",
		Platform:   "linux/amd64,linux/arm64",
		Secrets:    []string{"id=aws,src=creds"},
		Format:     "docker",
		Bin:        "podman",
		Env:        podmanEnv,
		Stdout:     &out,
		Stderr:     &errOut,
	}, req)
	assert.True(t, req.FromDeclaredDockerfile())

	docker := clientBuildRequest(ctx, "t", nil, nil, "docker", nil, nil, nil)
	assert.Empty(t, docker.Format, "docker build takes no --format")
	assert.Empty(t, docker.Platform, "no platforms builds for the host")
	assert.Nil(t, docker.Secrets)
}

// The command the build runs. The 1.x image handler ran, from inside the
// build context,
//
//	docker build -t <tag> -f Dockerfile.client . --platform=<platforms> --secret <spec>
//
// and this is the same build, run in the same directory: the same file,
// context, tag and platforms, and the secrets exactly as given, so docker
// reads every form of them, a relative source included, as it did.
func TestBuildClientImageCommand(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "creds")
	for _, tc := range []struct {
		name, bin, base string
		platforms       []string
		secrets         []string
		want            func(ctx string) string
	}{
		{
			name: "docker, the agent base, several platforms and every form of secret",
			bin:  "docker", base: agentBase,
			platforms: []string{"linux/amd64", "linux/arm64"},
			secrets:   []string{"id=creds.txt", "id=aws,src=creds", `"id=q,src=a,b.txt"`, "id=abs,src=" + abs, "id=PW,env=PW"},
			want: func(ctx string) string {
				return "docker build --tag reg/client:deploy-1 --file " + filepath.Join(ctx, "Dockerfile.client") +
					` --secret id=creds.txt --secret id=aws,src=creds --secret "id=q,src=a,b.txt" --secret id=abs,src=` + abs +
					" --secret id=PW,env=PW --platform linux/amd64,linux/arm64 " + ctx
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
			require.NoError(t, buildClientImage(project, ctx, "reg/client:deploy-1", tc.platforms, tc.secrets, clientEngine(t)))
			assert.Equal(t, []string{tc.want(ctx)}, cmd.calls)
			assert.Equal(t, []string{ctx}, cmd.dirs, "run in the build context")
		})
	}
}

// The base image registry login, the build and the push of a client deploy
// all run on the one engine it resolves, its connection env included, so the
// image the build makes is the one the push finds, logged in where it is
// pushed from.
func TestDeployClientImageRunsOnOneEngine(t *testing.T) {
	for _, bin := range []string{"docker", "podman"} {
		t.Run(bin, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile.client"), []byte("FROM "+agentBase+"\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements-client.txt"), nil, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "packages-client.txt"), nil, 0o600))
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			c, err := config.GetCurrentContext()
			require.NoError(t, err)
			c.Token = "ctx-token"
			require.NoError(t, c.SetContext())
			require.NoError(t, config.CFG.RemoteClientRegistry.SetHomeString("registry.example.com/agents/client"))
			cmd := withClientBuild(t, bin)

			var got ClientDeploy
			captureStdout(t, func() { got, err = DeployClientImage(InputClientDeploy{Path: dir}, nil) })
			require.NoError(t, err)

			require.Len(t, cmd.calls, 4, "%v", cmd.calls)
			assert.Equal(t, bin+" login images.astronomer.cloud -u cli --password-stdin", cmd.calls[0])
			assert.Equal(t, "ctx-token", cmd.stdins[0], "the token goes on stdin")
			assert.True(t, strings.HasPrefix(cmd.calls[1], bin+" build --tag "+got.Image+" "), cmd.calls[1])
			assert.Equal(t, bin+" push "+got.Image, cmd.calls[2])
			assert.Equal(t, bin+" rmi "+got.Image, cmd.calls[3])
			for i, env := range cmd.envs {
				if bin == "podman" {
					assert.Equal(t, podmanEnv, env, cmd.calls[i])
				} else {
					assert.Nil(t, env, cmd.calls[i])
				}
			}
			for _, call := range cmd.calls {
				assert.NotContains(t, call, "ctx-token", "the token is never an argument")
			}
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
	out := captureStdout(t, func() { err = buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil, clientEngine(t)) })
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
	out := captureStdout(t, func() { err = buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil, clientEngine(t)) })
	require.NoError(t, err)
	assert.Contains(t, out, "to stdout\n")
	require.Len(t, cmd.stdios, 1)
	assert.Equal(t, os.Stderr, cmd.stdios[0].Err, "stderr goes to stderr")
	assert.NotEqual(t, cmd.stdios[0].Out, cmd.stdios[0].Err, "the two streams stay apart")
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
			err := buildClientImage(project, ctx, "reg/client:deploy-1", nil, nil, clientEngine(t))
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

// A Dockerfile.client the build could not stat for another reason is named
// as the project's too, with the cause; any other error passes through.
func TestClientDockerfileError(t *testing.T) {
	df := filepath.Join(t.TempDir(), clientDockerfile)
	denied := fmt.Errorf("the Dockerfile this project declares (%s) could not be read: %w", df, &fs.PathError{Op: "stat", Path: df, Err: fs.ErrPermission})
	err := clientDockerfileError(denied, "/the/project", df)
	assert.Equal(t, "the Dockerfile.client in the project at /the/project, which the client image is built from, could not be read: permission denied", err.Error())
	require.ErrorIs(t, err, fs.ErrPermission)

	other := errors.New("no container engine")
	assert.Same(t, other, clientDockerfileError(other, "/the/project", df))
	elsewhere := &fs.PathError{Op: "chdir", Path: "/somewhere", Err: fs.ErrNotExist}
	assert.Same(t, error(elsewhere), clientDockerfileError(elsewhere, "/the/project", df), "a path error about another file is not Dockerfile.client's")
}

// A relative context is made absolute before the build, so the Dockerfile,
// the context and the directory the build runs in are the same place.
func TestBuildClientImageWithARelativeContext(t *testing.T) {
	project, ctx := clientProject(t, agentBase)
	t.Chdir(filepath.Dir(ctx))
	cmd := withClientBuild(t, "docker")
	require.NoError(t, buildClientImage(project, filepath.Base(ctx), "reg/client:deploy-1", nil, nil, clientEngine(t)))
	abs, err := filepath.Abs(filepath.Base(ctx))
	require.NoError(t, err)
	assert.Equal(t, []string{"docker build --tag reg/client:deploy-1 --file " + filepath.Join(abs, "Dockerfile.client") + " " + abs}, cmd.calls)
	assert.Equal(t, []string{abs}, cmd.dirs)
}
