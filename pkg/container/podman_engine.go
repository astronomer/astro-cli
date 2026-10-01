package container

import (
	"encoding/json"
	"fmt"
)

// PodmanEngine is the set of read-only Podman queries the Manager relies on to
// find the user's running machine. It is an interface so tests can substitute a
// fake without a live Podman install.
type PodmanEngine interface {
	InspectMachine(name string) (*InspectedMachine, error)
	ListMachines() ([]ListedMachine, error)
}

// podmanEngine is the default PodmanEngine that shells out to the `podman` CLI.
type podmanEngine struct{}

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
