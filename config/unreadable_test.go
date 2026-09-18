package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// A config this process could not read is one it must not write over.
//
// viper takes its write target from SetConfigFile, which runs before the read,
// so a failed read leaves the object holding nothing but registered defaults
// while still pointed at the user's file — and configExists, the only guard
// the setters had, tests ConfigFileUsed() and was therefore still true. The
// next `astro config set`, `astro login` or context switch serialized
// AllSettings() over the top.
//
// Measured before the guard: a home config with one syntax error, plus
// `astro config set -g page_size 50`, replaced the file's contexts and token
// with 54 lines of defaults and reported success.
func (s *Suite) TestSaveRefusesAConfigItCouldNotRead() {
	fs := afero.NewMemMapFs()
	const dir = "unreadable-home"
	s.NoError(os.Setenv("ASTRO_HOME", dir))
	defer os.Unsetenv("ASTRO_HOME")

	file := filepath.Join(dir, ConfigDir, ConfigFileNameWithExt)
	const original = "context: my-org\ncontexts:\n  my-org:\n    token: SECRET\nbroken: [unclosed\n"
	s.NoError(afero.WriteFile(fs, file, []byte(original), 0o600))

	initHome(fs)

	// The read failed, so the file is on the list and a write is refused
	// rather than performed.
	s.True(unreadableConfigs[HomeConfigFile], "a config that would not parse should be recorded")
	err := saveConfig(viperHome, HomeConfigFile)
	s.Error(err, "writing a file this process could not read destroys it")
	s.Contains(err.Error(), "refusing to write")

	// And the refusal is the point: the file is untouched.
	after, readErr := afero.ReadFile(fs, file)
	s.NoError(readErr)
	s.Equal(original, string(after), "the user's config must survive")
	s.Contains(string(after), "SECRET")
}

// The same for the project config, which has the same shape and the same
// consequence.
//
// Its file is where a converted project's Deployment mappings live — the ones
// the conversion's own leftover note tells somebody to go and read — and
// `astro deploy` and `astro config set -p` both write it.
func (s *Suite) TestSaveRefusesAProjectConfigItCouldNotRead() {
	fs := afero.NewMemMapFs()
	file := filepath.Join(WorkingPath, ConfigDir, ConfigFileNameWithExt)
	const original = "project:\n  deployment: prod-1\nbroken: [unclosed\n"
	s.NoError(afero.WriteFile(fs, file, []byte(original), 0o600))

	initProject(fs)

	s.True(unreadableConfigs[file], "a project config that would not parse should be recorded")
	err := saveConfig(viperProject, file)
	s.Error(err, "writing a project config this process could not read destroys it")

	after, readErr := afero.ReadFile(fs, file)
	s.NoError(readErr)
	s.Equal(original, string(after), "the project's own config must survive")
}

// A config that reads cleanly is still writable, including after an earlier
// run in the same process recorded a failure for it.
func (s *Suite) TestSaveAllowsAConfigItCouldRead() {
	fs := afero.NewMemMapFs()
	const dir = "readable-home"
	s.NoError(os.Setenv("ASTRO_HOME", dir))
	defer os.Unsetenv("ASTRO_HOME")

	file := filepath.Join(dir, ConfigDir, ConfigFileNameWithExt)
	s.NoError(afero.WriteFile(fs, file, []byte("broken: [unclosed\n"), 0o600))
	initHome(fs)
	s.True(unreadableConfigs[HomeConfigFile])

	// Repaired, and read again: the record clears, or a file fixed between
	// runs would stay unwritable for good.
	s.NoError(afero.WriteFile(fs, file, []byte("context: my-org\n"), 0o600))
	initHome(fs)
	s.False(unreadableConfigs[HomeConfigFile], "a config that now parses is writable again")
}
