package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The paths this package works out when it loads come from the real home. The
// test binary's isolation repoints them before any test runs, so a reader that
// gets to them before InitConfig does is in the temporary home too. Run it with
// -shuffle: it must not depend on another test having reached InitConfig.
func TestHomePathsStartInTheIsolatedHome(t *testing.T) {
	home := os.Getenv("HOME")
	for name, path := range map[string]string{
		"HomePath":       HomePath,
		"HomeConfigPath": HomeConfigPath,
		"HomeConfigFile": HomeConfigFile,
	} {
		if !filepath.IsAbs(path) || !hasPrefixDir(path, filepath.Dir(home)) {
			t.Errorf("%s = %q, not in the isolated home %q", name, path, home)
		}
	}
	if want := HomeConfigFileForTesting(); HomeConfigFile != want {
		t.Errorf("HomeConfigFile = %q, but InitConfig would use %q", HomeConfigFile, want)
	}
}

func hasPrefixDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && len(rel) >= 1 && rel[0] != '.'
}
