package airflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/messages"

	clicommand "github.com/docker/cli/cli/command"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type DockerImage struct {
	imageName string
}

func DockerImageInit(image string) *DockerImage {
	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	return &DockerImage{imageName: image}
}

func (d *DockerImage) Build(path string) error {
	// Change to location of Dockerfile
	err := os.Chdir(path)
	if err != nil {
		return err
	}
	imageName := imageName(d.imageName, "latest")
	// Build image
	err = dockerExec("build", "-t", imageName, ".")
	if err != nil {
		return errors.Wrapf(err, "command 'docker build -t %s failed", d.imageName)
	}

	return nil
}

func (d *DockerImage) Push(cloudDomain, token, remoteImageTag string) error {
	registry := "registry." + cloudDomain
	remoteImage := fmt.Sprintf("%s/%s", registry, imageName(d.imageName, remoteImageTag))

	err := dockerExec("tag", imageName(d.imageName, "latest"), remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker tag %s %s' failed", d.imageName, remoteImage)
	}

	// Push image to registry
	fmt.Println(messages.PushingImagePrompt)

	configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(registry)
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
	responseBody, err := cli.ImagePush(ctx, remoteImage, types.ImagePushOptions{RegistryAuth: encodedAuth})
	if err != nil {
		log.Debugf("Error pushing image to docker: %v", err)
		return err
	}
	defer responseBody.Close()
	out := clicommand.NewOutStream(os.Stdout)
	err = jsonmessage.DisplayJSONMessagesToStream(responseBody, out, nil)
	if err != nil {
		return err
	}

	// Delete the image tags we just generated
	err = dockerExec("rmi", remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker rmi %s' failed", remoteImage)
	}
	return nil
}
