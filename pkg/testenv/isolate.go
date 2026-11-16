package testenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// inheritedEnv marks a process that a test binary started from itself: a
// helper that re-executes the test binary with -test.run to be a second CLI
// process. Isolate sets it in the parent. A helper that finds it works in the
// home its parent made, and never makes or clears one of its own.
const inheritedEnv = "ASTRO_TEST_ISOLATED_HOME"

// strictEnv makes a change to the developer's real config fail the package
// instead of warning. CI makes it strict without it.
const strictEnv = "ASTRO_TEST_STRICT_REALCONFIG"

// Isolate makes the test binary hermetic, and returns the function that ends
// the run. It is the first call in a package's TestMain, once per binary:
//
//	func TestMain(m *testing.M) {
//		finish := Isolate()
//		os.Exit(finish(m.Run()))
//	}
//
// It does five things. A package whose test binary links pkg/secrets passes the
// keyring hooks as Options (the root's pkg/testing/testenv does for every root
// package).
//
//   - Unsets every variable in behaviorEnv, so the tests run the same under a
//     shell that exports ASTRO_API_TOKEN and on a CI runner. A test that needs
//     one sets it itself with t.Setenv (or UseAPIToken), which the test
//     restores afterward.
//   - Makes a fresh temporary directory and points HOME, ASTRO_HOME,
//     XDG_CONFIG_HOME, XDG_CACHE_HOME and XDG_DATA_HOME (and USERPROFILE,
//     APPDATA and LOCALAPPDATA on Windows) into it. finish removes it.
//     ASTRO_HOME is the same directory as HOME, as it is when the variable is
//     unset. Packages that worked out a path from the real home when they were
//     loaded (config does) register an AfterIsolate hook that works it out
//     again, so nothing starts out pointing at the real one.
//   - Makes the default keyring an in-memory one, through Options.MockKeyring.
//   - Points uv's managed Pythons at a directory shared by every test binary
//     (see uvPythonDir), so tests that install one download it once, and never
//     into ~/.local.
//   - Pins where the go command keeps its own state to what `go env` says
//     before HOME moves.
//
// finish(code) removes the directory and returns code, with two exceptions
// that return 1 so a package whose tests passed still fails. The OS keychain
// was asked for and refused: a caller may have fallen back on the error, so a
// passing test proves nothing. And the real ~/.astro/config.yaml or its lock
// file changed, when CI is set or ASTRO_TEST_STRICT_REALCONFIG=1: on a
// developer's machine that change can be a real `astro login` in another
// terminal, so without either it is a warning on stderr.
//
// A helper process (see inheritedEnv) keeps its parent's home: it checks that
// HOME and ASTRO_HOME are in the temporary directory, exits 2 if they are not,
// and changes nothing else.
//
// It is for TestMain, where there is no *testing.T to Setenv through; a test
// must never call it.
func Isolate(opts ...Options) (finish func(code int) int) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.KeepHome {
		// Only the guard: the suite's own commands get homes of their own.
		return keepHome(guards{watch: realStateWatch(), strict: CIIsSet() || os.Getenv(strictEnv) == "1", refusals: func() int { return 0 }})
	}
	if os.Getenv(inheritedEnv) != "" {
		inherit(o)
		return func(code int) int { return code }
	}
	refusals := o.KeyringRefusals
	if refusals == nil {
		refusals = func() int { return 0 }
	}
	// Both are read here, before isolate unsets CI with the rest.
	return isolate(guards{
		watch:    realStateWatch(),
		strict:   CIIsSet() || os.Getenv(strictEnv) == "1",
		refusals: refusals,
	}, o)
}

// Options are what a caller hands Isolate. The zero value is right for a package
// that never reaches pkg/secrets.
type Options struct {
	// MockKeyring installs the in-memory keyring (and tells pkg/secrets, whose
	// guard refuses the OS keychain in a test binary otherwise), and
	// KeyringRefusals is how many times the OS keychain was asked for and
	// refused. A package whose test binary links pkg/secrets passes both
	// (secrets.MockKeyringForTests and secrets.OSKeyringRefusals); this module
	// cannot import pkg/secrets itself, which uses it in its own tests.
	MockKeyring     func()
	KeyringRefusals func() int
	// KeepHome leaves HOME, the XDG variables and the environment as they are and
	// only watches the real config. It is for e2e, whose harness reads the
	// developer's docker configuration and uv cache on purpose, and gives every
	// command it runs a home of its own.
	KeepHome bool
}

func (o Options) mockKeyring() {
	if o.MockKeyring != nil {
		o.MockKeyring()
	}
}

// keepHome is Isolate for Options.KeepHome.
func keepHome(g guards) func(code int) int {
	return func(code int) int {
		code, msg := g.verdict(code)
		if msg != "" {
			fmt.Fprint(os.Stderr, msg)
		}
		return code
	}
}

// Main is the whole TestMain of a package with nothing to add to it:
//
//	func TestMain(m *testing.M) { testenv.Main(m) }
//
// It isolates the run, runs the tests, ends the run and exits with the code
// Isolate's finish gives. A package that does more around m.Run, as cmd/astro
// does with the emit watch, calls Isolate itself.
func Main(m *testing.M, opts ...Options) {
	finish := Isolate(opts...)
	os.Exit(finish(m.Run()))
}

// afterIsolate are the hooks AfterIsolate registered.
var afterIsolate []func()

// AfterIsolate registers f to run once Isolate has moved HOME and the rest, for
// a package that worked a path out from the real home when it was loaded. It is
// called from an init function, before TestMain, and only in a test binary.
func AfterIsolate(f func()) { afterIsolate = append(afterIsolate, f) }

// CIIsSet reports whether the CI environment variable says CI is on, by the same
// rule as pkg/input's (input.Refusal): anything but empty, "0" or "false", in any
// case and ignoring spaces.
func CIIsSet() bool { return truthy(os.Getenv("CI")) }

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false":
		return false
	}
	return true
}

// guards are what finish judges the run against.
type guards struct {
	// watch is the developer's real config, and strict whether a change to it
	// fails the package or only warns.
	watch  *stateWatch
	strict bool
	// refusals is how many times the OS keychain was asked for and refused.
	refusals func() int
}

// verdict is the exit code for a run whose tests returned code, and what to say
// about it on stderr.
func (g guards) verdict(code int) (exit int, msg string) {
	if n := g.refusals(); n > 0 {
		return 1, fmt.Sprintf("\ntestenv: FAIL: this package's tests asked the OS keychain %d time(s) and were refused, even though they passed "+
			"(a caller may have fallen back on the error). See docs/architecture.md, \"Tests never touch the machine\".\n", n)
	}
	changed := g.watch.changed()
	if len(changed) == 0 {
		return code, ""
	}
	list := strings.Join(changed, "\n  ")
	if g.strict {
		return 1, fmt.Sprintf("\ntestenv: FAIL: this package's tests touched the real machine, even though they passed:\n  %s\n"+
			"A test reached the developer's own ~/.astro instead of the temporary home. "+
			"See docs/architecture.md, \"Tests never touch the machine\".\n", list)
	}
	return code, fmt.Sprintf("\ntestenv: WARNING: the developer's real config changed during this run:\n  %s\n"+
		"A test may have reached the real ~/.astro instead of the temporary home, or something else on this machine "+
		"(an astro login in another terminal) changed it. It fails the package when CI is set or %s=1.\n", list, strictEnv)
}

// tempRoot is the temporary home, for dief to remove.
var tempRoot string

// isolate is Isolate with the guards given, so a test can point them at a
// decoy instead of the developer's home.
func isolate(g guards, o Options) func(code int) int {
	for _, k := range behaviorEnv {
		if err := os.Unsetenv(k); err != nil { //nolint:usetesting // for the whole package run
			dief("unsetting %s: %v", k, err)
		}
	}
	// Before anything is made, so a refusal leaves nothing behind.
	uvDir, err := prepareUVDir()
	if err != nil {
		dief("the shared uv directory is not usable: %v; remove it", err)
	}
	pinToolchain()

	root, err := os.MkdirTemp("", "astro-test-")
	if err != nil {
		dief("making the test home: %v", err)
	}
	tempRoot = root
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
	if runtime.GOOS == goosWindows {
		setDir("USERPROFILE", home)
		setDir("APPDATA", filepath.Join(root, "appdata"))
		setDir("LOCALAPPDATA", filepath.Join(root, "localappdata"))
	}
	setenv("UV_PYTHON_INSTALL_DIR", uvDir)
	setenv(inheritedEnv, root)

	o.mockKeyring()
	for _, f := range afterIsolate {
		f()
	}

	return func(code int) int {
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintf(os.Stderr, "testenv: removing %s: %v\n", root, err)
		}
		code, msg := g.verdict(code)
		if msg != "" {
			fmt.Fprint(os.Stderr, msg)
		}
		return code
	}
}

// inherit is Isolate for a helper process.
func inherit(o Options) {
	// The marker is the parent's temporary root, and HOME is inside it. A stale
	// or invented marker (exported from a shell, say) fails here, loudly, rather
	// than switching the isolation off for the binary.
	root := os.Getenv(inheritedEnv)
	if home := os.Getenv("HOME"); !within(home, root) {
		dief("a helper process runs with HOME=%q, which is not inside %s=%q: its parent's home. "+
			"A helper works in the home its parent made, never a real one; unset %s if it is stale", home, inheritedEnv, root, inheritedEnv)
	}
	if dir := os.Getenv("ASTRO_HOME"); !inTestArea(dir) {
		dief("a helper process runs with ASTRO_HOME=%q, which is not in the test home %q or the temporary directory %s", dir, root, os.TempDir())
	}
	o.mockKeyring()
	for _, f := range afterIsolate {
		f()
	}
}

// RequireTemp exits the process, as a helper must, unless every path is inside
// the temporary directory the test binaries use. A helper that re-executes the
// test binary calls it on the config path it is about to write, so a mistake in
// how its parent built the environment fails loudly instead of writing to a
// real ~/.astro.
func RequireTemp(paths ...string) {
	for _, p := range paths {
		if !inTestArea(p) {
			dief("the path %q is not inside the test home or the temporary directory %s, so this helper would write outside it", p, os.TempDir())
		}
	}
}

// inTestArea reports whether path is in the test binary's home or in the
// temporary directory (where a test's own t.TempDir is). The home is checked
// against the root Isolate recorded, so a TMPDIR changed since does not matter.
func inTestArea(path string) bool {
	return within(path, os.TempDir()) || (os.Getenv(inheritedEnv) != "" && within(path, os.Getenv(inheritedEnv)))
}

// within reports whether path is dir or below it, after symlinks (macOS keeps
// its temporary directory behind /var -> /private/var), ignoring case where
// the file system does (Windows).
func within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	a, b := resolve(dir), resolve(path)
	if runtime.GOOS == goosWindows {
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

// dief reports a setup failure and exits, taking the temporary home with it.
func dief(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testenv: "+format+"\n", args...)
	if tempRoot != "" {
		os.RemoveAll(tempRoot) //nolint:errcheck // exiting
	}
	os.Exit(2)
}

// pinToolchain fixes where the go command keeps its own state to what `go env`
// says now, before HOME moves, so settings saved with `go env -w` still apply.
// A test that runs `go list` (internal/archlint does) would otherwise find an
// empty module cache and build cache under the temporary home. A variable that
// is already set is left alone. None of it is the CLI's: they are caches of the
// Go toolchain, which a test run fills as `go test` itself does. Without a go
// command on the PATH there is nothing to pin.
func pinToolchain() {
	names := []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"}
	out, err := exec.Command("go", append([]string{"env"}, names...)...).Output() //nolint:gosec,noctx // fixed arguments; a few ms before any test
	if err != nil {
		return
	}
	values := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(values) != len(names) {
		return
	}
	for i, name := range names {
		if v := strings.TrimRight(values[i], "\r"); v != "" && os.Getenv(name) == "" {
			setenv(name, v)
		}
	}
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
