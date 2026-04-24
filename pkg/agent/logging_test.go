package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LoggingSuite struct {
	suite.Suite
}

func TestLoggingSuite(t *testing.T) {
	suite.Run(t, new(LoggingSuite))
}

func (s *LoggingSuite) TestRotateIfLarge_UnderThreshold() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "agent.log")
	require.NoError(s.T(), os.WriteFile(path, []byte("small"), logFilePerm))

	rotateIfLarge(path, 1024)

	_, err := os.Stat(path)
	s.NoError(err)
	_, err = os.Stat(path + ".old")
	s.True(os.IsNotExist(err), ".old should not exist below threshold")
}

func (s *LoggingSuite) TestRotateIfLarge_OverThreshold() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "agent.log")
	big := []byte(strings.Repeat("x", 2048))
	require.NoError(s.T(), os.WriteFile(path, big, logFilePerm))

	rotateIfLarge(path, 1024)

	_, err := os.Stat(path)
	s.True(os.IsNotExist(err), "original should be renamed away")
	got, err := os.ReadFile(path + ".old")
	s.NoError(err)
	s.Equal(big, got, ".old should hold the prior contents")
}

func (s *LoggingSuite) TestRotateIfLarge_Missing() {
	// No file at path — nothing to do, no error.
	rotateIfLarge(filepath.Join(s.T().TempDir(), "missing.log"), 1024)
}
