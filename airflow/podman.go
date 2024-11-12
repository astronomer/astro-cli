package airflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/briandowns/spinner"
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
	State          string
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
func (p *Command) Execute(suffix string) (string, error) {
	cmd := exec.Command(p.Command, p.Args...)
	var out bytes.Buffer
	s := spinner.New(spinner.CharSets[14], 200*time.Millisecond) // Use a character set and speed
	s.Suffix = suffix
	s.Start()
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	s.Stop()
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

func setDockerHost(machine *Machine) error {
	if machine == nil {
		return fmt.Errorf("Machine does not exist")
	}
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	fmt.Println("Setting DOCKER_HOST to", dockerHost)
	err := os.Setenv("DOCKER_HOST", dockerHost)
	return err
}

func StartPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "start", name},
	}
	output, err := podmanCmd.Execute(" starting podman machine...")
	if err != nil {
		return fmt.Errorf("error stopping Podman machine: %s, output: %s", err, output)
	}
	return nil
}

// StopPodmanMachine stops the Podman machine.
func StopPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "stop", name},
	}
	output, err := podmanCmd.Execute(" stopping podman machine...")
	if err != nil {
		return fmt.Errorf("error stopping Podman machine: %s, output: %s", err, output)
	}
	return nil
}

func deletePodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "rm", "-f", name},
	}
	output, err := podmanCmd.Execute(" removing podman machine...")
	if err != nil {
		return fmt.Errorf("error removing Podman machine: %s, output: %s", err, output)
	}
	return nil
}

// InspectPodmanMachine inspects a given podman machine name.
func InspectPodmanMachine(machineName string) (*Machine, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "inspect", machineName},
	}
	output, err := podmanCmd.Execute("")
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
	output, err := podmanCmd.Execute("Listing podman machine...")
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

func InitPodmanMachineCMD() error {
	projectName, err := ProjectNameUnique()
	if err != nil {
		return fmt.Errorf("error retrieving working directory: %w", err)
	}
	machineName := "astro-" + formatMachineName(projectName)

	machine, err := InspectPodmanMachine(machineName)
	if machine != nil {
		if machine.State == "stopped" {
			err = StartPodmanMachine(machineName)
			if err != nil {
				return fmt.Errorf("error starting Podman machine: %s", err)
			}
		}
		setDockerHost(machine)
		return nil
	}

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "init", machineName, "--now"},
	}
	output, err := podmanCmd.Execute(" initializing podman machine")
	if err != nil {
		return fmt.Errorf("error starting Podman machine: %s, output: %s", err, output)
	}

	machine, err = InspectPodmanMachine(machineName)
	if err != nil {
		return err
	}
	setDockerHost(machine)
	return nil
}

func SetPodmanDockerHost() error {
	projectName, err := ProjectNameUnique()
	if err != nil {
		return fmt.Errorf("error retrieving working directory: %w", err)
	}
	machineName := "astro-" + formatMachineName(projectName)
	machine, err := InspectPodmanMachine(machineName)
	if err != nil {
		return err
	}
	setDockerHost(machine)
	return nil

}

func StopAndKillPodmanMachine() error {
	projectName, err := ProjectNameUnique()
	if err != nil {
		return fmt.Errorf("error retrieving working directory: %w", err)
	}
	machineName := "astro-" + formatMachineName(projectName)

	err = StopPodmanMachine(machineName)
	if err != nil {
		return err
	}

	err = deletePodmanMachine(machineName)
	if err != nil {
		return err
	}

	return nil
}
