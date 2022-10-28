package mocks

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	mock "github.com/stretchr/testify/mock"
)

type DockerBind struct {
	mock.Mock
}

func (_m *DockerBind) ImageBuild(ctx context.Context, buildContext io.Reader, options *types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	ret := _m.Called(ctx, buildContext, *options)

	var r0 types.ImageBuildResponse
	if rf, ok := ret.Get(0).(func(context.Context, io.Reader, *types.ImageBuildOptions) types.ImageBuildResponse); ok {
		r0 = rf(ctx, buildContext, options)
	} else {
		r0 = ret.Get(0).(types.ImageBuildResponse)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, io.Reader, *types.ImageBuildOptions) error); ok {
		r1 = rf(ctx, buildContext, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

func (_m *DockerBind) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	ret := _m.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)

	var r0 container.ContainerCreateCreatedBody
	if rf, ok := ret.Get(0).(func(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *specs.Platform, string) container.ContainerCreateCreatedBody); ok {
		r0 = rf(ctx, config, hostConfig, networkingConfig, platform, containerName)
	} else {
		r0 = ret.Get(0).(container.ContainerCreateCreatedBody)
	}

	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *specs.Platform, string) error); ok {
		r1 = rf(ctx, config, hostConfig, networkingConfig, platform, containerName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

func (_m *DockerBind) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, types.ContainerStartOptions) error); ok {
		r0 = rf(ctx, containerID, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Logs provides a mock function with given fields: follow, containerNames
func (_m *DockerBind) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	ret := _m.Called()

	r0 := make(<-chan container.ContainerWaitOKBody)
	if rf, ok := ret.Get(0).(func(context.Context, string, container.WaitCondition) <-chan container.ContainerWaitOKBody); ok {
		r0 = rf(ctx, containerID, condition)
	} else {
		r0 = ret.Get(0).(<-chan container.ContainerWaitOKBody)
	}

	r1 := make(<-chan error, 1)
	if rf, ok := ret.Get(0).(func(context.Context, string, container.WaitCondition) <-chan error); ok {
		r1 = rf(ctx, containerID, condition)
	} else {
		errChannel := make(chan error, 1)
		errChannel <- ret.Error(1)
		r1 = errChannel
	}

	return r0, r1

}

// PS provides a mock function with given fields:
func (_m *DockerBind) ContainerLogs(ctx context.Context, containerID string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	ret := _m.Called(ctx, containerID, options)

	var r0 io.ReadCloser
	if rf, ok := ret.Get(0).(func(context.Context, string, types.ContainerLogsOptions) io.ReadCloser); ok {
		r0 = rf(ctx, containerID, options)
	} else {
		r0 = ret.Get(0).(io.ReadCloser)
	}

	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, types.ContainerLogsOptions) error); ok {
		r1 = rf(ctx, containerID, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewContainerHandler interface {
	mock.TestingT
	Cleanup(func())
}

// NewContainerHandler creates a new instance of ContainerHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewContainerHandler(t mockConstructorTestingTNewContainerHandler) *DockerBind {
	mock := &DockerBind{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
