package otto

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
)

type BinarySuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
}

func (s *BinarySuite) SetupTest() {
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	config.HomeConfigPath = s.tmpDir
}

func (s *BinarySuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
}

func TestBinarySuite(t *testing.T) {
	suite.Run(t, new(BinarySuite))
}

func (s *BinarySuite) TestBinDir() {
	s.Equal(filepath.Join(s.tmpDir, "bin"), BinDir())
}

func (s *BinarySuite) TestBinaryPath() {
	s.Equal(filepath.Join(s.tmpDir, "bin", "otto"), BinaryPath())
}

func (s *BinarySuite) TestInstalledVersion_NotInstalled() {
	v, err := InstalledVersion()
	s.NoError(err)
	s.Equal("", v)
}

func (s *BinarySuite) TestInstalledVersion_WithVersion() {
	binDir := BinDir()
	require.NoError(s.T(), os.MkdirAll(binDir, 0o755))

	info := map[string]any{
		"piConfig": map[string]string{"name": "otto", "configDir": ".astro/otto"},
		"version":  "0.0.3",
	}
	data, _ := json.Marshal(info)
	require.NoError(s.T(), os.WriteFile(filepath.Join(binDir, pkgJSON), data, 0o644))

	v, err := InstalledVersion()
	s.NoError(err)
	s.Equal("0.0.3", v)
}

func (s *BinarySuite) TestInstalledVersion_NoVersionField() {
	binDir := BinDir()
	require.NoError(s.T(), os.MkdirAll(binDir, 0o755))

	info := map[string]any{
		"piConfig": map[string]string{"name": "otto"},
	}
	data, _ := json.Marshal(info)
	require.NoError(s.T(), os.WriteFile(filepath.Join(binDir, pkgJSON), data, 0o644))

	v, err := InstalledVersion()
	s.NoError(err)
	s.Equal("", v)
}

func (s *BinarySuite) TestInstalledVersion_InvalidJSON() {
	binDir := BinDir()
	require.NoError(s.T(), os.MkdirAll(binDir, 0o755))
	require.NoError(s.T(), os.WriteFile(filepath.Join(binDir, pkgJSON), []byte("not json"), 0o644))

	_, err := InstalledVersion()
	s.Error(err)
	s.Contains(err.Error(), "parsing otto metadata")
}

func (s *BinarySuite) TestDownloadURL() {
	url := downloadURL()
	s.Contains(url, cdnBaseURL)
	s.Contains(url, "/latest/")
	s.Contains(url, "otto-")
	s.Contains(url, ".tar.gz")
}

func (s *BinarySuite) TestIsUpdateAvailable_NotInstalled() {
	// With no package.json, IsUpdateAvailable returns (false, "", nil) without
	// contacting the CDN.
	available, latest, err := IsUpdateAvailable()
	s.NoError(err)
	s.False(available)
	s.Empty(latest)
}

func (s *BinarySuite) TestEnsureBinary_AlreadyInstalled() {
	binDir := BinDir()
	require.NoError(s.T(), os.MkdirAll(binDir, 0o755))

	// Write a package.json with a version >= MinVersion
	data, _ := json.Marshal(packageInfo{Version: MinVersion})
	require.NoError(s.T(), os.WriteFile(filepath.Join(binDir, pkgJSON), data, 0o644))

	// EnsureBinary should succeed without downloading
	err := EnsureBinary()
	s.NoError(err)
}

func (s *BinarySuite) TestDownloadAndInstall_FromMockServer() {
	// Create a mock CDN that serves a tar.gz
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		writeMockTarGz(s.T(), w, "0.0.5")
	}))
	defer server.Close()

	binDir := BinDir()
	require.NoError(s.T(), os.MkdirAll(binDir, 0o755))

	// Download and extract
	// Override the URL by calling the internal HTTP client directly
	resp, err := http.Get(server.URL + "/otto-test.tar.gz")
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	gz, err := gzip.NewReader(resp.Body)
	require.NoError(s.T(), err)
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		target := filepath.Join(binDir, filepath.Clean(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			f, _ := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			_, _ = f.ReadFrom(tr)
			f.Close()
		}
	}

	// Verify extracted files
	s.FileExists(filepath.Join(binDir, pkgJSON))

	// Verify version
	v, err := InstalledVersion()
	s.NoError(err)
	s.Equal("0.0.5", v)
}

// Regression: `astro otto update` used to be a no-op when an older `otto`
// already existed — the rename only fired if `otto` was missing, so the
// freshly-extracted `otto-<os>-<arch>` sat unused next to the old binary.
func (s *BinarySuite) TestRenamePlatformBinary_OverwritesExisting() {
	binDir := BinDir()
	s.Require().NoError(os.MkdirAll(binDir, dirPerm))

	binPath := filepath.Join(binDir, "otto")
	platform := filepath.Join(binDir, "otto-"+runtime.GOOS+"-"+runtime.GOARCH)
	s.Require().NoError(os.WriteFile(binPath, []byte("old"), binPerm))
	s.Require().NoError(os.WriteFile(platform, []byte("new"), binPerm))

	s.Require().NoError(renamePlatformBinary(binDir))

	got, err := os.ReadFile(binPath)
	s.NoError(err)
	s.Equal("new", string(got), "otto should be replaced with the freshly-extracted binary")
	_, err = os.Stat(platform)
	s.True(os.IsNotExist(err), "platform-suffixed binary should be consumed by the rename")
}

// writeMockTarGz writes a minimal tar.gz to w with a package.json and a fake binary.
func writeMockTarGz(t *testing.T, w http.ResponseWriter, version string) {
	t.Helper()
	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// package.json
	pkgData, _ := json.Marshal(map[string]any{
		"piConfig": map[string]string{"name": "otto", "configDir": ".astro/otto"},
		"version":  version,
	})
	assert.NoError(t, tw.WriteHeader(&tar.Header{
		Name: pkgJSON,
		Mode: 0o644,
		Size: int64(len(pkgData)),
	}))
	_, err := tw.Write(pkgData)
	assert.NoError(t, err)

	// Fake binary
	binData := []byte("#!/bin/sh\necho otto " + version)
	assert.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "otto",
		Mode: 0o755,
		Size: int64(len(binData)),
	}))
	_, err = tw.Write(binData)
	assert.NoError(t, err)
}
