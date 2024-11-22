package runtimes

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockCommand struct {
	mock.Mock
}

func (m *MockCommand) Execute() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (s *ContainerRuntimeSuite) TestStartDocker() {
	s.Run("Docker is running, returns nil", func() {
		checkDockerCmd := new(MockCommand)
		// Simulate that `docker ps` runs successfully
		checkDockerCmd.On("Execute").Return("", nil).Once()

		openDockerCmd := new(MockCommand)
		// Simulate that `docker ps` runs successfully
		openDockerCmd.On("Execute").Return("", nil).Once()

		initializer := DockerInitializer{
			CheckDockerCmd: checkDockerCmd.Execute,
			OpenDockerCmd:  checkDockerCmd.Execute,
		}

		err := InitializeDocker(initializer, defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker is running")
		checkDockerCmd.AssertExpectations(s.T())
		openDockerCmd.AssertNotCalled(s.T(), "Execute")
	})

	s.Run("Docker is not running, tries to start and waits", func() {
		checkDockerCmd := new(MockCommand)
		// Simulate that the initial `docker ps` fails.
		checkDockerCmd.On("Execute").Return("", fmt.Errorf("docker not running")).Once()
		// Simulate that `docker ps` works after trying to open docker.
		checkDockerCmd.On("Execute").Return("", nil).Once()

		openDockerCmd := new(MockCommand)
		// Simulate that `open -a docker` succeeds.
		openDockerCmd.On("Execute").Return("", nil).Once()

		dockerInitializer := DockerInitializer{
			CheckDockerCmd: checkDockerCmd.Execute,
			OpenDockerCmd:  openDockerCmd.Execute,
		}

		err := InitializeDocker(dockerInitializer, defaultTimeoutSeconds)
		assert.Nil(s.T(), err, "Expected no error when docker starts after retry")
		checkDockerCmd.AssertExpectations(s.T())
		openDockerCmd.AssertExpectations(s.T())
	})

	s.Run("Docker fails to open", func() {
		checkDockerCmd := new(MockCommand)
		// Simulate `docker ps` failing.
		checkDockerCmd.On("Execute").Return("", fmt.Errorf("docker not running")).Once()

		openDockerCmd := new(MockCommand)
		// Simulate `open -a docker` failing.
		openDockerCmd.On("Execute").Return("", fmt.Errorf("failed to open docker")).Once()

		dockerInitializer := DockerInitializer{
			CheckDockerCmd: checkDockerCmd.Execute,
			OpenDockerCmd:  openDockerCmd.Execute,
		}

		err := InitializeDocker(dockerInitializer, defaultTimeoutSeconds)
		assert.Equal(s.T(), fmt.Errorf(dockerOpenNotice), err, "Expected timeout error")
		checkDockerCmd.AssertExpectations(s.T())
		openDockerCmd.AssertExpectations(s.T())
	})

	s.Run("Docker open succeeds but check times out", func() {
		checkDockerCmd := new(MockCommand)
		// Simulate `docker ps` failing indefinitely.
		checkDockerCmd.On("Execute").Return("", fmt.Errorf("docker not running"))

		openDockerCmd := new(MockCommand)
		// Simulate `open -a docker` succeeding.
		openDockerCmd.On("Execute").Return("", nil).Once()

		dockerInitializer := DockerInitializer{
			CheckDockerCmd: checkDockerCmd.Execute,
			OpenDockerCmd:  openDockerCmd.Execute,
		}

		// Simulate the timeout after 1 second.
		err := InitializeDocker(dockerInitializer, 1)
		assert.Equal(s.T(), fmt.Errorf(timeoutErrMsg), err, "Expected timeout error")
		checkDockerCmd.AssertExpectations(s.T())
		openDockerCmd.AssertExpectations(s.T())
	})
}
