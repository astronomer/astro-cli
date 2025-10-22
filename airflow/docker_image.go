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
	"strings"

	"github.com/astronomer/astro-cli/airflow/runtimes"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/astronomer/astro-cli/pkg/util"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/sirupsen/logrus"
)

const (
	astroRunContainer  = "astro-run"
	pullingImagePrompt = "Pulling image from Astronomer registry"
	prefix             = "Bearer "

	// Enhanced error message for 403 authentication issues
	imagePush403ErrMsg = `failed to push image due to authentication error (403 Forbidden).

This commonly occurs due to:
1. Invalid cached Docker credentials
2. Incompatible containerd snapshotter configuration

To resolve:
• Run 'docker logout' for each Astro registry to clear cached credentials
• Ensure containerd snapshotter is disabled (Docker Desktop users)
• Try running 'astro deploy' again

For detailed troubleshooting steps, visit:
https://support.astronomer.io/hc/en-us/articles/41427905156243-403-errors-on-image-push`
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
		if strings.HasPrefix(line, "FROM ") && !strings.HasPrefix(line, fmt.Sprintf("FROM %s", QuayBaseImageName)) && !strings.HasPrefix(line, fmt.Sprintf("FROM %s", AstroImageRegistryBaseImageName)) {
			return false, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scanning file: %w", err)
	}
	return true, nil
}

func (d *DockerImage) Build(dockerfilePath, buildSecretString string, buildConfig airflowTypes.ImageBuildConfig) error {
	// Start the spinner.
	s := spinner.NewSpinner("Building project image…")
	if !logger.IsLevelEnabled(logrus.DebugLevel) {
		s.Start()
		defer s.Stop()
	}

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

	// Route output streams according to verbosity.
	var stdout, stderr io.Writer
	var outBuff bytes.Buffer
	if logger.IsLevelEnabled(logrus.DebugLevel) {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = &outBuff
		stderr = &outBuff
	}

	// Build the image
	err = cmdExec(containerRuntime, stdout, stderr, args...)
	if err != nil {
		s.FinalMSG = ""
		s.Stop()
		fmt.Println(strings.TrimSpace(outBuff.String()) + "\n")
		return errors.New("an error was encountered while building the image, see the build logs for details")
	}

	spinner.StopWithCheckmark(s, "Project image has been updated")
	return nil
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
	if logger.IsLevelEnabled(logrus.WarnLevel) {
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

func (d *DockerImage) Push(remoteImage, username, token string, getImageRepoSha bool) (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	err = cmdExec(containerRuntime, nil, nil, "tag", d.imageName, remoteImage)
	if err != nil {
		return "", fmt.Errorf("command '%s tag %s %s' failed: %w", containerRuntime, d.imageName, remoteImage, err)
	}

	registry, err := d.getRegistryToAuth(remoteImage)
	if err != nil {
		return "", err
	}

	// Push image to registry
	// Note: Caller is responsible for printing appropriate message

	configFile := cliConfig.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(registry)
	if err != nil {
		logger.Debugf("Error reading credentials: %v", err)
		return "", fmt.Errorf("error reading credentials: %w", err)
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
			// Check for 403 errors only after both methods fail
			if is403Error(err) {
				return "", errors.New(imagePush403ErrMsg)
			}
			return "", err
		}
	}
	sha := ""
	if getImageRepoSha {
		sha, err = d.GetImageRepoSHA(registry)
		if err != nil {
			return "", err
		}
	}
	// Delete the image tags we just generated
	err = cmdExec(containerRuntime, nil, nil, "rmi", remoteImage)
	if err != nil {
		return "", fmt.Errorf("command '%s rmi %s' failed: %w", containerRuntime, remoteImage, err)
	}
	return sha, nil
}

func (d *DockerImage) GetImageRepoSHA(registry string) (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}

	// Get all repo digests of the image
	out := &bytes.Buffer{}
	err = cmdExec(containerRuntime, out, nil, "inspect", "--format={{json .RepoDigests}}", d.imageName)
	if err != nil {
		return "", fmt.Errorf("failed to get digests for image %s: %w", d.imageName, err)
	}

	// Parse and clean the output
	var repoDigests []string
	digestOutput := strings.TrimSpace(out.String())
	err = json.Unmarshal([]byte(digestOutput), &repoDigests)
	if err != nil {
		return "", fmt.Errorf("failed to parse digests for image %s: %w", d.imageName, err)
	}
	// Filter repo digests based on the provided repo name
	for _, digest := range repoDigests {
		parts := strings.Split(digest, "@")
		if len(parts) == 2 && strings.HasPrefix(parts[0], registry) {
			return parts[1], nil // Return the digest for the matching repo name
		}
	}

	// If no matching digest is found
	return "", fmt.Errorf("no matching digest found for registry %s in image %s", registry, d.imageName)
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

	// Handle different image name formats:
	// 1. Standard format: registry.com/namespace/repo:tag
	// 2. ECR format: 123456789012.dkr.ecr.us-west-2.amazonaws.com/repo:tag
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("internal logic error: unsure how to get registry from image name %q", imageName)
	}

	// Both formats have the registry as the first part before the first "/"
	// Standard format: registry.com/namespace/repo:tag -> registry.com
	// ECR format: 123456789012.dkr.ecr.us-west-2.amazonaws.com/repo:tag -> 123456789012.dkr.ecr.us-west-2.amazonaws.com
	return parts[0], nil
}

func (d *DockerImage) Pull(remoteImage, username, token string) error {
	if remoteImage == "" {
		remoteImage = d.imageName
	}

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

func (d *DockerImage) RunDAG(dagID, envFile, settingsFile, containerName, dagFile, executionDate string, taskLogs bool) error {
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

func (d *DockerImage) RunCommand(args []string, mountDirs map[string]string, stdout, stderr io.Writer) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	// Default docker run arguments
	dockerArgs := []string{"run", "--rm"}

	// Add volume mounts from the map
	for hostDir, containerDir := range mountDirs {
		dockerArgs = append(dockerArgs, "-v", hostDir+":"+containerDir)
	}

	// Add the image name and the remaining arguments
	dockerArgs = append(dockerArgs, d.imageName)
	args = append(dockerArgs, args...)

	logger.Debugf("Running command in container: '%s'", strings.Join(args, " "))

	return cmdExec(containerRuntime, stdout, stderr, args...)
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

	// docker push <imageName> - capture stderr to preserve error details
	var stderr bytes.Buffer
	err = cmdExec(containerRuntime, os.Stdout, &stderr, "push", imageName)
	if err != nil {
		// Include stderr output in the error so we can detect 403 errors
		stderrOutput := stderr.String()
		if stderrOutput != "" {
			return fmt.Errorf("%w: %s", err, stderrOutput)
		}
		return err
	}
	return nil
}

// is403Error checks if the error is a 403 authentication error
func is403Error(err error) bool {
	if err == nil {
		return false
	}

	// Check the entire error chain, not just the top-level error
	for currentErr := err; currentErr != nil; currentErr = errors.Unwrap(currentErr) {
		errStr := strings.ToLower(currentErr.Error())

		if strings.Contains(errStr, "403") ||
			strings.Contains(errStr, "forbidden") ||
			strings.Contains(errStr, "authentication required") {
			return true
		}
	}

	return false
}
