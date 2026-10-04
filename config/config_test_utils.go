package config

import (
	"io/fs"
	"os"
	"path/filepath"

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
