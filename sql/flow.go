package sql

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	SQLCliDockefilePathEnvVar = "SQL_CLI_DOCKERFILE_PATH"
	// TODO Remove the need for user to have the Dockerfile.sql_cli in their local machine alongside where the CLI binary resides to make this work.
	SQLCliDockerfileName  = "Dockerfile.sql_cli"
	SQLCliDockerImageName = "sql_cli"
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd []string, flags map[string]string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		err = fmt.Errorf("docker client initialisation failed %w", err)
		return err
	}

	astroSQLCliVersion, err := GetPypiVersion(astroSQLCliProjectURL)
	if err != nil {
		return err
	}

	opts := types.ImageBuildOptions{
		Dockerfile: SQLCliDockerfileName,
		Tags:       []string{SQLCliDockerImageName},
	}

	if astroSQLCliVersion != "" {
		opts.BuildArgs = map[string]*string{"VERSION": &astroSQLCliVersion}
	}

	dockerfilePath := os.Getenv(SQLCliDockefilePathEnvVar)
	if dockerfilePath == "" {
		return EnvVarNotSetError(SQLCliDockefilePathEnvVar)
	}

	body, err := cli.ImageBuild(ctx, getContext(dockerfilePath), opts)
	if err != nil {
		err = fmt.Errorf("image building failed %w ", err)
		return err
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)
	if err != nil {
		err = fmt.Errorf("image build response read failed %w", err)
		return err
	}

	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: SQLCliDockerImageName,
		Cmd:   cmd,
		Tty:   false,
	}, nil, nil, nil, "")
	if err != nil {
		err = fmt.Errorf("docker container creation failed %w", err)
		return err
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		err = fmt.Errorf("docker container start failed %w", err)
		return err
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			err = fmt.Errorf("docker client run failed %w", err)
			return err
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		err = fmt.Errorf("docker container logs fetching failed %w", err)
		return err
	}

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, cout)

	if err != nil {
		err = fmt.Errorf("docker logs forwarding failed %w", err)
		return err
	}

	return nil
}
