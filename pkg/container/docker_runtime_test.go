package container

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// fakeDockerEngine is a scripted DockerEngine for the docker-runtime tests.
// isRunning returns the next queued error each call (then the last one forever),
// so we can simulate "down, then up" sequences without a live Docker.
type fakeDockerEngine struct {
	isRunningErrs []error
	startErr      error
	startCalls    int
}

func (f *fakeDockerEngine) IsRunning() (string, error) {
	var err error
	switch {
	case len(f.isRunningErrs) == 0:
		err = nil
	case len(f.isRunningErrs) == 1:
		err = f.isRunningErrs[0]
	default:
		err = f.isRunningErrs[0]
		f.isRunningErrs = f.isRunningErrs[1:]
	}
	return "", err
}

func (f *fakeDockerEngine) Start() (string, error) {
	f.startCalls++
	return "", f.startErr
}

type DockerRuntimeSuite struct {
	suite.Suite
}

func TestDockerRuntime(t *testing.T) {
	suite.Run(t, new(DockerRuntimeSuite))
}

func (s *DockerRuntimeSuite) TestInitializeNonMacIsNoop() {
	s.Run("non-mac does not touch the engine", func() {
		eng := &fakeDockerEngine{isRunningErrs: []error{fmt.Errorf("docker not running")}}
		rt := &DockerRuntime{Engine: eng, OSChecker: nonMacChecker{}, fb: NoopFeedback{}}
		err := rt.Initialize()
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 0, eng.startCalls)
	})
}

func (s *DockerRuntimeSuite) TestStartDocker() {
	s.Run("Docker is running, returns nil", func() {
		eng := &fakeDockerEngine{isRunningErrs: []error{nil}}
		rt := CreateDockerRuntime(eng, macChecker{})
		err := rt.Initialize()
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 0, eng.startCalls)
	})

	s.Run("Docker is not running, tries to start and waits", func() {
		eng := &fakeDockerEngine{isRunningErrs: []error{fmt.Errorf("docker not running"), nil}}
		rt := CreateDockerRuntime(eng, macChecker{})
		err := rt.Initialize()
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 1, eng.startCalls)
	})

	s.Run("Docker fails to open", func() {
		eng := &fakeDockerEngine{
			isRunningErrs: []error{fmt.Errorf("docker not running")},
			startErr:      fmt.Errorf("failed to open docker"),
		}
		rt := CreateDockerRuntime(eng, macChecker{})
		err := rt.Initialize()
		assert.EqualError(s.T(), err, dockerOpenNotice)
	})

	s.Run("Docker open succeeds but check times out", func() {
		eng := &fakeDockerEngine{isRunningErrs: []error{fmt.Errorf("docker not running")}}
		rt := CreateDockerRuntime(eng, macChecker{})
		err := rt.initializeDocker(1)
		assert.EqualError(s.T(), err, timeoutErrMsg)
	})
}

// macChecker / nonMacChecker are trivial OSChecker stubs for the docker tests.
type macChecker struct{}

func (macChecker) IsMac() bool     { return true }
func (macChecker) IsWindows() bool { return false }

type nonMacChecker struct{}

func (nonMacChecker) IsMac() bool     { return false }
func (nonMacChecker) IsWindows() bool { return false }
