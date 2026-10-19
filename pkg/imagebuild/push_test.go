package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// engineCall is one command an Engine ran.
type engineCall struct {
	bin   string
	args  []string
	env   []string
	stdin string
}

func (c engineCall) String() string { return c.bin + " " + strings.Join(c.args, " ") }

// engineCmd is a Commander that records each command, with its env and what
// it was given on stdin, and answers through reply, keyed by the command's
// first argument.
type engineCmd struct {
	calls []engineCall
	reply map[string]func(args []string, s rt.Stdio) error
}

func (f *engineCmd) Run(_ context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	c := engineCall{bin: name, args: args, env: env}
	if s.In != nil {
		in, err := io.ReadAll(s.In)
		if err != nil {
			return err
		}
		c.stdin = string(in)
	}
	f.calls = append(f.calls, c)
	if r := f.reply[args[0]]; r != nil {
		return r(args, s)
	}
	return nil
}

func (f *engineCmd) commands() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.String())
	}
	return out
}

var podmanEnv = []string{"CONTAINER_HOST=unix:///run/podman.sock"}

func testEngine(cmd *engineCmd, bin string) Engine {
	var env []string
	if bin == "podman" {
		env = podmanEnv
	}
	return Engine{Cmd: cmd, Bin: bin, Env: env}
}

// A login hands the secret to the engine on stdin, never in its arguments,
// with a "Bearer " prefix dropped.
func TestLoginPassesTheSecretOnStdin(t *testing.T) {
	for _, secret := range []string{"s3cret-token", "Bearer s3cret-token"} {
		cmd := &engineCmd{}
		require.NoError(t, testEngine(cmd, "docker").Login(context.Background(), "images.example.com", "cli", secret))
		require.Len(t, cmd.calls, 1)
		assert.Equal(t, "docker login images.example.com -u cli --password-stdin", cmd.calls[0].String())
		assert.Equal(t, "s3cret-token", cmd.calls[0].stdin)
		for _, a := range cmd.calls[0].args {
			assert.NotContains(t, a, "s3cret", "the secret is never an argument")
		}
	}
}

// With no credential the engine checks the one it holds, and is given no
// stdin to prompt on.
func TestLoginWithoutACredentialUsesTheStoredOne(t *testing.T) {
	cmd := &engineCmd{}
	require.NoError(t, testEngine(cmd, "podman").Login(context.Background(), "byo.example.com", "", ""))
	require.Len(t, cmd.calls, 1)
	assert.Equal(t, "podman login byo.example.com", cmd.calls[0].String())
	assert.Empty(t, cmd.calls[0].stdin)
	assert.Equal(t, podmanEnv, cmd.calls[0].env, "the engine's connection env")
}

func TestLoginRefusesASecretWithoutAUsername(t *testing.T) {
	cmd := &engineCmd{}
	err := testEngine(cmd, "docker").Login(context.Background(), "r.example.com", "", "s3cret-token")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
	assert.Empty(t, cmd.calls)
}

// A failed login names the command and carries the engine's stderr, with the
// secret redacted from it.
func TestLoginFailureRedactsTheSecret(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"login": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Error: bad token s3cret-token\n")
			return errors.New("exit status 1")
		},
	}}
	err := testEngine(cmd, "docker").Login(context.Background(), "r.example.com", "cli", "Bearer s3cret-token")
	require.Error(t, err)
	assert.Equal(t, "docker login r.example.com failed: exit status 1: Error: bad token [redacted]", err.Error())
}

func TestTagAndLabel(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"image": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, "3.1-2\n"); return nil },
	}}
	e := testEngine(cmd, "docker")
	require.NoError(t, e.Tag(context.Background(), "local:1", "reg.example.com/a/b:t"))
	v, err := e.Label(context.Background(), "local:1", "io.astronomer.docker.runtime.version")
	require.NoError(t, err)
	assert.Equal(t, "3.1-2", v)
	assert.Equal(t, []string{
		"docker tag local:1 reg.example.com/a/b:t",
		`docker image inspect --format {{ index .Config.Labels "io.astronomer.docker.runtime.version" }} local:1`,
	}, cmd.commands())
}

func TestLabelOfAMissingImageFails(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"image": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Error: No such image: gone:1\n")
			return errors.New("exit status 1")
		},
	}}
	_, err := testEngine(cmd, "docker").Label(context.Background(), "gone:1", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No such image: gone:1")
}

// HasEnvPrefix matches a variable's name, never its value, and an image with
// no environment at all has none to match.
func TestHasEnvPrefix(t *testing.T) {
	for name, tc := range map[string]struct {
		env  string
		want bool
	}{
		"a name with the prefix":       {`["PATH=/bin","ASTRO_AGENT_CLIENT_PROCESS_SPAWNER__PYTHON_EXECUTABLE=/usr/local/bin/python"]`, true},
		"only a value with the prefix": {`["PATH=/bin","X=ASTRO_AGENT_CLIENT_PROCESS_SPAWNER"]`, false},
		"no environment":               {"null\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
				"image": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, tc.env); return nil },
			}}
			got, err := testEngine(cmd, "docker").HasEnvPrefix(context.Background(), "local:1", "ASTRO_AGENT_CLIENT_PROCESS_SPAWNER")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, []string{"docker image inspect --format {{json .Config.Env}} local:1"}, cmd.commands())
		})
	}

	t.Run("a missing image fails", func(t *testing.T) {
		cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
			"image": func(_ []string, s rt.Stdio) error {
				fmt.Fprint(s.Err, "Error: No such image: gone:1\n")
				return errors.New("exit status 1")
			},
		}}
		_, err := testEngine(cmd, "docker").HasEnvPrefix(context.Background(), "gone:1", "X")
		require.ErrorContains(t, err, "No such image: gone:1")
	})
}

func TestTagFailureCarriesTheEngineOutput(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"tag": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Error response from daemon: No such image: local:1\n")
			return errors.New("exit status 1")
		},
	}}
	err := testEngine(cmd, "docker").Tag(context.Background(), "local:1", "r.example.com/a:t")
	require.EqualError(t, err, "docker tag local:1 r.example.com/a:t failed: exit status 1: Error response from daemon: No such image: local:1")
}

// Save writes the tarball with the engine's own CLI and streams what the
// engine says on stderr to progress as it runs.
func TestSaveStreamsItsProgress(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"save": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Err, "Saving layer 1/2\n"); return nil },
	}}
	var progress strings.Builder
	require.NoError(t, testEngine(cmd, "podman").Save(context.Background(), "local:1", "/tmp/out.tar", &progress))
	assert.Equal(t, []string{"podman save --output /tmp/out.tar local:1"}, cmd.commands())
	assert.Equal(t, "Saving layer 1/2\n", progress.String())

	// No progress writer is fine: the output is only kept for an error.
	require.NoError(t, testEngine(cmd, "docker").Save(context.Background(), "local:1", "/tmp/out.tar", nil))
}

func TestSaveFailureCarriesTheEngineOutput(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"save": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Error: no space left on device\n")
			return errors.New("exit status 1")
		},
	}}
	var progress strings.Builder
	err := testEngine(cmd, "docker").Save(context.Background(), "local:1", "/tmp/out.tar", &progress)
	require.EqualError(t, err, "docker save local:1 failed: exit status 1: Error: no space left on device")
	assert.Equal(t, "Error: no space left on device\n", progress.String(), "what the engine said still reached the progress")
}

// A push tags the source as the remote reference, pushes with the progress
// to Out, and drops the remote tag. A credential runs no login, and never
// reaches an argument.
func TestPushTagsPushesAndUntags(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, "pushed\n"); return nil },
	}}
	var out strings.Builder
	digest, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{
		Source:   "astro-deploy/proj-abc123",
		Image:    "images.example.com/org/dep:deploy-1",
		Username: "cli",
		Secret:   "Bearer s3cret-token",
		Out:      &out,
	})
	require.NoError(t, err)
	assert.Empty(t, digest, "no digest was asked for")
	assert.Equal(t, []string{
		"docker tag astro-deploy/proj-abc123 images.example.com/org/dep:deploy-1",
		"docker --version",
		"docker push images.example.com/org/dep:deploy-1",
		"docker rmi images.example.com/org/dep:deploy-1",
	}, cmd.commands())
	for _, c := range cmd.calls {
		assert.NotContains(t, c.String(), "s3cret", "the secret is never an argument")
		assert.Empty(t, c.stdin)
	}
	assert.Equal(t, "pushed\n", out.String())
}

// Without a credential, and with the image already under its remote name,
// the push is the push and the untag alone.
func TestPushWithTheStoredCredential(t *testing.T) {
	cmd := &engineCmd{}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "reg.example.com/client:deploy-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"docker push reg.example.com/client:deploy-1", "docker rmi reg.example.com/client:deploy-1"}, cmd.commands())
}

// A 403 from the registry is a *PushForbiddenError, saying whether the push
// carried its own credential, and wrapping what the engine said.
func TestPushForbidden(t *testing.T) {
	for _, withCredential := range []bool{true, false} {
		cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
			"push": func(_ []string, s rt.Stdio) error {
				fmt.Fprint(s.Err, "denied: 403 Forbidden\n")
				return errors.New("exit status 1")
			},
		}}
		req := PushRequest{Image: "r.example.com/a:t"}
		if withCredential {
			req.Username, req.Secret = "cli", "tok"
		}
		_, err := testEngine(cmd, "docker").Push(context.Background(), req)
		require.ErrorIs(t, err, ErrPushForbidden)
		var forbidden *PushForbiddenError
		require.ErrorAs(t, err, &forbidden)
		assert.Equal(t, withCredential, forbidden.WithCredential)
		assert.True(t, strings.HasPrefix(err.Error(), ErrPushForbidden.Error()+": "), err.Error())
		assert.True(t, errorChainContains(err, "denied: 403 Forbidden"), "the engine's failure is wrapped")
	}
}

func errorChainContains(err error, s string) bool {
	var found bool
	var walk func(error)
	walk = func(e error) {
		if e == nil || found {
			return
		}
		if strings.Contains(e.Error(), s) {
			found = true
			return
		}
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			for _, x := range u.Unwrap() {
				walk(x)
			}
		case interface{ Unwrap() error }:
			walk(u.Unwrap())
		}
	}
	walk(err)
	return found
}

// The forms a registry refuses an upload in, and the further forms it refuses
// a push's own credential in.
func TestForbiddenForms(t *testing.T) {
	for said, want := range map[string][2]bool{ // {stored credential, own credential}
		"unexpected status: 403 forbidden":                                                                     {true, true},
		"denied: requested access to the resource is forbidden":                                                {true, true},
		"unauthorized: authentication required":                                                                {true, true},
		"failed with status: 401 unauthorized":                                                                 {false, true},
		"unauthorized: incorrect username or password":                                                         {false, true},
		"denied: requested access to the resource is denied":                                                   {false, true},
		"permission denied while trying to connect to the docker daemon socket at unix:///var/run/docker.sock": {false, false},
		"open /tmp/astro-push-auth-1/config.json: permission denied":                                           {false, false},
		"error: access denied":                                                                                 {false, false},
		"exit status 1: manifest unknown":                                                                      {false, false},
		"layer size 14030 bytes":                                                                               {false, false},
		"exit status 1":                                                                                        {false, false},
		"dial tcp: lookup r.example.com: no such host":                                                         {false, false},
	} {
		assert.Equal(t, want[0], pushForbidden(said), "stored: %s", said)
		assert.Equal(t, want[1], credentialRefused(said), "own: %s", said)
	}
}

// A push whose own credential the registry rejects, as a 401, unauthorized
// or denied, is a *PushForbiddenError over the engine's failure; a push that
// fails for another reason is not.
func TestPushCredentialRefused(t *testing.T) {
	for _, said := range []string{
		"unexpected status from HEAD request to https://r.example.com/v2/a/blobs/sha256:1: 401 Unauthorized",
		"unauthorized: incorrect username or password",
		"denied: requested access to the resource is denied",
	} {
		cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
			"push": func(_ []string, s rt.Stdio) error { fmt.Fprintln(s.Err, said); return errors.New("exit status 1") },
		}}
		_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "tok"})
		require.ErrorIs(t, err, ErrPushForbidden, said)
		var forbidden *PushForbiddenError
		require.ErrorAs(t, err, &forbidden)
		assert.True(t, forbidden.WithCredential)
		assert.True(t, errorChainContains(err, said), "the cause is wrapped")
	}
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprintln(s.Err, "dial tcp: lookup r.example.com: no such host")
			return errors.New("exit status 1")
		},
	}}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "tok"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrPushForbidden)
}

// A request refused before anything runs leaves no tag behind: no image, or
// a secret without a username.
func TestPushRefusedRequestLeavesNoTag(t *testing.T) {
	for _, req := range []PushRequest{
		{Source: "l:1", Image: "", Username: "u", Secret: "s"},
		{Source: "l:1", Image: "r.example.com/a:t", Secret: "s"},
	} {
		cmd := &engineCmd{}
		_, err := testEngine(cmd, "docker").Push(context.Background(), req)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "\"s\"")
		assert.Empty(t, cmd.calls, "%+v", req)
	}
}

// Only what the engine said decides a refusal, never the reference it ran
// on: a tag or repository holding "403" or "forbidden" that the engine echoes
// in an unrelated failure is not a 403.
func TestPushForbiddenIgnoresTheReference(t *testing.T) {
	image := "r.example.com/forbidden-apps/a:deploy-403"
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprintf(s.Err, "The push refers to repository [r.example.com/forbidden-apps/a]\n%s: manifest blob unknown\n", image)
			return errors.New("exit status 1")
		},
	}}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: image})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrPushForbidden)
	assert.Contains(t, err.Error(), "manifest blob unknown")
}

func ranVerb(cmd *engineCmd, verb string) bool {
	for _, c := range cmd.calls {
		if len(c.args) > 0 && c.args[0] == verb {
			return true
		}
	}
	return false
}

// Any other failed push carries the engine's stderr, and the remote name it
// tagged is dropped all the same.
func TestPushFailure(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "manifest blob unknown\n")
			return errors.New("exit status 1")
		},
	}}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Source: "l:1", Image: "r.example.com/a:t"})
	require.EqualError(t, err, "docker push r.example.com/a:t failed: exit status 1: manifest blob unknown")
	assert.NotErrorIs(t, err, ErrPushForbidden)
	assert.Equal(t, []string{"docker tag l:1 r.example.com/a:t", "docker push r.example.com/a:t", "docker rmi r.example.com/a:t"}, cmd.commands())
}

// A refused push, with or without its own credential, drops the remote name
// it tagged; one that tagged nothing (Image pushed as it is) untags only on
// success, as before.
func TestRefusedPushLeavesNoTag(t *testing.T) {
	refuse := map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprintln(s.Err, "denied: 403 Forbidden")
			return errors.New("exit status 1")
		},
	}
	for _, req := range []PushRequest{
		{Source: "l:1", Image: "r.example.com/a:t"},
		{Source: "l:1", Image: "r.example.com/a:t", Username: "cli", Secret: "tok"},
	} {
		cmd := &engineCmd{reply: refuse}
		_, err := testEngine(cmd, "docker").Push(context.Background(), req)
		require.ErrorIs(t, err, ErrPushForbidden)
		assert.True(t, ranVerb(cmd, "rmi"), "the tag is dropped: %v", cmd.commands())
	}
	cmd := &engineCmd{reply: refuse}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t"})
	require.Error(t, err)
	assert.False(t, ranVerb(cmd, "rmi"), "an image pushed as it is keeps its name on a failure")
}

// Docker records the pushed digest among the image's repo digests; the one
// for the pushed repository is returned, read before the tag is dropped.
func TestPushDigestFromDockersRepoDigests(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, "Docker version 28.5.1\n"); return nil },
		"image": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Out, `["other.example.com/a@sha256:111","byo.example.com/team/repo@sha256:222","byo.example.com/team/repo-two@sha256:333"]`+"\n")
			return nil
		},
	}}
	digest, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Source: "rel/airflow:latest", Image: "byo.example.com/team/repo:deploy-1", Digest: true})
	require.NoError(t, err)
	assert.Equal(t, "sha256:222", digest)
	assert.Equal(t, []string{
		"docker tag rel/airflow:latest byo.example.com/team/repo:deploy-1",
		"docker --version",
		"docker push byo.example.com/team/repo:deploy-1",
		"docker image inspect --format {{json .RepoDigests}} byo.example.com/team/repo:deploy-1",
		"docker rmi byo.example.com/team/repo:deploy-1",
	}, cmd.commands())
}

func TestPushDigestNotRecorded(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"image": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Out, `["other.example.com/a@sha256:111"]`)
			return nil
		},
	}}
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "byo.example.com/repo:t", Digest: true})
	require.ErrorContains(t, err, "no digest was recorded for byo.example.com/repo")
}

// Podman does not record the digest it pushed, so it writes it to a
// --digestfile, on the engine's connection env like every other command.
func TestPushDigestUnderPodman(t *testing.T) {
	var file string
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, "podman version 5.6.0\n"); return nil },
		"push": func(args []string, _ rt.Stdio) error {
			require.Equal(t, "--digestfile", args[1])
			file = args[2]
			return os.WriteFile(file, []byte("sha256:444"), 0o600)
		},
	}}
	digest, err := testEngine(cmd, "podman").Push(context.Background(), PushRequest{Image: "byo.example.com/repo:t", Digest: true})
	require.NoError(t, err)
	assert.Equal(t, "sha256:444", digest)
	for _, c := range cmd.calls {
		assert.Equal(t, podmanEnv, c.env, c.String())
	}
	assert.Equal(t, []string{"podman --version", "podman push --digestfile " + file + " byo.example.com/repo:t", "podman rmi byo.example.com/repo:t"}, cmd.commands())
	assert.NoFileExists(t, file, "the digest file is removed")
}

// A reference's registry is read as docker reads it, and a Docker Hub
// credential is stored under the key each engine looks it up by.
func TestRegistryOf(t *testing.T) {
	for image, want := range map[string]string{
		"reg.example.com/a/b:t":      "reg.example.com",
		"localhost:5000/a:t":         "localhost:5000",
		"localhost/a:t":              "localhost",
		"myhost:5000/a":              "myhost:5000",
		"docker.io/library/alpine:3": "docker.io",
		"index.docker.io/org/repo:t": "index.docker.io",
		"org/repo:t":                 "docker.io",
		"alpine:3":                   "docker.io",
		"123.dkr.ecr.aws.com/repo:t": "123.dkr.ecr.aws.com",
	} {
		got, err := registryOf(image)
		require.NoError(t, err, image)
		assert.Equal(t, want, got, image)
	}
	_, err := registryOf("")
	require.Error(t, err)

	hub := []string{"https://index.docker.io/v1/", "docker.io"}
	assert.Equal(t, hub, authKeys("docker.io"))
	assert.Equal(t, hub, authKeys("index.docker.io"))
	assert.Equal(t, []string{"localhost:5000"}, authKeys("localhost:5000"))
}

func TestRepository(t *testing.T) {
	for in, want := range map[string]string{
		"reg.example.com/a/b:t":          "reg.example.com/a/b",
		"localhost:5000/a:t":             "localhost:5000/a",
		"localhost:5000/a":               "localhost:5000/a",
		"reg.example.com/a@sha256:1":     "reg.example.com/a",
		"reg.example.com/a:t@sha256:123": "reg.example.com/a",
	} {
		assert.Equal(t, want, repository(in), in)
	}
}
