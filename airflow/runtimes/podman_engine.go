package runtimes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/airflow/runtimes/types"
	"github.com/astronomer/astro-cli/config"
)

const (
	podmanStatusRunning   = "running"
	podmanStatusStopped   = "stopped"
	composeProjectLabel   = "com.docker.compose.project"
	podmanInitSlowMessage = " Sorry, this is taking a bit longer than expected.\n " +
		" This initial download will be cached once finished.\n "
	podmanMachineAlreadyRunningErrMsg = "astro needs a podman machine to run your project, " +
		"but it looks like a machine is already running. " +
		"Mac hosts are limited to one running machine at a time. " +
		"Please stop the other machine and try again"
)

// podmanEngine is the default implementation of PodmanEngine.
type podmanEngine struct{}

// InitializeMachine initializes our astro Podman machine.
func (e podmanEngine) InitializeMachine(name string) error {
	// Grab some optional configurations from the config file.
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"init",
			name,
			"--memory",
			config.CFG.MachineMemory.GetString(),
			"--cpus",
			config.CFG.MachineCPU.GetString(),
			"--now",
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error initializing machine: %s", output)
	}
	return nil
}

// StartMachine starts our astro Podman machine.
func (e podmanEngine) StartMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"start",
			name,
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error starting machine: %s", output)
	}
	return nil
}

// StopMachine stops the given Podman machine.
func (e podmanEngine) StopMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"stop",
			name,
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error stopping machine: %s", output)
	}
	return nil
}

// RemoveMachine removes the given Podman machine completely,
// such that it can only be started again by re-initializing.
func (e podmanEngine) RemoveMachine(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"rm",
			"-f",
			name,
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error removing machine: %s", output)
	}
	return nil
}

// InspectMachine inspects a given podman machine name.
func (e podmanEngine) InspectMachine(name string) (*types.InspectedMachine, error) {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"inspect",
			name,
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error inspecting machine: %s", output)
	}

	var machines []types.InspectedMachine
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
func (e podmanEngine) SetMachineAsDefault(name string) error {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"system",
			"connection",
			"default",
			name,
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error setting default connection: %s", output)
	}
	return nil
}

// ListMachines lists all Podman machines.
func (e podmanEngine) ListMachines() ([]types.ListedMachine, error) {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"machine",
			"ls",
			"--format",
			"json",
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error listing machines: %s", output)
	}
	var machines []types.ListedMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

// ListContainers lists all pods in the machine.
func (e podmanEngine) ListContainers() ([]types.ListedContainer, error) {
	podmanCmd := Command{
		Command: podman,
		Args: []string{
			"ps",
			"--format",
			"json",
		},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error listing containers: %s", output)
	}
	var containers []types.ListedContainer
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
