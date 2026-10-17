package imagebuild

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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
	// And their throwaway credentials, so no test sweeps the real temp dir.
	authRoot := filepath.Join(dir, "tmp")
	if err := os.Mkdir(authRoot, 0o700); err != nil {
		panic(err)
	}
	tempDir = func() string { return authRoot }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// credEngine is a Commander that reads a push's credential the way the
// engine would, from the file its env and arguments point at — docker from
// DOCKER_CONFIG's config.json, podman from --authfile, else
// REGISTRY_AUTH_FILE — and records what each push found there.
type credEngine struct {
	bin string

	mu    sync.Mutex
	calls []engineCall
	// pushed is, per image, the credential the push found and the file it
	// read, with the file's whole content and the context metadata beside it
	// as the push saw them.
	pushed map[string]pushedWith
	// fail fails the calls whose first argument is the key, writing the
	// string to stderr.
	fail map[string]string
}

type pushedWith struct {
	user, secret string
	file         string
	config       map[string]any
	contextMeta  string
}

func (f *credEngine) Run(_ context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, engineCall{bin: name, args: args, env: env})
	if args[0] == "--version" {
		fmt.Fprintf(s.Out, "%s version 1\n", f.bin)
		return nil
	}
	if said, ok := f.fail[args[0]]; ok {
		fmt.Fprintln(s.Err, said)
		return errors.New("exit status 1")
	}
	if args[0] != "push" {
		return nil
	}
	image := args[len(args)-1]
	file := f.credFile(env, args)
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	host, _, _ := strings.Cut(image, "/")
	auths, _ := cfg["auths"].(map[string]any)
	entry, _ := auths[host].(map[string]any)
	raw, err := base64.StdEncoding.DecodeString(fmt.Sprint(entry["auth"]))
	if err != nil {
		return err
	}
	user, secret, _ := strings.Cut(string(raw), ":")
	meta, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "contexts", "meta", "colima", "meta.json"))
	if f.pushed == nil {
		f.pushed = map[string]pushedWith{}
	}
	f.pushed[image] = pushedWith{user: user, secret: secret, file: file, config: cfg, contextMeta: string(meta)}
	return nil
}

func (f *credEngine) credFile(env, args []string) string {
	if f.bin == "podman" {
		if file := argOf(args, "--authfile"); file != "" {
			return file
		}
		return envValue(env, "REGISTRY_AUTH_FILE")
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
// a colima context leaves it: a current context, a credential helper and
// stored logins of its own, and the context's metadata.
func userDockerDir(t *testing.T) (dir string, config []byte) {
	t.Helper()
	dir = t.TempDir()
	config = []byte(`{"auths":{"other.example.com":{"auth":"b3RoZXI6c2VjcmV0"}},"credsStore":"desktop","credHelpers":{"gcr.io":"gcloud"},"currentContext":"colima"}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), config, 0o600))
	meta := filepath.Join(dir, "contexts", "meta", "colima")
	require.NoError(t, os.MkdirAll(meta, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(meta, "meta.json"), []byte(`{"Name":"colima","Endpoints":{"docker":{"Host":"unix:///colima.sock"}}}`), 0o600))
	t.Setenv("DOCKER_CONFIG", dir)
	return dir, config
}

// withTempDir makes throwaway credentials in a directory of the test's own.
func withTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := tempDir
	tempDir = func() string { return dir }
	t.Cleanup(func() { tempDir = orig })
	return dir
}

// A docker push with a credential runs no login: it pushes with
// DOCKER_CONFIG and REGISTRY_AUTH_FILE pointing at one file made for it,
// holding that credential ("Bearer " dropped) and the user's current context
// and contexts, but none of their credential helpers or logins. Tag and untag
// stay on the user's config. Afterwards the file is gone and the user's
// config is as it was.
func TestPushWithACredentialUsesAThrowawayDockerConfig(t *testing.T) {
	userDir, userConfig := userDockerDir(t)
	tmpRoot := withTempDir(t)
	cmd := &credEngine{bin: "docker"}

	_, err := Engine{Cmd: cmd, Bin: "docker"}.Push(context.Background(), PushRequest{Source: "local:1", Image: "r.example.com/a:t", Username: "cli", Secret: "Bearer s3cret-token"})
	require.NoError(t, err)

	verbs := []string{}
	for _, c := range cmd.calls {
		verbs = append(verbs, c.args[0])
		if c.args[0] != "push" {
			assert.Equal(t, userDir, userDockerConfigDir(c.env), "%s runs on the user's config", c.String())
			continue
		}
		dir := envValue(c.env, "DOCKER_CONFIG")
		assert.Equal(t, tmpRoot, filepath.Dir(dir))
		assert.Equal(t, filepath.Join(dir, "config.json"), envValue(c.env, "REGISTRY_AUTH_FILE"), "the same file for an engine taken for podman")
		assert.Equal(t, []string{"push", "r.example.com/a:t"}, c.args)
	}
	assert.Equal(t, []string{"tag", "--version", "push", "rmi"}, verbs, "no login")

	got := cmd.pushed["r.example.com/a:t"]
	assert.Equal(t, "cli", got.user)
	assert.Equal(t, "s3cret-token", got.secret)
	assert.Equal(t, "colima", got.config["currentContext"], "the user's current context is carried")
	assert.NotContains(t, got.config, "credsStore")
	assert.NotContains(t, got.config, "credHelpers")
	assert.Len(t, got.config["auths"], 1, "the push's credential alone")
	assert.Contains(t, got.contextMeta, "unix:///colima.sock", "the context's endpoint is readable")

	assert.NoFileExists(t, got.file)
	assert.NoDirExists(t, filepath.Dir(got.file), "the throwaway directory is removed")
	data, err := os.ReadFile(filepath.Join(userDir, "config.json"))
	require.NoError(t, err)
	assert.Equal(t, string(userConfig), string(data), "the user's config is untouched")
	assertNoToken(t, userDir, "s3cret-token")
}

// assertNoToken fails if any file under dir holds token, plainly or in a
// docker auth entry.
func assertNoToken(t *testing.T, dir, token string) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString([]byte("cli:" + token))
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assert.NotContains(t, string(data), token, path)
		assert.NotContains(t, string(data), encoded, path)
		return nil
	}))
}

// A podman push with a credential points --authfile, REGISTRY_AUTH_FILE and
// DOCKER_CONFIG at the one throwaway file, with podman's connection env, and
// removes it afterwards.
func TestPushWithACredentialUsesAThrowawayPodmanAuthfile(t *testing.T) {
	withTempDir(t)
	cmd := &credEngine{bin: "podman"}
	_, err := Engine{Cmd: cmd, Bin: "podman", Env: podmanEnv}.Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "s3cret-token"})
	require.NoError(t, err)

	got := cmd.pushed["r.example.com/a:t"]
	assert.Equal(t, "s3cret-token", got.secret)
	for _, c := range cmd.calls {
		if c.args[0] != "push" {
			continue
		}
		assert.Equal(t, []string{"push", "--authfile", got.file, "r.example.com/a:t"}, c.args)
		assert.Equal(t, got.file, envValue(c.env, "REGISTRY_AUTH_FILE"))
		assert.Equal(t, filepath.Dir(got.file), envValue(c.env, "DOCKER_CONFIG"))
		assert.Equal(t, podmanEnv[0], c.env[0], "podman's connection env is kept")
	}
	assert.NoDirExists(t, filepath.Dir(got.file))
}

// A push that fails, refused or not, still removes its credential.
func TestPushRemovesTheThrowawayCredentialOnFailure(t *testing.T) {
	for _, said := range []string{"unauthorized: authentication required", "manifest blob unknown"} {
		userDir, _ := userDockerDir(t)
		tmpRoot := withTempDir(t)
		cmd := &credEngine{bin: "docker", fail: map[string]string{"push": said}}
		_, err := Engine{Cmd: cmd, Bin: "docker"}.Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "s3cret-token"})
		require.Error(t, err)
		left, globErr := filepath.Glob(filepath.Join(tmpRoot, authDirPattern))
		require.NoError(t, globErr)
		assert.Empty(t, left, said)
		assertNoToken(t, userDir, "s3cret-token")
	}
}

// The throwaway directory and file are the user's alone while they exist.
func TestThrowawayAuthIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX modes")
	}
	userDockerDir(t)
	withTempDir(t)
	a, err := newPushAuth("r.example.com", "cli", "tok", false, nil)
	require.NoError(t, err)
	defer a.remove()
	info, err := os.Stat(a.dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(a.dir, "config.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Two pushes at once with different credentials for the same registry each
// push with their own, from a file of their own.
func TestConcurrentPushesKeepTheirOwnCredentials(t *testing.T) {
	userDockerDir(t)
	withTempDir(t)
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
			files := map[string]bool{}
			for i, err := range errs {
				require.NoError(t, err)
				got := cmd.pushed[fmt.Sprintf("r.example.com/dep-%d:t", i)]
				assert.Equal(t, fmt.Sprintf("token-%d", i), got.secret)
				assert.False(t, files[got.file], "a file used by two pushes")
				files[got.file] = true
			}
		})
	}
}

// A push without a credential runs on the engine's own config, exactly as
// before: no throwaway file, no --authfile, no DOCKER_CONFIG or
// REGISTRY_AUTH_FILE of ours.
func TestPushWithoutACredentialUsesTheUsersConfig(t *testing.T) {
	tmpRoot := withTempDir(t)
	for _, bin := range []string{"docker", "podman"} {
		cmd := &engineCmd{}
		e := testEngine(cmd, bin)
		_, err := e.Push(context.Background(), PushRequest{Source: "l:1", Image: "r.example.com/a:t"})
		require.NoError(t, err)
		for _, c := range cmd.calls {
			assert.NotContains(t, c.args, "--authfile", c.String())
			assert.Equal(t, e.Env, c.env, c.String())
		}
	}
	left, err := filepath.Glob(filepath.Join(tmpRoot, authDirPattern))
	require.NoError(t, err)
	assert.Empty(t, left)
}

// Docker reads a TLS daemon's client certificates from its config directory
// unless DOCKER_CERT_PATH says otherwise, so a push on the throwaway config
// is pointed back at the user's for them, and only then.
func TestThrowawayConfigKeepsTheTLSCertificates(t *testing.T) {
	userDir, _ := userDockerDir(t)
	withTempDir(t)
	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"TLS verify, no cert path", []string{"DOCKER_TLS_VERIFY=1"}, userDir},
		{"TLS, no cert path", []string{"DOCKER_TLS=1"}, userDir},
		{"TLS with a cert path of its own", []string{"DOCKER_TLS_VERIFY=1", "DOCKER_CERT_PATH=/certs"}, "/certs"},
		{"no TLS", nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, err := newPushAuth("r.example.com", "cli", "tok", false, c.env)
			require.NoError(t, err)
			defer a.remove()
			assert.Equal(t, c.want, envValue(a.cmdEnv(c.env), "DOCKER_CERT_PATH"))
		})
	}
}

// A push removes the throwaway directories a killed push left: the user's
// own, older than an hour. A recent one may be a push still running, and
// anything else in the temp dir is not ours.
func TestPushRemovesStaleThrowawayCredentials(t *testing.T) {
	userDockerDir(t)
	root := withTempDir(t)
	old := filepath.Join(root, "astro-push-auth-old")
	recent := filepath.Join(root, "astro-push-auth-recent")
	other := filepath.Join(root, "something-else")
	for _, dir := range []string{old, recent, other} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600))
	}
	longAgo := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(old, longAgo, longAgo))
	require.NoError(t, os.Chtimes(other, longAgo, longAgo))

	cmd := &credEngine{bin: "docker"}
	_, err := Engine{Cmd: cmd, Bin: "docker"}.Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Username: "cli", Secret: "tok"})
	require.NoError(t, err)
	assert.NoDirExists(t, old)
	assert.DirExists(t, recent)
	assert.DirExists(t, other)
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
// default context and the credential alone.
func TestThrowawayConfigWithoutAUserConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeAuthConfig(dir, filepath.Join(t.TempDir(), "absent"), "r.example.com", "cli", "tok"))
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"auths":{"r.example.com":{"auth":"`+base64.StdEncoding.EncodeToString([]byte("cli:tok"))+`"}}}`, string(data))
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
