package airflow

import (
	"context"
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/astronomer/astro-cli/pkg/logger"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/registry"
)

type DockerRegistry struct {
	registry string
	cli      DockerRegistryAPI
}

func DockerRegistryInit(registryName string) (*DockerRegistry, error) {
	runtime, err := runtimes.GetContainerRuntime()
	if err != nil {
		logger.Info("Please make sure you have Docker or Podman installed and running.")
		return nil, err
	}
	cli, err := runtime.NewDockerClient()
	if err != nil {
		return nil, err
	}
	return &DockerRegistry{registry: registryName, cli: cli}, nil
}

// Login executes a docker login similar to docker login command
func (d *DockerRegistry) Login(username, token string) error {
	ctx := context.Background()

	d.cli.NegotiateAPIVersion(ctx)

	// Remove http|https from serverAddress
	serverAddress := registry.ConvertToHostname(d.registry)

	authConfig := registrytypes.AuthConfig{
		ServerAddress: serverAddress,
		Username:      username,
		Password:      token,
	}

	if username == "" && token == "" {
		configFile := cliConfig.LoadDefaultConfigFile(os.Stderr)

		creds := configFile.GetCredentialsStore(d.registry)
		auth, err := creds.Get(d.registry)
		if err != nil {
			return err
		}
		authConfig.Username = auth.Username
		authConfig.Password = auth.Password
	}

	logger.Debugf("docker creds %v \n", authConfig)
	_, err := d.cli.RegistryLogin(ctx, authConfig)
	if err != nil {
		return fmt.Errorf("registry login error: %w", err)
	}

	cliAuthConfig := cliTypes.AuthConfig(authConfig)

	// Get this idea from docker login cli
	cliAuthConfig.RegistryToken = ""

	configFile := cliConfig.LoadDefaultConfigFile(os.Stderr)

	creds := configFile.GetCredentialsStore(serverAddress)

	if err := creds.Store(cliAuthConfig); err != nil {
		return fmt.Errorf("error saving credentials: %w", err)
	}
	return nil
}
