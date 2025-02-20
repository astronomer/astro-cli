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
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
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
	Engine      PodmanEngine
	OSChecker   OSChecker
	MachineName string // if set, use this machine name instead of the default
}

// getMachineName returns the effective machine name based on the runtime object.
func (rt PodmanRuntime) getMachineName() string {
	if rt.MachineName != "" {
		return rt.MachineName
	}
	return podmanMachineName
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
	// we need to initialize our machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}
	return rt.ensureMachine()
}

func (rt PodmanRuntime) Configure() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}

	// If the machine is running, configure it for usage.
	if rt.astroMachineIsRunning() {
		return rt.getAndConfigureMachineForUsage(rt.getMachineName())
	}

	return errors.New(projectNotRunningErrMsg)
}

func (rt PodmanRuntime) ConfigureOrKill() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if isDockerHostSet() {
		return nil
	}

	if rt.astroMachineIsRunning() {
		return rt.getAndConfigureMachineForUsage(rt.getMachineName())
	}

	if err := rt.stopAndKillMachine(); err != nil {
		return err
	}

	return errors.New(projectNotRunningErrMsg)
}

func (rt PodmanRuntime) Kill() error {
	if isDockerHostSetToAstroMachine() {
		return rt.stopAndKillMachine()
	}
	return nil
}

func (rt PodmanRuntime) ensureMachine() error {
	s := sp.NewSpinner(containerRuntimeInitMessage)
	defer s.Stop()

	nonAstroMachineName := rt.isAnotherMachineRunning()
	if nonAstroMachineName != "" && rt.OSChecker.IsMac() {
		if err := rt.getAndConfigureMachineForUsage(nonAstroMachineName); err != nil {
			return err
		}

		containers, err := rt.Engine.ListContainers()
		if err != nil {
			return err
		}

		if len(containers) > 0 {
			return errors.New(podmanMachineAlreadyRunningErrMsg)
		}

		s.Start()
		err = rt.Engine.StopMachine(nonAstroMachineName)
		if err != nil {
			return err
		}
	}

	// Use the effective machine name.
	machineName := rt.getMachineName()
	machine := rt.getAstroMachine() // getAstroMachine still uses podmanMachineName; adjust if needed.
	if machine != nil {
		iMachine, err := rt.Engine.InspectMachine(machineName)
		if err != nil {
			return err
		}
		if iMachine.State == podmanStatusRunning {
			return rt.configureMachineForUsage(iMachine)
		}
		if iMachine.State == podmanStatusStopped {
			s.Start()
			if err := rt.Engine.StartMachine(machineName); err != nil {
				return err
			}
			return rt.configureMachineForUsage(iMachine)
		}
	}

	s.Start()
	time.Sleep(1 * time.Second)
	if err := rt.Engine.InitializeMachine(machineName, s); err != nil {
		return err
	}
	sp.StopWithCheckmark(s, "Astro machine initialized")

	return rt.getAndConfigureMachineForUsage(machineName)
}

// stopAndKillMachine attempts to stop and kill the machine.
func (rt PodmanRuntime) stopAndKillMachine() error {
	if !rt.astroMachineExists() {
		return nil
	}

	if rt.astroMachineIsRunning() {
		containers, err := rt.Engine.ListContainers()
		if err != nil {
			return err
		}
		projectNames := make(map[string]struct{})
		for _, item := range containers {
			if projectName, exists := item.Labels[composeProjectLabel]; exists {
				projectNames[projectName] = struct{}{}
			}
		}
		if len(projectNames) > 0 {
			return nil
		}
		err := rt.Engine.StopMachine(rt.getMachineName())
		if err != nil {
			return err
		}
	}

	err := rt.Engine.RemoveMachine(rt.getMachineName())
	if err != nil {
		return err
	}

	return nil
}

// getDockerHost returns the Docker host value based on the machine details and OS.
func (rt PodmanRuntime) getDockerHost(machine *types.InspectedMachine) string {
	if rt.OSChecker.IsWindows() {
		return "npipe:////./pipe/podman-" + machine.Name
	}
	return "unix://" + machine.ConnectionInfo.PodmanSocket.Path
}

// configureMachineForUsage sets the DOCKER_HOST variable and default connection.
func (rt PodmanRuntime) configureMachineForUsage(machine *types.InspectedMachine) error {
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}

	dockerHost := rt.getDockerHost(machine)
	err := os.Setenv("DOCKER_HOST", dockerHost)
	if err != nil {
		return fmt.Errorf("error setting DOCKER_HOST: %s", err)
	}
	return rt.Engine.SetMachineAsDefault(machine.Name)
}

// getAndConfigureMachineForUsage gets our machine then configures it.
func (rt PodmanRuntime) getAndConfigureMachineForUsage(name string) error {
	machine, err := rt.Engine.InspectMachine(name)
	if err != nil {
		return err
	}
	return rt.configureMachineForUsage(machine)
}

// getAstroMachine gets our machine; adjust this if you need to support non-astro machines.
func (rt PodmanRuntime) getAstroMachine() *types.ListedMachine {
	machines, _ := rt.Engine.ListMachines()
	return findMachineByName(machines, podmanMachineName)
}

// astroMachineExists checks if our machine exists.
func (rt PodmanRuntime) astroMachineExists() bool {
	machine := rt.getAstroMachine()
	return machine != nil
}

// astroMachineIsRunning checks if our machine is running.
func (rt PodmanRuntime) astroMachineIsRunning() bool {
	machine := rt.getAstroMachine()
	return machine != nil && machine.Running
}

// isAnotherMachineRunning checks if another, non-default machine is running.
func (rt PodmanRuntime) isAnotherMachineRunning() string {
	machines, _ := rt.Engine.ListMachines()
	for _, machine := range machines {
		if machine.Running && machine.Name != podmanMachineName {
			return machine.Name
		}
	}
	return ""
}

// findMachineByName finds a machine by name from a list.
func findMachineByName(items []types.ListedMachine, name string) *types.ListedMachine {
	for _, item := range items {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

// isDockerHostSet checks if DOCKER_HOST is set.
func isDockerHostSet() bool {
	return os.Getenv("DOCKER_HOST") != ""
}

// isDockerHostSetToAstroMachine checks if DOCKER_HOST points to the default machine.
func isDockerHostSetToAstroMachine() bool {
	return strings.Contains(os.Getenv("DOCKER_HOST"), podmanMachineName)
}

// NewDockerClient creates a new Docker client configured to communicate with the machine.
// It calls ensureMachine() to guarantee the machine exists and logs debug information.
func (rt PodmanRuntime) NewDockerClient() (*client.Client, error) {
	logrus.Debug("Starting NewDockerClient")
	if err := rt.ensureMachine(); err != nil {
		logrus.Debugf("Failed to ensure machine: %v", err)
		return nil, fmt.Errorf("failed to ensure machine: %w", err)
	}

	machineName := rt.getMachineName()
	logrus.Debugf("Inspecting machine: %s", machineName)
	machine, err := rt.Engine.InspectMachine(machineName)
	if err != nil {
		logrus.Debugf("Failed to inspect machine %s: %v", machineName, err)
		return nil, fmt.Errorf("failed to inspect machine %s: %w", machineName, err)
	}
	if machine == nil {
		logrus.Debugf("Machine %s not found after ensureMachine", machineName)
		return nil, fmt.Errorf("machine %s not found", machineName)
	}

	dockerHost := rt.getDockerHost(machine)
	logrus.Debugf("Docker host resolved to: %s", dockerHost)

	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logrus.Debugf("Failed to create Docker client with host %s: %v", dockerHost, err)
		return nil, err
	}
	logrus.Debug("Successfully created Docker client")
	return cli, nil
}
