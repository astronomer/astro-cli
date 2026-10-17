package imagebuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// pushAuth is where a push that carries its own credential keeps it: a
// throwaway directory made for that one push, never the user's own engine
// config. Its login and its push run with extra args and env pointing the
// engine there, and remove deletes it, credential and all.
//
// So a deploy's token is never left behind in the user's credential store or
// keychain, and two pushes running at once, with different credentials for
// the same registry, cannot overwrite each other's: each has its own
// directory.
type pushAuth struct {
	dir string
	// args go right after the subcommand (podman's --authfile); env is
	// appended to the engine's env (docker's DOCKER_CONFIG).
	args []string
	env  []string
}

// authDirPerm and authFilePerm keep the throwaway credential the user's alone.
const (
	authDirPerm  = 0o700
	authFilePerm = 0o600
)

// newPushAuth makes the throwaway credential directory for one push on an
// engine: podman's --authfile, or docker's config directory.
func newPushAuth(podman bool, env []string) (*pushAuth, error) {
	dir, err := os.MkdirTemp("", "astro-push-auth-*")
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
	if podman {
		a.args = []string{"--authfile", filepath.Join(dir, "auth.json")}
		return a, nil
	}
	if err := writeDockerConfig(dir, userDockerConfigDir(env)); err != nil {
		a.remove()
		return nil, err
	}
	a.env = []string{"DOCKER_CONFIG=" + dir}
	return a, nil
}

// remove deletes the directory and the credential in it; a pushAuth with no
// directory (a push without a credential) has nothing to remove.
func (a *pushAuth) remove() {
	if a.dir == "" {
		return
	}
	os.RemoveAll(a.dir) //nolint:errcheck // best-effort: a temp dir the OS also cleans
}

// cmdArgs is the engine subcommand sub, pointed at the credential directory,
// with the rest of its arguments.
func (a *pushAuth) cmdArgs(sub string, rest ...string) []string {
	args := append([]string{sub}, a.args...)
	return append(args, rest...)
}

func (a *pushAuth) cmdEnv(env []string) []string {
	if len(a.env) == 0 {
		return env
	}
	// A copy: the engine's own env is shared with every other command.
	out := make([]string, 0, len(env)+len(a.env))
	out = append(out, env...)
	return append(out, a.env...)
}

// placeholderAuth is a registry no one pushes to, listed in the throwaway
// docker config so it holds an "auths" entry. A docker config with no
// credentials of any kind has docker detect the platform's credential helper
// (the keychain, Docker Desktop's, wincred) and store the login there; one
// that already holds an entry stores the login in the file, which the push's
// cleanup deletes.
const placeholderAuth = "astro-cli-push.invalid"

// dockerConfigFile is the part of docker's config.json the throwaway config
// carries: what decides which daemon docker talks to, and the placeholder
// that keeps the credential in the file. credsStore, credHelpers and the
// user's own auths are left out on purpose.
type dockerConfigFile struct {
	Auths          map[string]struct{} `json:"auths"`
	CurrentContext string              `json:"currentContext,omitempty"`
}

// writeDockerConfig writes the throwaway docker config into dir from the
// user's config directory userDir: the user's current context, and their
// contexts (each context's endpoint and TLS material), so docker reaches the
// same daemon it does for the user. DOCKER_CONTEXT and DOCKER_HOST in the
// environment still take precedence over the config file, as for any docker
// command, since they pass through untouched.
func writeDockerConfig(dir, userDir string) error {
	cfg := dockerConfigFile{Auths: map[string]struct{}{placeholderAuth: {}}}
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
		return fmt.Errorf("writing the push's docker config: %w", err)
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
// the symlink privilege). docker only reads it for a login and a push. A user
// with no contexts directory has nothing to carry.
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
	if dir := lastEnv(env, "DOCKER_CONFIG"); dir != "" {
		return dir
	}
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker")
}

// lastEnv is the value of key in env, the last one winning as it does for a
// command run with it.
func lastEnv(env []string, key string) string {
	val := ""
	for _, kv := range env {
		if len(kv) > len(key) && kv[:len(key)] == key && kv[len(key)] == '=' {
			val = kv[len(key)+1:]
		}
	}
	return val
}

// loginWith logs the engine in to registry into a, as Login does into the
// engine's own config.
func (e Engine) loginWith(ctx context.Context, a *pushAuth, registry, username, secret string) (refused bool, err error) {
	return e.loginIn(ctx, a.cmdEnv(e.Env), a.cmdArgs("login"), registry, username, secret)
}
