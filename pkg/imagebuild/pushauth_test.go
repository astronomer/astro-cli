package imagebuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// TestMain points DOCKER_CONFIG at an empty directory for the whole package,
// so no test reads, or could write, the developer's own docker config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "imagebuild-docker-config-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("DOCKER_CONFIG", dir) //nolint:usetesting // for every test, before any runs
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// credEngine is a Commander that behaves as an engine's credential handling
// does: a login writes the token where its config says (DOCKER_CONFIG's
// config.json for docker, --authfile for podman, else the user's own config
// directory), and a push reads it from where its own config says. It records
// what each push found, so a test can tell which credential a push used.
type credEngine struct {
	bin string

	mu    sync.Mutex
	calls []engineCall
	// pushedWith is the token each pushed image found in its config.
	pushedWith map[string]string
	// seen is what a login saw in the config directory it was pointed at:
	// the config.json, and the context metadata it could read.
	seenConfig  []string
	seenContext []string
	// fail fails the calls whose first argument is the key.
	fail map[string]error
}

func (f *credEngine) Run(_ context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	c := engineCall{bin: name, args: args, env: env}
	if s.In != nil {
		in, err := io.ReadAll(s.In)
		if err != nil {
			return err
		}
		c.stdin = string(in)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	if args[0] == "--version" {
		fmt.Fprintf(s.Out, "%s version 1\n", f.bin)
		return nil
	}
	if err := f.fail[args[0]]; err != nil {
		return err
	}
	switch args[0] {
	case "login":
		path := f.credPath(env, args)
		if f.bin == "docker" {
			data, _ := os.ReadFile(path)
			f.seenConfig = append(f.seenConfig, string(data))
			meta, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "contexts", "meta", "colima", "meta.json"))
			f.seenContext = append(f.seenContext, string(meta))
		}
		return os.WriteFile(path, []byte(`{"token":"`+c.stdin+`"}`), 0o600)
	case "push":
		data, err := os.ReadFile(f.credPath(env, args))
		if err != nil {
			return err
		}
		var cred struct{ Token string }
		if err := json.Unmarshal(data, &cred); err != nil {
			return err
		}
		if f.pushedWith == nil {
			f.pushedWith = map[string]string{}
		}
		f.pushedWith[args[len(args)-1]] = cred.Token
	}
	return nil
}

// credPath is where a command's credential lives, by the engine's rules.
func (f *credEngine) credPath(env, args []string) string {
	if f.bin == "podman" {
		for i, a := range args {
			if a == "--authfile" {
				return args[i+1]
			}
		}
		return filepath.Join(os.Getenv("DOCKER_CONFIG"), "podman-auth.json")
	}
	return filepath.Join(userDockerConfigDir(env), "config.json")
}

// argOf is the value following flag in args, "" without one.
func argOf(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// userDockerDir is a user's docker config directory, as Docker Desktop with
// a colima context leaves it: a current context, a credential helper and a
// stored login of its own, and the context's metadata.
func userDockerDir(t *testing.T) (dir string, config []byte) {
	t.Helper()
	dir = t.TempDir()
	config = []byte(`{"auths":{"other.example.com":{}},"credsStore":"desktop","credHelpers":{"gcr.io":"gcloud"},"currentContext":"colima"}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), config, 0o600))
	meta := filepath.Join(dir, "contexts", "meta", "colima")
	require.NoError(t, os.MkdirAll(meta, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(meta, "meta.json"), []byte(`{"Name":"colima","Endpoints":{"docker":{"Host":"unix:///colima.sock"}}}`), 0o600))
	t.Setenv("DOCKER_CONFIG", dir)
	return dir, config
}

// A docker push with a credential logs in and pushes with DOCKER_CONFIG set
// to a directory of its own, which carries the user's current context and
// contexts but none of their credential helpers or logins, so the token lands
// in that directory's config.json rather than the keychain or the user's
// config. The tag and untag stay on the user's config. Afterwards the
// directory is gone and the user's config is as it was.
func TestPushWithACredentialUsesAThrowawayDockerConfig(t *testing.T) {
	userDir, userConfig := userDockerDir(t)
	cmd := &credEngine{bin: "docker"}
	e := Engine{Cmd: cmd, Bin: "docker"}

	_, err := e.Push(context.Background(), PushRequest{Source: "local:1", Image: "r.example.com/a:t", Username: "cli", Secret: "s3cret-token"})
	require.NoError(t, err)

	var tmp string
	for _, c := range cmd.calls {
		cfg := envDockerConfig(c.env)
		switch c.args[0] {
		case "login", "push":
			require.NotEmpty(t, cfg, c.String())
			if tmp == "" {
				tmp = cfg
			}
			assert.Equal(t, tmp, cfg, "the login and the push share one directory")
		default:
			assert.Empty(t, cfg, "%s runs on the user's config", c.String())
		}
	}
	assert.NotEqual(t, userDir, tmp)
	assert.Equal(t, "s3cret-token", cmd.pushedWith["r.example.com/a:t"], "the push used the login")

	require.Len(t, cmd.seenConfig, 1)
	var seen map[string]any
	require.NoError(t, json.Unmarshal([]byte(cmd.seenConfig[0]), &seen))
	assert.Equal(t, "colima", seen["currentContext"], "the user's current context is carried")
	assert.NotContains(t, seen, "credsStore")
	assert.NotContains(t, seen, "credHelpers")
	assert.Equal(t, map[string]any{placeholderAuth: map[string]any{}}, seen["auths"], "none of the user's logins")
	assert.Contains(t, cmd.seenContext[0], "unix:///colima.sock", "the context's endpoint is readable")

	assert.NoDirExists(t, tmp, "the throwaway config is removed")
	got, err := os.ReadFile(filepath.Join(userDir, "config.json"))
	require.NoError(t, err)
	assert.Equal(t, string(userConfig), string(got), "the user's config is untouched")
	assertNoToken(t, userDir, "s3cret-token")
}

// assertNoToken fails if any file under dir holds token.
func assertNoToken(t *testing.T, dir, token string) {
	t.Helper()
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assert.NotContains(t, string(data), token, path)
		return nil
	}))
}

// The throwaway directory goes whether the login or the push fails.
func TestPushRemovesTheThrowawayConfigOnFailure(t *testing.T) {
	for _, at := range []string{"login", "push"} {
		t.Run(at, func(t *testing.T) {
			userDir, _ := userDockerDir(t)
			cmd := &credEngine{bin: "docker", fail: map[string]error{at: errors.New("exit status 1")}}
			_, err := Engine{Cmd: cmd, Bin: "docker"}.Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "s3cret-token"})
			require.Error(t, err)
			var tmp string
			for _, c := range cmd.calls {
				if c.args[0] == "login" {
					tmp = envDockerConfig(c.env)
				}
			}
			require.NotEmpty(t, tmp)
			assert.NoDirExists(t, tmp)
			assertNoToken(t, userDir, "s3cret-token")
		})
	}
}

// A podman push with a credential logs in to and pushes with an --authfile
// of its own, in an owner-only directory, removed afterwards.
func TestPushWithACredentialUsesAThrowawayPodmanAuthfile(t *testing.T) {
	cmd := &credEngine{bin: "podman"}
	e := Engine{Cmd: cmd, Bin: "podman", Env: podmanEnv}
	var authfile string
	_, err := e.Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "s3cret-token"})
	require.NoError(t, err)

	var logins, pushes int
	for _, c := range cmd.calls {
		switch c.args[0] {
		case "login":
			logins++
			authfile = argOf(c.args, "--authfile")
			assert.Equal(t, []string{"login", "--authfile", authfile, "r.example.com", "-u", "cli", "--password-stdin"}, c.args)
		case "push":
			pushes++
			assert.Equal(t, []string{"push", "--authfile", authfile, "r.example.com/a:t"}, c.args)
		}
		assert.Equal(t, podmanEnv, c.env, "podman's connection env, and no DOCKER_CONFIG")
	}
	assert.Equal(t, 1, logins)
	assert.Equal(t, 1, pushes)
	assert.Equal(t, "s3cret-token", cmd.pushedWith["r.example.com/a:t"])
	assert.NoDirExists(t, filepath.Dir(authfile))
}

// The throwaway directory is the user's alone while it exists.
func TestThrowawayAuthIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX modes")
	}
	userDockerDir(t)
	for _, podman := range []bool{false, true} {
		a, err := newPushAuth(podman, nil)
		require.NoError(t, err)
		info, err := os.Stat(a.dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		if !podman {
			info, err := os.Stat(filepath.Join(a.dir, "config.json"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
		a.remove()
		assert.NoDirExists(t, a.dir)
	}
}

// Two pushes at once with different credentials for the same registry each
// log in to a directory of their own, and each pushes with its own token.
func TestConcurrentPushesKeepTheirOwnCredentials(t *testing.T) {
	userDockerDir(t)
	for _, bin := range []string{"docker", "podman"} {
		t.Run(bin, func(t *testing.T) {
			cmd := &credEngine{bin: bin}
			e := Engine{Cmd: cmd, Bin: bin}
			var wg sync.WaitGroup
			errs := make([]error, 8)
			for i := range errs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = e.Push(context.Background(), PushRequest{
						Image:    fmt.Sprintf("r.example.com/dep-%d:t", i),
						Username: "cli",
						Secret:   fmt.Sprintf("token-%d", i),
					})
				}()
			}
			wg.Wait()
			dirs := map[string]bool{}
			for i, err := range errs {
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("token-%d", i), cmd.pushedWith[fmt.Sprintf("r.example.com/dep-%d:t", i)])
			}
			for _, c := range cmd.calls {
				if c.args[0] == "login" {
					dir := envDockerConfig(c.env)
					if bin == "podman" {
						dir = filepath.Dir(argOf(c.args, "--authfile"))
					}
					assert.False(t, dirs[dir], "a directory used by two logins")
					dirs[dir] = true
				}
			}
			assert.Len(t, dirs, len(errs))
		})
	}
}

// A push without a credential runs on the engine's own config, exactly as
// before: no throwaway directory, no --authfile, no DOCKER_CONFIG.
func TestPushWithoutACredentialUsesTheUsersConfig(t *testing.T) {
	for _, bin := range []string{"docker", "podman"} {
		cmd := &engineCmd{}
		_, err := testEngine(cmd, bin).Push(context.Background(), PushRequest{Source: "l:1", Image: "r.example.com/a:t"})
		require.NoError(t, err)
		for _, c := range cmd.calls {
			assert.NotContains(t, c.args, "--authfile", c.String())
			assert.Empty(t, envDockerConfig(c.env), c.String())
		}
	}
}

// The user's docker config is the one their docker reads: DOCKER_CONFIG in
// the engine's env, then the process's, then ~/.docker.
func TestUserDockerConfigDir(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", "/from/process")
	assert.Equal(t, "/from/engine", userDockerConfigDir([]string{"DOCKER_CONFIG=/x", "DOCKER_CONFIG=/from/engine"}))
	assert.Equal(t, "/from/process", userDockerConfigDir(nil))
	t.Setenv("DOCKER_CONFIG", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".docker"), userDockerConfigDir(nil))
}

// A user with no docker config, or one naming no context, gets docker's
// default context and nothing else.
func TestThrowawayDockerConfigWithoutAUserConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeDockerConfig(dir, filepath.Join(t.TempDir(), "absent")))
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"auths":{"`+placeholderAuth+`":{}}}`, string(data))
	assert.NoDirExists(t, filepath.Join(dir, "contexts"))
}

// Where a symlink cannot be made, the contexts are copied, TLS material and
// all, owner-only.
func TestCopyTree(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "tls", "docker"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, "tls", "docker", "key.pem"), []byte("key"), 0o600))
	dst := filepath.Join(t.TempDir(), "contexts")
	require.NoError(t, copyTree(src, dst))
	data, err := os.ReadFile(filepath.Join(dst, "tls", "docker", "key.pem"))
	require.NoError(t, err)
	assert.Equal(t, "key", string(data))
}
