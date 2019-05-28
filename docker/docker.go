package docker

import (
	"context"
	"os"
	"os/exec"

	"github.com/docker/docker/registry"

	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
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

// ExecLogin executes a docker login similar to docker login command
func ExecLogin(serverAddress, username, password string) error {
	var response registrytypes.AuthenticateOKBody
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	// Remove http|https from serverAddress
	serverAddress = registry.ConvertToHostname(serverAddress)

	authConfig := &types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      username,
		Password:      password,
	}

	response, _ = cli.RegistryLogin(ctx, *authConfig)

	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	creds := configFile.GetCredentialsStore(serverAddress)

	if err := creds.Store(types.AuthConfig(*authConfig)); err != nil {
		return errors.Errorf("Error saving credentials: %v", err)
	}

	if response.Status != "" {
		return errors.Errorf("Error saving credentials: %v", response.Status)
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
