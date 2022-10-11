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
	"github.com/astronomer/astro-cli/pkg/ansi"
)

const (
	DockerCmd          = "docker"
	EchoCmd            = "echo"
	pushingImagePrompt = "Pushing image to Astronomer registry"
)

var errGetImageLabel = errors.New("error getting image label")

type DockerImage struct {
	imageName string
}

type dagRunInfo struct {
	failedTask        string
	tasksRun          int
	successfullyTasks int
	failedTasks       int
	time              string
}

func DockerImageInit(image string) *DockerImage {
	return &DockerImage{imageName: image}
}

func (d *DockerImage) Build(config airflowTypes.ImageBuildConfig) error {
	// Change to location of Dockerfile
	err := os.Chdir(config.Path)
	if err != nil {
		return err
	}

	args := []string{
		"build",
		"-t",
		d.imageName,
		".",
	}
	if config.NoCache {
		args = append(args, "--no-cache")
	}

	if len(config.TargetPlatforms) > 0 {
		args = append(args, fmt.Sprintf("--platform=%s", strings.Join(config.TargetPlatforms, ",")))
	}
	// Build image
	var stdout, stderr io.Writer
	if config.Output {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = nil
		stderr = nil
	}
	err = cmdExec(DockerCmd, stdout, stderr, args...)
	if err != nil {
		return fmt.Errorf("command 'docker build -t %s failed: %w", d.imageName, err)
	}

	return nil
}

func (d *DockerImage) Pytest(pytestFile, airflowHome, envFile string, pytestArgs []string, config airflowTypes.ImageBuildConfig) (string, error) {
	// delete container
	err := cmdExec(DockerCmd, nil, nil, "rm", "astro-pytest")
	if err != nil {
		log.Debug(err)
	}
	// Change to location of Dockerfile
	err = os.Chdir(config.Path)
	if err != nil {
		return "", err
	}
	args := []string{
		"run",
		"-i",
		"--name",
		"astro-pytest",
		"-v",
		airflowHome + "/dags:/usr/local/airflow/dags:ro",
		"-v",
		airflowHome + "/plugins:/usr/local/airflow/plugins:z",
		"-v",
		airflowHome + "/include:/usr/local/airflow/include:z",
		"-v",
		airflowHome + "/.astro:/usr/local/airflow/.astro:z",
		"-v",
		airflowHome + "/tests:/usr/local/airflow/tests:z",
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
	if config.Output {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = nil
		stderr = nil
	}
	// run pytest
	err = cmdExec(DockerCmd, stdout, stderr, args...)
	if err != nil {
		// delete container
		err2 := cmdExec(DockerCmd, nil, stderr, "rm", "astro-pytest")
		if err2 != nil {
			log.Debug(err2)
		}
		return "", err
	}

	// get exit code
	args = []string{
		"inspect",
		"astro-pytest",
		"--format='{{.State.ExitCode}}'",
	}
	var outb bytes.Buffer
	err = cmdExec(DockerCmd, &outb, stderr, args...)
	if err != nil {
		return "", fmt.Errorf("command 'docker inspect astro-pytest failed: %w", err)
	}

	// delete container
	err = cmdExec(DockerCmd, nil, stderr, "rm", "astro-pytest")
	if err != nil {
		log.Debug(err)
	}

	return outb.String(), nil
}

func (d *DockerImage) Push(registry, username, token, remoteImage string) error {
	err := cmdExec(DockerCmd, nil, nil, "tag", d.imageName, remoteImage)
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
		return err
	}

	// Delete the image tags we just generated
	err = cmdExec(DockerCmd, nil, nil, "rmi", remoteImage)
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
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	labelFmt := fmt.Sprintf("{{ index .Config.Labels %q }}", labelName)
	var label string
	err := cmdExec(DockerCmd, stdout, stderr, "inspect", "--format", labelFmt, d.imageName)
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
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	var labels map[string]string
	err := cmdExec(DockerCmd, stdout, stderr, "inspect", "--format", "{{ json .Config.Labels }}", d.imageName)
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
	err := cmdExec(DockerCmd, nil, nil, "tag", localImage, d.imageName)
	if err != nil {
		return fmt.Errorf("command 'docker tag %s %s' failed: %w", localImage, d.imageName, err)
	}
	return nil
}

func (d *DockerImage) RunTest(dagID, envFile, settingsFile, startDate string, taskLogs bool) error {
	// delete container
	stderr := new(bytes.Buffer)
	err := cmdExec(DockerCmd, nil, stderr, "rm", "astro-run")
	if err != nil {
		log.Warn(err)
	}
	log.Debugf("testing!!")
	args := []string{
		"run",
		"-i",
		"--name",
		"astro-run",
		"-v",
		config.WorkingPath + "/dags:/usr/local/airflow/dags",
		"-v",
		config.WorkingPath + "/" + settingsFile + ":/usr/local/" + settingsFile,
		"-e",
		"DAG_DIR=./dags/",
		"-e",
		"DAG_ID=" + dagID,
		"-e",
		"SETTINGS_FILE=/usr/local/" + settingsFile,
	}
	fileExist, err := util.Exists(config.WorkingPath + "/" + envFile)
	if err != nil {
		log.Debug(err)
	}
	if fileExist {
		args = append(args, []string{"--env-file", envFile}...)
	}
	args = append(args, []string{d.imageName}...)
	if startDate != "" {
		startDateArgs := []string{"-e", "START_DATE=" + startDate}
		args = append(args, startDateArgs...)
	}

	fmt.Println("\nStarting a DAG run for " + dagID + "...")
	fmt.Println("\nLoading DAGS...")

	runInfo, err := RunCommandCh(taskLogs, "\n", DockerCmd, args...)
	if err != nil {
		log.Error("command 'docker run -it %s failed: %w", d.imageName, err)
	}

	// delete container
	err = cmdExec(DockerCmd, nil, stderr, "rm", "astro-run")
	if err != nil {
		log.Debug(err)
	}
	fmt.Println("\nDAG Run Summary 🏁")
	fmt.Println("\n  DAG name: " + dagID)
	fmt.Printf("  Total tasks ran: %d\n", runInfo.tasksRun)
	fmt.Printf("  Successful tasks: %d\n", runInfo.successfullyTasks)
	if runInfo.failedTasks != 0 {
		fmt.Printf("  Error: The task %v in DAG %v appears to have failed\n", ansi.Bold(runInfo.failedTask), ansi.Bold(dagID))
	}
	if runInfo.time != "" {
		fmt.Printf(" Time to run: %v seconds\n", runInfo.time)
	}

	fmt.Println("\nSee the output of this command for errors. To view task logs, use the --task-logs` flag.")
	return nil
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

// When login and push do not work use bash to run docker commands
func useBash(authConfig *cliTypes.AuthConfig, image string) error {
	var err error
	if authConfig.Username != "" { // Case for cloud image push where we have both registry user & pass, for software login happens during `astro login` itself
		err = cmdExec(EchoCmd, nil, nil, fmt.Sprintf("%q", authConfig.Password), "|", DockerCmd, "login", authConfig.ServerAddress, "-u", authConfig.Username, "--password-stdin")
	}
	if err != nil {
		return err
	}
	// docker push <image>
	err = cmdExec(DockerCmd, os.Stdout, os.Stderr, "push", image)
	if err != nil {
		return err
	}
	return nil
}

// RunCommandCh runs an arbitrary command and streams output to a channnel.
func RunCommandCh(taskLogs bool, cutset string, command string, flags ...string) (dagRunInfo, error) { //stdoutCh chan<- string,
	var (
		tasks         int
		successfulRun int
		time          string
		failedTask    string
	)
	cmd := exec.Command(command, flags...)
	log.Debugf("testing!!")

	stdOutput, err := cmd.StdoutPipe()
	if err != nil {
		return dagRunInfo{}, fmt.Errorf("RunCommand: cmd.StdoutPipe(): %v", err)
	}

	stdError, err := cmd.StderrPipe()
	if err != nil {
		return dagRunInfo{}, fmt.Errorf("RunCommand: cmd.StderrPipe(): %v", err)
	}

	if err := cmd.Start(); err != nil {
		return dagRunInfo{}, fmt.Errorf("RunCommand: cmd.Start(): %v", err)
	}

	for {
		bufOut := make([]byte, 1024)
		bufErr := make([]byte, 1024)
		n, err1 := stdOutput.Read(bufOut)
		o, err2 := stdError.Read(bufErr)

		if o == 0 && n == 0 {
			break
		}
		if err1 != nil {
			if err1 != io.EOF {
				log.Fatal(err1)
			}
		}
		if err2 != nil {
			if err2 != io.EOF {
				log.Fatal(err2)
			}
		}
		outText := strings.TrimSpace(string(bufOut[:n]))
		// fmt.Println("out:"+outText)

		errText := strings.TrimSpace(string(bufErr[:o]))
		if errText != "" && !strings.Contains(errText, "+ python ./run_local_dag.py") {
			fmt.Println("\n\t" + errText)
		}

		for {
			// Take the index of any of the given cutset
			n := strings.IndexAny(outText, cutset)
			if n == -1 {
				// If not found, but still have data, parse it
				if strings.Contains(outText, "Running task ") {
					if taskLogs {
						fmt.Printf("\n")
					}
					taskName := strings.ReplaceAll(outText, "Running task ", "")
					fmt.Printf("\nRunning task " + ansi.Bold(taskName) + "...")
					failedTask = taskName
					// fmt.Println("\n" + outText + "...")
					tasks++
				} else if strings.Contains(outText, "Time:  ") {
					// fmt.Println("\n" + outText)
					time = strings.ReplaceAll(outText, "Time:  ", "")
				} else if strings.Contains(outText, " successfully!") {
					// fmt.Printf(ansi.Green("✔ ") + ansi.Bold(strings.ReplaceAll(outText, " ran successfully!", "")) + " ran successfully!\n\n")
					// fmt.Println(ansi.Green("\nTask " + outText))
					if taskLogs {
						// fmt.Printf("\n")
						// fmt.Printf("\n" + ansi.Green("✔ ") + ansi.Bold(strings.ReplaceAll(outText, " ran successfully!", "")) + " ran successfully!\n\n")
					}
					fmt.Printf(ansi.Green("success ✔\n\n"))
					successfulRun++
				} else if time == "" {
					// log.Debugf("\t" + outText)
					if taskLogs {
						fmt.Println("\t" + outText)
					}
				}
				break
			}
			// parse data from cutset
			if strings.Contains(outText[:n], "Running task ") {
				taskName := strings.ReplaceAll(outText[:n], "Running task ", "")
				fmt.Printf("\nRunning task " + taskName + "...")
				if taskLogs {
					fmt.Printf("\n")
				}
				// fmt.Println("\n" + outText[:n] + "...")
				failedTask = taskName
				tasks++
			} else if strings.Contains(outText[:n], "Time:  ") {
				// fmt.Println("\n" + outText[:n])
				time = strings.ReplaceAll(outText[:n], "Time:  ", "")
			} else if strings.Contains(outText[:n], " ran successfully!") {
				if taskLogs {
					// fmt.Printf("\n")
					// fmt.Printf("\n" + ansi.Green("✔ ") + ansi.Bold(strings.ReplaceAll(outText[:n], " ran successfully!", "")) + " ran successfully!\n\n")
				}
				// fmt.Printf(ansi.Green("✔ ") + ansi.Bold(strings.ReplaceAll(outText[:n], " ran successfully!", "")) + " ran successfully!\n\n")
				fmt.Printf(ansi.Green("success ✔\n\n"))
				successfulRun++
			} else if time == "" {
				// log.Debugf("\t" + outText[:n])
				if taskLogs {
					fmt.Println("\t" + outText[:n])
				}
			}
			// If cutset is last element, stop there.
			if n == len(outText) {
				break
			}
			// Shift the text and start again.
			outText = outText[n+1:]
		}
		if o == 0 && n == 0 {
			break
		}
	}
	// if tasks - successfulRun != 0 {
	// 	fmt.Println("\nThe last task to run appears to have failed")
	// }
	runInfo := dagRunInfo{
		failedTask:        failedTask,
		tasksRun:          tasks,
		successfullyTasks: successfulRun,
		failedTasks:       tasks - successfulRun,
		time:              time,
	}
	return runInfo, nil
}
