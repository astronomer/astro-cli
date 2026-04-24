package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
)

var errStubNetwork = errors.New("simulated network failure")

type UpdateSuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
}

func (s *UpdateSuite) SetupTest() {
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	config.HomeConfigPath = s.tmpDir
	s.Require().NoError(os.MkdirAll(BinDir(), dirPerm))
}

func (s *UpdateSuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
}

func TestUpdateSuite(t *testing.T) {
	suite.Run(t, new(UpdateSuite))
}

// writeInstalled writes a package.json so InstalledVersion returns v.
func (s *UpdateSuite) writeInstalled(v string) {
	s.T().Helper()
	data, err := json.Marshal(packageInfo{Version: v})
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(BinDir(), pkgJSON), data, stateFilePerm))
}

// writeCache writes the .update-check state file.
func (s *UpdateSuite) writeCache(state updateState) {
	s.T().Helper()
	s.Require().NoError(writeUpdateState(state))
}

func (s *UpdateSuite) TestHintUpdateAvailable_NoStateFile() {
	s.writeInstalled("0.0.5")
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestHintUpdateAvailable_NoCachedVersion() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LastCheck: time.Now().UTC().Format(time.RFC3339)})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestHintUpdateAvailable_NotInstalled() {
	// No package.json — InstalledVersion returns ("", nil). No hint.
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestHintUpdateAvailable_CacheEqualToInstalled() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "0.0.5"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestHintUpdateAvailable_CacheOlderThanInstalled() {
	s.writeInstalled("0.0.7")
	s.writeCache(updateState{LatestKnown: "0.0.5"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestHintUpdateAvailable_CacheNewerThanInstalled() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Contains(buf.String(), "Otto 0.0.9 is available")
	s.Contains(buf.String(), "astro agent update")
}

func (s *UpdateSuite) TestHintUpdateAvailable_InstalledUnparseable() {
	// Unparseable installed version → treat as "upgrade suggested" to get the
	// user back to a known-good build.
	s.writeInstalled("not-a-version")
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Contains(buf.String(), "Otto 0.0.9 is available")
}

func (s *UpdateSuite) TestHintUpdateAvailable_CacheUnparseable() {
	// Unparseable cached "latest" → no hint (we can't assert newer without a
	// valid version to compare).
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "not-a-version"})
	var buf bytes.Buffer
	hintUpdateAvailable(&buf)
	s.Empty(buf.String())
}

// --- refreshUpdateCacheAsync ---
// `cdnBaseURL` is a const, so LatestVersion can't be pointed at a test server
// without a refactor. The tests here cover the throttle-skip path and the
// read/write helpers — the network-hit branch is exercised by the real-auth
// smoke path.

func (s *UpdateSuite) TestRefreshUpdateCache_SkipsWhenFresh() {
	// Write a state with a recent LastCheck. refreshUpdateCacheAsync should
	// not touch the network and should leave the state alone.
	fresh := updateState{
		LastCheck:   time.Now().UTC().Format(time.RFC3339),
		LatestKnown: "0.0.3",
	}
	s.writeCache(fresh)

	refreshUpdateCacheAsync()

	got, err := readUpdateState()
	s.NoError(err)
	s.Equal(fresh.LatestKnown, got.LatestKnown, "LatestKnown should be unchanged when cache is fresh")
	s.Equal(fresh.LastCheck, got.LastCheck, "LastCheck should be unchanged when cache is fresh")
}

// Round-trip via the read/write helpers so the on-disk shape stays stable.
func (s *UpdateSuite) TestUpdateStateFile_RoundTrip() {
	want := updateState{
		LastCheck:   "2026-04-01T12:00:00Z",
		LatestKnown: "0.0.9",
	}
	s.Require().NoError(writeUpdateState(want))

	got, err := readUpdateState()
	s.NoError(err)
	s.Equal(want, got)
}

// Confirm the state file is written at the expected path relative to BinDir.
func (s *UpdateSuite) TestUpdateStateFile_PathIsBinDirSibling() {
	s.Require().NoError(writeUpdateState(updateState{LatestKnown: "0.0.9"}))

	expected := filepath.Join(BinDir(), updateStateFile)
	_, err := os.Stat(expected)
	s.NoError(err, "state file should live next to the binary")
}

// --- autoUpdate ---
// Exercises the download-when-newer-is-cached path with a stubbed downloader.
// The gating on `agent.auto_update` lives in Start() and is covered by the
// real-auth smoke path.

// stubDownloader returns a downloader that records whether it was called and
// returns a fixed error (nil for success). The install side-effect is
// simulated by rewriting package.json to the given version.
func (s *UpdateSuite) stubDownloader(installAs string, returnErr error) (download func() error, called *bool) {
	flag := false
	return func() error {
		flag = true
		if returnErr != nil {
			return returnErr
		}
		s.writeInstalled(installAs)
		return nil
	}, &flag
}

func (s *UpdateSuite) TestAutoUpdate_NewerCached_DownloadsAndReportsProgress() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	download, called := s.stubDownloader("0.0.9", nil)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.True(*called, "downloader should fire when cache is newer")
	s.Contains(buf.String(), "Updating Otto 0.0.5 → 0.0.9")
}

func (s *UpdateSuite) TestAutoUpdate_CacheEqualToInstalled_SkipsDownload() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "0.0.5"})
	download, called := s.stubDownloader("0.0.5", nil)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.False(*called, "downloader should not fire when versions match")
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestAutoUpdate_CacheOlderThanInstalled_SkipsDownload() {
	s.writeInstalled("0.0.7")
	s.writeCache(updateState{LatestKnown: "0.0.5"})
	download, called := s.stubDownloader("0.0.5", nil)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.False(*called, "downloader should not fire when cache is older")
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestAutoUpdate_NoInstalled_SkipsDownload() {
	// First-run case — EnsureBinary is responsible for the initial install.
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	download, called := s.stubDownloader("0.0.9", nil)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.False(*called, "auto-update should not handle first-run install")
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestAutoUpdate_NoCachedVersion_SkipsDownload() {
	s.writeInstalled("0.0.5")
	// cache has LastCheck but no LatestKnown (old schema)
	s.writeCache(updateState{LastCheck: time.Now().UTC().Format(time.RFC3339)})
	download, called := s.stubDownloader("0.0.9", nil)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.False(*called, "auto-update should wait for the async refresh to populate the cache")
	s.Empty(buf.String())
}

func (s *UpdateSuite) TestAutoUpdate_DownloadFails_SoftFallsBack() {
	s.writeInstalled("0.0.5")
	s.writeCache(updateState{LatestKnown: "0.0.9"})
	download, called := s.stubDownloader("", errStubNetwork)

	var buf bytes.Buffer
	autoUpdate(&buf, download)

	s.True(*called)
	s.Contains(buf.String(), "Updating Otto 0.0.5 → 0.0.9")
	s.Contains(buf.String(), "Otto update failed")
	s.Contains(buf.String(), "continuing with 0.0.5")
}
