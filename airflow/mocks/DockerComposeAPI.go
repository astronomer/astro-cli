// Code generated by mockery v2.13.1. DO NOT EDIT.

package mocks

import (
	context "context"

	api "github.com/docker/compose/v2/pkg/api"

	mock "github.com/stretchr/testify/mock"

	types "github.com/compose-spec/compose-go/types"
)

// DockerComposeAPI is an autogenerated mock type for the DockerComposeAPI type
type DockerComposeAPI struct {
	mock.Mock
}

// Build provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.BuildOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Convert provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Convert(ctx context.Context, project *types.Project, options api.ConvertOptions) ([]byte, error) {
	ret := _m.Called(ctx, project, options)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.ConvertOptions) []byte); ok {
		r0 = rf(ctx, project, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *types.Project, api.ConvertOptions) error); ok {
		r1 = rf(ctx, project, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Copy provides a mock function with given fields: ctx, project, opts
func (_m *DockerComposeAPI) Copy(ctx context.Context, project *types.Project, opts api.CopyOptions) error {
	ret := _m.Called(ctx, project, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.CopyOptions) error); ok {
		r0 = rf(ctx, project, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Create provides a mock function with given fields: ctx, project, opts
func (_m *DockerComposeAPI) Create(ctx context.Context, project *types.Project, opts api.CreateOptions) error {
	ret := _m.Called(ctx, project, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.CreateOptions) error); ok {
		r0 = rf(ctx, project, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Down provides a mock function with given fields: ctx, projectName, options
func (_m *DockerComposeAPI) Down(ctx context.Context, projectName string, options api.DownOptions) error {
	ret := _m.Called(ctx, projectName, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.DownOptions) error); ok {
		r0 = rf(ctx, projectName, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Events provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Events(ctx context.Context, project string, options api.EventsOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.EventsOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Exec provides a mock function with given fields: ctx, project, opts
func (_m *DockerComposeAPI) Exec(ctx context.Context, project string, opts api.RunOptions) (int, error) {
	ret := _m.Called(ctx, project, opts)

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context, string, api.RunOptions) int); ok {
		r0 = rf(ctx, project, opts)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, api.RunOptions) error); ok {
		r1 = rf(ctx, project, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Images provides a mock function with given fields: ctx, projectName, options
func (_m *DockerComposeAPI) Images(ctx context.Context, projectName string, options api.ImagesOptions) ([]api.ImageSummary, error) {
	ret := _m.Called(ctx, projectName, options)

	var r0 []api.ImageSummary
	if rf, ok := ret.Get(0).(func(context.Context, string, api.ImagesOptions) []api.ImageSummary); ok {
		r0 = rf(ctx, projectName, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.ImageSummary)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, api.ImagesOptions) error); ok {
		r1 = rf(ctx, projectName, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Kill provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Kill(ctx context.Context, project *types.Project, options api.KillOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.KillOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// List provides a mock function with given fields: ctx, options
func (_m *DockerComposeAPI) List(ctx context.Context, options api.ListOptions) ([]api.Stack, error) {
	ret := _m.Called(ctx, options)

	var r0 []api.Stack
	if rf, ok := ret.Get(0).(func(context.Context, api.ListOptions) []api.Stack); ok {
		r0 = rf(ctx, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.Stack)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, api.ListOptions) error); ok {
		r1 = rf(ctx, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Logs provides a mock function with given fields: ctx, projectName, consumer, options
func (_m *DockerComposeAPI) Logs(ctx context.Context, projectName string, consumer api.LogConsumer, options api.LogOptions) error {
	ret := _m.Called(ctx, projectName, consumer, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.LogConsumer, api.LogOptions) error); ok {
		r0 = rf(ctx, projectName, consumer, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pause provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Pause(ctx context.Context, project string, options api.PauseOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PauseOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Port provides a mock function with given fields: ctx, project, service, port, options
func (_m *DockerComposeAPI) Port(ctx context.Context, project string, service string, port int, options api.PortOptions) (string, int, error) {
	ret := _m.Called(ctx, project, service, port, options)

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context, string, string, int, api.PortOptions) string); ok {
		r0 = rf(ctx, project, service, port, options)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 int
	if rf, ok := ret.Get(1).(func(context.Context, string, string, int, api.PortOptions) int); ok {
		r1 = rf(ctx, project, service, port, options)
	} else {
		r1 = ret.Get(1).(int)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(context.Context, string, string, int, api.PortOptions) error); ok {
		r2 = rf(ctx, project, service, port, options)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// Ps provides a mock function with given fields: ctx, projectName, options
func (_m *DockerComposeAPI) Ps(ctx context.Context, projectName string, options api.PsOptions) ([]api.ContainerSummary, error) {
	ret := _m.Called(ctx, projectName, options)

	var r0 []api.ContainerSummary
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PsOptions) []api.ContainerSummary); ok {
		r0 = rf(ctx, projectName, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.ContainerSummary)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, api.PsOptions) error); ok {
		r1 = rf(ctx, projectName, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Pull provides a mock function with given fields: ctx, project, opts
func (_m *DockerComposeAPI) Pull(ctx context.Context, project *types.Project, opts api.PullOptions) error {
	ret := _m.Called(ctx, project, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.PullOptions) error); ok {
		r0 = rf(ctx, project, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Push provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.PushOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Remove provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Remove(ctx context.Context, project *types.Project, options api.RemoveOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.RemoveOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Restart provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Restart(ctx context.Context, project *types.Project, options api.RestartOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.RestartOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RunOneOffContainer provides a mock function with given fields: ctx, project, opts
func (_m *DockerComposeAPI) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	ret := _m.Called(ctx, project, opts)

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.RunOptions) int); ok {
		r0 = rf(ctx, project, opts)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *types.Project, api.RunOptions) error); ok {
		r1 = rf(ctx, project, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Start provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Start(ctx context.Context, project *types.Project, options api.StartOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.StartOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Stop provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Stop(ctx context.Context, project *types.Project, options api.StopOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.StopOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Top provides a mock function with given fields: ctx, projectName, services
func (_m *DockerComposeAPI) Top(ctx context.Context, projectName string, services []string) ([]api.ContainerProcSummary, error) {
	ret := _m.Called(ctx, projectName, services)

	var r0 []api.ContainerProcSummary
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) []api.ContainerProcSummary); ok {
		r0 = rf(ctx, projectName, services)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.ContainerProcSummary)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, []string) error); ok {
		r1 = rf(ctx, projectName, services)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UnPause provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) UnPause(ctx context.Context, project string, options api.PauseOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PauseOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Up provides a mock function with given fields: ctx, project, options
func (_m *DockerComposeAPI) Up(ctx context.Context, project *types.Project, options api.UpOptions) error {
	ret := _m.Called(ctx, project, options)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.Project, api.UpOptions) error); ok {
		r0 = rf(ctx, project, options)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewDockerComposeAPI interface {
	mock.TestingT
	Cleanup(func())
}

// NewDockerComposeAPI creates a new instance of DockerComposeAPI. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewDockerComposeAPI(t mockConstructorTestingTNewDockerComposeAPI) *DockerComposeAPI {
	mock := &DockerComposeAPI{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
