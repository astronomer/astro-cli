package container

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// scriptedPodman is a PodmanEngine fake that records calls and returns canned
// data, for the Manager ContainerRuntime tests. Unlike the read-only fakePodman
// in manager_test.go, it tracks side-effecting calls so we can assert behavior.
type scriptedPodman struct {
	machines   []ListedMachine
	containers []ListedContainer
	inspect    *InspectedMachine
	inspectErr error

	initCalls    int
	startCalls   int
	stopCalls    []string
	removeCalls  int
	defaultCalls []string
}

func (p *scriptedPodman) InitializeMachine(string, Config, Feedback) error {
	p.initCalls++
	return nil
}
func (p *scriptedPodman) StartMachine(string) error { p.startCalls++; return nil }
func (p *scriptedPodman) StopMachine(name string) error {
	p.stopCalls = append(p.stopCalls, name)
	return nil
}
func (p *scriptedPodman) RemoveMachine(string) error { p.removeCalls++; return nil }
func (p *scriptedPodman) SetMachineAsDefault(name string) error {
	p.defaultCalls = append(p.defaultCalls, name)
	return nil
}

func (p *scriptedPodman) InspectMachine(string) (*InspectedMachine, error) {
	return p.inspect, p.inspectErr
}
func (p *scriptedPodman) ListMachines() ([]ListedMachine, error)     { return p.machines, nil }
func (p *scriptedPodman) ListContainers() ([]ListedContainer, error) { return p.containers, nil }

type ManagerRuntimeSuite struct {
	suite.Suite
}

func TestManagerRuntime(t *testing.T) {
	suite.Run(t, new(ManagerRuntimeSuite))
}

func (s *ManagerRuntimeSuite) SetupTest() {
	_ = os.Unsetenv("DOCKER_HOST")
	_ = os.Unsetenv("CONTAINER_HOST")
}

func newPodmanManager(pe PodmanEngine, h host) *Manager {
	return &Manager{engine: Podman, cfg: Config{}, fb: NoopFeedback{}, podman: pe, host: h}
}

func astroInspected(state string) *InspectedMachine {
	return &InspectedMachine{
		Name:           podmanMachineName,
		State:          state,
		ConnectionInfo: ConnectionInfo{PodmanSocket: PodmanSocket{Path: "/path/to/astro-machine.sock"}},
	}
}

// --- Initialize (EnsureMachine) ---

func (s *ManagerRuntimeSuite) TestInitializeDockerHostAlreadySet() {
	s.Run("DOCKER_HOST is already set, abort initialization", func() {
		s.T().Setenv("DOCKER_HOST", "some_value")
		pe := &scriptedPodman{}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Initialize())
		assert.Equal(s.T(), 0, pe.initCalls)
	})
}

func (s *ManagerRuntimeSuite) TestInitializeNoMachines() {
	s.Run("No machines, initialize podman and set default", func() {
		pe := &scriptedPodman{machines: nil}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Initialize())
		assert.Equal(s.T(), 1, pe.initCalls)
		assert.Equal(s.T(), []string{podmanMachineName}, pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestInitializeAstroMachineAlreadyRunning() {
	s.Run("Astro machine already running, just set default", func() {
		pe := &scriptedPodman{machines: []ListedMachine{{Name: podmanMachineName, Running: true}}}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Initialize())
		assert.Equal(s.T(), 0, pe.initCalls)
		assert.Equal(s.T(), 0, pe.startCalls)
		assert.Equal(s.T(), []string{podmanMachineName}, pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestInitializeAstroMachineStopped() {
	s.Run("Astro machine stopped, start it and set default", func() {
		pe := &scriptedPodman{machines: []ListedMachine{{Name: podmanMachineName, Running: false}}}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Initialize())
		assert.Equal(s.T(), 1, pe.startCalls)
		assert.Equal(s.T(), []string{podmanMachineName}, pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestInitializeAnotherMachineRunningOnMac() {
	s.Run("Another machine running on mac with no containers, stop it then init astro", func() {
		pe := &scriptedPodman{
			machines:   []ListedMachine{{Name: "other-machine", Running: true}},
			containers: nil,
		}
		m := newPodmanManager(pe, fakeHost{mac: true})
		assert.NoError(s.T(), m.Initialize())
		assert.Contains(s.T(), pe.stopCalls, "other-machine")
		assert.Equal(s.T(), 1, pe.initCalls)
	})
}

func (s *ManagerRuntimeSuite) TestInitializeAnotherMachineWithContainersOnMac() {
	s.Run("Another machine running on mac with containers, return error", func() {
		pe := &scriptedPodman{
			machines:   []ListedMachine{{Name: "other-machine", Running: true}},
			containers: []ListedContainer{{Name: "c1", Labels: map[string]string{composeProjectLabel: "p1"}}},
		}
		m := newPodmanManager(pe, fakeHost{mac: true})
		assert.Error(s.T(), m.Initialize())
		assert.Equal(s.T(), 0, pe.initCalls)
	})
}

// --- Configure ---

func (s *ManagerRuntimeSuite) TestConfigureDockerHostAlreadySet() {
	s.Run("DOCKER_HOST is already set, abort configure", func() {
		s.T().Setenv("DOCKER_HOST", "some_value")
		pe := &scriptedPodman{}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Configure())
		assert.Empty(s.T(), pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestConfigureAstroMachineRunning() {
	s.Run("Astro machine running, configure it for usage", func() {
		pe := &scriptedPodman{
			machines: []ListedMachine{{Name: podmanMachineName, Running: true}},
			inspect:  astroInspected(podmanStatusRunning),
		}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Configure())
		assert.Equal(s.T(), "unix:///path/to/astro-machine.sock", os.Getenv("DOCKER_HOST"))
		assert.Equal(s.T(), "unix:///path/to/astro-machine.sock", os.Getenv("CONTAINER_HOST"))
		assert.Equal(s.T(), []string{podmanMachineName}, pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestConfigureWindowsOmitsContainerHost() {
	s.Run("On windows, DOCKER_HOST uses npipe and CONTAINER_HOST is empty", func() {
		pe := &scriptedPodman{
			machines: []ListedMachine{{Name: podmanMachineName, Running: true}},
			inspect:  astroInspected(podmanStatusRunning),
		}
		m := newPodmanManager(pe, fakeHost{windows: true})
		assert.NoError(s.T(), m.Configure())
		assert.Equal(s.T(), "npipe:////./pipe/podman-astro-machine", os.Getenv("DOCKER_HOST"))
		assert.Empty(s.T(), os.Getenv("CONTAINER_HOST"))
	})
}

func (s *ManagerRuntimeSuite) TestConfigureAstroMachineNotRunning() {
	s.Run("Astro machine not running, return error", func() {
		pe := &scriptedPodman{machines: nil}
		m := newPodmanManager(pe, fakeHost{})
		assert.Error(s.T(), m.Configure())
	})
}

// --- ConfigureOrKill ---

func (s *ManagerRuntimeSuite) TestConfigureOrKillDockerHostAlreadySet() {
	s.Run("DOCKER_HOST set, no-op", func() {
		s.T().Setenv("DOCKER_HOST", "some_value")
		m := newPodmanManager(&scriptedPodman{}, fakeHost{})
		assert.NoError(s.T(), m.ConfigureOrKill())
	})
}

func (s *ManagerRuntimeSuite) TestConfigureOrKillAstroMachineRunning() {
	s.Run("Astro machine running, configure for usage", func() {
		pe := &scriptedPodman{
			machines: []ListedMachine{{Name: podmanMachineName, Running: true}},
			inspect:  astroInspected(podmanStatusRunning),
		}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.ConfigureOrKill())
		assert.Equal(s.T(), []string{podmanMachineName}, pe.defaultCalls)
	})
}

func (s *ManagerRuntimeSuite) TestConfigureOrKillAstroMachineStopped() {
	s.Run("Astro machine stopped, remove it and error", func() {
		pe := &scriptedPodman{machines: []ListedMachine{{Name: podmanMachineName, Running: false}}}
		m := newPodmanManager(pe, fakeHost{})
		assert.Error(s.T(), m.ConfigureOrKill())
		assert.Equal(s.T(), 1, pe.removeCalls)
	})
}

func (s *ManagerRuntimeSuite) TestConfigureOrKillAstroMachineAbsent() {
	s.Run("Astro machine absent, return error", func() {
		pe := &scriptedPodman{machines: nil}
		m := newPodmanManager(pe, fakeHost{})
		assert.Error(s.T(), m.ConfigureOrKill())
		assert.Equal(s.T(), 0, pe.removeCalls)
	})
}

// --- Kill ---

func (s *ManagerRuntimeSuite) TestKillOtherProjectRunning() {
	s.Run("Other project running, do not stop/kill", func() {
		s.T().Setenv("DOCKER_HOST", "unix:///path/to/astro-machine.sock")
		pe := &scriptedPodman{
			machines:   []ListedMachine{{Name: podmanMachineName, Running: true}},
			containers: []ListedContainer{{Name: "c1", Labels: map[string]string{composeProjectLabel: "p1"}}},
		}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Kill())
		assert.Empty(s.T(), pe.stopCalls)
		assert.Equal(s.T(), 0, pe.removeCalls)
	})
}

func (s *ManagerRuntimeSuite) TestKill() {
	s.Run("No other projects, stop and remove", func() {
		s.T().Setenv("DOCKER_HOST", "unix:///path/to/astro-machine.sock")
		pe := &scriptedPodman{
			machines: []ListedMachine{{Name: podmanMachineName, Running: true}},
		}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Kill())
		assert.Contains(s.T(), pe.stopCalls, podmanMachineName)
		assert.Equal(s.T(), 1, pe.removeCalls)
	})
}

func (s *ManagerRuntimeSuite) TestKillDockerHostNotAstroMachine() {
	s.Run("DOCKER_HOST not pointed at astro machine, no-op", func() {
		s.T().Setenv("DOCKER_HOST", "unix:///path/to/other-machine.sock")
		pe := &scriptedPodman{machines: []ListedMachine{{Name: podmanMachineName, Running: true}}}
		m := newPodmanManager(pe, fakeHost{})
		assert.NoError(s.T(), m.Kill())
		assert.Empty(s.T(), pe.stopCalls)
		assert.Equal(s.T(), 0, pe.removeCalls)
	})
}

func (s *ManagerRuntimeSuite) TestConfigureInspectError() {
	s.Run("inspect error surfaces", func() {
		pe := &scriptedPodman{
			machines:   []ListedMachine{{Name: podmanMachineName, Running: true}},
			inspectErr: errors.New("boom"),
		}
		m := newPodmanManager(pe, fakeHost{})
		assert.Error(s.T(), m.Configure())
	})
}
