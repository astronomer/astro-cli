package runtimes

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockDockerInitializer struct {
	mock.Mock
}

func (d *MockDockerInitializer) CheckDockerCmd() (string, error) {
	args := d.Called()
	return args.String(0), args.Error(1)
}

func (d *MockDockerInitializer) OpenDockerCmd() (string, error) {
	args := d.Called()
	return args.String(0), args.Error(1)
}

func (s *ContainerRuntimeSuite) TestStartDocker() {
	s.Run("Docker is running, returns nil", func() {
		// Create mock initializer.
		mockInitializer := new(MockDockerInitializer)
		// Simulate that the initial `docker ps` succeeds and we exit early.
		mockInitializer.On("CheckDockerCmd").Return("", nil).Once()
		// Run our test and assert expectations.
		err := InitializeDocker(mockInitializer, defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker is running")
		mockInitializer.AssertExpectations(s.T())
	})

	s.Run("Docker is not running, tries to start and waits", func() {
		// Create mock initializer.
		mockInitializer := new(MockDockerInitializer)
		// Simulate that the initial `docker ps` fails.
		mockInitializer.On("CheckDockerCmd").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate that `open -a docker` succeeds.
		mockInitializer.On("OpenDockerCmd").Return("", nil).Once()
		// Simulate that `docker ps` works after trying to open docker.
		mockInitializer.On("CheckDockerCmd").Return("", nil).Once()
		// Run our test and assert expectations.
		err := InitializeDocker(mockInitializer, defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker starts after retry")
		mockInitializer.AssertExpectations(s.T())
	})

	s.Run("Docker fails to open", func() {
		// Create mock initializer.
		mockInitializer := new(MockDockerInitializer)
		// Simulate `docker ps` failing.
		mockInitializer.On("CheckDockerCmd").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate `open -a docker` failing.
		mockInitializer.On("OpenDockerCmd").Return("", fmt.Errorf("failed to open docker")).Once()
		// Run our test and assert expectations.
		err := InitializeDocker(mockInitializer, defaultTimeoutSeconds)
		assert.Equal(s.T(), fmt.Errorf(dockerOpenNotice), err, "Expected timeout error")
		mockInitializer.AssertExpectations(s.T())
	})

	s.Run("Docker open succeeds but check times out", func() {
		// Create mock initializer.
		mockInitializer := new(MockDockerInitializer)
		// Simulate `docker ps` failing continuously.
		mockInitializer.On("CheckDockerCmd").Return("", fmt.Errorf("docker not running"))
		// Simulate `open -a docker` failing.
		mockInitializer.On("OpenDockerCmd").Return("", nil).Once()
		// Run our test and assert expectations.
		// Simulate the timeout after 1 second.
		err := InitializeDocker(mockInitializer, 1)
		assert.Equal(s.T(), fmt.Errorf(timeoutErrMsg), err, "Expected timeout error")
		mockInitializer.AssertExpectations(s.T())
	})
}
