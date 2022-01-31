package airflow

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"

	"github.com/compose-spec/compose-go/loader"
	composeTypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	dockerStateUp = "running"

	projectStopTimeout = 5

	// Docker is the docker command.
	Docker = "docker"

	healthCheckBreakPoint = 25 // Maximum number of tries to wait for health check to pass
	healthyProjectStatus  = "health_status: healthy"
	execDieStatus         = "exec_die"
)

type DockerCompose struct {
	airflowHome    string
	projectName    string
	envFile        string
	composeService api.Service
	imageHandler   ImageHandler
}

func DockerComposeInit(airflowHome, envFile string) (*DockerCompose, error) {
	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving working directory")
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing docker client")
	}
	composeService := compose.NewComposeService(dockerClient, &configfile.ConfigFile{})
	imageHandler := DockerImageInit(projectName)

	return &DockerCompose{
		airflowHome:    airflowHome,
		projectName:    projectName,
		envFile:        envFile,
		composeService: composeService,
		imageHandler:   imageHandler,
	}, nil
}

func (d *DockerCompose) Start(dockerfile string) error {
	// Get project containers
	psInfo, err := d.composeService.Ps(context.TODO(), d.projectName, api.PsOptions{All: true})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStatusCheck)
	}

	if len(psInfo) > 0 {
		// Ensure project is not already running
		for idx := range psInfo {
			info := psInfo[idx]
			if checkServiceState(info.State, dockerStateUp) {
				return errors.New("cannot start, project already running")
			}
		}
	}

	// Build this project image
	err = d.imageHandler.Build(".")
	if err != nil {
		return err
	}

	labels, err := d.imageHandler.GetImageLabels()
	if err != nil {
		return err
	}

	// recreate the project, to pass the labels
	project, err := createProject(d.projectName, d.airflowHome, d.envFile, labels)
	if err != nil {
		return err
	}

	// Start up our project
	err = d.composeService.Up(context.TODO(), project, api.UpOptions{})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerRecreate)
	}

	err = d.webserverHealthCheck()
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerRecreate)
	}

	parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
	fmt.Printf(messages.ContainerLinkWebserver+"\n", parts[len(parts)-1])
	fmt.Printf(messages.ContainerLinkPostgres+"\n", config.CFG.PostgresPort.GetString())
	fmt.Printf(messages.ContainerUserPassword + "\n")

	return nil
}

func (d *DockerCompose) Kill() error {
	// Shut down our project
	err := d.composeService.Down(context.TODO(), d.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStop)
	}

	return nil
}

func (d *DockerCompose) Logs(follow bool, containerNames ...string) error {
	psInfo, err := d.composeService.Ps(context.TODO(), d.projectName, api.PsOptions{All: true})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStatusCheck)
	}

	if len(psInfo) == 0 {
		return errors.New("cannot view logs, project not running")
	}

	logger := &ComposeLogger{logger: logrus.New()}
	err = d.composeService.Logs(context.TODO(), d.projectName, logger, api.LogOptions{Services: containerNames, Follow: follow})
	if err != nil {
		return err
	}

	return nil
}

func (d *DockerCompose) Stop() error {
	labels, err := d.imageHandler.GetImageLabels()
	if err != nil {
		return err
	}

	project, err := createProject(d.projectName, d.airflowHome, d.envFile, labels)
	if err != nil {
		return err
	}
	// Pause our project
	stopTimeout := time.Duration(projectStopTimeout)
	err = d.composeService.Stop(context.TODO(), project, api.StopOptions{Timeout: &stopTimeout})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerPause)
	}

	return nil
}

func (d *DockerCompose) PS() error {
	// List project containers
	psInfo, err := d.composeService.Ps(context.TODO(), d.projectName, api.PsOptions{All: true})
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStatusCheck)
	}

	// Columns for table
	infoColumns := []string{"Name", "State", "Ports"}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight) // nolint:gomnd

	// Append data to table
	// Fix this
	fmt.Fprintln(tw, strings.Join(infoColumns, "\t"))
	for idx := range psInfo {
		info := psInfo[idx]
		ports := []string{}
		for _, port := range info.Publishers {
			ports = append(ports, strconv.Itoa(port.PublishedPort))
		}
		data := []string{info.Name, info.State, strings.Join(ports, ",")}
		fmt.Fprintln(tw, strings.Join(data, "\t"))
	}

	// Flush to stdout
	return tw.Flush()
}

func (d *DockerCompose) Run(args []string, user string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}

	execConfig := &types.ExecConfig{
		AttachStdout: true,
		Cmd:          args,
	}
	if user != "" {
		execConfig.User = user
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))
	containerID, err := d.getWebServerContainerID()
	if err != nil {
		return err
	}

	response, err := cli.ContainerExecCreate(context.Background(), containerID, *execConfig)
	if err != nil {
		return errors.New("airflow is not running, Start it with 'astro airflow start'")
	}

	execID := response.ID
	if execID == "" {
		return errors.New("exec ID is empty")
	}

	execStartCheck := types.ExecStartCheck{
		Detach: execConfig.Detach,
	}

	resp, _ := cli.ContainerExecAttach(context.Background(), execID, execStartCheck)

	return execPipe(resp, os.Stdin, os.Stdout, os.Stderr)
}

// ExecCommand executes a command on webserver container, and sends the response as string, this can be clubbed with Run()
func (d *DockerCompose) ExecCommand(containerID, command string) string {
	cmd := exec.Command("docker", "exec", "-it", containerID, "bash", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		_ = errors.Wrapf(err, "error encountered")
	}

	stringOut := string(out)
	return stringOut
}

func (d *DockerCompose) GetContainerID(containerName string) (string, error) {
	psInfo, err := d.composeService.Ps(context.TODO(), d.projectName, api.PsOptions{All: true})
	if err != nil {
		return "", errors.Wrap(err, messages.ErrContainerStatusCheck)
	}

	for idx := range psInfo {
		info := psInfo[idx]
		if strings.Contains(info.Name, containerName) {
			return info.ID, nil
		}
	}
	return "", err
}

// getWebServerContainerID return webserver container id
func (d *DockerCompose) getWebServerContainerID() (string, error) {
	return d.GetContainerID(config.CFG.WebserverContainerName.GetString())
}

// createProject creates project with yaml config as context
func createProject(projectName, airflowHome, envFile string, labels map[string]string) (*composeTypes.Project, error) {
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile, labels, DockerEngine)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	if err != nil {
		return nil, err
	}

	var configs []composeTypes.ConfigFile
	composeConfig := composeTypes.ConfigFile{
		Content:  []byte(yaml),
		Filename: "docker-compose.yml",
	}
	configs = append(configs, composeConfig)

	composeFile := "docker-compose.override.yml"
	composeBytes, err := ioutil.ReadFile(composeFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "Failed to open the compose file: %s", composeFile)
	}
	if err == nil {
		overrideConfig := composeTypes.ConfigFile{Content: composeBytes, Filename: composeFile}
		configs = append(configs, overrideConfig)
	}

	loaderOption := func(opts *loader.Options) {
		opts.Name = projectName
	}

	project, err := loader.Load(composeTypes.ConfigDetails{
		ConfigFiles: configs,
		WorkingDir:  airflowHome,
		Environment: map[string]string{},
	}, loaderOption)

	return project, err
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]
	return scrubbedState == expectedState
}

func (d *DockerCompose) webserverHealthCheck() error {
	healthCheckCounter := 0
	err := d.composeService.Events(context.Background(), d.projectName, api.EventsOptions{
		Services: []string{config.CFG.WebserverContainerName.GetString()}, Consumer: func(event api.Event) error {
			if event.Status == healthyProjectStatus {
				fmt.Println("\nProject is running! All components are now available.")
				// have to return an error to break from the event listener loop
				return errComposeProjectRunning
			} else if event.Status == execDieStatus {
				fmt.Println("Waiting for Airflow components to spin up...")
				time.Sleep(webserverHealthCheckInterval)
			}
			healthCheckCounter += 1
			if healthCheckCounter > healthCheckBreakPoint {
				return errHealthCheckBreakPointReached
			}
			return nil
		},
	})
	if err != nil && !errors.Is(err, errComposeProjectRunning) && !errors.Is(err, errHealthCheckBreakPointReached) {
		return err
	}
	return nil
}

// execPipe does pipe stream into stdout/stdin and stderr
// so now we can pipe out during exec'ing any commands inside container
func execPipe(resp types.HijackedResponse, inStream io.Reader, outStream, errorStream io.Writer) error {
	var err error
	receiveStdout := make(chan error, 1)
	if outStream != nil || errorStream != nil {
		go func() {
			// always do this because we are never tty
			_, err = stdcopy.StdCopy(outStream, errorStream, resp.Reader)
			receiveStdout <- err
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inStream != nil {
			_, err := io.Copy(resp.Conn, inStream)
			if err != nil {
				fmt.Println("Error copying input stream: ", err.Error())
			}
		}

		err := resp.CloseWrite()
		if err != nil {
			fmt.Println("Error closing response body: ", err.Error())
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		if err != nil {
			return err
		}
	case <-stdinDone:
		if outStream != nil || errorStream != nil {
			if err := <-receiveStdout; err != nil {
				return err
			}
		}
	}

	return nil
}

type ComposeLogger struct {
	logger *logrus.Logger
}

func (l *ComposeLogger) Log(service, container, message string) {
	l.logger.Infof("%s | %s", service, message)
}

func (l *ComposeLogger) Status(container, msg string) {
	l.logger.Infof("%s | %s", container, msg)
}

func (l *ComposeLogger) Register(container string) {
}
