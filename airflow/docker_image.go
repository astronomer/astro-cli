package airflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/api/types/image"

	"github.com/astronomer/astro-cli/airflow/runtimes"

	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
)

const (
	EchoCmd            = "echo"
	pushingImagePrompt = "Pushing image to Astronomer registry"
	astroRunContainer  = "astro-run"
	pullingImagePrompt = "Pulling image from Astronomer registry"
	prefix             = "Bearer "
)

var errGetImageLabel = errors.New("error getting image label")

var getDockerClient = func() (client.APIClient, error) {
	return client.NewClientWithOpts(client.FromEnv)
}

type DockerImage struct {
	imageName string
}

func DockerImageInit(imageName string) *DockerImage {
	return &DockerImage{imageName: imageName}
}

func shouldAddPullFlag(dockerfilePath string) (bool, error) {
	file, err := os.Open(dockerfilePath)
	if err != nil {
		return false, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	// scan the Dockerfile to check if any FROM instructions reference an image other than astro runtime
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "FROM ") && !strings.HasPrefix(line, fmt.Sprintf("FROM %s:", FullAstroRuntimeImageName)) {
			return false, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scanning file: %w", err)
	}
	return true, nil
}

func (d *DockerImage) Build(dockerfilePath, buildSecretString string, buildConfig airflowTypes.ImageBuildConfig) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	args := []string{"build"}
	addPullFlag, err := shouldAddPullFlag(dockerfilePath)
	if err != nil {
		return fmt.Errorf("reading dockerfile: %w", err)
	}
	if runtimes.IsPodman(containerRuntime) {
		args = append(args, "--format", "docker")
	}
	if addPullFlag {
		args = append(args, "--pull")
	}
	err = os.Chdir(buildConfig.Path)
	if err != nil {
		return err
	}
	args = append(args, []string{
		"-t",
		d.imageName,
		"-f",
		dockerfilePath,
		".",
	}...)
	if buildConfig.NoCache {
		args = append(args, "--no-cache")
	}

	for _, label := range buildConfig.Labels {
		args = append(args, "--label", label)
	}

	if len(buildConfig.TargetPlatforms) > 0 {
		args = append(args, fmt.Sprintf("--platform=%s", strings.Join(buildConfig.TargetPlatforms, ",")))
	}
	if buildSecretString != "" {
		buildSecretArgs := []string{
			"--secret",
			buildSecretString,
		}
		args = append(args, buildSecretArgs...)
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
	fmt.Println(args)
	err = cmdExec(containerRuntime, stdout, stderr, args...)
	if err != nil {
		return fmt.Errorf("command '%s build -t %s failed: %w", containerRuntime, d.imageName, err)
	}
	return err
}

func (d *DockerImage) Pytest(pytestFile, airflowHome, envFile, testHomeDirectory string, pytestArgs []string, htmlReport bool, buildConfig airflowTypes.ImageBuildConfig) (string, error) {
	// delete container
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	err = cmdExec(containerRuntime, nil, nil, "rm", "astro-pytest")
	if err != nil {
		logger.Debug(err)
	}
	// Change to location of Dockerfile
	err = os.Chdir(buildConfig.Path)
	if err != nil {
		return "", err
	}
	args := []string{
		"create",
		"-i",
		"--name",
		"astro-pytest",
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

	// create pytest container
	err = cmdExec(containerRuntime, stdout, stderr, args...)
	if err != nil {
		return "", err
	}

	// cp DAGs folder
	args = []string{
		"cp",
		airflowHome + "/dags",
		"astro-pytest:/usr/local/airflow/",
	}
	docErr := cmdExec(containerRuntime, stdout, stderr, args...)
	if docErr != nil {
		return "", docErr
	}

	// cp .astro folder
	// on some machine .astro is being docker ignored, but not
	// on every machine, hence to keep behavior consistent
	// copying the .astro folder explicitly
	args = []string{
		"cp",
		airflowHome + "/.astro",
		"astro-pytest:/usr/local/airflow/",
	}
	docErr = cmdExec(containerRuntime, stdout, stderr, args...)
	if docErr != nil {
		return "", docErr
	}

	// start pytest container
	err = cmdExec(containerRuntime, stdout, stderr, []string{"start", "astro-pytest", "-a"}...)
	if err != nil {
		logger.Debugf("Error starting pytest container: %s", err.Error())
	}

	// get exit code
	args = []string{
		"inspect",
		"astro-pytest",
		"--format='{{.State.ExitCode}}'",
	}
	var outb bytes.Buffer
	docErr = cmdExec(containerRuntime, &outb, stderr, args...)
	if docErr != nil {
		logger.Debug(docErr)
	}

	if htmlReport {
		// Copy the dag-test-report.html file from the container to the destination folder
		docErr = cmdExec(containerRuntime, nil, stderr, "cp", "astro-pytest:/usr/local/airflow/dag-test-report.html", "./"+testHomeDirectory)
		if docErr != nil {
			logger.Debugf("Error copying dag-test-report.html file from the pytest container: %s", docErr.Error())
		}
	}

	// Persist the include folder from the Docker container to local include folder
	docErr = cmdExec(containerRuntime, nil, stderr, "cp", "astro-pytest:/usr/local/airflow/include/", ".")
	if docErr != nil {
		logger.Debugf("Error copying include folder from the pytest container: %s", docErr.Error())
	}

	// delete container
	docErr = cmdExec(containerRuntime, nil, stderr, "rm", "astro-pytest")
	if docErr != nil {
		logger.Debugf("Error removing the astro-pytest container: %s", docErr.Error())
	}

	return outb.String(), err
}

func (d *DockerImage) ConflictTest(workingDirectory, testHomeDirectory string, buildConfig airflowTypes.ImageBuildConfig) (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	// delete container
	err = cmdExec(containerRuntime, nil, nil, "rm", "astro-temp-container")
	if err != nil {
		logger.Debug(err)
	}
	// Change to location of Dockerfile
	err = os.Chdir(buildConfig.Path)
	if err != nil {
		return "", err
	}
	args := []string{
		"build",
		"-t",
		"conflict-check:latest",
		"-f",
		"conflict-check.Dockerfile",
		".",
	}

	// Create a buffer to capture the command output
	var stdout, stderr bytes.Buffer
	multiStdout := io.MultiWriter(&stdout, os.Stdout)
	multiStderr := io.MultiWriter(&stderr, os.Stdout)

	// Start the command execution
	err = cmdExec(containerRuntime, multiStdout, multiStderr, args...)
	if err != nil {
		return "", err
	}
	// Get the exit code
	exitCode := ""
	if _, ok := err.(*exec.ExitError); ok {
		// The command exited with a non-zero status
		exitCode = parseExitCode(stderr.String())
	} else if err != nil {
		// An error occurred while running the command
		return "", err
	}
	// Run a temporary container to copy the file from the image
	err = cmdExec(containerRuntime, nil, nil, "create", "--name", "astro-temp-container", "conflict-check:latest")
	if err != nil {
		return exitCode, err
	}
	// Copy the result.txt file from the container to the destination folder
	err1 := cmdExec(containerRuntime, nil, nil, "cp", "astro-temp-container:/usr/local/airflow/conflict-test-results.txt", "./"+testHomeDirectory)
	if err1 != nil {
		// Remove the temporary container
		err = cmdExec(containerRuntime, nil, nil, "rm", "astro-temp-container")
		if err != nil {
			return exitCode, err
		}
		return exitCode, err1
	}

	// Remove the temporary container
	err = cmdExec(containerRuntime, nil, nil, "rm", "astro-temp-container")
	if err != nil {
		return exitCode, err
	}
	return exitCode, nil
}

func parseExitCode(logs string) string {
	re := regexp.MustCompile(`exit code: (\d+)`)
	match := re.FindStringSubmatch(logs)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func (d *DockerImage) CreatePipFreeze(altImageName, pipFreezeFile string) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	// Define the Docker command and arguments
	imageName := d.imageName
	if altImageName != "" {
		imageName = altImageName
	}
	dockerArgs := []string{"run", "--rm", imageName, "pip", "freeze"}

	// Create a file to store the command output
	file, err := os.Create(pipFreezeFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Run the Docker command
	err = cmdExec(containerRuntime, file, os.Stderr, dockerArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (d *DockerImage) Push(remoteImage, username, token string) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	err = cmdExec(containerRuntime, nil, nil, "tag", d.imageName, remoteImage)
	if err != nil {
		return fmt.Errorf("command '%s tag %s %s' failed: %w", containerRuntime, d.imageName, remoteImage, err)
	}

	registry, err := d.getRegistryToAuth(remoteImage)
	if err != nil {
		return err
	}

	// Push image to registry
	fmt.Println(pushingImagePrompt)

	configFile := cliConfig.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(registry)
	if err != nil {
		logger.Debugf("Error reading credentials: %v", err)
		return fmt.Errorf("error reading credentials: %w", err)
	}

	if username == "" && token == "" {
		registryDomain := strings.Split(registry, "/")[0]
		creds := configFile.GetCredentialsStore(registryDomain)
		authConfig, err = creds.Get(registryDomain)
		if err != nil {
			logger.Debugf("Error reading credentials for domain: %s from %s credentials store: %v", containerRuntime, registryDomain, err)
		}
	} else {
		if username != "" {
			authConfig.Username = username
		}
		authConfig.Password = token
		authConfig.ServerAddress = registry
	}

	logger.Debugf("Exec Push %s creds %v \n", containerRuntime, authConfig)

	err = d.pushWithClient(&authConfig, remoteImage)
	if err != nil {
		// if it does not work with the go library use bash to run docker commands. Support for (old?) versions of Colima
		err = pushWithBash(&authConfig, remoteImage)
		if err != nil {
			return err
		}
	}

	// Delete the image tags we just generated
	err = cmdExec(containerRuntime, nil, nil, "rmi", remoteImage)
	if err != nil {
		return fmt.Errorf("command '%s rmi %s' failed: %w", containerRuntime, remoteImage, err)
	}
	return nil
}

func (d *DockerImage) GetImageSha() (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	// Get the digest of the pushed image
	remoteDigest := ""
	out := &bytes.Buffer{}
	err = cmdExec(containerRuntime, out, nil, "inspect", "--format={{index .RepoDigests 0}}", d.imageName)
	if err != nil {
		return remoteDigest, fmt.Errorf("failed to get digest for image %s: %w", d.imageName, err)
	}
	// Parse and clean the output
	digestOutput := strings.TrimSpace(out.String())
	if digestOutput != "" {
		parts := strings.Split(digestOutput, "@")
		if len(parts) == 2 {
			remoteDigest = parts[1] // Extract the digest part (after '@')
		}
	}
	return remoteDigest, nil
}

func (d *DockerImage) pushWithClient(authConfig *cliTypes.AuthConfig, remoteImage string) error {
	ctx := context.Background()

	cli, err := getDockerClient()
	if err != nil {
		logger.Debugf("Error setting up new Client ops %v", err)
		return err
	}
	cli.NegotiateAPIVersion(ctx)
	buf, err := json.Marshal(authConfig)
	if err != nil {
		logger.Debugf("Error negotiating api version: %v", err)
		return err
	}
	encodedAuth := base64.URLEncoding.EncodeToString(buf)
	responseBody, err := cli.ImagePush(ctx, remoteImage, image.PushOptions{RegistryAuth: encodedAuth})
	if err != nil {
		logger.Debugf("Error pushing image to docker: %v", err)
		// if NewClientWithOpt does not work use bash to run docker commands
		return err
	}
	defer responseBody.Close()
	return displayJSONMessagesToStream(responseBody, nil)
}

// Get the registry name to authenticate against
func (d *DockerImage) getRegistryToAuth(imageName string) (string, error) {
	domain, err := config.GetCurrentDomain()
	if err != nil {
		return "", err
	}

	if domain == "localhost" {
		return config.CFG.LocalRegistry.GetString(), nil
	}
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) != 2 || !strings.Contains(parts[1], "/") {
		// This _should_ be impossible for users to hit
		return "", fmt.Errorf("internal logic error: unsure how to get registry from image name %q", imageName)
	}
	return parts[0], nil
}

func (d *DockerImage) Pull(remoteImage, username, token string) error {
	// Pulling image to registry
	fmt.Println(pullingImagePrompt)
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	if username != "" { // Case for cloud image push where we have both registry user & pass, for software login happens during `astro login` itself
		var registry string
		if registry, err = d.getRegistryToAuth(remoteImage); err != nil {
			return err
		}
		pass := token
		pass = strings.TrimPrefix(pass, prefix)
		cmd := "echo \"" + pass + "\"" + " | " + containerRuntime + " login " + registry + " -u " + username + " --password-stdin"
		err = cmdExec("bash", os.Stdout, os.Stderr, "-c", cmd) // This command will only work on machines that have bash. If users have issues we will revist
	}
	if err != nil {
		return err
	}
	// docker pull <image>
	err = cmdExec(containerRuntime, os.Stdout, os.Stderr, "pull", remoteImage)
	if err != nil {
		return err
	}

	return nil
}

var displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
	out := streams.NewOut(os.Stdout)
	err := jsonmessage.DisplayJSONMessagesToStream(responseBody, out, nil)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerImage) GetLabel(altImageName, labelName string) (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	labelFmt := fmt.Sprintf("{{ index .Config.Labels %q }}", labelName)
	var label string
	imageName := d.imageName
	if altImageName != "" {
		imageName = altImageName
	}
	err = cmdExec(containerRuntime, stdout, stderr, "inspect", "--format", labelFmt, imageName)
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

func (d *DockerImage) DoesImageExist(imageName string) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err = cmdExec(containerRuntime, stdout, stderr, "manifest", "inspect", imageName)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerImage) ListLabels() (map[string]string, error) {
	var labels map[string]string

	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return labels, err
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err = cmdExec(containerRuntime, stdout, stderr, "inspect", "--format", "{{ json .Config.Labels }}", d.imageName)
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
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	err = cmdExec(containerRuntime, nil, nil, "tag", localImage, d.imageName)
	if err != nil {
		return fmt.Errorf("command '%s tag %s %s' failed: %w", containerRuntime, localImage, d.imageName, err)
	}
	return nil
}

func (d *DockerImage) Run(dagID, envFile, settingsFile, containerName, dagFile, executionDate string, taskLogs bool) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	stdout := os.Stdout
	stderr := os.Stderr
	// delete container
	err = cmdExec(containerRuntime, nil, nil, "rm", astroRunContainer)
	if err != nil {
		logger.Debug(err)
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
		logger.Debug(err)
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
			logger.Debug(err)
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

	if executionDate != "" {
		cmdArgs = append(cmdArgs, []string{"--execution-date", executionDate}...)
	}

	if taskLogs {
		cmdArgs = append(cmdArgs, []string{"--verbose"}...)
	}

	args = append(args, cmdArgs...)

	fmt.Println("\nStarting a DAG run for " + dagID + "...")
	fmt.Println("\nLoading DAGs...")
	logger.Debug("args passed to docker command:")
	logger.Debug(args)

	cmdErr := cmdExec(containerRuntime, stdout, stderr, args...)
	// add back later fmt.Println("\nSee the output of this command for errors. To view task logs, use the '--task-logs' flag.")
	if cmdErr != nil {
		logger.Debug(cmdErr)
		fmt.Println("\nSee the output of this command for errors.")
		fmt.Println("If you are having an issue with loading your settings file make sure both the 'variables' and 'connections' fields exist and that there are no yaml syntax errors.")
		fmt.Println("If you are getting a missing `airflow_settings.yaml` or `astro-run-dag` error try restarting airflow with `astro dev restart`.")
	}
	if containerName == "" {
		// delete container
		err = cmdExec(containerRuntime, nil, nil, "rm", astroRunContainer)
		if err != nil {
			logger.Debug(err)
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
func pushWithBash(authConfig *cliTypes.AuthConfig, imageName string) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	if authConfig.Username != "" { // Case for cloud image push where we have both registry user & pass, for software login happens during `astro login` itself
		pass := authConfig.Password
		pass = strings.TrimPrefix(pass, prefix)
		cmd := "echo \"" + pass + "\"" + " | " + containerRuntime + " login " + authConfig.ServerAddress + " -u " + authConfig.Username + " --password-stdin"
		err = cmdExec("bash", os.Stdout, os.Stderr, "-c", cmd) // This command will only work on machines that have bash. If users have issues we will revist
		if err != nil {
			return err
		}
	}
	// docker push <imageName>
	return cmdExec(containerRuntime, os.Stdout, os.Stderr, "push", imageName)
}
