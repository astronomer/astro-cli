package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// restoreConfigGlobals puts back the package state initHome and initProject
// assign, when the test ends.
//
// The rest of this suite writes its fixture to whatever HomeConfigFile
// currently holds and only then calls InitConfig, so a test that leaves that
// pointing at its own directory silently breaks whichever test testify runs
// next. The suite runs methods in name order, which is not an order anyone is
// maintaining.
//
// The viper objects and the filesystem behind them go back too. A test that
// runs initHome against a real OsFs leaves viperHome bound to it; a later
// test that writes without re-initializing would then put its defaults
// through the OS filesystem, over the developer's own ~/.astro/config.yaml.
// Nothing does that today, which is the kind of thing that stays true until
// somebody adds a test.
func (s *Suite) restoreConfigGlobals() {
	origFile, origPath := HomeConfigFile, HomeConfigPath
	origHomeV, origProjectV, origFs := viperHome, viperProject, configFs
	s.T().Cleanup(func() {
		delete(unreadableConfigs, HomeConfigFile)
		HomeConfigFile, HomeConfigPath = origFile, origPath
		viperHome, viperProject, configFs = origHomeV, origProjectV, origFs
	})
}

// withAstroHome is restoreConfigGlobals with ASTRO_HOME pointed at dir, which
// is what most of these tests want: unsetting the variable afterwards does
// not undo what initHome did to the globals while it was set.
func (s *Suite) withAstroHome(dir string) {
	s.restoreConfigGlobals()
	orig, had := os.LookupEnv("ASTRO_HOME")
	s.NoError(os.Setenv("ASTRO_HOME", dir))
	s.T().Cleanup(func() {
		if had {
			_ = os.Setenv("ASTRO_HOME", orig)
		} else {
			_ = os.Unsetenv("ASTRO_HOME")
		}
	})
}

// Reading the home config must not create one.
//
// InitConfig runs unconditionally from main, before cobra has parsed argv, so
// this ran for every command — including the core tree, which reads no config/
// setting at all. Measured before the change, on a home with no .astro in it:
// one `astro init` left a 54-line config.yaml and a config.yaml.lock behind,
// and so did `astro version`.
func (s *Suite) TestInitHomeDoesNotCreateAConfig() {
	fs := afero.NewMemMapFs()
	const dir = "home-that-should-stay-empty"
	s.withAstroHome(dir)

	initHome(fs)

	fileThere, err := afero.Exists(fs, filepath.Join(dir, ConfigDir, ConfigFileNameWithExt))
	s.NoError(err)
	s.False(fileThere, "reading a config that is not there must not write one")

	dirThere, err := afero.DirExists(fs, filepath.Join(dir, ConfigDir))
	s.NoError(err)
	s.False(dirThere, "and must not leave the .astro directory behind either")

	// Absent is not the same as corrupt. Recording it would make the first
	// write refuse itself, because saveConfig checks this map.
	s.False(unreadableConfigs[HomeConfigFile], "a config that does not exist yet is not unreadable")
}

// The values still resolve without a file, which is why not creating one
// changes nothing downstream: CreateConfig only ever wrote out the defaults
// registered in initHome, so the file it left carried no value the process
// did not already hold.
func (s *Suite) TestDefaultsResolveWithNoHomeConfig() {
	fs := afero.NewMemMapFs()
	const dir = "home-with-no-config"
	s.withAstroHome(dir)

	initHome(fs)

	s.Equal(CFG.PageSize.Default, viperHome.GetString(CFG.PageSize.Path))
	s.True(configExists(viperHome), "the write target is set by SetConfigFile, not by the file being there")
}

// And the first write creates it, which is the other half of the bargain: the
// file appears when somebody sets a value, not when somebody runs a command.
//
// A real filesystem here, because saveConfig takes the lock with os.MkdirAll
// and flock rather than through the afero fs.
func (s *Suite) TestFirstWriteCreatesTheHomeConfig() {
	dir := s.T().TempDir()
	s.withAstroHome(dir)

	fs := afero.NewOsFs()
	initHome(fs)

	file := filepath.Join(dir, ConfigDir, ConfigFileNameWithExt)
	_, err := os.Stat(file)
	s.True(os.IsNotExist(err), "nothing yet")

	s.NoError(CFG.PageSize.SetHomeString("50"))

	written, err := os.ReadFile(file)
	s.NoError(err, "setting a value must create the file it is stored in")
	s.Contains(string(written), "page_size")

	// And it is created 0600. This file holds the API token, and viper's
	// WriteConfigAs would otherwise make it 0644 — CreateConfig used to set
	// the mode at startup, so moving creation to the first write moved the
	// responsibility with it.
	//
	// Not on Windows, which has no POSIX mode bits: Go reports 0666 there
	// whatever it was asked for.
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(file)
		s.NoError(statErr)
		s.Equal(filePerm, info.Mode().Perm(), "the home config holds the API token")
	}

	// And it reads back, so a later process sees the value.
	initHome(fs)
	s.Equal("50", viperHome.GetString(CFG.PageSize.Path))
}

// A config that was corrupt and is now gone is writable again.
//
// The record exists to stop a write replacing a file whose contents this
// process could not see. Once the file is deleted there is nothing left to
// destroy, and leaving it on the list would make `astro login` fail on a home
// the user had already cleaned up by hand.
func (s *Suite) TestRemovingABrokenConfigClearsTheRecord() {
	fs := afero.NewMemMapFs()
	const dir = "home-repaired-by-deletion"
	s.withAstroHome(dir)

	file := filepath.Join(dir, ConfigDir, ConfigFileNameWithExt)
	s.NoError(afero.WriteFile(fs, file, []byte("broken: [unclosed\n"), 0o600))
	initHome(fs)
	s.True(unreadableConfigs[HomeConfigFile], "a file that would not parse is recorded")

	s.NoError(fs.Remove(file))
	initHome(fs)
	s.False(unreadableConfigs[HomeConfigFile], "a file that is gone cannot be destroyed by writing one")

	// Nor did reading the now-absent file put it back.
	back, err := afero.Exists(fs, file)
	s.NoError(err)
	s.False(back)
}

// A write that fails leaves the config as it was, and nothing beside it.
//
// saveConfig publishes the file through a temp file and a rename. If the
// rename fails — a full disk, a quota, a read-only directory entry — the temp
// file is taken back out, and the file that was there is untouched: a failed
// save must not leave an empty config where there had been a full one, since
// an empty config parses cleanly and the next run would read a healthy file
// full of defaults and report nothing wrong.
func (s *Suite) TestAFailedWriteLeavesTheConfigAsItWas() {
	s.restoreConfigGlobals()
	memfs := afero.NewMemMapFs()
	dir := filepath.Join(s.T().TempDir(), ConfigDir)
	file := filepath.Join(dir, ConfigFileNameWithExt)
	s.Require().NoError(afero.WriteFile(memfs, file, []byte("before: kept\n"), 0o600))

	configFs = renameFailsFs{memfs}
	v := viper.New()
	v.SetConfigType(ConfigFileType)
	v.Set("after", "lost")

	s.Error(saveConfig(v, file), "the rename cannot succeed")

	got, err := afero.ReadFile(memfs, file)
	s.NoError(err)
	s.Equal("before: kept\n", string(got), "a failed write must leave the config as it was")
	entries, err := afero.ReadDir(memfs, dir)
	s.NoError(err)
	s.Len(entries, 1, "a failed write must not leave its temp file behind")
}

// renameFailsFs is a filesystem on which every write succeeds and no rename
// does.
type renameFailsFs struct{ afero.Fs }

func (renameFailsFs) Rename(string, string) error { return os.ErrPermission }

// The project config gets the same treatment as the home config: a file that
// is gone is not a file a write could destroy.
//
// Without this, a project config that failed to parse and was then deleted —
// by hand, or by a conversion rewriting the project — stayed on the
// unreadable list for the life of the process, and the next write refused
// itself over a file that was no longer there.
func (s *Suite) TestInitProjectClearsTheRecordWhenTheFileIsGone() {
	fs := afero.NewMemMapFs()
	s.restoreConfigGlobals()

	file := filepath.Join(WorkingPath, ConfigDir, ConfigFileNameWithExt)
	s.NoError(afero.WriteFile(fs, file, []byte("broken: [unclosed\n"), 0o600))
	s.T().Cleanup(func() { delete(unreadableConfigs, file) })

	initProject(fs)
	s.True(unreadableConfigs[file], "a project config that would not parse is recorded")

	s.NoError(fs.Remove(file))
	initProject(fs)
	s.False(unreadableConfigs[file], "and the record goes when the file does")
}
