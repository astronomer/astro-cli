package sql

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/sql/include"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	SQLCliDockerfilePath  = ".Dockerfile.sql_cli"
	SQLCliDockerImageName = "sql_cli"
	PythonVersion         = "3.9"
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd, args []string, flags map[string]string, absoluteProjectDir string, mountDirectory string) error {
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

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, PythonVersion, astroSQLCliVersion))
	err = ioutil.WriteFile(SQLCliDockerfilePath, dockerfileContent, 0644)
	if err != nil {
		return fmt.Errorf("error writing dockerfile %w", err)
	}
	defer os.Remove(SQLCliDockerfilePath)

	opts := types.ImageBuildOptions{
		Dockerfile: SQLCliDockerfilePath,
		Tags:       []string{SQLCliDockerImageName},
	}

	body, err := cli.ImageBuild(ctx, getContext(SQLCliDockerfilePath), opts)

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

	cmd = append(cmd, args...)

	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: SQLCliDockerImageName,
		Cmd:   cmd,
		Tty:   false,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: absoluteProjectDir,
				Target: mountDirectory,
			},
		},
	}, nil, nil, "")
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

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
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
