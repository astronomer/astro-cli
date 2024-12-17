package runtimes

import (
	"fmt"
	"testing"

	"github.com/astronomer/astro-cli/airflow/runtimes/mocks"
	"github.com/stretchr/testify/suite"

	"github.com/stretchr/testify/assert"
)

var (
	mockDockerEngine    *mocks.DockerEngine
	mockDockerOSChecker *mocks.OSChecker
)

type DockerRuntimeSuite struct {
	suite.Suite
}

func TestDockerRuntime(t *testing.T) {
	suite.Run(t, new(DockerRuntimeSuite))
}

func (s *DockerRuntimeSuite) SetupTest() {
	// Reset some variables to defaults.
	mockDockerEngine = new(mocks.DockerEngine)
	mockDockerOSChecker = new(mocks.OSChecker)
}

func (s *DockerRuntimeSuite) TestStartDocker() {
	s.Run("Docker is running, returns nil", func() {
		// Simulate that the initial `docker ps` succeeds and we exit early.
		mockDockerEngine.On("IsRunning").Return("", nil).Once()
		mockDockerOSChecker.On("IsMac").Return(true)
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine, mockDockerOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err, "Expected no error when docker is running")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker is not running, tries to start and waits", func() {
		// Simulate that the initial `docker ps` fails.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate that `open -a docker` succeeds.
		mockDockerEngine.On("Start").Return("", nil).Once()
		// Simulate that `docker ps` works after trying to open docker.
		mockDockerEngine.On("IsRunning").Return("", nil).Once()
		mockDockerOSChecker.On("IsMac").Return(true)
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine, mockDockerOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err, "Expected no error when docker starts after retry")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker fails to open", func() {
		// Simulate `docker ps` failing.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate `open -a docker` failing.
		mockDockerEngine.On("Start").Return("", fmt.Errorf("failed to open docker")).Once()
		mockDockerOSChecker.On("IsMac").Return(true)
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine, mockDockerOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Equal(s.T(), dockerOpenNotice, err.Error(), "Expected timeout error")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker open succeeds but check times out", func() {
		// Simulate `docker ps` failing continuously.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running"))
		// Simulate `open -a docker` failing.
		mockDockerEngine.On("Start").Return("", nil).Once()
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine, mockDockerOSChecker)
		// Run our test and assert expectations.
		// Call the helper method directly with custom timeout.
		// Simulate the timeout after 1 second.
		err := rt.initializeDocker(1)
		assert.Equal(s.T(), timeoutErrMsg, err.Error(), "Expected timeout error")
		mockDockerEngine.AssertExpectations(s.T())
	})
}
