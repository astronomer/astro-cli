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

	cliCommand "github.com/docker/cli/cli/command"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	log "github.com/sirupsen/logrus"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	// "github.com/astronomer/astro-cli/pkg/ansi"
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

func (d *DockerImage) RunTest(dagID, envFile, settingsFile, startDate string) error {
	args := []string{
		"run",
		"-i",
		"-v",
		config.WorkingPath+"/dags:/usr/local/airflow/dags",
		"-v",
		config.WorkingPath+"/"+settingsFile+":/usr/local/"+settingsFile,
		"-e",
		"DAG_DIR=./dags/",
		"-e",
		"DAG_ID="+dagID,
		"-e",
		"SETTINGS_FILE=/usr/local/"+settingsFile,
	}
	if startDate != "" {
		startDateArgs := []string{"-e", "START_DATE="+startDate}
		args = append(args, startDateArgs...)
	}

	endArgs := []string{"--env-file", envFile, d.imageName,}
	args = append(args, endArgs...)

	// Run Image
	err := cmdExec(DockerCmd, os.Stdout, os.Stderr, args...)
	if err != nil {
		return err
	}

	return nil

}


// func (d *DockerImage) RunTest(dagID, envFile, settingsFile, startDate string) error {
// 	args := []string{
// 		"run",
// 		"-i",
// 		"-v",
// 		config.WorkingPath+"/dags:/usr/local/airflow/dags",
// 		"-v",
// 		config.WorkingPath+"/"+settingsFile+":/usr/local/"+settingsFile,
// 		"-e",
// 		"DAG_DIR=./dags/",
// 		"-e",
// 		"DAG_ID="+dagID,
// 		"-e",
// 		"SETTINGS_FILE=/usr/local/"+settingsFile,
// 		"--env-file",
// 		envFile,
// 		// "dimberman/local-airflow-test:0.0.2",
// 		d.imageName,
// 	}
// 	if startDate != "" {
// 		startDateArgs := []string{"-e", "START_DATE="+startDate}
// 		args = append(args, startDateArgs...)
// 	}
// 	// Run Image
// 	// var stdout, stderr io.Writer
// 	var stdBuffer bytes.Buffer
// 	mw := io.Writer(&stdBuffer)

// 	// fmt.Println(args)

// 	// ch := make(chan string)

// 	// args = []string{
// 	// 	"ps",
// 	// }

// 	// go func() {
// 	// 	err := RunCommandCh(ch, "\n", DockerCmd, args...)
// 	// 	if err != nil {
// 	// 		log.Fatal("command 'docker run -it %s failed: %w", d.imageName, err)
// 	// 	}
//     // }()
// 	fmt.Println("Running DAG "+dagID+"...")
// 	fmt.Println("\nLoading DAGS...")
// 	err := cmdExec(DockerCmd, mw, mw, args...)
// 	if err != nil {
// 		return err
// 	}

// 	outText := stdBuffer.String()
// 	tasks := 0
// 	success := 0
// 	for {
// 		// Take the index of any of the given cutset
// 		n := strings.IndexAny(outText, "\n")
// 		if n == -1 {
// 			break
// 		}

// 		if strings.Contains(outText[:n], "Running task ") {
// 			fmt.Println("\n"+outText[:n]+"...")
// 			tasks = tasks + 1
// 		} else if strings.Contains(outText[:n], " successfully!") {
// 			fmt.Println(ansi.Green("\n"+outText[:n]))
// 			success = success + 1
// 		} else if strings.Contains(outText[:n], "Time:  ")  {
// 			fmt.Println("\n"+outText[:n])
// 		} else {
// 			log.Debugf("\t"+outText[:n])
// 		}
// 		if n == len(outText) {
// 			break
// 	}
// 		// Shift the text and start again.
// 		outText = outText[n+1:]
// 	}

// 	if tasks == success {
// 		fmt.Printf("All tasks in %s ran successfully", dagID)
// 	} else {
// 		fmt.Printf("%s out of %s tasks ran successfully", success, tasks)
// 	}

//     // for v := range ch {
//     //         fmt.Println(v)
//     // }
// 	return nil
// }

// func (d *DockerImage) RunTest(dagID, envFile, settingsFile, startDate string) error {
// 	args := []string{
// 		"run",
// 		"-i",
// 		"-v",
// 		config.WorkingPath+"/dags:/usr/local/airflow/dags",
// 		"-v",
// 		config.WorkingPath+"/"+settingsFile+":/usr/local/"+settingsFile,
// 		"-e",
// 		"DAG_DIR=./dags/",
// 		"-e",
// 		"DAG_ID="+dagID,
// 		"-e",
// 		"SETTINGS_FILE=/usr/local/"+settingsFile,
// 		"--env-file",
// 		envFile,
// 		d.imageName,
// 	}
// 	if startDate != "" {
// 		startDateArgs := []string{"-e", "START_DATE="+startDate}
// 		args = append(args, startDateArgs...)
// 	}
// 	// Run Image
// 	// fmt.Println(args)
// 	ch := make(chan string)

// 	go func() {
// 		err := RunCommandCh(ch, "\n", DockerCmd, args...)
// 		if err != nil {
// 			log.Fatal("command 'docker run -it %s failed: %w", d.imageName, err)
// 		}
//     }()

//     for v := range ch {
//             fmt.Println(v)
//     }
// 	return nil
// }

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
// func RunCommandCh(stdoutCh chan<- string, cutset string, command string, flags ...string) error {
//     cmd := exec.Command(command, flags...)

// 	// fmt.Println(command)
// 	fmt.Println(flags)
// 	fmt.Println("\n\n")

//     stdOutput, err := cmd.StdoutPipe()
//     if err != nil {
//         return fmt.Errorf("RunCommand: cmd.StdoutPipe(): %v", err)
//     }

// 	stdError, err := cmd.StderrPipe()
//     if err != nil {
//         return fmt.Errorf("RunCommand: cmd.StderrPipe(): %v", err)
//     }

//     if err := cmd.Start(); err != nil {
//             return fmt.Errorf("RunCommand: cmd.Start(): %v", err)
//     }

//     go func() {
//             defer close(stdoutCh)

//             for {
//                     bufOut := make([]byte, 1024)
// 					bufErr := make([]byte, 1024)
//                     n, err := stdOutput.Read(bufOut)
//                     o, err := stdError.Read(bufErr)

//                     if err != nil {
// 						if err != io.EOF {
// 								log.Fatal(err)
// 						}
// 						if o == 0 {
// 								break
// 						}
// 						if n == 0 {
// 							break
// 						}
//                     }
//                     outText := strings.TrimSpace(string(bufOut[:n]))
// 					// fmt.Println("out:"+outText)
	
//                     errText := strings.TrimSpace(string(bufErr[:o]))
// 					fmt.Println("error:"+errText)
//                     for {
//                             // Take the index of any of the given cutset
//                             n := strings.IndexAny(outText, cutset)
//                             if n == -1 {
//                                     // If not found, but still have data, send it
//                                     if len(outText) > 0 {// && strings.Contains(outText, "Running") {
//                                             stdoutCh <- outText
//                                     }
//                                     break
//                             }
//                             // Send data up to the found cutset
// 							// if strings.Contains(outText[:n], "Running task ") {
// 							// 	stdoutCh <-"\n"+outText[:n]+"\n"
// 							// } else if strings.Contains(outText[:n], " successfully!") {
// 							// 	stdoutCh <-"\n"+outText[:n]+"\n"
// 							// } else {
// 							// 	stdoutCh <- outText[:n]
// 							// }
// 							// fmt.Println(outText[:n])
// 							stdoutCh <- outText[:n]
//                             // If cutset is last element, stop there.
//                             if n == len(outText) {
//                                     break
//                             }
//                             // Shift the text and start again.
//                             outText = outText[n+1:]
//                     }
//             }
//     }()

//     if err := cmd.Wait(); err != nil {
//             return fmt.Errorf("RunCommand: cmd.Wait(): %v", err)
//     }
//     return nil
// }