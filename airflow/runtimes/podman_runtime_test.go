package runtimes

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow/runtimes/mocks"
	"github.com/astronomer/astro-cli/airflow/runtimes/types"
	"github.com/stretchr/testify/suite"

	"github.com/stretchr/testify/assert"
)

var (
	mockListedMachines        []types.ListedMachine
	mockListedContainers      []types.ListedContainer
	mockInspectedAstroMachine *types.InspectedMachine
	mockInspectedOtherMachine *types.InspectedMachine
	mockPodmanEngine          *mocks.PodmanEngine
	mockPodmanOSChecker       *mocks.OSChecker
)

type PodmanRuntimeSuite struct {
	suite.Suite
}

func TestPodmanRuntime(t *testing.T) {
	suite.Run(t, new(PodmanRuntimeSuite))
}

// Setenv is a helper function to set an environment variable.
// It panics if an error occurs.
func (s *PodmanRuntimeSuite) Setenv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		panic(err)
	}
}

// Unsetenv is a helper function to unset an environment variable.
// It panics if an error occurs.
func (s *PodmanRuntimeSuite) Unsetenv(key string) {
	if err := os.Unsetenv(key); err != nil {
		panic(err)
	}
}

func (s *PodmanRuntimeSuite) SetupTest() {
	// Reset some variables to defaults.
	s.Unsetenv("DOCKER_HOST")
	mockPodmanEngine = new(mocks.PodmanEngine)
	mockPodmanOSChecker = new(mocks.OSChecker)
	mockListedMachines = []types.ListedMachine{}
	mockListedContainers = []types.ListedContainer{}
	mockInspectedAstroMachine = &types.InspectedMachine{
		Name: "astro-machine",
		ConnectionInfo: types.ConnectionInfo{
			PodmanSocket: types.PodmanSocket{
				Path: "/path/to/astro-machine.sock",
			},
		},
	}
	mockInspectedOtherMachine = &types.InspectedMachine{
		Name: "other-machine",
		ConnectionInfo: types.ConnectionInfo{
			PodmanSocket: types.PodmanSocket{
				Path: "/path/to/other-machine.sock",
			},
		},
	}
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeDockerHostAlreadySet() {
	s.Run("DOCKER_HOST is already set, abort initialization", func() {
		// Set up mocks.
		s.Setenv("DOCKER_HOST", "some_value")
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitialize() {
	s.Run("No machines running on mac, initialize podman", func() {
		// Set up mocks.
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InitializeMachine", podmanMachineName).Return(nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeWindows() {
	s.Run("No machines running on windows, initialize podman", func() {
		// Set up mocks.
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InitializeMachine", podmanMachineName).Return(nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(true)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeWithAnotherMachineRunningOnMac() {
	s.Run("Another machine running on mac, stop it and start the astro machine", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "other-machine",
				Running: true,
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil).Once()
		mockPodmanEngine.On("InspectMachine", mockListedMachines[0].Name).Return(mockInspectedOtherMachine, nil).Once()
		mockPodmanEngine.On("SetMachineAsDefault", mockListedMachines[0].Name).Return(nil).Once()
		mockPodmanEngine.On("ListContainers").Return(mockListedContainers, nil)
		mockPodmanEngine.On("StopMachine", mockListedMachines[0].Name).Return(nil)
		mockPodmanEngine.On("ListMachines").Return([]types.ListedMachine{}, nil).Once()
		mockPodmanEngine.On("InitializeMachine", podmanMachineName).Return(nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil).Once()
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil).Once()
		mockPodmanOSChecker.On("IsMac").Return(true)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeWithAnotherMachineRunningWithExistingContainersOnMac() {
	s.Run("Another machine running on mac, that has existing containers, return error message", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "other-machine",
				Running: true,
			},
		}
		mockListedContainers = []types.ListedContainer{
			{
				Name:   "container1",
				Labels: map[string]string{composeProjectLabel: "project1"},
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil).Once()
		mockPodmanEngine.On("InspectMachine", mockListedMachines[0].Name).Return(mockInspectedOtherMachine, nil).Once()
		mockPodmanEngine.On("SetMachineAsDefault", mockListedMachines[0].Name).Return(nil).Once()
		mockPodmanEngine.On("ListContainers").Return(mockListedContainers, nil)
		mockPodmanOSChecker.On("IsMac").Return(true)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Error(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeAstroMachineAlreadyRunning() {
	s.Run("Astro machine is already running, just configure it for usage", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockInspectedAstroMachine.State = podmanStatusRunning
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeInitializeAstroMachineExistsButStopped() {
	s.Run("Astro machine already exists, but is in stopped state, start and configure it for usage", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockInspectedAstroMachine.State = podmanStatusStopped
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanEngine.On("StartMachine", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Initialize()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureDockerHostAlreadySet() {
	s.Run("DOCKER_HOST is already set, abort configure", func() {
		// Set up mocks.
		s.Setenv("DOCKER_HOST", "some_value")
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Configure()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureAstroMachineRunning() {
	s.Run("Astro machine is already running, so configure it for usage", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Configure()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureAstroMachineNotRunning() {
	s.Run("Astro machine is not already running, so return error message", func() {
		// Set up mocks.
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Configure()
		assert.Error(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureOrKillDockerHostAlreadySet() {
	s.Run("Astro machine is not already running, so return error message", func() {
		// Set up mocks.
		s.Setenv("DOCKER_HOST", "some_value")
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.ConfigureOrKill()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureOrKillAstroMachineRunning() {
	s.Run("Astro machine is already running, so configure it for usage", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("InspectMachine", podmanMachineName).Return(mockInspectedAstroMachine, nil)
		mockPodmanEngine.On("SetMachineAsDefault", podmanMachineName).Return(nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.ConfigureOrKill()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureOrKillAstroMachineStopped() {
	s.Run("Astro machine is stopped, proceed to kill it", func() {
		// Set up mocks.
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: false,
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanEngine.On("RemoveMachine", podmanMachineName).Return(nil)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.ConfigureOrKill()
		assert.Error(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeConfigureOrKillAstroMachineNotRunning() {
	s.Run("Astro machine is not already running, so return error message", func() {
		// Set up mocks.
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.ConfigureOrKill()
		assert.Error(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeKillOtherProjectRunning() {
	s.Run("Astro machine running, but another project is still running, so do not stop and kill machine", func() {
		// Set up mocks.
		s.Setenv("DOCKER_HOST", podmanMachineName)
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockListedContainers = []types.ListedContainer{
			{
				Name:   "container1",
				Labels: map[string]string{composeProjectLabel: "project1"},
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		mockPodmanEngine.On("ListContainers").Return(mockListedContainers, nil)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Kill()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestPodmanRuntimeKill() {
	s.Run("Astro machine running, no other projects running, so stop and kill the machine", func() {
		// Set up mocks.
		s.Setenv("DOCKER_HOST", podmanMachineName)
		mockListedMachines = []types.ListedMachine{
			{
				Name:    "astro-machine",
				Running: true,
			},
		}
		mockPodmanEngine.On("ListMachines").Return(mockListedMachines, nil)
		mockPodmanOSChecker.On("IsWindows").Return(false)
		mockPodmanEngine.On("ListContainers").Return(mockListedContainers, nil)
		mockPodmanEngine.On("StopMachine", podmanMachineName).Return(nil)
		mockPodmanEngine.On("RemoveMachine", podmanMachineName).Return(nil)
		// Create the runtime with our mock engine and os checker.
		rt := CreatePodmanRuntime(mockPodmanEngine, mockPodmanOSChecker)
		// Run our test and assert expectations.
		err := rt.Kill()
		assert.Nil(s.T(), err)
		mockPodmanEngine.AssertExpectations(s.T())
		mockPodmanOSChecker.AssertExpectations(s.T())
	})
}

func (s *PodmanRuntimeSuite) TestFindMachineByName() {
	s.Run("Returns machine when name matches", func() {
		machines := []types.ListedMachine{
			{Name: "astro-machine"},
			{Name: "other-machine"},
		}
		result := findMachineByName(machines, "astro-machine")
		assert.NotNil(s.T(), result)
		assert.Equal(s.T(), "astro-machine", result.Name)
	})

	s.Run("Returns nil when no match found", func() {
		machines := []types.ListedMachine{
			{Name: "astro-machine"},
			{Name: "other-machine"},
		}
		result := findMachineByName(machines, "non-existent-machine")
		assert.Nil(s.T(), result)
	})

	s.Run("Returns nil when list is empty", func() {
		var machines []types.ListedMachine
		result := findMachineByName(machines, "astro-machine")
		assert.Nil(s.T(), result)
	})
}

func (s *PodmanRuntimeSuite) TestIsDockerHostSet() {
	s.Run("DOCKER_HOST is set and returns true", func() {
		s.Setenv("DOCKER_HOST", "some_value")
		result := isDockerHostSet()
		assert.True(s.T(), result)
	})

	s.Run("DOCKER_HOST is set and returns true", func() {
		s.Unsetenv("DOCKER_HOST")
		result := isDockerHostSet()
		assert.False(s.T(), result)
	})
}

func (s *PodmanRuntimeSuite) TestIsDockerHostSetToAstroMachine() {
	s.Run("DOCKER_HOST is set to astro-machine and returns true", func() {
		s.Setenv("DOCKER_HOST", "unix:///path/to/astro-machine.sock")
		result := isDockerHostSetToAstroMachine()
		assert.True(s.T(), result)
	})

	s.Run("DOCKER_HOST is set to other-machine and returns false", func() {
		s.Setenv("DOCKER_HOST", "unix:///path/to/other-machine.sock")
		result := isDockerHostSetToAstroMachine()
		assert.False(s.T(), result)
	})

	s.Run("DOCKER_HOST is not set and returns false", func() {
		s.Unsetenv("DOCKER_HOST")
		result := isDockerHostSetToAstroMachine()
		assert.False(s.T(), result)
	})
}
