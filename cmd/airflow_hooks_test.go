package cmd

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
)

type AirflowHooksSuite struct {
	suite.Suite
	tempDir string
}

func TestAirflowHooks(t *testing.T) {
	suite.Run(t, new(AirflowHooksSuite))
}

func (s *AirflowHooksSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir, err := os.MkdirTemp("", "test_hooks_temp_dir_*")
	if err != nil {
		s.T().Fatalf("failed to create temp dir: %v", err)
	}
	s.tempDir = dir
	config.WorkingPath = s.tempDir
}

func (s *AirflowHooksSuite) SetupSubTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir, err := os.MkdirTemp("", "test_hooks_temp_dir_*")
	if err != nil {
		s.T().Fatalf("failed to create temp dir: %v", err)
	}
	s.tempDir = dir
	config.WorkingPath = s.tempDir
}

func (s *AirflowHooksSuite) TearDownTest() {
	os.RemoveAll(s.tempDir)
}

func (s *AirflowHooksSuite) TearDownSubTest() {
	os.RemoveAll(s.tempDir)
}

var (
	_ suite.SetupSubTest    = (*AirflowHooksSuite)(nil)
	_ suite.TearDownSubTest = (*AirflowHooksSuite)(nil)
)

func (s *AirflowHooksSuite) TestEnsureProjectDirOrInit_AlreadyProjectDir() {
	s.Run("returns nil when already an Astro project directory", func() {
		// Create .astro/config.yaml to mark this as a valid Astro project
		astroDir := s.tempDir + "/.astro"
		s.NoError(os.MkdirAll(astroDir, 0o755))
		configFile, err := os.Create(astroDir + "/config.yaml")
		s.NoError(err)
		configFile.Close()

		err = EnsureProjectDirOrInit(&cobra.Command{}, []string{})
		s.NoError(err)
	})
}

func (s *AirflowHooksSuite) TestEnsureProjectDirOrInit_NotProjectDir_UserConfirms() {
	s.Run("initializes project when user confirms", func() {
		// tempDir is not an Astro project dir; user inputs "y" to confirm init
		defer testUtil.MockUserInput(s.T(), "y\n")()

		err := EnsureProjectDirOrInit(&cobra.Command{}, []string{})
		s.NoError(err)

		// Verify the project was initialized (Dockerfile should exist)
		_, statErr := os.Stat(s.tempDir + "/Dockerfile")
		s.NoError(statErr, "Dockerfile should have been created by init")
	})
}

func (s *AirflowHooksSuite) TestEnsureProjectDirOrInit_NotProjectDir_UserDeclines() {
	s.Run("returns error when user declines initialization", func() {
		// tempDir is not an Astro project dir; user inputs "n" to decline
		defer testUtil.MockUserInput(s.T(), "n\n")()

		err := EnsureProjectDirOrInit(&cobra.Command{}, []string{})
		s.Error(err)
		s.Contains(err.Error(), "this is not an Astro project directory")
	})
}

func (s *AirflowHooksSuite) TestEnsureProjectDirOrInit_InvalidPath() {
	s.Run("returns error when path is not resolvable", func() {
		config.WorkingPath = "./\000x"

		err := EnsureProjectDirOrInit(&cobra.Command{}, []string{})
		s.Error(err)
		s.Contains(err.Error(), "failed to verify that your working directory is an Astro project")
	})
}
