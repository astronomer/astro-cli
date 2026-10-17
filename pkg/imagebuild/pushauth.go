package imagebuild

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pushAuth is where a push that carries its own credential keeps it: one
// file, written for that push into a directory of its own and deleted with it,
// never the user's own engine config. No login is run: the credential is
// written into the file directly, in the format docker's config.json and
// containers' auth.json share ({"auths": {"<registry>": {"auth": …}}}), and
// the push is pointed at it every way an engine looks for one — DOCKER_CONFIG
// for docker, REGISTRY_AUTH_FILE and --authfile for podman — so that an
// engine taken for the other still finds it, and finds nothing of the user's.
//
// So a deploy's token is never left in the user's credential store or
// keychain, and two pushes running at once with different credentials for
// the same registry cannot overwrite each other's: each has its own file.
type pushAuth struct {
	dir string
	// args go right after the subcommand (podman's --authfile); env is
	// appended to the engine's env.
	args []string
	env  []string
}

// authDirPerm and authFilePerm keep the throwaway credential the user's alone.
const (
	authDirPerm  = 0o700
	authFilePerm = 0o600
)

// authDirPattern names a throwaway credential directory in the temp dir.
const authDirPattern = "astro-push-auth-*"

// staleAuthAge is how old a throwaway credential directory has to be before
// a later push takes it for one a killed push left behind and removes it.
// A push holds its directory for as long as its upload runs, so this is well
// past the longest one.
const staleAuthAge = time.Hour

// tempDir is the directory throwaway credentials are made in; a var so tests
// keep theirs apart.
var tempDir = os.TempDir

// newPushAuth writes the credential for one push to registry into a
// directory of its own: username and secret (a "Bearer " prefix dropped),
// the user's docker context so docker reaches the daemon it always does, and
// nothing else of the user's. env is the engine's env; podman adds
// --authfile.
func newPushAuth(registry, username, secret string, podman bool, env []string) (*pushAuth, error) {
	removeStaleAuth()
	dir, err := os.MkdirTemp(tempDir(), authDirPattern)
	if err != nil {
		return nil, fmt.Errorf("creating the push's credential directory: %w", err)
	}
	a := &pushAuth{dir: dir}
	// MkdirTemp makes it 0700 already; said again so no umask or platform
	// default can widen it.
	if err := os.Chmod(dir, authDirPerm); err != nil {
		a.remove()
		return nil, fmt.Errorf("securing the push's credential directory: %w", err)
	}
	userDir := userDockerConfigDir(env)
	if err := writeAuthConfig(dir, userDir, registry, username, strings.TrimPrefix(secret, bearerPrefix)); err != nil {
		a.remove()
		return nil, err
	}
	file := filepath.Join(dir, "config.json")
	a.env = []string{"DOCKER_CONFIG=" + dir, "REGISTRY_AUTH_FILE=" + file}
	// Docker reads a TLS daemon's client certificates from its config
	// directory unless told otherwise, and that is no longer the user's.
	if tlsWithoutCertPath(env) && userDir != "" {
		a.env = append(a.env, "DOCKER_CERT_PATH="+userDir)
	}
	if podman {
		a.args = []string{"--authfile", file}
	}
	return a, nil
}

// tlsWithoutCertPath reports a docker env that talks TLS to its daemon
// (DOCKER_TLS_VERIFY or DOCKER_TLS) and leaves the certificates to the config
// directory (no DOCKER_CERT_PATH).
func tlsWithoutCertPath(env []string) bool {
	return (envValue(env, "DOCKER_TLS_VERIFY") != "" || envValue(env, "DOCKER_TLS") != "") &&
		envValue(env, "DOCKER_CERT_PATH") == ""
}

// remove deletes the directory and the credential in it; a pushAuth with no
// directory (a push without a credential) has nothing to remove.
func (a *pushAuth) remove() {
	if a.dir == "" {
		return
	}
	os.RemoveAll(a.dir) //nolint:errcheck // best-effort: a temp dir the OS also cleans
}

// cmdArgs is the engine subcommand sub, pointed at the credential, with the
// rest of its arguments.
func (a *pushAuth) cmdArgs(sub string, rest ...string) []string {
	args := append([]string{sub}, a.args...)
	return append(args, rest...)
}

// cmdEnv is env with the credential's env after it, so it wins.
func (a *pushAuth) cmdEnv(env []string) []string {
	if len(a.env) == 0 {
		return env
	}
	// A copy: the engine's own env is shared with every other command.
	out := make([]string, 0, len(env)+len(a.env))
	out = append(out, env...)
	return append(out, a.env...)
}

// removeStaleAuth removes the throwaway credential directories a push that
// never got to its cleanup (killed, or its machine put to sleep) left in the
// temp dir: the user's own, older than staleAuthAge. Errors are ignored; the
// next push tries again.
func removeStaleAuth() {
	dirs, err := filepath.Glob(filepath.Join(tempDir(), authDirPattern))
	if err != nil {
		return
	}
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || time.Since(info.ModTime()) < staleAuthAge || !ownedByUser(info) {
			continue
		}
		os.RemoveAll(dir) //nolint:errcheck // best-effort, retried by the next push
	}
}

// authEntry is one registry's credential, as docker and containers store it.
type authEntry struct {
	Auth string `json:"auth"`
}

// authConfigFile is the throwaway credential file: the push's credential,
// and the user's current docker context, which decides the daemon docker
// talks to. Containers' auth.json readers ignore the context.
type authConfigFile struct {
	Auths          map[string]authEntry `json:"auths"`
	CurrentContext string               `json:"currentContext,omitempty"`
}

// writeAuthConfig writes the throwaway credential file into dir, and carries
// the user's docker contexts over from userDir (their endpoints and TLS
// material), so the current context it names resolves. DOCKER_CONTEXT and
// DOCKER_HOST in the environment still take precedence over the file, as for
// any docker command, since they pass through untouched.
func writeAuthConfig(dir, userDir, registry, username, secret string) error {
	cfg := authConfigFile{Auths: map[string]authEntry{
		registry: {Auth: base64.StdEncoding.EncodeToString([]byte(username + ":" + secret))},
	}}
	if userDir != "" {
		cfg.CurrentContext = readCurrentContext(filepath.Join(userDir, "config.json"))
		if err := carryContexts(filepath.Join(userDir, "contexts"), filepath.Join(dir, "contexts")); err != nil {
			return err
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, authFilePerm); err != nil {
		return fmt.Errorf("writing the push's credential: %w", err)
	}
	return nil
}

// readCurrentContext is the currentContext of the docker config at path, ""
// when it names none or cannot be read: docker's default context.
func readCurrentContext(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.CurrentContext
}

// carryContexts makes the user's contexts directory src visible at dst: a
// symlink where the platform allows one, a copy otherwise (Windows without
// the symlink privilege). docker only reads it for a push. A user with no
// contexts directory has nothing to carry.
func carryContexts(src, dst string) error {
	info, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the docker contexts at %s: %w", src, err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	return copyTree(src, dst)
}

// copyTree copies the directory src to dst, files owner-only.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, authDirPerm)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target, authFilePerm)
	})
}

// userDockerConfigDir is the docker config directory the user's own docker
// commands read: DOCKER_CONFIG from the engine's env or the process's, else
// ~/.docker. "" when there is no home to find it in.
func userDockerConfigDir(env []string) string {
	if dir := envValue(env, "DOCKER_CONFIG"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker")
}

// envValue is key's value as a command run with env sees it: the last
// key= in env, else the process's own.
func envValue(env []string, key string) string {
	val, found := "", false
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			val, found = v, true
		}
	}
	if found {
		return val
	}
	return os.Getenv(key)
}
