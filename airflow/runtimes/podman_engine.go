package runtimes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/config"
)

const (
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

type PodmanEngine interface {
	InitializeMachine(name string) error
	StartMachine(name string) error
	StopMachine(name string) error
	RemoveMachine(name string) error
	InspectMachine(name string) (*InspectedMachine, error)
	SetMachineAsDefault(name string) error
	ListMachines() ([]ListedMachine, error)
	ListContainers() ([]ListedContainer, error)
}

type DefaultPodmanEngine struct{}

// InitializeMachine initializes our astro Podman machine.
func (e DefaultPodmanEngine) InitializeMachine(name string) error {
	podmanMachineMemory := config.CFG.PodmanMEM.GetString()
	podmanMachineCpu := config.CFG.PodmanCPU.GetString()
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"machine", "init", name, "--memory", podmanMachineMemory, "--cpus", podmanMachineCpu, "--now"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error initializing machine: %s", output)
	}
	return nil
}

// StartMachine starts our astro Podman machine.
func (e DefaultPodmanEngine) StartMachine(name string) error {
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
func (e DefaultPodmanEngine) StopMachine(name string) error {
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
func (e DefaultPodmanEngine) RemoveMachine(name string) error {
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
func (e DefaultPodmanEngine) InspectMachine(name string) (*InspectedMachine, error) {
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

// SetMachineAsDefault sets the given Podman machine as the default.
func (e DefaultPodmanEngine) SetMachineAsDefault(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args:    []string{"system", "connection", "default", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error setting default connection: %s", output)
	}
	return nil
}

// ListMachines lists all Podman machines.
func (e DefaultPodmanEngine) ListMachines() ([]ListedMachine, error) {
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
func (e DefaultPodmanEngine) ListContainers() ([]ListedContainer, error) {
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

// ErrorFromOutput returns an error from the output string.
// This is used to extract the meaningful error message from podman command output.
func ErrorFromOutput(prefix, output string) error {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Error: ") {
			errMsg := strings.Trim(strings.TrimSpace(line), "Error: ") //nolint
			return fmt.Errorf(prefix, errMsg)
		}
	}
	// If we didn't find an error message, return the entire output
	errMsg := strings.TrimSpace(output)
	return fmt.Errorf(prefix, errMsg)
}
