package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// A process reading the config while another saves it sees the config as it
// was or as it is now, never an empty one.
//
// The reader takes no lock, as initHome takes none: it runs at the start of
// every command. When saveConfig wrote the file in place, a reader arriving
// between the truncate and the write read an empty file, which is valid YAML
// with no contexts in it, and the command failed with "no context set". That
// is how TestConcurrentForcedRenewalsRefreshOnce in cmd/astro flaked: two
// `astro auth token --force` processes, one starting while the other saved
// its renewal.
//
// Not on Windows. There a rename cannot replace a file a reader has open, and
// four readers in a tight loop have it open nearly all the time, so the test
// would measure how long fsatomic's retry can be starved rather than what a
// reader sees. pkg/fsatomic's own tests cover the Windows half.
func (s *Suite) TestAReaderNeverSeesAConfigHalfWritten() {
	if runtime.GOOS == "windows" {
		s.T().Skip("a reader in a tight loop starves a Windows rename by design")
	}
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
	file := filepath.Join(s.T().TempDir(), ConfigDir, ConfigFileNameWithExt)

	// Big enough that a write is not one syscall.
	padding := strings.Repeat("x", 64<<10)
	writer := func(i int) *viper.Viper {
		v := viper.New()
		v.SetConfigType(ConfigFileType)
		v.Set("context", "astronomer.io")
		v.Set("contexts.astronomer_io.domain", "astronomer.io")
		v.Set("contexts.astronomer_io.padding", padding)
		v.Set("writes", i)
		return v
	}
	s.Require().NoError(saveConfig(writer(0), file))

	const saves = 100
	var done atomic.Bool
	var reads, bad atomic.Int32
	var firstBad atomic.Value
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !done.Load() {
				// The bytes rather than a viper read of them: parsing is
				// most of a read's cost, and a reader that spends its time
				// parsing is a reader that is rarely looking at the file.
				data, err := os.ReadFile(file)
				reads.Add(1)
				if err != nil || !strings.HasPrefix(string(data), "context: astronomer.io\n") || !strings.Contains(string(data), padding) {
					if bad.Add(1) == 1 {
						firstBad.Store(fmt.Sprintf("err=%v, %d bytes", err, len(data)))
					}
				}
			}
		}()
	}
	for i := 1; i <= saves; i++ {
		s.Require().NoError(saveConfig(writer(i), file))
	}
	done.Store(true)
	wg.Wait()

	s.Positive(reads.Load())
	s.Zero(bad.Load(), "of %d reads during %d saves, %d saw a config that was not whole (first: %v)",
		reads.Load(), saves, bad.Load(), firstBad.Load())
}

// A save keeps the mode the owner gave the file, and makes a new one 0600.
// Publishing through a new file would otherwise reset it to the temp file's.
func (s *Suite) TestASaveKeepsTheConfigsMode() {
	if runtime.GOOS == "windows" {
		s.T().Skip("Windows has no POSIX mode bits")
	}
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
	file := filepath.Join(s.T().TempDir(), ConfigDir, ConfigFileNameWithExt)
	v := viper.New()
	v.SetConfigType(ConfigFileType)
	v.Set("a", "b")

	s.Require().NoError(saveConfig(v, file))
	info, err := os.Stat(file)
	s.Require().NoError(err)
	s.Equal(filePerm, info.Mode().Perm())

	s.Require().NoError(os.Chmod(file, 0o640))
	s.Require().NoError(saveConfig(v, file))
	info, err = os.Stat(file)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o640), info.Mode().Perm())
}

// A config its owner made read-only is not replaced. A rename needs only the
// directory to be writable, so publishing through one would otherwise
// overwrite a file viper's in-place write refused to open.
func (s *Suite) TestASaveRefusesAReadOnlyConfig() {
	if os.Geteuid() == 0 {
		s.T().Skip("root writes a read-only file regardless")
	}
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
	file := filepath.Join(s.T().TempDir(), ConfigDir, ConfigFileNameWithExt)
	s.Require().NoError(os.MkdirAll(filepath.Dir(file), 0o700))
	s.Require().NoError(os.WriteFile(file, []byte("a: old\n"), 0o400))
	// Writable again, so TempDir can remove it on Windows.
	s.T().Cleanup(func() { _ = os.Chmod(file, 0o600) })
	v := viper.New()
	v.SetConfigType(ConfigFileType)
	v.Set("a", "new")

	err := saveConfig(v, file)
	s.Require().Error(err)
	s.ErrorIs(err, os.ErrPermission)
	raw, rerr := os.ReadFile(file)
	s.Require().NoError(rerr)
	s.Equal("a: old\n", string(raw))
}

// A config.yaml that is a symlink — a dotfile manager's — stays one, and the
// save lands in the file it points at.
func (s *Suite) TestASaveWritesThroughASymlinkedConfig() {
	if runtime.GOOS == "windows" {
		s.T().Skip("creating a symlink needs a privilege Windows runners do not grant")
	}
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
	dotfiles := filepath.Join(s.T().TempDir(), "dotfiles")
	s.Require().NoError(os.MkdirAll(dotfiles, 0o700))
	target := filepath.Join(dotfiles, "astro.yaml")
	s.Require().NoError(os.WriteFile(target, []byte("a: old\n"), 0o600))
	file := filepath.Join(s.T().TempDir(), ConfigDir, ConfigFileNameWithExt)
	s.Require().NoError(os.MkdirAll(filepath.Dir(file), 0o700))
	s.Require().NoError(os.Symlink(target, file))

	v := viper.New()
	v.SetConfigType(ConfigFileType)
	v.Set("a", "new")
	s.Require().NoError(saveConfig(v, file))

	info, err := os.Lstat(file)
	s.Require().NoError(err)
	s.NotZero(info.Mode()&os.ModeSymlink, "config.yaml must still be the link")
	got, err := os.ReadFile(target)
	s.Require().NoError(err)
	s.Equal("a: new\n", string(got))
}
