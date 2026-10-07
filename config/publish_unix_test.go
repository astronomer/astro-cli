//go:build !windows

package config

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// someoneElse is an owner no file in a test's temp dir really has.
var someoneElse = owner{uid: os.Getuid() + 4242, gid: os.Getgid() + 4242}

// ownedBySomeoneElse makes the config itself — not the temp file a save
// makes beside it — read as owned by someoneElse.
func (s *Suite) ownedBySomeoneElse() {
	orig := ownerOf
	s.T().Cleanup(func() { ownerOf = orig })
	ownerOf = func(info iofs.FileInfo) (owner, bool) {
		if info.Name() == ConfigFileNameWithExt {
			return someoneElse, true
		}
		return statOwner(info)
	}
}

// asRoot makes this process look like root to publishConfig, and records the
// chowns it asks for instead of making them, returning them through the slice
// it hands back. chownErr is what each chown returns.
func (s *Suite) asRoot(chownErr error) *[]chownCall {
	origEuid, origChown := geteuid, chownFile
	s.T().Cleanup(func() { geteuid, chownFile = origEuid, origChown })
	calls := &[]chownCall{}
	geteuid = func() int { return 0 }
	chownFile = func(name string, uid, gid int) error {
		*calls = append(*calls, chownCall{name, owner{uid, gid}})
		return chownErr
	}
	return calls
}

type chownCall struct {
	name string
	to   owner
}

// savedConfig writes a config holding "a: old" to a temp dir and returns it,
// its file's identity, and a viper object that saves "a: new" over it.
func (s *Suite) savedConfig() (file string, before os.FileInfo, v *viper.Viper) {
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
	file = filepath.Join(s.T().TempDir(), ConfigDir, ConfigFileNameWithExt)
	s.Require().NoError(os.MkdirAll(filepath.Dir(file), 0o700))
	s.Require().NoError(os.WriteFile(file, []byte("a: old\n"), 0o600))
	before, err := os.Stat(file)
	s.Require().NoError(err)
	v = viper.New()
	v.SetConfigType(ConfigFileType)
	v.Set("a", "new")
	return file, before, v
}

// assertSaved checks the save landed, and whether it replaced the file
// (a rename: a new inode) or rewrote it in place (the same one).
func (s *Suite) assertSaved(file string, before os.FileInfo, replaced bool) {
	raw, err := os.ReadFile(file)
	s.Require().NoError(err)
	s.Equal("a: new\n", string(raw))
	after, err := os.Stat(file)
	s.Require().NoError(err)
	if replaced {
		s.False(os.SameFile(before, after), "published by rename")
	} else {
		s.True(os.SameFile(before, after), "written in place")
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	s.Require().NoError(err)
	for _, e := range entries {
		s.Contains([]string{ConfigFileNameWithExt, ConfigFileNameWithExt + ".lock"}, e.Name(), "a temp file was left behind")
	}
}

// The ordinary save: the new file has the owner the old one had, so it is
// published by rename and nobody is chowned, root or not.
func (s *Suite) TestASaveByTheOwnerReplacesWithoutAChown() {
	file, before, v := s.savedConfig()
	calls := s.asRoot(nil)

	s.Require().NoError(saveConfig(v, file))
	s.Empty(*calls)
	s.assertSaved(file, before, true)
}

// Root saving a config that belongs to someone else gives the new file back
// to them before it is published: `sudo astro login` must not leave the user
// a home config they cannot read.
func (s *Suite) TestRootSavingAUsersConfigKeepsItsOwner() {
	file, before, v := s.savedConfig()
	s.ownedBySomeoneElse()
	calls := s.asRoot(nil)

	s.Require().NoError(saveConfig(v, file))

	s.Require().Len(*calls, 1, "root must give the file back to its owner")
	got := (*calls)[0]
	s.Equal(someoneElse, got.to)
	// The resolved path: a macOS temp dir is behind the /var symlink.
	resolved, err := filepath.EvalSymlinks(file)
	s.Require().NoError(err)
	s.Equal(filepath.Dir(resolved), filepath.Dir(got.name), "the temp file, beside the config")
	s.NotEqual(resolved, got.name, "chowned before the rename, not after")
	s.assertSaved(file, before, true)
}

// Root that cannot chown — no CAP_CHOWN in a container — writes the file in
// place, as it always did, rather than taking it from its owner or failing a
// save that used to work.
func (s *Suite) TestRootThatCannotChownWritesInPlace() {
	file, before, v := s.savedConfig()
	s.ownedBySomeoneElse()
	calls := s.asRoot(&os.PathError{Op: "chown", Path: file, Err: syscall.EPERM})

	s.Require().NoError(saveConfig(v, file))
	s.Len(*calls, 1, "root tries")
	s.assertSaved(file, before, false)
}

// A chown that fails for any other reason falls back the same way: none of
// them makes the in-place write worse than it was.
func (s *Suite) TestAnyFailedChownWritesInPlace() {
	file, before, v := s.savedConfig()
	s.ownedBySomeoneElse()
	s.asRoot(errors.New("invalid argument: uid not mapped"))

	s.Require().NoError(saveConfig(v, file))
	s.assertSaved(file, before, false)
}

// Anyone but root saving a file another user owns — a shared project's
// group-writable .astro/config.yaml — writes it in place, so the file stays
// its owner's and their next save still works.
func (s *Suite) TestANonRootSaveOfAnotherUsersConfigWritesInPlace() {
	file, before, v := s.savedConfig()
	s.ownedBySomeoneElse()
	calls := s.asRoot(nil)
	geteuid = func() int { return someoneElse.uid + 1 }

	s.Require().NoError(saveConfig(v, file))
	s.Empty(*calls, "only root can give a file away")
	s.assertSaved(file, before, false)
}
