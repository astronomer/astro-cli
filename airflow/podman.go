package airflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Command represents a command to be executed.
type Command struct {
	Command string
	Args    []string
}

type PodmanSocket struct {
	Path string
}

type ConnectionInfo struct {
	PodmanSocket PodmanSocket
}

type Machine struct {
	Name           string
	ConnectionInfo ConnectionInfo
}

// Execute runs the Podman command and returns the output.
//func (p *Command) Execute() (string, error) {
//	err := cmdExec(p.Command, os.Stdout, os.Stderr, p.Args...)
//	if err != nil {
//		return err
//	}
//	return nil
//}

// Execute runs the Podman command and returns the output.
func (p *Command) Execute() (string, error) {
	cmd := exec.Command(p.Command, p.Args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// formatMachineName converts a string to kebab case and removes special characters.
func formatMachineName(s string) string {
	// Convert to lower case
	s = strings.ToLower(s)
	// Replace special characters with hyphens
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	// Trim leading and trailing hyphens
	s = strings.Trim(s, "-")
	return s
}

func SetDockerHost(machine *Machine) error {
	if machine == nil {
		return fmt.Errorf("Machine does not exist")
	}
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	fmt.Println("Setting DOCKER_HOST to", dockerHost)
	err := os.Setenv("DOCKER_HOST", dockerHost)
	return err
}

// StartPodmanMachine starts the Podman machine.
func StartPodmanMachine(name string) error {
	machineName := "astro-" + formatMachineName(name)

	machine, err := InspectPodmanMachine(machineName)
	if machine != nil {
		SetDockerHost(machine)
		return nil
	}

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "init", machineName, "--now"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error starting Podman machine: %s, output: %s", err, output)
	}

	machine, err = InspectPodmanMachine(machineName)
	if err != nil {
		return err
	}
	SetDockerHost(machine)
	return nil
}

// StopPodmanMachine stops the Podman machine.
func StopPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "stop", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error stopping Podman machine: %s, output: %s", err, output)
	}
	return nil
}

// InspectPodmanMachine inspects a given podman machine name.
func InspectPodmanMachine(machineName string) (*Machine, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "inspect", machineName},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, fmt.Errorf("error inspecting Podman machine: %s, output: %s", err, output)
	}

	var machines []Machine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, fmt.Errorf("machine not found: %s", machineName)
	}

	return &machines[0], nil
}

// ListPodmanMachines lists all Podman machines.
func ListPodmanMachines() ([]Machine, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "ls", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, fmt.Errorf("error listing Podman machines: %s, output: %s", err, output)
	}
	var machines []Machine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

func FindMachineByName(items []Machine, name string) *Machine {
	for _, item := range items {
		if item.Name == name {
			return &item // Return a pointer to the found item
		}
	}
	return nil // Return nil if no item was found
}
