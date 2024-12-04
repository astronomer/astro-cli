package runtimes

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockDockerEngine struct {
	mock.Mock
}

func (d *MockDockerEngine) IsRunning() (string, error) {
	args := d.Called()
	return args.String(0), args.Error(1)
}

func (d *MockDockerEngine) Start() (string, error) {
	args := d.Called()
	return args.String(0), args.Error(1)
}

func (s *ContainerRuntimeSuite) TestStartDocker() {
	s.Run("Docker is running, returns nil", func() {
		// Create mock initializer.
		mockDockerEngine := new(MockDockerEngine)
		// Simulate that the initial `docker ps` succeeds and we exit early.
		mockDockerEngine.On("IsRunning").Return("", nil).Once()
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine)
		// Run our test and assert expectations.
		err := rt.InitializeDocker(defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker is running")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker is not running, tries to start and waits", func() {
		// Create mock initializer.
		mockDockerEngine := new(MockDockerEngine)
		// Simulate that the initial `docker ps` fails.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate that `open -a docker` succeeds.
		mockDockerEngine.On("Start").Return("", nil).Once()
		// Simulate that `docker ps` works after trying to open docker.
		mockDockerEngine.On("IsRunning").Return("", nil).Once()
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine)
		// Run our test and assert expectations.
		err := rt.InitializeDocker(defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker starts after retry")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker fails to open", func() {
		// Create mock initializer.
		mockDockerEngine := new(MockDockerEngine)
		// Simulate `docker ps` failing.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate `open -a docker` failing.
		mockDockerEngine.On("Start").Return("", fmt.Errorf("failed to open docker")).Once()
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine)
		// Run our test and assert expectations.
		err := rt.InitializeDocker(defaultTimeoutSeconds)
		assert.Equal(s.T(), fmt.Errorf(dockerOpenNotice), err, "Expected timeout error")
		mockDockerEngine.AssertExpectations(s.T())
	})

	s.Run("Docker open succeeds but check times out", func() {
		// Create mock initializer.
		mockDockerEngine := new(MockDockerEngine)
		// Simulate `docker ps` failing continuously.
		mockDockerEngine.On("IsRunning").Return("", fmt.Errorf("docker not running"))
		// Simulate `open -a docker` failing.
		mockDockerEngine.On("Start").Return("", nil).Once()
		// Create the runtime with our mock engine.
		rt := CreateDockerRuntime(mockDockerEngine)
		// Run our test and assert expectations.
		// Simulate the timeout after 1 second.
		err := rt.InitializeDocker(1)
		assert.Equal(s.T(), fmt.Errorf(timeoutErrMsg), err, "Expected timeout error")
		mockDockerEngine.AssertExpectations(s.T())
	})
}
