package runtimes

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow/runtimes/types"
	sp "github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/briandowns/spinner"
)

const (
	podmanMachineName       = "astro-machine"
	projectNotRunningErrMsg = "this astro project is not running"
)

type PodmanEngine interface {
	InitializeMachine(name string, s *spinner.Spinner) error
	StartMachine(name string) error
	StopMachine(name string) error
	RemoveMachine(name string) error
	InspectMachine(name string) (*types.InspectedMachine, error)
	SetMachineAsDefault(name string) error
	ListMachines() ([]types.ListedMachine, error)
	ListContainers() ([]types.ListedContainer, error)
}

type PodmanRuntime struct {
	Engine    PodmanEngine
	OSChecker OSChecker
}

// CreatePodmanRuntime creates a new PodmanRuntime using the provided PodmanEngine.
// The engine allows us to interact with the external podman environment. For unit testing,
// we provide a mock engine that can be used to simulate the podman environment.
func CreatePodmanRuntime(engine PodmanEngine, osChecker OSChecker) PodmanRuntime {
	return PodmanRuntime{Engine: engine, OSChecker: osChecker}
}

// CreatePodmanRuntimeWithDefaults creates a new PodmanRuntime using the default PodmanEngine and OSChecker.
func CreatePodmanRuntimeWithDefaults() PodmanRuntime {
	return PodmanRuntime{Engine: GetPodmanEngine(), OSChecker: CreateOSChecker()}
}

// GetPodmanEngine creates a new PodmanEngine using the default implementation.
func GetPodmanEngine() PodmanEngine {
	return new(podmanEngine)
}

func (rt PodmanRuntime) Initialize() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to initialize our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}
	return rt.ensureMachine()
}

func (rt PodmanRuntime) Configure() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}

	// If the astro machine is running, we just configure it
	// for usage, so the regular compose commands can carry out.
	if rt.astroMachineIsRunning() {
		return rt.getAndConfigureMachineForUsage(podmanMachineName)
	}

	// Otherwise, we return an error indicating that the project isn't running.
	return errors.New(projectNotRunningErrMsg)
}

func (rt PodmanRuntime) ConfigureOrKill() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}

	// If the astro machine is running, we just configure it
	// for usage, so the regular compose kill can carry out.
	// We follow up with a machine kill in the post run hook.
	if rt.astroMachineIsRunning() {
		return rt.getAndConfigureMachineForUsage(podmanMachineName)
	}

	// The machine is already not running,
	// so we can just ensure its fully killed.
	if err := rt.stopAndKillMachine(); err != nil {
		return err
	}

	// We also return an error indicating that you can't kill
	// a project that isn't running.
	return errors.New(projectNotRunningErrMsg)
}

func (rt PodmanRuntime) Kill() error {
	// If we're in podman mode, and DOCKER_HOST is set to the astro machine (in the pre-run hook),
	// we'll ensure that the machine is killed.
	if isDockerHostSetToAstroMachine() {
		return rt.stopAndKillMachine()
	}
	return nil
}

func (rt PodmanRuntime) ensureMachine() error {
	// Show a spinner message while we're initializing the machine.
	s := sp.NewSpinner(containerRuntimeInitMessage)
	defer s.Stop()

	// Check if another, non-astro Podman machine is running
	nonAstroMachineName := rt.isAnotherMachineRunning()
	// If there is another machine running, and it has no running containers, stop it.
	// Otherwise, we assume the user has some other project running that we don't want to interfere with.
	if nonAstroMachineName != "" && rt.OSChecker.IsMac() {
		// First, configure the other running machine for usage, so we can check it for containers.
		if err := rt.getAndConfigureMachineForUsage(nonAstroMachineName); err != nil {
			return err
		}

		// Then check the machine for running containers.
		containers, err := rt.Engine.ListContainers()
		if err != nil {
			return err
		}

		// There's some other containers running on this machine, so we don't want to stop it.
		// We want the user to stop it manually and restart astro.
		if len(containers) > 0 {
			return errors.New(podmanMachineAlreadyRunningErrMsg)
		}

		// If we made it here, we're going to stop the other machine
		// and start our own machine, so start the spinner and begin the process.
		s.Start()
		err = rt.Engine.StopMachine(nonAstroMachineName)
		if err != nil {
			return err
		}
	}

	// Check if our astro Podman machine exists.
	machine := rt.getAstroMachine()

	// If the machine exists, inspect it and decide what to do.
	if machine != nil {
		// Inspect the machine and get its details.
		iMachine, err := rt.Engine.InspectMachine(podmanMachineName)
		if err != nil {
			return err
		}

		// If the machine is already running,
		// just go ahead and configure it for usage.
		if iMachine.State == podmanStatusRunning {
			return rt.configureMachineForUsage(iMachine)
		}

		// If the machine is stopped,
		// start it, then configure it for usage.
		if iMachine.State == podmanStatusStopped {
			s.Start()
			if err := rt.Engine.StartMachine(podmanMachineName); err != nil {
				return err
			}
			return rt.configureMachineForUsage(iMachine)
		}
	}

	// Otherwise, initialize the machine
	s.Start()
	// time delay of 1 second to display containerRuntimeInitMessage before initializing astro-machine
	time.Sleep(1 * time.Second)
	if err := rt.Engine.InitializeMachine(podmanMachineName, s); err != nil {
		return err
	}
	sp.StopWithCheckmark(s, "astro-machine initialized")

	return rt.getAndConfigureMachineForUsage(podmanMachineName)
}

// stopAndKillMachine attempts to stop and kill the Podman machine.
// If other projects are running, it will leave the machine up.
func (rt PodmanRuntime) stopAndKillMachine() error {
	// If the machine doesn't exist, exit early.
	if !rt.astroMachineExists() {
		return nil
	}

	// If the machine exists, and its running, we need to check
	// if any other projects are running. If other projects are running,
	// we'll leave the machine up, otherwise we stop and kill it.
	if rt.astroMachineIsRunning() {
		// Get the containers that are running on our machine.
		containers, err := rt.Engine.ListContainers()
		if err != nil {
			return err
		}

		// Check the container labels to identify if other projects are running.
		projectNames := make(map[string]struct{})
		for _, item := range containers {
			// Check if "project.name" exists in the Labels map
			if projectName, exists := item.Labels[composeProjectLabel]; exists {
				// Add the project name to the map (map keys are unique)
				projectNames[projectName] = struct{}{}
			}
		}

		// At this point in the command hook lifecycle, our project has already been stopped,
		// and we are checking to see if any additional projects are running.
		if len(projectNames) > 0 {
			return nil
		}

		// If we made it this far, we can stop the machine,
		// as there are no more projects running.
		err = rt.Engine.StopMachine(podmanMachineName)
		if err != nil {
			return err
		}
	}

	// If we make it here, the machine was already stopped, or was just stopped above.
	// We can now remove it.
	err := rt.Engine.RemoveMachine(podmanMachineName)
	if err != nil {
		return err
	}

	return nil
}

// configureMachineForUsage does two things:
//   - Sets the DOCKER_HOST environment variable to the machine's socket path
//     This allows the docker compose library to function as expected.
//   - Sets the podman default connection to the machine
//     This allows the podman command to function as expected.
func (rt PodmanRuntime) configureMachineForUsage(machine *types.InspectedMachine) error {
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}

	// Compute our DOCKER_HOST value depending on the OS.
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	if rt.OSChecker.IsWindows() {
		dockerHost = "npipe:////./pipe/podman-" + machine.Name
	}

	// Set the DOCKER_HOST environment variable for compose.
	err := os.Setenv("DOCKER_HOST", dockerHost)
	if err != nil {
		return fmt.Errorf("error setting DOCKER_HOST: %s", err)
	}

	// Set the podman default connection to our machine.
	return rt.Engine.SetMachineAsDefault(machine.Name)
}

// getAndConfigureMachineForUsage gets our astro machine
// then configures the host machine to use it.
func (rt PodmanRuntime) getAndConfigureMachineForUsage(name string) error {
	machine, err := rt.Engine.InspectMachine(name)
	if err != nil {
		return err
	}
	return rt.configureMachineForUsage(machine)
}

// getAstroMachine gets our astro podman machine.
func (rt PodmanRuntime) getAstroMachine() *types.ListedMachine {
	machines, _ := rt.Engine.ListMachines()
	return findMachineByName(machines, podmanMachineName)
}

// astroMachineExists checks if our astro podman machine exists.
func (rt PodmanRuntime) astroMachineExists() bool {
	machine := rt.getAstroMachine()
	return machine != nil
}

// astroMachineIsRunning checks if our astro podman machine is running.
func (rt PodmanRuntime) astroMachineIsRunning() bool {
	machine := rt.getAstroMachine()
	return machine != nil && machine.Running
}

// isAnotherMachineRunning checks if another, non-astro podman machine is running.
func (rt PodmanRuntime) isAnotherMachineRunning() string {
	machines, _ := rt.Engine.ListMachines()
	for _, machine := range machines {
		if machine.Running && machine.Name != podmanMachineName {
			return machine.Name
		}
	}
	return ""
}

// findMachineByName finds a machine by name from a list of machines.
func findMachineByName(items []types.ListedMachine, name string) *types.ListedMachine {
	for _, item := range items {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

// isDockerHostSet checks if the DOCKER_HOST environment variable is set.
func isDockerHostSet() bool {
	return os.Getenv("DOCKER_HOST") != ""
}

// isDockerHostSetToAstroMachine checks if the DOCKER_HOST environment variable
// is pointing to the astro machine.
func isDockerHostSetToAstroMachine() bool {
	return strings.Contains(os.Getenv("DOCKER_HOST"), podmanMachineName)
}
