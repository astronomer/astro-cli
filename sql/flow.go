package sql

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/sql/include"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
)

type MountVolume struct {
	SourceDirectory string
	TargetDirectory string
}

const (
	SQLCliDockerfilePath      = ".Dockerfile.sql_cli"
	SQLCLIDockerfileWriteMode = 0o600
	SQLCliDockerImageName     = "sql_cli"
	PythonVersion             = "3.9"
)

var (
	dockerClientInit = NewDockerClient
	ioCopy           = io.Copy
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd, args []string, flags map[string]string, mountDirs []string) error {
	ctx := context.Background()
	cli, err := dockerClientInit()
	if err != nil {
		err = fmt.Errorf("docker client initialization failed %w", err)
		return err
	}

	astroSQLCliVersion, err := getPypiVersion(astroSQLCliProjectURL)
	if err != nil {
		return err
	}

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, PythonVersion, astroSQLCliVersion))
	err = os.WriteFile(SQLCliDockerfilePath, dockerfileContent, SQLCLIDockerfileWriteMode)
	if err != nil {
		return fmt.Errorf("error writing dockerfile %w", err)
	}
	defer os.Remove(SQLCliDockerfilePath)

	opts := types.ImageBuildOptions{
		Dockerfile: SQLCliDockerfilePath,
		Tags:       []string{SQLCliDockerImageName},
	}

	body, err := cli.ImageBuild(ctx, getContext(SQLCliDockerfilePath), &opts)
	if err != nil {
		err = fmt.Errorf("image building failed %w", err)
		return err
	}
	buf := new(strings.Builder)
	_, err = ioCopy(buf, body.Body)
	if err != nil {
		err = fmt.Errorf("image build response read failed %w", err)
		return err
	}

	cmd = append(cmd, args...)

	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}

	binds := []string{}
	for _, moundDir := range mountDirs {
		binds = append(binds, []string{moundDir + ":" + moundDir}...)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: SQLCliDockerImageName,
		Cmd:   cmd,
		Tty:   true,
	}, &container.HostConfig{
		Binds: binds,
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
			err = fmt.Errorf("docker container wait failed %w", err)
			return err
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		err = fmt.Errorf("docker container logs fetching failed %w", err)
		return err
	}

	_, err = ioCopy(os.Stdout, cout)

	if err != nil {
		err = fmt.Errorf("docker logs forwarding failed %w", err)
		return err
	}

	return nil
}
