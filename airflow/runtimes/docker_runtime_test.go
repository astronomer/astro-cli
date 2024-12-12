package runtimes

import (
	"fmt"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/airflow/runtimes/mocks"
	"github.com/stretchr/testify/suite"

	"github.com/stretchr/testify/assert"
)

var (
	mockDockerEngine        *mocks.DockerEngine
	mockDockerOSChecker     *mocks.OSChecker
	mockDockerFileChecker   *mocks.FileChecker
	mockDockerEnvChecker    *mocks.EnvChecker
	mockDockerHostInspector HostInterrogator
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
	mockDockerFileChecker = new(mocks.FileChecker)
	mockDockerEnvChecker = new(mocks.EnvChecker)
	mockDockerHostInspector = CreateHostInspector(mockDockerOSChecker, mockDockerFileChecker, mockDockerEnvChecker)
}

func (s *DockerRuntimeSuite) TestGetDockerEngineReturnsOrbstackEngine() {
	s.Run("Get Docker Engine returns Orbstack Engine for orbctl", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(true)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)

		engine := GetDockerEngine(mockDockerHostInspector)
		assert.IsType(s.T(), new(orbstackEngine), engine, "Expected orbstack engine to be returned")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
}

func (s *DockerRuntimeSuite) TestGetDockerEngineReturnsDockerEngine() {
	s.Run("Get Docker Engine returns Docker Engine for docker", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+docker).Return(true)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+docker).Return(false)

		engine := GetDockerEngine(mockDockerHostInspector)
		assert.IsType(s.T(), new(dockerEngine), engine, "Expected docker engine to be returned")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
}

func (s *DockerRuntimeSuite) TestGetDockerEngineFallbackToDocker() {
	s.Run("Get Docker Engine Binary finds no binaries in $PATH", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+docker).Return(false)

		binary, err := GetDockerEngineBinary(mockDockerHostInspector)
		assert.Nil(s.T(), err, "Expected no error when docker binary is found")
		assert.Equal(s.T(), docker, binary, "Expected docker binary to returned by default")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
}

func (s *DockerRuntimeSuite) TestGetDockerEngineFindsOrbstack() {
	s.Run("Get Docker Engine Binary finds Orbstack in $PATH", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(true)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)

		binary, err := GetDockerEngineBinary(mockDockerHostInspector)
		assert.Nil(s.T(), err, "Expected no error when orbctl binary is found")
		assert.Equal(s.T(), orbctl, binary, "Expected orbctl binary to be found")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
}

func (s *DockerRuntimeSuite) TestGetDockerEngineBinaryFindsDocker() {
	s.Run("Get Docker Engine Binary finds Docker in $PATH", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+docker).Return(true)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+docker).Return(false)

		binary, err := GetDockerEngineBinary(mockDockerHostInspector)
		assert.Nil(s.T(), err, "Expected no error when docker binary is found")
		assert.Equal(s.T(), docker, binary, "Expected docker binary to be found")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
}

func (s *DockerRuntimeSuite) TestGetDockerEngineBinaryFindsNoBinaryFallbackToDocker() {
	s.Run("Get Docker Engine Binary finds Docker in $PATH", func() {
		paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
		mockDockerEnvChecker.On("GetEnvVar", "PATH").Return(strings.Join(paths, ":"), nil)
		mockDockerOSChecker.On("IsWindows").Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+orbctl).Return(false)
		mockDockerFileChecker.On("FileExists", paths[0]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[1]+"/"+docker).Return(false)
		mockDockerFileChecker.On("FileExists", paths[2]+"/"+docker).Return(false)

		binary, err := GetDockerEngineBinary(mockDockerHostInspector)
		assert.Nil(s.T(), err, "Expected no error when docker binary is found")
		assert.Equal(s.T(), docker, binary, "Expected docker binary to be found")
		mockDockerEnvChecker.AssertExpectations(s.T())
		mockDockerOSChecker.AssertExpectations(s.T())
		mockDockerFileChecker.AssertExpectations(s.T())
	})
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
		assert.Equal(s.T(), fmt.Errorf(dockerOpenNotice), err, "Expected timeout error")
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
		assert.Equal(s.T(), fmt.Errorf(timeoutErrMsg), err, "Expected timeout error")
		mockDockerEngine.AssertExpectations(s.T())
	})
}
