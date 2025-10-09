package airflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/astronomer/astro-cli/pkg/logger"
	cliConfig "github.com/docker/cli/cli/config"
	cliTypes "github.com/docker/cli/cli/config/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/registry"
)

// DockerLogin is a testable variable that holds the docker login function
var DockerLogin = dockerLogin

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

// dockerLogin performs docker login using bash command instead of Docker API
// This is useful for OAuth-based registries that require special authentication flows
func dockerLogin(registryName, username, token string) error {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	if username != "" && token != "" {
		// Remove Bearer prefix if present (consistent with pushWithBash)
		const prefix = "Bearer "
		pass := strings.TrimPrefix(token, prefix)
		cmd := "echo \"" + pass + "\"" + " | " + containerRuntime + " login " + registryName + " -u " + username + " --password-stdin"
		err = cmdExec("bash", nil, os.Stderr, "-c", cmd)
		if err != nil {
			return fmt.Errorf("docker login failed: %w", err)
		}
	}

	return nil
}
