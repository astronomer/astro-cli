package instancelocate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

// This file is one template, copied into every test package of the modules that
// cannot import the root module's pkg/testing/testenv (internal/archlint fails
// a copy that differs from internal/archlint/testdata/hermetic_template.go.txt
// in anything but the package clause, so the copies cannot drift). Change the
// template, then copy it over each; the TestMain of a package that links
// pkg/secrets passes it the keyring hooks, the others pass nothing.
//
// It does to a test binary what testenv.Isolate does: it unsets the variables
// that steer the CLI, moves HOME, ASTRO_HOME and the XDG and Windows state
// directories into a temporary directory finish removes, points uv's managed
// Pythons at a private directory shared by every test binary, and installs the
// in-memory keyring. It fails the run, though its tests passed, when the OS
// keychain was asked for and refused, and, when CI is set or
// ASTRO_TEST_STRICT_REALCONFIG=1, when the developer's real
// ~/.astro/config.yaml or its lock file changed (otherwise it warns, because a
// real login in another terminal changes it too). A helper process that
// re-executes the test binary keeps the home its parent made.

// keyringHooks are what a package that links pkg/secrets passes: the function
// that installs the in-memory keyring and tells pkg/secrets, and the count of
// OS keychain refusals.
type keyringHooks struct {
	Mock     func()
	Refusals func() int
}

const (
	inheritedHome = "ASTRO_TEST_ISOLATED_HOME"
	strictHome    = "ASTRO_TEST_STRICT_REALCONFIG"
)

// behaviorVars are the variables, besides every ASTRO_ and ASTRONOMER_ one, that
// change what the CLI does and that a shell or a CI runner may export.
var behaviorVars = []string{
	"CI", "JENKINS_URL", "HUDSON_URL", "TF_BUILD", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI",
	"UV_EXCLUDE_NEWER", "UV_NO_SYNC", "NO_COLOR", "CLICOLOR",
}

func isolateHome(kr ...keyringHooks) (finish func(code int) int) {
	var hooks keyringHooks
	if len(kr) > 0 {
		hooks = kr[0]
	}
	mockKeyring := func() {
		if hooks.Mock != nil {
			hooks.Mock()
		}
	}
	if os.Getenv(inheritedHome) != "" {
		requireInherited()
		mockKeyring()
		return func(code int) int { return code }
	}

	// Before the environment is cleared: CI decides whether a change to the real
	// config fails the run.
	strict := ciSet() || os.Getenv(strictHome) == "1"
	watch := watchRealConfig()
	clearBehaviorEnv()
	python := prepareUVDir()

	root, err := os.MkdirTemp("", "astro-test-")
	if err != nil {
		exitf("%v", err)
	}
	home := filepath.Join(root, "home")
	dirs := map[string]string{
		"HOME":                  home,
		"ASTRO_HOME":            home,
		"XDG_CONFIG_HOME":       filepath.Join(root, "config"),
		"XDG_CACHE_HOME":        filepath.Join(root, "cache"),
		"XDG_DATA_HOME":         filepath.Join(root, "data"),
		"UV_PYTHON_INSTALL_DIR": python,
	}
	if runtime.GOOS == "windows" {
		dirs["USERPROFILE"] = home
		dirs["APPDATA"] = filepath.Join(root, "appdata")
		dirs["LOCALAPPDATA"] = filepath.Join(root, "localappdata")
	}
	for name, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			os.RemoveAll(root)
			exitf("%v", err)
		}
		if err := os.Setenv(name, dir); err != nil { //nolint:usetesting // for the whole package run
			os.RemoveAll(root)
			exitf("%v", err)
		}
	}
	if err := os.Setenv(inheritedHome, root); err != nil { //nolint:usetesting // for the whole package run
		os.RemoveAll(root)
		exitf("%v", err)
	}
	mockKeyring()

	return func(code int) int {
		os.RemoveAll(root)
		if hooks.Refusals != nil {
			if n := hooks.Refusals(); n > 0 {
				fmt.Fprintf(os.Stderr, "\nisolateHome: FAIL: the tests asked the OS keychain %d time(s) and were refused, even though they passed\n", n)
				return 1
			}
		}
		for _, p := range watch.changed() {
			if strict {
				fmt.Fprintf(os.Stderr, "\nisolateHome: FAIL: the tests changed %s, which is the developer's own, even though they passed\n", p)
				return 1
			}
			fmt.Fprintf(os.Stderr, "\nisolateHome: WARNING: %s, the developer's own, changed during this run; it fails the run when CI is set or %s=1\n", p, strictHome)
		}
		return code
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "isolateHome: "+format+"\n", args...)
	os.Exit(2)
}

// ciSet reports whether CI says yes, as CI systems spell it.
func ciSet() bool {
	ci := strings.ToLower(os.Getenv("CI"))
	return ci != "" && ci != "0" && ci != "false" && ci != "no"
}

// requireInherited is what a helper process checks: it works in the home its
// parent made, and never a real one.
func requireInherited() {
	for _, name := range []string{"HOME", "ASTRO_HOME"} {
		if dir := os.Getenv(name); !within(dir, os.TempDir()) {
			exitf("a helper process runs with %s=%q, outside the temporary directory %s", name, dir, os.TempDir())
		}
	}
}

// clearBehaviorEnv unsets every ASTRO_ and ASTRONOMER_ variable and behaviorVars.
func clearBehaviorEnv() {
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		drop := strings.HasPrefix(upper, "ASTRO_") || strings.HasPrefix(upper, "ASTRONOMER_")
		for _, v := range behaviorVars {
			drop = drop || upper == v
		}
		if drop {
			os.Unsetenv(name) //nolint:usetesting // for the whole package run
		}
	}
}

// realConfig is the developer's own ~/.astro/config.yaml and its lock file, as
// they were before the run, by metadata only: they are never opened.
type realConfig struct {
	paths  []string
	before []string
}

func stampOf(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	return fmt.Sprintf("%d %d", info.ModTime().UnixNano(), info.Size())
}

func watchRealConfig() *realConfig {
	var bases []string
	if h := os.Getenv("ASTRO_HOME"); h != "" {
		bases = append(bases, h)
	}
	if h, err := os.UserHomeDir(); err == nil {
		bases = append(bases, h)
	}
	w := &realConfig{}
	for _, base := range bases {
		cfg := filepath.Join(base, ".astro", "config.yaml")
		w.paths = append(w.paths, cfg, cfg+".lock")
	}
	for _, p := range w.paths {
		w.before = append(w.before, stampOf(p))
	}
	return w
}

// changed lists the watched files that differ from before the run.
func (w *realConfig) changed() []string {
	var out []string
	for i, p := range w.paths {
		if stampOf(p) != w.before[i] {
			out = append(out, p)
		}
	}
	return out
}

// prepareUVDir makes the directory the test binaries share for uv's managed
// Pythons, and refuses one it did not make or that someone else could change:
// uv runs the interpreters in it. Each level is made with Mkdir, so a race is
// lost to EEXIST, then Lstat'ed: a real directory, this user's, closed to others.
func prepareUVDir() string {
	parent := filepath.Join(os.TempDir(), "astro-test-uv-"+strconv.Itoa(os.Getuid()))
	python := filepath.Join(parent, "python")
	for _, dir := range []string{parent, python} {
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			exitf("making the shared uv directory %s: %v", dir, err)
		}
		info, err := os.Lstat(dir)
		if err != nil {
			exitf("%v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ownedByMe(info) ||
			(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
			exitf("the shared uv directory %s is not a private directory of this user's (a link, or someone else's, or open to others); remove it", dir)
		}
	}
	return python
}

// ownedByMe reports whether info says the file belongs to this user; where the
// platform records no owner there is nothing to compare.
func ownedByMe(info fs.FileInfo) bool {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return true
	}
	uid := v.Elem().FieldByName("Uid")
	return !uid.IsValid() || uid.Uint() == uint64(os.Getuid())
}

// within reports whether path is dir or below it, after symlinks, ignoring case
// where the file system does.
func within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	a, b := resolve(dir), resolve(path)
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	rel, err := filepath.Rel(a, b)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve is p with symlinks followed in the part that exists.
func resolve(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	for q := p; ; q = filepath.Dir(q) {
		if r, err := filepath.EvalSymlinks(q); err == nil {
			return filepath.Join(r, strings.TrimPrefix(p, q))
		}
		if filepath.Dir(q) == q {
			return p
		}
	}
}
