package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// A test binary's TestMain moves HOME and ASTRO_HOME into a temporary directory
// (testenv.Isolate), but HomePath, HomeConfigPath and HomeConfigFile were worked
// out from the real home when this package loaded, and initHome is the only
// thing that would recompute them. Until a test reached InitConfig, anything
// reading them (the API registry, the root hooks, otto, the login store, the
// OpenAPI cache) would use the developer's own ~/.astro. So the isolation tells
// this package to work them out again. testing.Testing keeps it out of the
// shipped binary, which imports testenv only for this.
func init() {
	if testing.Testing() {
		testenv.AfterIsolate(resolveHomeForTesting)
	}
}

// resolveHomeForTesting recomputes the home paths from the isolated HOME and
// ASTRO_HOME, by the rule initHome uses, and stops the run if any of them is
// not in the temporary directory.
func resolveHomeForTesting() {
	HomePath, _ = fileutil.GetHomeDir() //nolint:errcheck // as at package load
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home = HomePath
	}
	HomeConfigPath = filepath.Join(home, ConfigDir)
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)
	testenv.RequireTemp(HomePath, HomeConfigPath, HomeConfigFile)
}

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

// HomeConfigFileForTesting is the path InitConfig will read the home config
// from, which HomeConfigFile is not until something has called InitConfig:
// the variable starts as the real home's, worked out when the package loads,
// before a test binary's TestMain has moved HOME and ASTRO_HOME. A test that
// writes its fixture into an in-memory filesystem and then calls InitConfig
// writes it here, so it is found whichever test ran first.
func HomeConfigFileForTesting() string {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home = HomePath
	}
	return filepath.Join(home, ConfigDir, ConfigFileNameWithExt)
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

// UseEnvironmentLoginForTesting applies token to the current context as the
// setup that accepts ASTRO_API_TOKEN does: it is the context's login for the
// process, and workspace (empty for none) its selection, whatever the saved
// context holds. It reports false, and changes nothing, when there is no
// current context to apply it to. restore forgets it.
func UseEnvironmentLoginForTesting(token, workspace string) (restore func(), ok bool) {
	d, err := GetCurrentDomain()
	if err != nil {
		return func() {}, false
	}
	c := Context{Domain: d}
	if !c.ContextExists() {
		return func() {}, false
	}
	if err := c.UseEnvironmentLogin(token, time.Now().Add(time.Hour)); err != nil {
		return func() {}, false
	}
	if err := c.SetEnvironmentContextKey("workspace", workspace); err != nil {
		forgetEnvironmentLogin()
		return func() {}, false
	}
	return forgetEnvironmentLogin, true
}
