package airflow

import (
	"context"
	"os"

	cliconfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/registry"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type DockerRegistry struct {
	registry string
}

func DockerRegistryInit(registryName string) *DockerRegistry {
	return &DockerRegistry{registry: registryName}
}

// Login executes a docker login similar to docker login command
func (d *DockerRegistry) Login(username, token string) error {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	cli.NegotiateAPIVersion(ctx)

	// Remove http|https from serverAddress
	serverAddress := registry.ConvertToHostname(d.registry)

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

	cliAuthConfig := cliTypes.AuthConfig(*authConfig)

	// Get this idea from docker login cli
	cliAuthConfig.RegistryToken = ""

	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	creds := configFile.GetCredentialsStore(serverAddress)

	if err := creds.Store(cliAuthConfig); err != nil {
		return errors.Errorf("Error saving credentials: %v", err)
	}
	return nil
}
