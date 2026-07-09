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

type PreDeploySuite struct {
	suite.Suite
	origHomeConfigPath string
}

func (s *PreDeploySuite) SetupTest() {
	if runtime.GOOS == windowsGOOS {
		s.T().Skip("shell-script fake binary does not run on windows")
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.origHomeConfigPath = config.HomeConfigPath
	config.HomeConfigPath = s.T().TempDir()
}

func (s *PreDeploySuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString(""))
}

func TestPreDeploySuite(t *testing.T) {
	suite.Run(t, new(PreDeploySuite))
}

// installFake places a shell script at BinaryPath standing in for the helper.
func (s *PreDeploySuite) installFake(script string) {
	require.NoError(s.T(), os.MkdirAll(BinDir(), dirPerm))
	require.NoError(s.T(), os.WriteFile(BinaryPath(), []byte(script), binPerm))
}

func (s *PreDeploySuite) TestPreDeployRunsHelperOnPath() {
	marker := filepath.Join(s.T().TempDir(), "ran")
	// The fake records its args so we can assert the subcommand and path.
	s.installFake("#!/bin/sh\necho \"$@\" > " + marker + "\n")

	require.NoError(s.T(), PreDeploy("/some/dbt/project"))

	content, err := os.ReadFile(marker)
	require.NoError(s.T(), err)
	s.Contains(string(content), "pre-deploy /some/dbt/project")
}

func (s *PreDeploySuite) TestPreDeploySurfacesFailureOutput() {
	s.installFake("#!/bin/sh\necho boom >&2\nexit 3\n")

	err := PreDeploy("/some/dbt/project")
	require.Error(s.T(), err)
	s.Contains(err.Error(), "boom")
}

func (s *PreDeploySuite) TestBestEffortStampNeverPanicsWhenHelperUnavailable() {
	// No binary installed and an unreachable CDN: must warn, not fail.
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("http://127.0.0.1:1"))
	s.NotPanics(func() { BestEffortStamp("/some/dbt/project") })
	_, statErr := os.Stat(BinaryPath())
	s.True(os.IsNotExist(statErr))
}

func (s *PreDeploySuite) TestBestEffortStampSwallowsHelperFailure() {
	s.installFake("#!/bin/sh\nif [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\nexit 1\n")
	s.NotPanics(func() { BestEffortStamp("/some/dbt/project") })
}
