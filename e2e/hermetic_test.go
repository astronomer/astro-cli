//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
)

// guardRealConfig is what this suite does where the other test packages call
// isolateHome (or testenv.Isolate): it does not move HOME for the test process,
// because the harness reads the developer's real docker configuration and uv
// cache from it on purpose (dockerConfigDir, uvCache). Every command the suite
// runs gets its own HOME, ASTRO_HOME and XDG_CACHE_HOME instead (project.env),
// and a vault that cannot reach the OS keychain (stampKeyringUnavailable).
//
// What it adds is the check the others make: the developer's own
// ~/.astro/config.yaml and its lock file must be as they were before the run,
// by metadata only. A run that changed either fails, even if every case
// passed, because it means a command reached a home it was not given.
func guardRealConfig() (finish func(code int) int) {
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
	return func(code int) int {
		for _, p := range watched {
			if stampOf(p) != before[p] {
				fmt.Fprintf(os.Stderr, "\ne2e: FAIL: the run changed %s, which is the developer's own, even though the cases passed\n", p)
				return 1
			}
		}
		return code
	}
}
