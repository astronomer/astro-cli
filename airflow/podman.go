package airflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/cmd/utils"

	"github.com/briandowns/spinner"
)

const (
	PodmanStatusRunning = "running"
	PodmanStatusStopped = "stopped"
	composeProjectLabel = "com.docker.compose.project"
	podmanInitMessage   = " Astro uses container technology to run your Airflow project. " +
		"Please wait while we get things started..."
	podmanInitSlowMessage = " Sorry for the wait, this is taking a bit longer than expected. " +
		"This initial download will be cached once finished."
	podmanMachineAlreadyRunningErrMsg = "astro needs a podman machine to run your project, " +
		"but it looks like a machine is already running. " +
		"Mac hosts are limited to one running machine at a time. " +
		"Please stop the other machine and try again"
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

// ListMachine contains information about a Podman machine
// as it is provided from the `podman machine ls --format json` command.
type ListMachine struct {
	Name     string
	Running  bool
	Starting bool
	LastUp   string
}

// InspectMachine contains information about a Podman machine
// as it is provided from the `podman machine inspect` command.
type InspectMachine struct {
	Name           string
	ConnectionInfo struct {
		PodmanSocket struct {
			Path string
		}
	}
	State string
}

// ListContainer contains information about a Podman container
// as it is provided from the `podman ps --format json` command.
type ListContainer struct {
	Name   string
	Labels map[string]string
}

// Execute runs the Podman command and returns the output.
func (p *Command) Execute() (string, error) {
	cmd := exec.Command(p.Command, p.Args...) //nolint:gosec
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func InitPodmanMachine() error {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = podmanInitMessage
	defer s.Stop()

	go func() {
		<-time.After(1 * time.Minute)
		s.Suffix = podmanInitSlowMessage
	}()

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
			return errors.New(podmanMachineAlreadyRunningErrMsg)
		}
		s.Start()
		err = StopPodmanMachine(nonAstroMachine)
		if err != nil {
			return err
		}
	}

	// Check if our astro Podman machine is running
	machine := GetAstroPodmanMachine()

	// If the machine exists, inspect it and decide what to do
	if machine != nil {
		m, err := InspectPodmanMachine()
		if err != nil {
			return err
		}
		if m.State == PodmanStatusRunning {
			err = ConfigureMachineForUsage(m)
			if err != nil {
				return err
			}
			return nil
		}
		if m.State == PodmanStatusStopped {
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
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error initializing machine: %s", output)
	}

	return GetAndConfigureMachineForUsage()
}

func StartPodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "start", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error starting machine: %s", output)
	}
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
		return ErrorFromOutput("error stopping machine: %s", output)
	}
	return nil
}

func RemovePodmanMachine(name string) error {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"machine", "rm", "-f", name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error removing machine: %s", output)
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
		return nil, ErrorFromOutput("error inspecting machine: %s", output)
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
		return nil, ErrorFromOutput("error listing machines: %s", output)
	}
	var machines []ListMachine
	err = json.Unmarshal([]byte(output), &machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

// ListContainers lists all pods in the machine.
func ListContainers() ([]ListContainer, error) {
	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"ps", "--format", "json"},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return nil, ErrorFromOutput("error listing containers: %s", output)
	}
	var containers []ListContainer
	err = json.Unmarshal([]byte(output), &containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

// StopAndKillPodmanMachine attempts to stop and kill the Podman machine.
// If other projects are running, it will leave the machine up.
func StopAndKillPodmanMachine() error {
	// If the machine doesn't exist, exist early.
	if !AstroPodmanMachineExists() {
		return nil
	}

	// If the machine exists, and its running, we need to check
	// if any other projects are running. If other projects are running,
	// we'll leave the machine up, otherwise we stop and kill it.
	if AstroPodmanMachineIsRunning() {
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
		err = StopPodmanMachine(podmanMachineName)
		if err != nil {
			return err
		}
	}

	// If we make it here, the machine was already stopped, or was just stopped above.
	// We can now remove it.
	err := RemovePodmanMachine(podmanMachineName)
	if err != nil {
		return err
	}

	return nil
}

func ConfigureMachineForUsage(machine *InspectMachine) error {
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	err := os.Setenv("DOCKER_HOST", dockerHost)
	if err != nil {
		return fmt.Errorf("error setting DOCKER_HOST: %s", err)
	}

	podmanCmd := Command{
		Command: "podman",
		Args:    []string{"system", "connection", "default", machine.Name},
	}
	output, err := podmanCmd.Execute()
	if err != nil {
		return ErrorFromOutput("error configuring default connection: %s", output)
	}

	return nil
}

func GetAndConfigureMachineForUsage() error {
	machine, err := InspectPodmanMachine()
	if err != nil {
		return err
	}
	return ConfigureMachineForUsage(machine)
}

func FindMachineByName(items []ListMachine, name string) *ListMachine {
	for _, item := range items {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

func GetAstroPodmanMachine() *ListMachine {
	machines, _ := ListPodmanMachines()
	return FindMachineByName(machines, podmanMachineName)
}

func AstroPodmanMachineExists() bool {
	machine := GetAstroPodmanMachine()
	return machine != nil
}

func AstroPodmanMachineIsRunning() bool {
	machine := GetAstroPodmanMachine()
	return machine != nil && machine.Running
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
