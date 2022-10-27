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
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
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

type DockerBinder struct {
	cli *client.Client
}

type DockerBind interface {
	ImageBuild(ctx context.Context, buildContext io.Reader, options *types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error)
	ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error)
	ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error)
}

func (d DockerBinder) ImageBuild(ctx context.Context, buildContext io.Reader, options *types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	return d.cli.ImageBuild(ctx, buildContext, *options)
}

func (d DockerBinder) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	return d.cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (d DockerBinder) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	return d.cli.ContainerStart(ctx, containerID, options)
}

func (d DockerBinder) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (body <-chan container.ContainerWaitOKBody, err <-chan error) {
	return d.cli.ContainerWait(ctx, containerID, condition)
}

func (d DockerBinder) ContainerLogs(ctx context.Context, containerID string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return d.cli.ContainerLogs(ctx, containerID, options)
}

var newDockerClient = func() (DockerBind, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerBinder{cli: cli}, nil
}

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd, args []string, flags map[string]string, mountDirs []string) {
	ctx := context.Background()
	cli, err := newDockerClient()
	if err != nil {
		err = fmt.Errorf("docker client initialisation failed for command %v: %w", cmd, err)
		panic(err)
	}

	astroSQLCliVersion := GetPypiVersion(astroSQLCliProjectURL)

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, PythonVersion, astroSQLCliVersion))
	err = os.WriteFile(SQLCliDockerfilePath, dockerfileContent, SQLCLIDockerfileWriteMode)
	if err != nil {
		err = fmt.Errorf("error writing dockerfile for command %v: %w", cmd, err)
		panic(err)
	}
	defer os.Remove(SQLCliDockerfilePath)

	opts := types.ImageBuildOptions{
		Dockerfile: SQLCliDockerfilePath,
		Tags:       []string{SQLCliDockerImageName},
	}

	body, err := cli.ImageBuild(ctx, getContext(SQLCliDockerfilePath), &opts)
	if err != nil {
		err = fmt.Errorf("image building failed for command %v: %w", cmd, err)
		panic(err)
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)
	if err != nil {
		err = fmt.Errorf("image build response read failed for command %v: %w", cmd, err)
		panic(err)
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
		err = fmt.Errorf("docker container creation failed for command %v: %w", cmd, err)
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		err = fmt.Errorf("docker container start failed for command %v: %w", cmd, err)
		panic(err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			err = fmt.Errorf("docker client run failed for command %v: %w", cmd, err)
			panic(err)
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		err = fmt.Errorf("docker container logs fetching failed for command %v: %w", cmd, err)
		panic(err)
	}

	_, err = io.Copy(os.Stdout, cout)

	if err != nil {
		err = fmt.Errorf("docker logs forwarding failed for command %v: %w", cmd, err)
		panic(err)
	}
}
