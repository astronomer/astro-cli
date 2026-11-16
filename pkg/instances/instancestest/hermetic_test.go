package instancestest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// inheritedHome is the variable that tells a process the test binary started
// from itself (a helper that re-executes it with -test.run) that its parent
// already made the temporary home. testenv.Isolate sets the same one.
const inheritedHome = "ASTRO_TEST_ISOLATED_HOME"

// isolateHome is a copy of what testenv.Isolate does to HOME, for a module that
// cannot import the root module's pkg/testing/testenv: it moves HOME,
// ASTRO_HOME and the XDG and Windows state directories into a temporary
// directory, so no test reads or writes the developer's own home, and the
// returned function removes it. It also fails a run, even a passing one, that
// changed the real ~/.astro/config.yaml or created its lock file. Keep it in
// step with testenv (internal/archlint requires every test package in the
// repository to have one or the other).
func isolateHome() (finish func(code int) int) {
	if parent := os.Getenv(inheritedHome); parent != "" {
		// A helper process works in the home its parent made, and never a real one.
		if home := os.Getenv("HOME"); !strings.HasPrefix(home, parent+string(filepath.Separator)) {
			fmt.Fprintf(os.Stderr, "isolateHome: a helper process runs with HOME=%q, outside its parent's %q\n", home, parent)
			os.Exit(2)
		}
		return func(code int) int { return code }
	}
	var bases []string
	if h := os.Getenv("ASTRO_HOME"); h != "" {
		bases = append(bases, h)
	}
	if h, err := os.UserHomeDir(); err == nil {
		bases = append(bases, h)
	}
	type stamp struct {
		exists bool
		mtime  int64
		size   int64
	}
	stampOf := func(path string) stamp {
		info, err := os.Stat(path)
		if err != nil {
			return stamp{}
		}
		return stamp{true, info.ModTime().UnixNano(), info.Size()}
	}
	var watched []string
	before := map[string]stamp{}
	for _, base := range bases {
		cfg := filepath.Join(base, ".astro", "config.yaml")
		for _, p := range []string{cfg, cfg + ".lock"} {
			watched = append(watched, p)
			before[p] = stampOf(p)
		}
	}

	root, err := os.MkdirTemp("", "astro-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolateHome:", err)
		os.Exit(2)
	}
	home := filepath.Join(root, "home")
	dirs := map[string]string{
		"HOME":            home,
		"ASTRO_HOME":      home,
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
	}
	if runtime.GOOS == "windows" {
		dirs["USERPROFILE"] = home
		dirs["APPDATA"] = filepath.Join(root, "appdata")
		dirs["LOCALAPPDATA"] = filepath.Join(root, "localappdata")
	}
	for name, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, "isolateHome:", err)
			os.Exit(2)
		}
		if err := os.Setenv(name, dir); err != nil { //nolint:usetesting // for the whole package run
			fmt.Fprintln(os.Stderr, "isolateHome:", err)
			os.Exit(2)
		}
	}

	if err := os.Setenv(inheritedHome, root); err != nil { //nolint:usetesting // for the whole package run
		fmt.Fprintln(os.Stderr, "isolateHome:", err)
		os.Exit(2)
	}

	return func(code int) int {
		_ = os.RemoveAll(root)
		for _, p := range watched {
			if stampOf(p) != before[p] {
				fmt.Fprintf(os.Stderr, "\nisolateHome: FAIL: the tests changed %s, which is the developer's own, even though they passed\n", p)
				return 1
			}
		}
		return code
	}
}
