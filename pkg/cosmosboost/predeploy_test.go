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

func (s *PreDeploySuite) TestRemoveSidecarsPreservesOtherAstroContent() {
	root := s.T().TempDir()
	// Project A: sidecar in an .astro dir that also holds user config.
	require.NoError(s.T(), os.MkdirAll(filepath.Join(root, "a", ".astro"), 0o755))
	require.NoError(s.T(), os.WriteFile(filepath.Join(root, "a", ".astro", "dbt_metadata.json"), []byte("{}"), 0o644))
	require.NoError(s.T(), os.WriteFile(filepath.Join(root, "a", ".astro", "config.yaml"), []byte("project:\n"), 0o644))
	// Project B (nested): sidecar alone — dir should be pruned.
	require.NoError(s.T(), os.MkdirAll(filepath.Join(root, "include", "b", ".astro"), 0o755))
	require.NoError(s.T(), os.WriteFile(filepath.Join(root, "include", "b", ".astro", "dbt_metadata.json"), []byte("{}"), 0o644))
	// A dbt_metadata.json OUTSIDE an .astro dir must not be touched.
	require.NoError(s.T(), os.WriteFile(filepath.Join(root, "a", "dbt_metadata.json"), []byte("user file"), 0o644))

	removed, err := RemoveSidecars(root)
	require.NoError(s.T(), err)
	s.Equal(2, removed)
	s.NoFileExists(filepath.Join(root, "a", ".astro", "dbt_metadata.json"))
	s.FileExists(filepath.Join(root, "a", ".astro", "config.yaml"), "user config under .astro must survive")
	s.NoDirExists(filepath.Join(root, "include", "b", ".astro"), "emptied .astro dir is pruned")
	s.FileExists(filepath.Join(root, "a", "dbt_metadata.json"), "non-.astro files are never touched")
}

func (s *PreDeploySuite) TestBestEffortStampRemovesStaleSidecarWhenHelperFails() {
	s.installFake("#!/bin/sh\nif [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\nexit 1\n")

	proj := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(filepath.Join(proj, "dbt_project.yml"), []byte("name: shop\n"), 0o644))
	require.NoError(s.T(), os.MkdirAll(filepath.Join(proj, ".astro"), 0o755))
	require.NoError(s.T(), os.WriteFile(filepath.Join(proj, ".astro", "dbt_metadata.json"), []byte(`{"stale": true}`), 0o644))

	BestEffortStamp(proj)

	s.NoFileExists(filepath.Join(proj, ".astro", "dbt_metadata.json"),
		"a failed stamping run must not leave the previous deploy's stale sidecar behind")
}

func (s *PreDeploySuite) TestBestEffortStampSkipsHelperWhenNoDbtContent() {
	// Unreachable CDN + no installed helper: reaching EnsureBinary would warn
	// and attempt a download. With no dbt content it must do neither.
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("http://127.0.0.1:1"))

	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(filepath.Join(dir, "some_dag.py"), []byte("# dag"), 0o644))

	BestEffortStamp(dir)

	s.NoFileExists(BinaryPath(), "no download may be attempted when there is nothing to stamp")
}
