package airflow

import (
	"context"
	"fmt"
	"os"

	cliconfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/registry"
	log "github.com/sirupsen/logrus"
)

type DockerRegistry struct {
	registry string
	cli      DockerRegistryAPI
}

func DockerRegistryInit(registryName string) (*DockerRegistry, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
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

	authConfig := types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      username,
		Password:      token,
	}

	log.Debugf("docker creds %v \n", authConfig)
	_, err := d.cli.RegistryLogin(ctx, authConfig)
	if err != nil {
		return fmt.Errorf("registry login error: %w", err)
	}

	cliAuthConfig := cliTypes.AuthConfig(authConfig)

	// Get this idea from docker login cli
	cliAuthConfig.RegistryToken = ""

	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	creds := configFile.GetCredentialsStore(serverAddress)

	if err := creds.Store(cliAuthConfig); err != nil {
		return fmt.Errorf("error saving credentials: %w", err)
	}
	return nil
}
