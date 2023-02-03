package airflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/astronomer/astro-cli/pkg/util"
	cliCommand "github.com/docker/cli/cli/command"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	log "github.com/sirupsen/logrus"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
)

const (
	EchoCmd            = "echo"
	pushingImagePrompt = "Pushing image to Astronomer registry"
	astroRunContainer  = "astro-run"
)

var errGetImageLabel = errors.New("error getting image label")

type DockerImage struct {
	imageName string
}

func DockerImageInit(image string) *DockerImage {
	return &DockerImage{imageName: image}
}

func (d *DockerImage) Build(buildConfig airflowTypes.ImageBuildConfig) error {
	dockerCommand := config.CFG.DockerCommand.GetString()

	err := os.Chdir(buildConfig.Path)
	if err != nil {
		return err
	}
	args := []string{
		"build",
		"-t",
		d.imageName,
		".",
	}
	if buildConfig.NoCache {
		args = append(args, "--no-cache")
	}

	if len(buildConfig.TargetPlatforms) > 0 {
		args = append(args, fmt.Sprintf("--platform=%s", strings.Join(buildConfig.TargetPlatforms, ",")))
	}
	// Build image
	var stdout, stderr io.Writer
	if buildConfig.Output {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = nil
		stderr = nil
	}
	err = cmdExec(dockerCommand, stdout, stderr, args...)
	if err != nil {
		return fmt.Errorf("command 'docker build -t %s failed: %w", d.imageName, err)
	}
	return err
}

func (d *DockerImage) Pytest(pytestFile, airflowHome, envFile string, pytestArgs []string, buildConfig airflowTypes.ImageBuildConfig) (string, error) {
	// delete container
	dockerCommand := config.CFG.DockerCommand.GetString()
	err := cmdExec(dockerCommand, nil, nil, "rm", "astro-pytest")
	if err != nil {
		log.Debug(err)
	}
	// Change to location of Dockerfile
	err = os.Chdir(buildConfig.Path)
	if err != nil {
		return "", err
	}
	args := []string{
		"run",
		"-i",
		"--name",
		"astro-pytest",
		"-v",
		airflowHome + "/dags:/usr/local/airflow/dags:rw",
		"-v",
		airflowHome + "/plugins:/usr/local/airflow/plugins:rw",
		"-v",
		airflowHome + "/include:/usr/local/airflow/include:rw",
		"-v",
		airflowHome + "/.astro:/usr/local/airflow/.astro:rw",
		"-v",
		airflowHome + "/tests:/usr/local/airflow/tests:rw",
	}
	fileExist, err := util.Exists(airflowHome + "/" + envFile)
	if err != nil {
		return "", err
	}
	if fileExist {
		args = append(args, []string{"--env-file", envFile}...)
	}
	args = append(args, []string{d.imageName, "pytest", pytestFile}...)
	args = append(args, pytestArgs...)
	// run pytest image
	var stdout, stderr io.Writer
	if buildConfig.Output {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = nil
		stderr = nil
	}
	// run pytest
	docErr := cmdExec(dockerCommand, stdout, stderr, args...)
	if docErr != nil {
		log.Debug(docErr)
	}

	// get exit code
	args = []string{
		"inspect",
		"astro-pytest",
		"--format='{{.State.ExitCode}}'",
	}
	var outb bytes.Buffer
	err = cmdExec(dockerCommand, &outb, stderr, args...)
	if err != nil {
		log.Debug(err)
	}

	// delete container
	err = cmdExec(dockerCommand, nil, stderr, "rm", "astro-pytest")
	if err != nil {
		log.Debug(err)
	}

	return outb.String(), docErr
}

func (d *DockerImage) Push(registry, username, token, remoteImage string) error {
	dockerCommand := config.CFG.DockerCommand.GetString()
	err := cmdExec(dockerCommand, nil, nil, "tag", d.imageName, remoteImage)
	if err != nil {
		return fmt.Errorf("command 'docker tag %s %s' failed: %w", d.imageName, remoteImage, err)
	}

	// Push image to registry
	fmt.Println(pushingImagePrompt)

	configFile := cliConfig.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(registry)
	if err != nil {
		log.Debugf("Error reading credentials: %v", err)
		return fmt.Errorf("error reading credentials: %w", err)
	}

	if username == "" && token == "" {
		registryDomain := strings.Split(registry, "/")[0]
		creds := configFile.GetCredentialsStore(registryDomain)
		authConfig, err = creds.Get(registryDomain)
		if err != nil {
			log.Debugf("Error reading credentials for domain: %s from docker credentials store: %v", registryDomain, err)
		}
	} else {
		if username != "" {
			authConfig.Username = username
		}
		authConfig.Password = token
		authConfig.ServerAddress = registry
	}

	log.Debugf("Exec Push docker creds %v \n", authConfig)

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Debugf("Error setting up new Client ops %v", err)
		// if NewClientWithOpt does not work use bash to run docker commands
		return useBash(&authConfig, remoteImage)
	}
	cli.NegotiateAPIVersion(ctx)
	buf, err := json.Marshal(authConfig)
	if err != nil {
		log.Debugf("Error negotiating api version: %v", err)
		return err
	}
	encodedAuth := base64.URLEncoding.EncodeToString(buf)
	responseBody, err := cli.ImagePush(ctx, remoteImage, types.ImagePushOptions{RegistryAuth: encodedAuth})
	if err != nil {
		log.Debugf("Error pushing image to docker: %v", err)
		// if NewClientWithOpt does not work use bash to run docker commands
		return useBash(&authConfig, remoteImage)
	}
	defer responseBody.Close()
	err = displayJSONMessagesToStream(responseBody, nil)
	if err != nil {
		return useBash(&authConfig, remoteImage)
	}
	// Delete the image tags we just generated
	err = cmdExec(dockerCommand, nil, nil, "rmi", remoteImage)
	if err != nil {
		return fmt.Errorf("command 'docker rmi %s' failed: %w", remoteImage, err)
	}
	return nil
}

var displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
	out := cliCommand.NewOutStream(os.Stdout)
	err := jsonmessage.DisplayJSONMessagesToStream(responseBody, out, nil)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerImage) GetLabel(labelName string) (string, error) {
	dockerCommand := config.CFG.DockerCommand.GetString()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	labelFmt := fmt.Sprintf("{{ index .Config.Labels %q }}", labelName)
	var label string
	err := cmdExec(dockerCommand, stdout, stderr, "inspect", "--format", labelFmt, d.imageName)
	if err != nil {
		return label, err
	}
	if execErr := stderr.String(); execErr != "" {
		return label, fmt.Errorf("%s: %w", execErr, errGetImageLabel)
	}
	label = stdout.String()
	label = strings.Trim(label, "\n")
	return label, nil
}

func (d *DockerImage) ListLabels() (map[string]string, error) {
	dockerCommand := config.CFG.DockerCommand.GetString()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	var labels map[string]string
	err := cmdExec(dockerCommand, stdout, stderr, "inspect", "--format", "{{ json .Config.Labels }}", d.imageName)
	if err != nil {
		return labels, err
	}
	if execErr := stderr.String(); execErr != "" {
		return labels, fmt.Errorf("%s: %w", execErr, errGetImageLabel)
	}
	err = json.Unmarshal(stdout.Bytes(), &labels)
	if err != nil {
		return labels, err
	}
	return labels, nil
}

func (d *DockerImage) TagLocalImage(localImage string) error {
	dockerCommand := config.CFG.DockerCommand.GetString()

	err := cmdExec(dockerCommand, nil, nil, "tag", localImage, d.imageName)
	if err != nil {
		return fmt.Errorf("command 'docker tag %s %s' failed: %w", localImage, d.imageName, err)
	}
	return nil
}

func (d *DockerImage) Run(dagID, envFile, settingsFile, containerName, dagFile string, taskLogs bool) error {
	dockerCommand := config.CFG.DockerCommand.GetString()

	stdout := os.Stdout
	stderr := os.Stderr
	// delete container
	err := cmdExec(dockerCommand, nil, nil, "rm", astroRunContainer)
	if err != nil {
		log.Debug(err)
	}
	var args []string
	if containerName != "" {
		args = []string{
			"exec",
			"-t",
			containerName,
		}
	}
	// check if settings file exists
	settingsFileExist, err := util.Exists("./" + settingsFile)
	if err != nil {
		log.Debug(err)
	}
	// docker exec
	if containerName == "" {
		args = []string{
			"run",
			"-t",
			"--name",
			astroRunContainer,
			"-v",
			config.WorkingPath + "/dags:/usr/local/airflow/dags:rw",
			"-v",
			config.WorkingPath + "/plugins:/usr/local/airflow/plugins:rw",
			"-v",
			config.WorkingPath + "/include:/usr/local/airflow/include:rw",
		}
		// if settings file exists append it to args
		if settingsFileExist {
			args = append(args, []string{"-v", config.WorkingPath + "/" + settingsFile + ":/usr/local/airflow/" + settingsFile}...)
		}
		// if env file exists append it to args
		fileExist, err := util.Exists(config.WorkingPath + "/" + envFile)
		if err != nil {
			log.Debug(err)
		}
		if fileExist {
			args = append(args, []string{"--env-file", envFile}...)
		}
		args = append(args, []string{d.imageName}...)
	}
	if !strings.Contains(dagFile, "dags/") {
		dagFile = "./dags/" + dagFile
	}
	cmdArgs := []string{
		"run_dag",
		dagFile,
		dagID,
	}
	// settings file exists append it to args
	if settingsFileExist {
		cmdArgs = append(cmdArgs, []string{"./" + settingsFile}...)
	}
	args = append(args, cmdArgs...)

	fmt.Println("\nStarting a DAG run for " + dagID + "...")
	fmt.Println("\nLoading DAGs...")

	cmdErr := cmdExec(dockerCommand, stdout, stderr, args...)
	// add back later fmt.Println("\nSee the output of this command for errors. To view task logs, use the '--task-logs' flag.")
	if cmdErr != nil {
		log.Debug(cmdErr)
		fmt.Println("\nSee the output of this command for errors.")
		fmt.Println("If you are having an issue with loading your settings file make sure both the 'variables' and 'connections' fields exist and that there are no yaml syntax errors.")
		fmt.Println("If you are getting a missing `airflow_settings.yaml` or `astro-run-dag` error try restarting airflow with `astro dev restart`.")
	}
	if containerName == "" {
		// delete container
		err = cmdExec(dockerCommand, nil, nil, "rm", astroRunContainer)
		if err != nil {
			log.Debug(err)
		}
	}
	return cmdErr
}

// Exec executes a docker command
var cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
	_, lookErr := exec.LookPath(cmd)
	if lookErr != nil {
		return fmt.Errorf("failed to find the %s command: %w", cmd, lookErr)
	}

	execCMD := exec.Command(cmd, args...)
	execCMD.Stdin = os.Stdin
	execCMD.Stdout = stdout
	execCMD.Stderr = stderr

	if cmdErr := execCMD.Run(); cmdErr != nil {
		return fmt.Errorf("failed to execute cmd: %w", cmdErr)
	}

	return nil
}

// When login and push do not work use bash to run docker commands, this function is for users using colima
func useBash(authConfig *cliTypes.AuthConfig, image string) error {
	dockerCommand := config.CFG.DockerCommand.GetString()

	var err error
	if authConfig.Username != "" { // Case for cloud image push where we have both registry user & pass, for software login happens during `astro login` itself
		cmd := "echo \"" + authConfig.Password + "\"" + " | docker login " + authConfig.ServerAddress + " -u " + authConfig.Username + " --password-stdin"
		err = cmdExec("bash", os.Stdout, os.Stderr, "-c", cmd) // This command will only work on machines that have bash. If users have issues we will revist
	}
	if err != nil {
		return err
	}
	// docker push <image>
	err = cmdExec(dockerCommand, os.Stdout, os.Stderr, "push", image)
	if err != nil {
		return err
	}
	// Delete the image tags we just generated
	err = cmdExec(dockerCommand, nil, nil, "rmi", image)
	if err != nil {
		return fmt.Errorf("command 'docker rmi %s' failed: %w", image, err)
	}
	return nil
}
