package cosmosboost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type UninstallSuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
	argsLog            string
}

func (s *UninstallSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	config.HomeConfigPath = s.tmpDir
	s.argsLog = filepath.Join(s.tmpDir, "helper-args.log")
}

func (s *UninstallSuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(""))
}

func TestUninstallSuite(t *testing.T) {
	suite.Run(t, new(UninstallSuite))
}

func (s *UninstallSuite) skipOnWindows() {
	if runtime.GOOS == windowsGOOS {
		s.T().Skip("shell-script fake helper does not run on windows")
	}
}

// fakeHelper is a shell script standing in for the real binary: it answers
// `version`, logs any other invocation, and exits with the given code.
func (s *UninstallSuite) fakeHelper(version string, uninstallExit int) []byte {
	return fmt.Appendf(nil, "#!/bin/sh\nif [ \"$1\" = version ]; then echo %s; exit 0; fi\necho \"$@\" >> %s\nexit %d\n",
		version, s.argsLog, uninstallExit)
}

func (s *UninstallSuite) installHelper(content []byte) {
	require.NoError(s.T(), os.MkdirAll(BinDir(), 0o755))
	require.NoError(s.T(), os.WriteFile(BinaryPath(), content, 0o755))
}

// serveHelper stands up a fake CDN serving content as the latest release.
func (s *UninstallSuite) serveHelper(version string, content []byte) {
	archive := buildTarGz(s.T(), content)
	archiveFile := archiveName(version)
	sum := sha256.Sum256(archive)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveFile)

	mux := http.NewServeMux()
	mux.HandleFunc("/latest/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, version)
	})
	mux.HandleFunc("/v"+version+"/"+archiveFile, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc(fmt.Sprintf("/v%s/%s_%s_SHA256SUMS", version, binaryName, version), func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, sums)
	})

	server := httptest.NewServer(mux)
	s.T().Cleanup(server.Close)
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(server.URL))
}

func (s *UninstallSuite) loggedArgs() string {
	logged, err := os.ReadFile(s.argsLog)
	require.NoError(s.T(), err)
	return string(logged)
}

func (s *UninstallSuite) TestUninstallDelegatesToHelper() {
	s.skipOnWindows()
	s.installHelper(s.fakeHelper(MinVersion, 0))
	root := s.T().TempDir()

	require.NoError(s.T(), Uninstall(root))

	s.Contains(s.loggedArgs(), "uninstall "+root)
	_, err := os.Stat(BinaryPath())
	s.True(os.IsNotExist(err), "binary should be removed")
}

func (s *UninstallSuite) TestUninstallUpdatesOldHelper() {
	s.skipOnWindows()
	s.installHelper(s.fakeHelper(MinVersion, usageExitCode))
	s.serveHelper("9.9.9", s.fakeHelper("9.9.9", 0))
	root := s.T().TempDir()

	require.NoError(s.T(), Uninstall(root))

	s.Contains(s.loggedArgs(), "uninstall "+root)
	_, err := os.Stat(BinaryPath())
	s.True(os.IsNotExist(err), "binary should be removed")
}

func (s *UninstallSuite) TestUninstallDownloadsWhenMissing() {
	s.skipOnWindows()
	s.serveHelper("9.9.9", s.fakeHelper("9.9.9", 0))
	root := s.T().TempDir()

	require.NoError(s.T(), Uninstall(root))

	s.Contains(s.loggedArgs(), "uninstall "+root)
	_, err := os.Stat(BinaryPath())
	s.True(os.IsNotExist(err), "binary should be removed")
}

func (s *UninstallSuite) TestUninstallErrorsWhenHelperUnavailable() {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(server.URL))

	err := Uninstall(s.T().TempDir())
	s.ErrorContains(err, "astro-cosmos-boost")
}

func (s *UninstallSuite) TestUninstallFailureKeepsBinary() {
	s.skipOnWindows()
	s.installHelper(s.fakeHelper(MinVersion, 1))

	err := Uninstall(s.T().TempDir())

	s.ErrorContains(err, "uninstall")
	_, statErr := os.Stat(BinaryPath())
	s.NoError(statErr, "binary should remain for a retry")
}

func (s *UninstallSuite) TestUninstallRemovesPartialExtracts() {
	s.skipOnWindows()
	s.installHelper(s.fakeHelper(MinVersion, 0))
	leftover := filepath.Join(BinDir(), ".cosmosboost-12345")
	require.NoError(s.T(), os.WriteFile(leftover, []byte("partial"), 0o644))

	require.NoError(s.T(), Uninstall(s.T().TempDir()))

	_, err := os.Stat(leftover)
	s.True(os.IsNotExist(err))
}

func (s *UninstallSuite) TestUninstallDefaultRoot() {
	s.skipOnWindows()
	s.installHelper(s.fakeHelper(MinVersion, 0))
	dir := s.T().TempDir()
	s.T().Chdir(dir)

	require.NoError(s.T(), Uninstall())

	s.Contains(s.loggedArgs(), "uninstall .")
}
