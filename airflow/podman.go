package airflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"

	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/bindings/play"
	"github.com/containers/podman/v3/pkg/bindings/pods"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var (
	defaultPodmanSockDir     = "/run"
	podConfigFile            = "pod-config.yml"
	podStateFile             = ".astro/pod-state.yaml"
	podContainerStateRunning = "running"

	webserverHealthCheckInterval = 5 * time.Second
)

type Podman struct {
	projectDir  string
	envFile     string
	projectName string
	podmanBind  PodmanBind
	conn        context.Context
}

type EnvVar struct {
	Name  string      `yaml:"name"`
	Value interface{} `yaml:"value"`
}

func PodmanInit(projectDir, envFile string) (*Podman, error) {
	binder := PodmanBinder{}
	conn, err := getConn(context.TODO(), &binder)
	if err != nil {
		return nil, err
	}

	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving working directory")
	}

	return &Podman{projectDir: projectDir, envFile: envFile, projectName: projectName, conn: conn, podmanBind: &binder}, nil
}

func (p *Podman) Start(dockerfile string) error {
	psInfo, err := p.listContainers()
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStatusCheck)
	}

	if len(psInfo) > 0 {
		for idx := range psInfo {
			info := psInfo[idx]
			if info.State == podContainerStateRunning {
				// move error to common message package
				return errors.New("cannot start, project already running")
			}
		}
	}

	imageBuilder, err := PodmanImageInit(p.conn, p.projectName, p.podmanBind)
	if err != nil {
		return err
	}
	err = imageBuilder.Build(".")
	if err != nil {
		return err
	}

	labels, err := imageBuilder.GetImageLabels()
	if err != nil {
		return err
	}

	_, err = p.startPod(p.projectName, p.projectDir, p.envFile, labels)
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerRecreate)
	}

	// A work around to make sure airflow db is up and running before doing anyother operations
	p.webserverHealthCheck()

	parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
	fmt.Printf(messages.ContainerLinkWebserver+"\n", parts[len(parts)-1])
	fmt.Printf(messages.ContainerLinkPostgres+"\n", config.CFG.PostgresPort.GetString())
	fmt.Printf(messages.ContainerUserPassword + "\n")

	return nil
}

func (p *Podman) PS() error {
	psInfo, err := p.listContainers()
	if err != nil {
		return err
	}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight) // nolint:gomnd

	// Columns for table
	infoColumns := []string{"Names", "State", "Ports"}
	fmt.Fprintln(tw, strings.Join(infoColumns, "\t"))
	// Append data to table
	for idx := range psInfo {
		info := psInfo[idx]
		ports := []string{}
		for _, port := range info.Ports {
			ports = append(ports, strconv.Itoa(int(port.ContainerPort)))
		}
		data := []string{strings.Join(info.Names, ","), info.State, strings.Join(ports, ",")}
		fmt.Fprintln(tw, strings.Join(data, "\t"))
	}

	// Flush to stdout
	return tw.Flush()
}

func (p *Podman) Kill() error {
	options := new(pods.RemoveOptions)
	options = options.WithForce(true)
	report, err := p.podmanBind.Remove(p.conn, p.projectName, options)
	if err != nil {
		return errors.Wrap(err, messages.ErrContainerStop)
	} else if report.Err != nil {
		return errors.Wrap(report.Err, messages.ErrContainerStop)
	}
	fmt.Println("Successfully removed Airflow pod")
	fmt.Println(report.Id)
	return nil
}

func (p *Podman) Logs(follow bool, containerNames ...string) error {
	psInfo, err := p.listContainers()
	if err != nil {
		return err
	}
	if len(psInfo) == 0 {
		return errors.New("cannot view logs, project not running")
	}

	for idx := range containerNames {
		containerName := containerNames[idx]
		err := p.getLogs(containerName, psInfo, follow)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Podman) Stop() error {
	_, err := p.podmanBind.Stop(p.conn, p.projectName, nil)
	return err
}

func (p *Podman) ExecCommand(containerID, command string) string {
	command = strings.TrimLeft(command, " ")
	command = strings.TrimRight(command, " ")
	execConfig := new(handlers.ExecCreateConfig)
	execConfig.AttachStdout = true
	execConfig.AttachStderr = true
	execConfig.Cmd = strings.Split(command, " ")

	execID, err := p.podmanBind.ExecCreate(p.conn, containerID, execConfig)
	if err != nil {
		return err.Error()
	}

	r, w, err := os.Pipe()
	if err != nil {
		return err.Error()
	}
	defer r.Close()

	streams := new(containers.ExecStartAndAttachOptions).WithOutputStream(w).WithErrorStream(w).WithAttachOutput(true).WithAttachError(true)
	err = p.podmanBind.ExecStartAndAttach(p.conn, execID, streams)
	if err != nil {
		return err.Error()
	}

	outputC := make(chan string)

	// copying output in a separate goroutine, so that printing doesn't remain blocked forever
	go func() {
		var output bytes.Buffer
		_, _ = io.Copy(&output, r)
		outputC <- output.String()
	}()

	w.Close()
	return <-outputC
}

func (p *Podman) Run(args []string, user string) error {
	containerID, err := p.getWebserverContainerID()
	if err != nil {
		return err
	}

	execConfig := new(handlers.ExecCreateConfig)
	execConfig.AttachStdout = true
	execConfig.AttachStderr = true
	execConfig.Cmd = args
	if user != "" {
		execConfig.User = user
	}

	execID, err := p.podmanBind.ExecCreate(p.conn, containerID, execConfig)
	if err != nil {
		return err
	}

	streams := new(containers.ExecStartAndAttachOptions).WithOutputStream(os.Stdout).WithErrorStream(os.Stderr).WithAttachOutput(true).WithAttachError(true)
	return p.podmanBind.ExecStartAndAttach(p.conn, execID, streams)
}

func (p *Podman) GetContainerID(containerName string) (string, error) {
	podContainerName := p.projectName + "-" + containerName
	psInfo, err := p.listContainers()
	if err != nil {
		return "", err
	}

	for idx := range psInfo {
		info := psInfo[idx]
		for _, name := range info.Names {
			if strings.Contains(name, podContainerName) {
				return info.ID, nil
			}
		}
	}

	return "", errors.New(messages.ErrContainerNotFound)
}

func (p *Podman) getLogs(containerName string, podInfo []entities.ListContainer, follow bool) error {
	var containerID string
	for idx := range podInfo {
		info := podInfo[idx]
		for _, name := range info.Names {
			if strings.Contains(name, containerName) {
				containerID = info.ID
				break
			}
			if containerID != "" {
				break
			}
		}
	}

	if containerID == "" {
		return nil
	}

	options := new(containers.LogOptions).WithStdout(true).WithStderr(true).WithFollow(follow)
	stdoutChan := make(chan string)
	stderrChan := make(chan string)

	go func() {
		for line := range stdoutChan {
			fmt.Fprintf(os.Stdout, "%s | %s\n", containerName, line)
		}
		close(stdoutChan)
	}()
	go func() {
		for line := range stderrChan {
			fmt.Fprintf(os.Stderr, "%s | %s\n", containerName, line)
		}
		close(stderrChan)
	}()
	// this will block until all the logs are streamed
	err := p.podmanBind.Logs(p.conn, containerID, options, stdoutChan, stderrChan)
	return err
}

func (p *Podman) listContainers() ([]entities.ListContainer, error) {
	options := new(containers.ListOptions).WithFilters(map[string][]string{"pod": {p.projectName}})
	containerInfo, err := p.podmanBind.List(p.conn, options)
	return containerInfo, err
}

func (p *Podman) startPod(projectName, projectDir, envFile string, imageLabels map[string]string) (string, error) {
	var podID string
	// in case pod already there, try running pod start
	exists, _ := p.podmanBind.Exists(p.conn, projectName, nil)
	if exists {
		report, err := p.podmanBind.Start(p.conn, projectName, nil)
		if err != nil {
			return podID, err
		}
		return report.Id, nil
	}
	err := generatePodState(projectName, projectDir, envFile, imageLabels)
	if err != nil {
		return podID, err
	}
	defer os.Remove(podStateFile)
	options := &play.KubeOptions{}
	if strings.HasPrefix(config.CFG.PodmanConnectionURI.GetString(), "ssh") {
		options = options.WithNetwork("podman")
	}
	podReport, err := p.podmanBind.Kube(p.conn, podStateFile, options)
	if err != nil {
		return podID, err
	}
	if len(podReport.Pods) > 0 {
		return podReport.Pods[0].ID, nil
	}
	return "", nil
}

func generatePodState(projectName, projectDir, envFile string, imageLabels map[string]string) error {
	// Create the podman config yaml if not exists
	if _, err := os.Stat(podConfigFile); errors.Is(err, os.ErrNotExist) {
		err = initFiles("", map[string]string{podConfigFile: include.PodmanConfigYml})
		if err != nil {
			return err
		}
	}
	configYAML, err := generateConfig(projectName, projectDir, envFile, imageLabels, PodmanEngine)
	if err != nil {
		return errors.Wrap(err, "failed to create pod state file")
	}
	return initFiles("", map[string]string{podStateFile: configYAML})
}

func getConn(ctx context.Context, binder PodmanBind) (context.Context, error) {
	var socket string
	connectionURI := config.CFG.PodmanConnectionURI.GetString()
	if connectionURI != "" {
		socket = connectionURI
	} else {
		// Get Podman socket location
		sockDir, ok := os.LookupEnv("XDG_RUNTIME_DIR")
		if !ok {
			sockDir = defaultPodmanSockDir
		}
		socket = "unix://" + sockDir + "/podman/podman.sock"
	}

	// podman connection is essentially a context
	return binder.NewConnection(ctx, socket)
}

func fmtPodmanEnvVars(envFile string) (string, error) {
	vars := make([]EnvVar, 0)
	envVars, err := godotenv.Read(envFile)
	if err != nil {
		return "", err
	}
	for varName, varValue := range envVars {
		vars = append(vars, EnvVar{Name: varName, Value: varValue})
	}
	if len(vars) < 1 {
		return "", nil
	}
	yamlData, err := yaml.Marshal(&vars)
	if err != nil {
		return "", err
	}
	yamlData = addRootIndent(yamlData, 4)
	return string(yamlData), nil
}

func addRootIndent(b []byte, n int) []byte {
	prefix := append([]byte("\n"), bytes.Repeat([]byte(" "), n)...)
	b = append(prefix[1:], b...) // Indent first line
	return bytes.ReplaceAll(b, []byte("\n"), prefix)
}

func readPodConfigFile() string {
	content, err := ioutil.ReadFile(podConfigFile)
	if err != nil {
		return ""
	}
	return string(content)
}

func (p *Podman) webserverHealthCheck() {
	for i := 0; i < 10; i++ {
		var resp string
		containerID, err := p.getWebserverContainerID()
		if err != nil {
			goto sleep
		}
		resp = p.ExecCommand(containerID, "airflow db check")
		if strings.Contains(resp, "Connection successful.") {
			break
		}
	sleep:
		fmt.Println("Waiting for Airflow containers to spin up...")
		time.Sleep(webserverHealthCheckInterval)
	}
}

func (p *Podman) getWebserverContainerID() (string, error) {
	return p.GetContainerID("webserver")
}
