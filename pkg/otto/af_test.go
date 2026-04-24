package otto

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
)

type AfSuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
}

func (s *AfSuite) SetupTest() {
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	config.HomeConfigPath = s.tmpDir
}

func (s *AfSuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
}

func TestAfSuite(t *testing.T) {
	suite.Run(t, new(AfSuite))
}

func (s *AfSuite) TestAfWrapperPath_Location() {
	// Wrapper always lives next to the Otto binary in ~/.astro/bin.
	expectedDir := filepath.Join(s.tmpDir, "bin")
	s.Equal(expectedDir, filepath.Dir(AfWrapperPath()))

	name := filepath.Base(AfWrapperPath())
	if runtime.GOOS == "windows" {
		s.Equal("af.cmd", name)
	} else {
		s.Equal("af", name)
	}
}

func (s *AfSuite) TestEnsureAfWrapper_CreatesFileWhenMissing() {
	s.Require().NoError(EnsureAfWrapper())

	info, err := os.Stat(AfWrapperPath())
	s.Require().NoError(err)
	s.False(info.IsDir())

	contents, err := os.ReadFile(AfWrapperPath())
	s.Require().NoError(err)
	// The wrapper must reference astro-airflow-mcp — if we ever swap the backend,
	// this test forces a conscious update rather than letting a typo slip.
	s.Contains(string(contents), "astro-airflow-mcp")
	s.Contains(string(contents), "af")
}

func (s *AfSuite) TestEnsureAfWrapper_IsIdempotent() {
	// Two consecutive calls should leave the file with identical mtime, proving
	// we skip the write when content already matches.
	s.Require().NoError(EnsureAfWrapper())
	firstInfo, err := os.Stat(AfWrapperPath())
	s.Require().NoError(err)

	// Touch mtime backwards so we can detect any rewrite unambiguously.
	past := firstInfo.ModTime().Add(-time.Hour)
	s.Require().NoError(os.Chtimes(AfWrapperPath(), past, past))

	s.Require().NoError(EnsureAfWrapper())
	secondInfo, err := os.Stat(AfWrapperPath())
	s.Require().NoError(err)
	s.Equal(past.Unix(), secondInfo.ModTime().Unix(), "idempotent call should not rewrite the file")
}

func (s *AfSuite) TestEnsureAfWrapper_RewritesWhenContentChanges() {
	s.Require().NoError(os.MkdirAll(filepath.Dir(AfWrapperPath()), 0o755))
	s.Require().NoError(os.WriteFile(AfWrapperPath(), []byte("#!/bin/sh\necho old-content\n"), 0o755))

	s.Require().NoError(EnsureAfWrapper())

	contents, err := os.ReadFile(AfWrapperPath())
	s.Require().NoError(err)
	s.Contains(string(contents), "astro-airflow-mcp", "stale wrapper should be overwritten")
}

func (s *AfSuite) TestBuildEnv_PrependsBinDirToPath() {
	// Simulate a PATH that does NOT contain ~/.astro/bin. BuildEnv must prepend
	// it so Otto's bash tool can resolve the `af` wrapper we install.
	s.Require().NoError(os.Setenv("PATH", "/usr/bin:/bin"))
	defer os.Unsetenv("PATH")

	cfg := &Config{}
	env := cfg.BuildEnv()

	var pathValue string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathValue = strings.TrimPrefix(e, "PATH=")
			break
		}
	}
	s.Require().NotEmpty(pathValue, "PATH entry missing from BuildEnv output")

	expectedPrefix := BinDir() + string(os.PathListSeparator)
	s.True(strings.HasPrefix(pathValue, expectedPrefix),
		"expected PATH to start with %q, got %q", expectedPrefix, pathValue)
	s.Contains(pathValue, "/usr/bin:/bin", "existing PATH entries should be preserved")
}
