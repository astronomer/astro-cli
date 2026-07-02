package cosmosboost

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type PrecomputeSuite struct {
	suite.Suite
	origHomeConfigPath string
}

func (s *PrecomputeSuite) SetupTest() {
	if runtime.GOOS == windowsGOOS {
		s.T().Skip("shell-script fake binary does not run on windows")
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.origHomeConfigPath = config.HomeConfigPath
	config.HomeConfigPath = s.T().TempDir()
}

func (s *PrecomputeSuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(""))
}

func TestPrecomputeSuite(t *testing.T) {
	suite.Run(t, new(PrecomputeSuite))
}

// installFake places a shell script at BinaryPath standing in for the helper.
func (s *PrecomputeSuite) installFake(script string) {
	require.NoError(s.T(), os.MkdirAll(BinDir(), dirPerm))
	require.NoError(s.T(), os.WriteFile(BinaryPath(), []byte(script), binPerm))
}

func (s *PrecomputeSuite) TestPrecomputeRunsHelperOnPath() {
	marker := filepath.Join(s.T().TempDir(), "ran")
	// The fake records its args so we can assert the subcommand and path.
	s.installFake("#!/bin/sh\necho \"$@\" > " + marker + "\n")

	require.NoError(s.T(), Precompute("/some/dbt/project"))

	content, err := os.ReadFile(marker)
	require.NoError(s.T(), err)
	s.Contains(string(content), "precompute /some/dbt/project")
}

func (s *PrecomputeSuite) TestPrecomputeSurfacesFailureOutput() {
	s.installFake("#!/bin/sh\necho boom >&2\nexit 3\n")

	err := Precompute("/some/dbt/project")
	require.Error(s.T(), err)
	s.Contains(err.Error(), "boom")
}

func (s *PrecomputeSuite) TestBestEffortStampNeverPanicsWhenHelperUnavailable() {
	// No binary installed and an unreachable CDN: must warn, not fail.
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("http://127.0.0.1:1"))
	s.NotPanics(func() { BestEffortStamp("/some/dbt/project") })
	_, statErr := os.Stat(BinaryPath())
	s.True(os.IsNotExist(statErr))
}

func (s *PrecomputeSuite) TestBestEffortStampSwallowsHelperFailure() {
	s.installFake("#!/bin/sh\nif [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\nexit 1\n")
	s.NotPanics(func() { BestEffortStamp("/some/dbt/project") })
}
