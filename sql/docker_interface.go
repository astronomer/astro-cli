package sql

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
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
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
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

func (d DockerBinder) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	return d.cli.ContainerRemove(ctx, containerID, options)
}

func NewDockerClient() (DockerBind, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerBinder{cli: cli}, nil
}
