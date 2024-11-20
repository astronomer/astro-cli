package airflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/astronomer/astro-cli/cmd/utils"
	"os"
	"os/exec"
	"time"

	"github.com/briandowns/spinner"
)

var (
	podmanMachineName = "astro"
	spinnerCharSet    = spinner.CharSets[14]
	spinnerRefresh    = 100 * time.Millisecond
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

type ListMachine struct {
	Name     string
	Running  bool
	Starting bool
	LastUp   string
}

type InspectMachine struct {
	Name           string
	ConnectionInfo ConnectionInfo
	State          string
}

type Container struct {
	Name   string
	Labels map[string]string
}

// Execute runs the Podman command and returns the output.
func (p *Command) Execute() (string, error) {
	cmd := exec.Command(p.Command, p.Args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func InitPodmanMachine() error {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = " Astro uses container technology to run your Airflow project. " +
		"Please wait while we get things ready. " +
		"This may take a few moments..."
	defer s.Stop()

	// Check if another, non-astro Podman machine is running
	nonAstroMachine := IsAnotherPodmanMachineRunning()
	if nonAstroMachine != "" && utils.IsMac() {
		// If there is another machine running, and it has no running containers, stop it.
		// Otherwise, we assume the user has some other project running that we don't want to interfere with.
		containers, err := ListContainers()
		if err != nil {
			return err
		}
		if len(containers) > 0 {
			return fmt.Errorf("astro needs a podman machine to run your project, " +
				"but it looks like a machine is already running. " +
				"Mac hosts are limited to one running machine at a time. " +
				"Please stop the other machine and try again")
		}
		s.Start()
		err = StopPodmanMachine(nonAstroMachine)
		if err != nil {
			return err
		}
	}

	// Check if our astro Podman machine is running
	machine := GetPodmanMachine()

	// If the machine exists, inspect it and decide what to do
	if machine != nil {
		m, err := InspectPodmanMachine()
		if err != nil {
			return err
		}
		if m.State == "running" {
			err = ConfigureMachineForUsage(m)
			if err != nil {
				return err
			}
			return nil
		}
		if m.State == "stopped" {
			s.Start()
			err = StartPodmanMachine(podmanMachineName)
			if err != nil {
				return err
			}
			err = ConfigureMachineForUsage(m)
			if err != nil {
				return err
			}
			return nil
		}
	}

	// Otherwise, initialize the machine
	s.Start()
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "init", podmanMachineName, "--memory", "4096", "--now"},
	}
	_, err := podmanCmd.Execute()
	if err != nil {
		return err
	}

	m, err := InspectPodmanMachine()
	if err != nil {
		return err
	}
	err = ConfigureMachineForUsage(m)
	if err != nil {
		return err
	}
	return nil
}

func StartPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "start", name},
	}
	_, err := podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error starting machine: %s", err)
	}
	return nil
}

// StopPodmanMachine stops the Podman machine.
func StopPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "stop", name},
	}
	_, err := podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error stopping podman machine: %s", err)
	}
	return nil
}

func RemovePodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "rm", "-f", name},
	}
	_, err := podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error removing podman machine: %s", err)
	}
	return nil
}

// InspectPodmanMachine inspects a given podman machine name.
func InspectPodmanMachine() (*InspectMachine, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "inspect", podmanMachineName},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, fmt.Errorf("error inspecting podman machine: %s, output: %s", err, output)
	}

	var machines []InspectMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, fmt.Errorf("machine not found: %s", podmanMachineName)
	}

	return &machines[0], nil
}

// ListPodmanMachines lists all Podman machines.
func ListPodmanMachines() ([]ListMachine, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "ls", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, fmt.Errorf("error listing podman machines: %s", err)
	}
	var machines []ListMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

// ListContainers lists all pods in the machine.
func ListContainers() ([]Container, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"ps", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, fmt.Errorf("error listing podman containers: %s", err)
	}
	var containers []Container
	err = json.Unmarshal([]byte(output), &containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func FindMachineByName(items []ListMachine, name string) *ListMachine {
	for _, item := range items {
		if item.Name == name {
			return &item // Return a pointer to the found item
		}
	}
	return nil // Return nil if no item was found
}

func StopAndKillPodmanMachine() error {
	if DoesPodmanMachineExist() {
		if IsPodmanMachineRunning() {
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
			// we are checking to see if any additional projects are running
			if len(projectNames) > 0 {
				return nil
			}

			err = StopPodmanMachine(podmanMachineName)
			if err != nil {
				return err
			}
		}

		err := RemovePodmanMachine(podmanMachineName)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConfigureMachineForUsage(machine *InspectMachine) error {
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	err := os.Setenv("DOCKER_HOST", dockerHost)

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"system", "connection", "default", machine.Name},
	}
	_, err = podmanCmd.Execute()
	if err != nil {
		return fmt.Errorf("error configuring default podman connection: %s", err)
	}

	return err
}

func GetAndConfigureMachineForUsage() error {
	machine, err := InspectPodmanMachine()
	if err != nil {
		return err
	}
	if err := ConfigureMachineForUsage(machine); err != nil {
		return err
	}
	return nil
}

func DoesPodmanMachineExist() bool {
	machines, _ := ListPodmanMachines()
	machine := FindMachineByName(machines, podmanMachineName)
	return machine != nil
}

func IsPodmanMachineRunning() bool {
	machines, _ := ListPodmanMachines()
	machine := FindMachineByName(machines, podmanMachineName)
	return machine != nil && machine.Running == true
}

func IsAnotherPodmanMachineRunning() string {
	machines, _ := ListPodmanMachines()
	for _, machine := range machines {
		if machine.Running && machine.Name != podmanMachineName {
			return machine.Name
		}
	}
	return ""
}

func GetPodmanMachine() *ListMachine {
	machines, _ := ListPodmanMachines()
	machine := FindMachineByName(machines, podmanMachineName)
	return machine
}
