package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	homedir "github.com/mitchellh/go-homedir"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// inheritedEnv marks a process that a test binary started from itself: a
// helper that re-executes the test binary with -test.run to be a second CLI
// process. Isolate sets it in the parent. A helper that finds it works in the
// home its parent made, and never makes or clears one of its own.
const inheritedEnv = "ASTRO_TEST_ISOLATED_HOME"

// Isolate makes the test binary hermetic, and returns the function that ends
// the run. It is the first call in a package's TestMain, once per binary:
//
//	func TestMain(m *testing.M) {
//		finish := Isolate()
//		os.Exit(finish(m.Run()))
//	}
//
// It does four things.
//
//   - Unsets every variable in behaviorEnv, so the tests run the same under a
//     shell that exports ASTRO_API_TOKEN and on a CI runner. A test that needs
//     one sets it itself with t.Setenv (or UseAPIToken), which the test
//     restores afterward.
//   - Makes a fresh temporary directory and points HOME, ASTRO_HOME,
//     XDG_CONFIG_HOME, XDG_CACHE_HOME and XDG_DATA_HOME (and USERPROFILE,
//     APPDATA and LOCALAPPDATA on Windows) into it. finish removes it.
//     ASTRO_HOME is the same directory as HOME, as it is when the variable is
//     unset.
//   - Makes the default keyring an in-memory one (secrets.MockKeyringForTests,
//     which is keyring.MockInit and the record pkg/secrets' guard needs, which
//     refuses the OS keychain in a test binary otherwise). A test that wants an error-injecting mock calls
//     keyring.MockInitWithError and restores keyring.MockInit in t.Cleanup.
//   - Points uv's managed Pythons at a directory shared by every test binary
//     (see uvPythonDir), so tests that install one download it once, and never
//     into ~/.local.
//
// finish(code) removes the directory and returns code, unless the run changed
// the real ~/.astro/config.yaml or created its lock file. It then says so on
// stderr and returns 1, so a package whose tests passed still fails.
//
// A helper process (see inheritedEnv) keeps its parent's home: it checks that
// HOME and ASTRO_HOME are in the temporary directory, exits 2 if they are not,
// and changes nothing else.
//
// It is for TestMain, where there is no *testing.T to Setenv through; a test
// must never call it.
func Isolate() (finish func(code int) int) {
	if os.Getenv(inheritedEnv) != "" {
		inherit()
		return func(code int) int { return code }
	}
	return isolate(realStateWatch())
}

// Main is the whole TestMain of a package with nothing to add to it:
//
//	func TestMain(m *testing.M) { testenv.Main(m) }
//
// It isolates the run, runs the tests, ends the run and exits with the code
// Isolate's finish gives. A package that does more around m.Run, as cmd/astro
// does with the emit watch, calls Isolate itself.
func Main(m *testing.M) {
	finish := Isolate()
	os.Exit(finish(m.Run()))
}

// isolate is Isolate with the real-state watch given, so a test can point it
// at a decoy instead of the developer's home.
func isolate(watch *stateWatch) func(code int) int {
	for _, k := range behaviorEnv {
		if err := os.Unsetenv(k); err != nil { //nolint:usetesting // for the whole package run
			dief("unsetting %s: %v", k, err)
		}
	}

	root, err := os.MkdirTemp("", "astro-test-")
	if err != nil {
		dief("making the test home: %v", err)
	}
	pinToolchain()
	home := filepath.Join(root, "home")
	for name, dir := range map[string]string{
		"HOME":            home,
		"ASTRO_HOME":      home,
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
	} {
		setDir(name, dir)
	}
	if runtime.GOOS == "windows" {
		setDir("USERPROFILE", home)
		setDir("APPDATA", filepath.Join(root, "appdata"))
		setDir("LOCALAPPDATA", filepath.Join(root, "localappdata"))
	}
	setDir("UV_PYTHON_INSTALL_DIR", uvPythonDir())
	// go-homedir caches the first home it resolves, which is the real one when
	// anything asked before TestMain ran (a package-level var in config does).
	homedir.Reset()
	setenv(inheritedEnv, root)

	mockKeyring()

	return func(code int) int {
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintf(os.Stderr, "testenv: removing %s: %v\n", root, err)
		}
		if n := secrets.OSKeyringRefusals(); n > 0 {
			fmt.Fprintf(os.Stderr, "\ntestenv: FAIL: this package's tests asked the OS keychain %d time(s) and were refused, even though they passed "+
				"(a caller may have fallen back on the error). See docs/architecture.md, \"Tests never touch the machine\".\n", n)
			return 1
		}
		if changed := watch.changed(); len(changed) > 0 {
			fmt.Fprintf(os.Stderr, "\ntestenv: FAIL: this package's tests touched the real machine, even though they passed:\n  %s\n"+
				"A test reached the developer's own ~/.astro instead of the temporary home. "+
				"See docs/architecture.md, \"Tests never touch the machine\".\n", strings.Join(changed, "\n  "))
			return 1
		}
		return code
	}
}

func mockKeyring() { secrets.MockKeyringForTests() }

// inherit is Isolate for a helper process.
func inherit() {
	for _, name := range []string{"HOME", "ASTRO_HOME"} {
		if dir := os.Getenv(name); !within(dir, os.TempDir()) {
			dief("a helper process runs with %s=%q, which is not in the temporary directory %s; "+
				"it must work in the home its parent made, never a real one", name, dir, os.TempDir())
		}
	}
	mockKeyring()
}

// RequireTemp exits the process, as a helper must, unless every path is inside
// the temporary directory the test binaries use. A helper that re-executes the
// test binary calls it on the config path it is about to write, so a mistake in
// how its parent built the environment fails loudly instead of writing to a
// real ~/.astro.
func RequireTemp(paths ...string) {
	for _, p := range paths {
		if !within(p, os.TempDir()) {
			dief("the path %q is not inside the temporary directory %s, so this helper would write outside the test home", p, os.TempDir())
		}
	}
}

// within reports whether path is dir or below it, after symlinks (macOS keeps
// its temporary directory behind /var -> /private/var).
func within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(resolve(dir), resolve(path))
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

func setenv(k, v string) {
	if err := os.Setenv(k, v); err != nil { //nolint:usetesting // for the whole package run
		dief("setting %s: %v", k, err)
	}
}

// dirPerm is the mode of every directory made here: the temporary home and the
// shared uv directory are the user's alone.
const dirPerm = 0o700

func setDir(k, dir string) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		dief("making %s: %v", dir, err)
	}
	setenv(k, dir)
}

func dief(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testenv: "+format+"\n", args...)
	os.Exit(2)
}

// pinToolchain fixes where the go command keeps its own state to where it is
// now, before HOME moves. A test that runs `go list` (internal/archlint does)
// would otherwise find an empty module cache and build cache under the
// temporary home. GOENV is pinned so the settings `go env -w` saved still
// apply. None of it is the CLI's: they are caches of the Go toolchain, which a
// test run fills as `go test` itself does.
func pinToolchain() {
	pin := func(k, v string) {
		if os.Getenv(k) == "" {
			setenv(k, v)
		}
	}
	gopath := os.Getenv("GOPATH")
	if home, err := os.UserHomeDir(); err == nil && gopath == "" {
		gopath = filepath.Join(home, "go")
	}
	if gopath != "" {
		pin("GOPATH", gopath)
		first, _, _ := strings.Cut(gopath, string(os.PathListSeparator))
		pin("GOMODCACHE", filepath.Join(first, "pkg", "mod"))
	}
	if cache, err := os.UserCacheDir(); err == nil {
		pin("GOCACHE", filepath.Join(cache, "go-build"))
	}
	if conf, err := os.UserConfigDir(); err == nil {
		pin("GOENV", filepath.Join(conf, "go", "env"))
	}
}

// uvPythonDir is where the test binaries share uv's managed Pythons: a
// directory per user under the OS temporary directory, which is neither the
// developer's home nor any test's.
//
// uv downloads a managed interpreter (tens of MB) into UV_PYTHON_INSTALL_DIR,
// by default ~/.local/share/uv/python, and the tests under cmd/local ask for
// one. With HOME in a directory each binary removes, they would download it in
// every package on every run; pointing at the developer's real directory would
// write into their home, which no test may. This is a plain directory uv owns
// and locks, so concurrent test binaries are safe, and it lasts until the OS
// clears its temporary directory. The user id is in the name, and the mode is
// 0700, so that on a shared /tmp one user's cache is not another's. uv's
// download cache needs no such setting: the CLI pins UV_CACHE_DIR to its own
// directory under the (isolated) cache home.
func uvPythonDir() string {
	return filepath.Join(os.TempDir(), "astro-test-uv-"+strconv.Itoa(os.Getuid()), "python")
}

// stateWatch remembers what the developer's real ~/.astro/config.yaml and its
// lock file looked like before a run, by metadata only: it never opens them.
type stateWatch struct {
	paths  []string
	before []fileStamp
}

type fileStamp struct {
	exists bool
	mtime  int64
	size   int64
}

func stampOf(path string) fileStamp {
	info, err := os.Stat(path) //nolint:gosec // metadata of a fixed name under the home; the file is never opened
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{exists: true, mtime: info.ModTime().UnixNano(), size: info.Size()}
}

// realStateWatch watches the config under the homes the CLI would have used
// before Isolate moved them: ASTRO_HOME if the shell exported it, and HOME.
func realStateWatch() *stateWatch {
	var bases []string
	if h := os.Getenv("ASTRO_HOME"); h != "" {
		bases = append(bases, h)
	}
	if h, err := os.UserHomeDir(); err == nil {
		bases = append(bases, h)
	}
	return newStateWatch(bases...)
}

func newStateWatch(bases ...string) *stateWatch {
	w := &stateWatch{}
	for _, base := range bases {
		cfg := filepath.Join(base, ".astro", "config.yaml")
		w.paths = append(w.paths, cfg, cfg+".lock")
	}
	for _, p := range w.paths {
		w.before = append(w.before, stampOf(p))
	}
	return w
}

// changed lists what differs from before the run.
func (w *stateWatch) changed() []string {
	var out []string
	for i, p := range w.paths {
		now, was := stampOf(p), w.before[i]
		switch {
		case now == was:
		case !was.exists:
			out = append(out, p+" was created")
		case !now.exists:
			out = append(out, p+" was removed")
		default:
			out = append(out, p+" was modified")
		}
	}
	return out
}
