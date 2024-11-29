package runtimes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
)

const (
	podmanMachineName     = "astro-machine"
	podmanMachineMemory   = "4096" // 4GB
	podmanStatusRunning   = "running"
	podmanStatusStopped   = "stopped"
	composeProjectLabel   = "com.docker.compose.project"
	podmanInitSlowMessage = " Sorry for the wait, this is taking a bit longer than expected. " +
		"This initial download will be cached once finished."
	podmanMachineAlreadyRunningErrMsg = "astro needs a podman machine to run your project, " +
		"but it looks like a machine is already running. " +
		"Mac hosts are limited to one running machine at a time. " +
		"Please stop the other machine and try again"
)

// ListedMachine contains information about a Podman machine
// as it is provided from the `podman machine ls --format json` command.
type ListedMachine struct {
	Name     string
	Running  bool
	Starting bool
	LastUp   string
}

// InspectedMachine contains information about a Podman machine
// as it is provided from the `podman machine inspect` command.
type InspectedMachine struct {
	Name           string
	ConnectionInfo struct {
		PodmanSocket struct {
			Path string
		}
	}
	State string
}

// ListedContainer contains information about a Podman container
// as it is provided from the `podman ps --format json` command.
type ListedContainer struct {
	Name   string
	Labels map[string]string
}

func InitializeMachine() error {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = containerRuntimeInitMessage
	defer s.Stop()

	go func() {
		<-time.After(1 * time.Minute)
		s.Suffix = podmanInitSlowMessage
	}()

	// Check if another, non-astro Podman machine is running
	nonAstroMachineName := IsAnotherMachineRunning()
	// If there is another machine running, and it has no running containers, stop it.
	// Otherwise, we assume the user has some other project running that we don't want to interfere with.
	if nonAstroMachineName != "" && isMac() {
		// First, configure the other running machine for usage.
		if err := GetAndConfigureMachineForUsage(nonAstroMachineName); err != nil {
			return err
		}

		// Then check the machine for running containers.
		containers, err := ListContainers()
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
		err = StopMachine(nonAstroMachineName)
		if err != nil {
			return err
		}
	}

	// Check if our astro Podman machine exists.
	machine := GetAstroMachine()

	// If the machine exists, inspect it and decide what to do.
	if machine != nil {
		// Inspect the machine and get its details.
		iMachine, err := InspectMachine(podmanMachineName)
		if err != nil {
			return err
		}

		// If the machine is already running,
		// just go ahead and configure it for usage.
		if iMachine.State == podmanStatusRunning {
			return ConfigureMachineForUsage(iMachine)
		}

		// If the machine is stopped,
		// start it, then configure it for usage.
		if iMachine.State == podmanStatusStopped {
			s.Start()
			if err = StartMachine(podmanMachineName); err != nil {
				return err
			}
			return ConfigureMachineForUsage(iMachine)
		}
	}

	// Otherwise, initialize the machine
	s.Start()
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "init", podmanMachineName, "--memory", podmanMachineMemory, "--now"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error initializing machine: %s", output)
	}

	return GetAndConfigureMachineForUsage(podmanMachineName)
}

// StartMachine starts our astro Podman machine.
func StartMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "start", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error starting machine: %s", output)
	}
	return nil
}

// StopMachine stops the given Podman machine.
func StopMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "stop", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error stopping machine: %s", output)
	}
	return nil
}

// RemoveMachine removes the given Podman machine completely,
// such that it can only be started again by re-initializing.
func RemoveMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "rm", "-f", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error removing machine: %s", output)
	}
	return nil
}

// InspectMachine inspects a given podman machine name.
func InspectMachine(name string) (*InspectedMachine, error) {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "inspect", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error inspecting machine: %s", output)
	}

	var machines []InspectedMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, fmt.Errorf("machine not found: %s", name)
	}

	return &machines[0], nil
}

// ListMachines lists all Podman machines.
func ListMachines() ([]ListedMachine, error) {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "ls", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error listing machines: %s", output)
	}
	var machines []ListedMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

// ListContainers lists all pods in the machine.
func ListContainers() ([]ListedContainer, error) {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"ps", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error listing containers: %s", output)
	}
	var containers []ListedContainer
	err = json.Unmarshal([]byte(output), &containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

// StopAndKillMachine attempts to stop and kill the Podman machine.
// If other projects are running, it will leave the machine up.
func StopAndKillMachine() error {
	// If the machine doesn't exist, exist early.
	if !AstroMachineExists() {
		return nil
	}

	// If the machine exists, and its running, we need to check
	// if any other projects are running. If other projects are running,
	// we'll leave the machine up, otherwise we stop and kill it.
	if AstroMachineIsRunning() {
		// Get the containers that are running on our machine.
		containers, err := ListContainers()
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
		err = StopMachine(podmanMachineName)
		if err != nil {
			return err
		}
	}

	// If we make it here, the machine was already stopped, or was just stopped above.
	// We can now remove it.
	err := RemoveMachine(podmanMachineName)
	if err != nil {
		return err
	}

	return nil
}

// ConfigureMachineForUsage does two things:
//   - Sets the DOCKER_HOST environment variable to the machine's socket path
//     This allows the docker compose library to function as expected.
//   - Sets the podman default connection to the machine
//     This allows the podman command to function as expected.
func ConfigureMachineForUsage(machine *InspectedMachine) error {
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}

	if !isWindows() {
		// Set the DOCKER_HOST environment variable for compose.
		dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
		err := os.Setenv("DOCKER_HOST", dockerHost)
		if err != nil {
			return fmt.Errorf("error setting DOCKER_HOST: %s", err)
		}
	}

	// Set the podman default connection to our machine.
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"system", "connection", "default", machine.Name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error configuring default connection: %s", output)
	}

	return nil
}

// GetAndConfigureMachineForUsage gets our astro machine
// then configures the host machine to use it.
func GetAndConfigureMachineForUsage(name string) error {
	machine, err := InspectMachine(name)
	if err != nil {
		return err
	}
	return ConfigureMachineForUsage(machine)
}

// FindMachineByName finds a machine by name from a list of machines.
func FindMachineByName(items []ListedMachine, name string) *ListedMachine {
	for _, item := range items {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

// GetAstroMachine gets our astro podman machine.
func GetAstroMachine() *ListedMachine {
	machines, _ := ListMachines()
	return FindMachineByName(machines, podmanMachineName)
}

// AstroMachineExists checks if our astro podman machine exists.
func AstroMachineExists() bool {
	machine := GetAstroMachine()
	return machine != nil
}

// AstroMachineIsRunning checks if our astro podman machine is running.
func AstroMachineIsRunning() bool {
	machine := GetAstroMachine()
	return machine != nil && machine.Running
}

// IsAnotherMachineRunning checks if another, non-astro podman machine is running.
func IsAnotherMachineRunning() string {
	machines, _ := ListMachines()
	for _, machine := range machines {
		if machine.Running && machine.Name != podmanMachineName {
			return machine.Name
		}
	}
	return ""
}

// ErrorFromOutput returns an error from the output string.
// This is used to extract the meaningful error message from podman command output.
func ErrorFromOutput(prefix, output string) error {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Error: ") {
			errMsg := strings.Trim(strings.TrimSpace(line), "Error: ")
			return fmt.Errorf(prefix, errMsg)
		}
	}
	// If we didn't find an error message, return the entire output
	errMsg := strings.TrimSpace(output)
	return fmt.Errorf(prefix, errMsg)
}

// IsDockerHostSet checks if the DOCKER_HOST environment variable is set.
func IsDockerHostSet() bool {
	return os.Getenv("DOCKER_HOST") != ""
}

// IsDockerHostSetToAstroMachine checks if the DOCKER_HOST environment variable
// is pointing to the astro machine.
func IsDockerHostSetToAstroMachine() bool {
	return strings.Contains(os.Getenv("DOCKER_HOST"), podmanMachineName)
}
