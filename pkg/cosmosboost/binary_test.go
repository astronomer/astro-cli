package cosmosboost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type BinarySuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
}

func (s *BinarySuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	config.HomeConfigPath = s.tmpDir
}

func (s *BinarySuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(""))
}

func TestBinarySuite(t *testing.T) {
	suite.Run(t, new(BinarySuite))
}

// fakeBinary is a shell script that answers `version` with the given
// version string, standing in for the real helper in the CDN archive.
func fakeBinary(version string) []byte {
	return []byte("#!/bin/sh\necho " + version + "\n")
}

// buildTarGz produces a GoReleaser-shaped archive: the binary plus a README
// at the archive root.
func buildTarGz(t *testing.T, binContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range map[string][]byte{
		"README.md": []byte("readme"),
		binaryName:  binContent,
	} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// fakeCDN serves the version file, archive, and checksums the way the
// release workflow lays them out. tamperSums serves a wrong checksum.
func (s *BinarySuite) fakeCDN(version string, archive []byte, tamperSums bool) *int {
	archiveFile := archiveName(version)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	if tamperSums {
		digest = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	sums := fmt.Sprintf("%s  %s\n", digest, archiveFile)

	count := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, version)
	})
	mux.HandleFunc("/v"+version+"/"+archiveFile, func(w http.ResponseWriter, _ *http.Request) {
		count++
		_, _ = w.Write(archive)
	})
	mux.HandleFunc(fmt.Sprintf("/v%s/%s_%s_SHA256SUMS", version, binaryName, version), func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, sums)
	})

	server := httptest.NewServer(mux)
	s.T().Cleanup(server.Close)
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(server.URL))
	return &count
}

func (s *BinarySuite) skipOnWindows() {
	if runtime.GOOS == windowsGOOS {
		s.T().Skip("shell-script fake binary does not run on windows")
	}
}

func (s *BinarySuite) TestBinDir() {
	s.Equal(filepath.Join(s.tmpDir, "bin"), BinDir())
}

func (s *BinarySuite) TestBaseURLDefault() {
	s.Equal(defaultBaseURL, BaseURL())
}

func (s *BinarySuite) TestBaseURLOverride() {
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("https://example.com/base/"))
	s.Equal("https://example.com/base", BaseURL())
}

func (s *BinarySuite) TestArchiveName() {
	name := archiveName("1.2.3")
	s.Contains(name, "astro-cosmos-boost_1.2.3_"+runtime.GOOS+"_"+runtime.GOARCH)
	if runtime.GOOS == windowsGOOS {
		s.True(len(name) > 4 && name[len(name)-4:] == ".zip")
	} else {
		s.Contains(name, ".tar.gz")
	}
}

func (s *BinarySuite) TestInstalledVersionNotInstalled() {
	s.Equal("", InstalledVersion())
}

func (s *BinarySuite) TestEnsureBinaryDownloadsVerifiesAndInstalls() {
	s.skipOnWindows()
	version := "1.0.0"
	downloads := s.fakeCDN(version, buildTarGz(s.T(), fakeBinary(version)), false)

	require.NoError(s.T(), EnsureBinary())

	s.Equal(1, *downloads)
	info, err := os.Stat(BinaryPath())
	require.NoError(s.T(), err)
	s.NotZero(info.Mode() & 0o100) // executable
	s.Equal(version, InstalledVersion())
}

func (s *BinarySuite) TestEnsureBinaryRejectsChecksumMismatch() {
	s.skipOnWindows()
	version := "1.0.0"
	s.fakeCDN(version, buildTarGz(s.T(), fakeBinary(version)), true)

	err := EnsureBinary()
	require.Error(s.T(), err)
	s.Contains(err.Error(), "checksum mismatch")
	_, statErr := os.Stat(BinaryPath())
	s.True(os.IsNotExist(statErr), "binary must not be installed on checksum mismatch")
}

func (s *BinarySuite) TestEnsureBinarySkipsWhenCurrent() {
	s.skipOnWindows()
	version := "9.9.9" // far above MinVersion
	downloads := s.fakeCDN(version, buildTarGz(s.T(), fakeBinary(version)), false)

	require.NoError(s.T(), EnsureBinary()) // installs
	require.NoError(s.T(), EnsureBinary()) // already current — no re-download
	s.Equal(1, *downloads)
}

func (s *BinarySuite) TestEnsureBinaryUpdatesBelowMinVersion() {
	s.skipOnWindows()
	// Pre-install a helper reporting a version below MinVersion.
	require.NoError(s.T(), os.MkdirAll(BinDir(), dirPerm))
	require.NoError(s.T(), os.WriteFile(BinaryPath(), fakeBinary("0.0.0"), binPerm))

	latest := "1.0.0"
	downloads := s.fakeCDN(latest, buildTarGz(s.T(), fakeBinary(latest)), false)

	require.NoError(s.T(), EnsureBinary())
	s.Equal(1, *downloads)
	s.Equal(latest, InstalledVersion())
}

// usageExitErr returns a genuine *exec.ExitError carrying the usage exit code,
// which is how an installed helper reports a call it does not understand.
func usageExitErr(t *testing.T) error {
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", usageExitCode)).Run()
	require.Error(t, err)
	return err
}

func (s *BinarySuite) TestWithUpdateRetryUpdatesHelperTooOldForTheCall() {
	s.skipOnWindows()
	// An installed helper that passes the version gate can still be too old to
	// know a call we make: version ordering cannot express which release added
	// what. Such a helper must be updated once and the call retried, rather than
	// the CLI keeping a copy of the helper's release history to consult.
	require.NoError(s.T(), os.MkdirAll(BinDir(), dirPerm))
	old := "9.9.9" // outranks MinVersion, so the gate leaves it alone
	require.NoError(s.T(), os.WriteFile(BinaryPath(), fakeBinary(old), binPerm))
	require.NoError(s.T(), EnsureBinary(), "the gate must accept it on version alone")

	latest := "0.0.1-alpha.2"
	downloads := s.fakeCDN(latest, buildTarGz(s.T(), fakeBinary(latest)), false)

	calls := 0
	err := withUpdateRetry(func() error {
		calls++
		if calls == 1 {
			return usageExitErr(s.T())
		}
		return nil
	})

	require.NoError(s.T(), err)
	s.Equal(2, calls, "the call must be retried after the update")
	s.Equal(1, *downloads, "the helper must be updated exactly once")
	s.Equal(latest, InstalledVersion())
}

func (s *BinarySuite) TestEnsureBinaryFailsWhenCDNUnreachable() {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close() // immediately unreachable
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(server.URL))

	err := EnsureBinary()
	require.Error(s.T(), err)
}

func (s *BinarySuite) TestEnsureBinaryErrorsWhenCDNServesBelowMinVersion() {
	s.skipOnWindows()
	// The CDN's latest lags below MinVersion (e.g. mid-rollback): the install
	// itself succeeds, but EnsureBinary must not report the gate as satisfied.
	old := "0.0.0"
	s.fakeCDN(old, buildTarGz(s.T(), fakeBinary(old)), false)

	err := EnsureBinary()
	require.Error(s.T(), err)
	s.Contains(err.Error(), "below minimum required version")
}
