package container

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	podmanMachineName   = "astro-machine"
	podmanStatusRunning = "running"
	podmanStatusStopped = "stopped"
	composeProjectLabel = "com.docker.compose.project"

	machineAlreadyRunningMsg = "astro needs a podman machine to run your project, " +
		"but it looks like a machine is already running. " +
		"Mac hosts are limited to one running machine at a time. " +
		"Please stop the other machine and try again"
)

// PodmanEngine is the set of Podman operations the Manager relies on. It is an
// interface so tests can substitute a fake without a live Podman install.
type PodmanEngine interface {
	InitializeMachine(name string, cfg Config, fb Feedback) error
	StartMachine(name string) error
	StopMachine(name string) error
	RemoveMachine(name string) error
	InspectMachine(name string) (*InspectedMachine, error)
	SetMachineAsDefault(name string) error
	ListMachines() ([]ListedMachine, error)
	ListContainers() ([]ListedContainer, error)
}

// podmanEngine is the default PodmanEngine that shells out to the `podman` CLI.
type podmanEngine struct{}

func (podmanEngine) InitializeMachine(name string, cfg Config, fb Feedback) error {
	args := []string{"machine", "init", name}
	if cfg.MachineMemory != "" {
		args = append(args, "--memory", cfg.MachineMemory)
	}
	if cfg.MachineCPU != "" {
		args = append(args, "--cpus", cfg.MachineCPU)
	}
	args = append(args, "--now")

	cmd := command{binary: podman, args: args}
	err := cmd.executeWithProgress(func(line string) {
		switch {
		case strings.Contains(line, "Looking up Podman Machine image"),
			strings.Contains(line, "Getting image source signatures"),
			strings.Contains(line, "Copying blob"):
			fb.Update("Downloading Astro machine image…")
		default:
			fb.Update("Starting Astro machine…")
		}
	})
	if err != nil {
		return fmt.Errorf("error initializing machine: %w", err)
	}
	return nil
}

func (podmanEngine) StartMachine(name string) error {
	out, err := (&command{binary: podman, args: []string{"machine", "start", name}}).execute()
	if err != nil {
		return errorFromOutput("error starting machine: ", out)
	}
	return nil
}

func (podmanEngine) StopMachine(name string) error {
	out, err := (&command{binary: podman, args: []string{"machine", "stop", name}}).execute()
	if err != nil {
		return errorFromOutput("error stopping machine: ", out)
	}
	return nil
}

func (podmanEngine) RemoveMachine(name string) error {
	out, err := (&command{binary: podman, args: []string{"machine", "rm", "-f", name}}).execute()
	if err != nil {
		return errorFromOutput("error removing machine: ", out)
	}
	return nil
}

func (podmanEngine) InspectMachine(name string) (*InspectedMachine, error) {
	out, err := (&command{binary: podman, args: []string{"machine", "inspect", name}}).execute()
	if err != nil {
		return nil, errorFromOutput("error inspecting machine: ", out)
	}
	var machines []InspectedMachine
	if err := json.Unmarshal([]byte(out), &machines); err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, fmt.Errorf("machine not found: %s", name)
	}
	return &machines[0], nil
}

func (podmanEngine) SetMachineAsDefault(name string) error {
	out, err := (&command{binary: podman, args: []string{"system", "connection", "default", name}}).execute()
	if err != nil {
		return errorFromOutput("error setting default connection: ", out)
	}
	return nil
}

func (podmanEngine) ListMachines() ([]ListedMachine, error) {
	out, err := (&command{binary: podman, args: []string{"machine", "ls", "--format", "json"}}).execute()
	if err != nil {
		return nil, errorFromOutput("error listing machines: ", out)
	}
	var machines []ListedMachine
	if err := json.Unmarshal([]byte(out), &machines); err != nil {
		return nil, err
	}
	return machines, nil
}

func (podmanEngine) ListContainers() ([]ListedContainer, error) {
	out, err := (&command{binary: podman, args: []string{"ps", "--format", "json"}}).execute()
	if err != nil {
		return nil, errorFromOutput("error listing containers: ", out)
	}
	var containers []ListedContainer
	if err := json.Unmarshal([]byte(out), &containers); err != nil {
		return nil, err
	}
	return containers, nil
}
