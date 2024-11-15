package airflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

type Container struct {
	Name   string
	Labels map[string]string
}

// Execute runs the Podman command and returns the output.
func (p *Command) Execute(message, finalMessage string) (string, error) {
	cmd := exec.Command(p.Command, p.Args...)
	var out bytes.Buffer
	s := spinner.New(spinner.CharSets[14], 200*time.Millisecond) // Use a character set and speed
	s.Suffix = " " + message
	if finalMessage != "" {
		s.FinalMSG = finalMessage + "\n"
	}
	s.Start()
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	s.Stop()
	return out.String(), err
}

func setDockerHost(machine *Machine) error {
	if machine == nil {
		return fmt.Errorf("Machine does not exist")
	}
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	err := os.Setenv("DOCKER_HOST", dockerHost)

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"system", "connection", "default", machine.Name},
	}
	output, err := podmanCmd.Execute("", "")
	if err != nil {
		return fmt.Errorf("error initalizing Podman machine: %s, output: %s", err, output)
	}

	return err
}

func StartPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "start", name},
	}
	output, err := podmanCmd.Execute("Starting machine...", "Machine started successfully.")
	if err != nil {
		if strings.Contains(output, "VM already running or starting") {
			return fmt.Errorf("please stop the existing running Podman machine and restart the astro project")
		}
		return fmt.Errorf("error starting Podman machine: %s, output: %s", err, output)
	}
	return nil
}

// StopPodmanMachine stops the Podman machine.
func StopPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "stop", name},
	}
	output, err := podmanCmd.Execute("Stopping machine...", "Machine stopped successfully.")
	if err != nil {
		return fmt.Errorf("error stopping Podman machine: %s, output: %s", err, output)
	}
	return nil
}

// ListContainers lists all pods in the machine.
func ListContainers() ([]Container, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"ps", "--format", "json"},
	}
	output, err := podmanCmd.Execute("", "")
	if err != nil {
		return nil, fmt.Errorf("error listing Podman containers: %s, output: %s", err, output)
	}
	var containers []Container
	err = json.Unmarshal([]byte(output), &containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func deletePodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "rm", "-f", name},
	}
	output, err := podmanCmd.Execute("Removing machine...", "Machine removed successfully.")
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
	output, err := podmanCmd.Execute("", "")
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
	output, err := podmanCmd.Execute("", "")
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

func InitPodmanMachine() error {
	machineName := "astro"

	machines, err := ListPodmanMachines()
	if err != nil {
		return err
	}
	machine := FindMachineByName(machines, machineName)

	if machine != nil {
		machine, err = InspectPodmanMachine(machineName)
		if err != nil {
			return err
		}
		// TODO: Handle all possible states
		if machine.State == "running" {
			setDockerHost(machine)
			return nil
		}
		if machine.State == "stopped" {
			err = StartPodmanMachine(machineName)
			if err != nil {
				return fmt.Errorf("error starting Podman machine: %s", err)
			}
			setDockerHost(machine)
			return nil
		}
	}

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "init", machineName, "--memory", "4096", "--now"},
	}
	output, err := podmanCmd.Execute("Astro uses container technology to run your Airflow project. "+
		"Please wait while we get things ready. This may take a few moments...", "Machine initialized successfully.")
	if err != nil {
		if strings.Contains(output, "VM already running or starting") {
			return fmt.Errorf("please stop the existing running Podman machine and restart the astro project")
		}
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
	machineName := "astro"
	machine, err := InspectPodmanMachine(machineName)
	if err != nil {
		return err
	}
	setDockerHost(machine)
	return nil

}

func StopAndKillPodmanMachine() error {
	machineName := "astro"

	machines, err := ListPodmanMachines()
	if err != nil {
		return err
	}
	machine := FindMachineByName(machines, machineName)

	if machine != nil {
		containers, err := ListContainers()
		if err != nil {
			return err
		}
		projectNames := make(map[string]struct{})
		for _, item := range containers {
			// Check if "project.name" exists in the Labels map
			if projectName, exists := item.Labels["com.docker.compose.project"]; exists {
				// Add the project name to the map (map keys are unique)
				projectNames[projectName] = struct{}{}
			}
		}
		// At this point our project has already been stopped, and
		// we are checking to see if any additional proejcts are running
		if len(projectNames) > 0 {
			return nil
		}

		err = StopPodmanMachine(machineName)
		if err != nil {
			return err
		}

		err = deletePodmanMachine(machineName)
		if err != nil {
			return err
		}
	}

	return nil
}

func IsPodmanMachineRunning(machineName string) bool {
	// List the running podman machines and find the one corresponding to this project.
	machines, _ := ListPodmanMachines()
	machine := FindMachineByName(machines, machineName)
	return machine != nil && machine.State == "running"
}
