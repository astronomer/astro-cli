package cosmosboost

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// installRecordingFake installs a fake helper that answers `version` and
// appends every invocation to the returned log path, so a test can assert which
// subcommands the CLI ran, and in which order.
func (s *PreDeploySuite) installRecordingFake() string {
	log := filepath.Join(s.T().TempDir(), "invocations.log")
	s.installFake("#!/bin/sh\n" +
		"if [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\n" +
		"echo \"$@\" >> " + log + "\n")
	return log
}

func (s *PreDeploySuite) invocations(log string) string {
	content, err := os.ReadFile(log)
	require.NoError(s.T(), err)
	return string(content)
}

func (s *PreDeploySuite) TestPreDeployRunsHelperOnPath() {
	log := s.installRecordingFake()

	require.NoError(s.T(), PreDeploy("/some/dbt/project"))

	s.Contains(s.invocations(log), "pre-deploy /some/dbt/project")
}

// The helper's output can name the files it manages, so it must never reach a
// user-visible message. Callers print these errors as warnings, so the detail
// belongs behind --verbosity debug and nowhere else.
func (s *PreDeploySuite) TestPreDeployKeepsHelperOutputOutOfTheError() {
	s.installFake("#!/bin/sh\necho 'removed /proj/.astro/secret_artifact.json' >&2\nexit 3\n")

	err := PreDeploy("/some/dbt/project")

	require.Error(s.T(), err)
	s.NotContains(err.Error(), "secret_artifact.json", "helper output must not leak into the error")
	s.Contains(err.Error(), "exit status 3", "the failure itself must still be reported")
	s.Contains(err.Error(), "--verbosity debug", "point the user at where the detail lives")
}

func (s *PreDeploySuite) TestUninstallKeepsHelperOutputOutOfTheError() {
	s.installFake("#!/bin/sh\necho 'kept /proj/.astro/secret_artifact.json' >&2\nexit 3\n")

	err := execUninstall([]string{"/some/dbt/project"})

	require.Error(s.T(), err)
	s.NotContains(err.Error(), "secret_artifact.json", "helper output must not leak into the error")
	s.Contains(err.Error(), "exit status 3")
}

func (s *PreDeploySuite) TestBestEffortPreDeployNeverPanicsWhenHelperUnavailable() {
	// No binary installed and an unreachable CDN: must warn, not fail.
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("http://127.0.0.1:1"))
	s.NotPanics(func() { BestEffortPreDeploy("/some/dbt/project") })
	_, statErr := os.Stat(BinaryPath())
	s.True(os.IsNotExist(statErr))
}

func (s *PreDeploySuite) TestBestEffortPreDeploySwallowsHelperFailure() {
	s.installFake("#!/bin/sh\nif [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\nexit 1\n")
	s.NotPanics(func() { BestEffortPreDeploy("/some/dbt/project") })
}

// Cleanup is entirely the helper's job: the CLI knows neither what the helper
// leaves behind nor where, so it can only delegate.
func (s *PreDeploySuite) TestBestEffortCleanupDelegatesToHelper() {
	log := s.installRecordingFake()

	BestEffortCleanup("/some/dbt/project")

	s.Contains(s.invocations(log), "uninstall /some/dbt/project")
}

func (s *PreDeploySuite) TestBestEffortCleanupSkipsWhenHelperNotInstalled() {
	// With the feature disabled we must not pull the helper onto the machine
	// just to clean up: an unreachable CDN would otherwise surface as an error.
	require.NoError(s.T(), config.CFG.CosmosBoostBaseURL.SetHomeString("http://127.0.0.1:1"))

	s.NotPanics(func() { BestEffortCleanup(s.T().TempDir()) })

	s.NoFileExists(BinaryPath(), "a disabled feature may never trigger a download")
}

func (s *PreDeploySuite) TestBestEffortCleanupSwallowsHelperFailure() {
	s.installFake("#!/bin/sh\nif [ \"$1\" = version ]; then echo 9.9.9; exit 0; fi\nexit 1\n")
	s.NotPanics(func() { BestEffortCleanup("/some/dbt/project") })
}

func (s *PreDeploySuite) TestBestEffortPreDeployCleansUpBeforeRunning() {
	log := s.installRecordingFake()

	BestEffortPreDeploy("/some/dbt/project")

	// Order matters: a failed pre-deploy step must not leave an earlier
	// deploy's artifacts in place, so cleanup runs first.
	ran := s.invocations(log)
	s.Less(strings.Index(ran, "uninstall"), strings.Index(ran, "pre-deploy"),
		"cleanup must run before the pre-deploy step")
}
