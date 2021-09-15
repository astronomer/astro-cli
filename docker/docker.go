package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/registry"

	"github.com/docker/docker/pkg/jsonmessage"

	clicommand "github.com/docker/cli/cli/command"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"

	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	// Docker is the docker command.
	Docker = "docker"
)

type loginOptions struct {
	serverAddress string
	user          string
	password      string
	passwordStdin bool
}

// Exec executes a docker command
func Exec(args ...string) error {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		return errors.Wrap(lookErr, "failed to find the docker binary")
	}

	cmd := exec.Command(Docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if cmdErr := cmd.Run(); cmdErr != nil {
		return errors.Wrapf(cmdErr, "failed to execute cmd")
	}

	return nil
}

// ExecPush does push image to registry using native docker client,
// instead of using `docker push` in bash
func ExecPush(serverAddress, token, image string) error {
	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(serverAddress)
	// TODO: rethink how to reuse creds store
	authConfig.Password = token

	log.Debugf("Exec Push docker creds %v \n", authConfig)
	if err != nil {
		log.Debugf("Error reading credentials: %v", err)
		return errors.Errorf("Error reading credentials: %v", err)
	}

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Debugf("Error setting up new Client ops %v", err)
		panic(err)
	}
	cli.NegotiateAPIVersion(ctx)
	buf, err := json.Marshal(authConfig)

	if err != nil {
		log.Debugf("Error negotiating api version: %v", err)
		return err
	}
	encodedAuth := base64.URLEncoding.EncodeToString(buf)
	responseBody, err := cli.ImagePush(ctx, image, types.ImagePushOptions{RegistryAuth: encodedAuth})

	if err != nil {
		log.Debugf("Error pushing image to docker: %v", err)
		return err
	}
	defer responseBody.Close()
	out := clicommand.NewOutStream(os.Stdout)
	return jsonmessage.DisplayJSONMessagesToStream(responseBody, out, nil)
}

// ExecLogin executes a docker login similar to docker login command
func ExecLogin(serverAddress, username, token string) error {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	cli.NegotiateAPIVersion(ctx)

	// Remove http|https from serverAddress
	serverAddress = registry.ConvertToHostname(serverAddress)

	authConfig := &types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      username,
		Password:      token,
	}

	log.Debugf("docker creds %v \n", authConfig)
	_, err = cli.RegistryLogin(ctx, *authConfig)
	if err != nil {
		return errors.Errorf("registry login error: %v", err)
	}

	// Get this idea from docker login cli
	authConfig.RegistryToken = ""

	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	creds := configFile.GetCredentialsStore(serverAddress)

	if err := creds.Store(*authConfig); err != nil {
		return errors.Errorf("Error saving credentials: %v", err)
	}
	return nil
}

// AirflowCommand is the main method of interaction with Airflow
func AirflowCommand(id string, airflowCommand string) string {

	cmd := exec.Command("docker", "exec", "-it", id, "bash", "-c", airflowCommand)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()

	if err != nil {
		_ = errors.Wrapf(err, "error encountered")
	}

	stringOut := string(out)
	return stringOut
}

// ExecPipe does pipe stream into stdout/stdin and stderr
// so now we can pipe out during exec'ing any commands inside container
func ExecPipe(resp types.HijackedResponse, inStream io.Reader, outStream, errorStream io.Writer) error {
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
			io.Copy(resp.Conn, inStream)
		}

		if err := resp.CloseWrite(); err != nil {
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
