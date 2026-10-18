package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

func CreateTempProject() (dir string, cleanup func(), err error) {
	projectDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", nil, err
	}
	astroDirPerms := 0o755
	err = os.Mkdir(filepath.Join(projectDir, ".astro"), fs.FileMode(astroDirPerms))
	if err != nil {
		return "", nil, err
	}
	configFile, err := os.Create(filepath.Join(projectDir, ConfigDir, ConfigFileNameWithExt))
	if err != nil {
		return "", nil, err
	}
	return projectDir, func() {
		configFile.Close()
		os.RemoveAll(projectDir) //nolint:errcheck // best-effort cleanup
	}, nil
}

// UseLoginsForTesting keeps logins in l for the rest of a test, in place of
// the shared vault, which is off in test binaries. It returns a func that
// restores the previous vault.
func UseLoginsForTesting(l *secrets.Logins) (restore func()) {
	prev := loginsOverride
	loginsOverride = l
	return func() { loginsOverride = prev }
}

// CountLoginResolutions counts, for the rest of a test, each time a context's
// login is resolved: read from the vault or the keyring, perhaps moved there
// with the config rewritten, perhaps with a warning on stderr. count says how
// many so far; restore stops counting.
func CountLoginResolutions() (count func() int, restore func()) {
	var n atomic.Int64
	prev := loginResolved
	loginResolved = func() { n.Add(1) }
	return func() int { return int(n.Load()) }, func() { loginResolved = prev }
}
