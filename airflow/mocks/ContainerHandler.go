// Code generated by mockery v2.20.2. DO NOT EDIT.

package mocks

import (
	astro "github.com/astronomer/astro-cli/astro-client"
	mock "github.com/stretchr/testify/mock"

	time "time"
)

// ContainerHandler is an autogenerated mock type for the ContainerHandler type
type ContainerHandler struct {
	mock.Mock
}

// Bash provides a mock function with given fields: container
func (_m *ContainerHandler) Bash(container string) error {
	ret := _m.Called(container)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(container)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ComposeExport provides a mock function with given fields: settingsFile, composeFile
func (_m *ContainerHandler) ComposeExport(settingsFile string, composeFile string) error {
	ret := _m.Called(settingsFile, composeFile)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(settingsFile, composeFile)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ExportSettings provides a mock function with given fields: settingsFile, envFile, connections, variables, pools, envExport
func (_m *ContainerHandler) ExportSettings(settingsFile string, envFile string, connections bool, variables bool, pools bool, envExport bool) error {
	ret := _m.Called(settingsFile, envFile, connections, variables, pools, envExport)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, bool, bool, bool, bool) error); ok {
		r0 = rf(settingsFile, envFile, connections, variables, pools, envExport)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ImportSettings provides a mock function with given fields: settingsFile, envFile, connections, variables, pools
func (_m *ContainerHandler) ImportSettings(settingsFile string, envFile string, connections bool, variables bool, pools bool) error {
	ret := _m.Called(settingsFile, envFile, connections, variables, pools)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, bool, bool, bool) error); ok {
		r0 = rf(settingsFile, envFile, connections, variables, pools)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Kill provides a mock function with given fields:
func (_m *ContainerHandler) Kill() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Logs provides a mock function with given fields: follow, containerNames
func (_m *ContainerHandler) Logs(follow bool, containerNames ...string) error {
	_va := make([]interface{}, len(containerNames))
	for _i := range containerNames {
		_va[_i] = containerNames[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, follow)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 error
	if rf, ok := ret.Get(0).(func(bool, ...string) error); ok {
		r0 = rf(follow, containerNames...)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PS provides a mock function with given fields:
func (_m *ContainerHandler) PS() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Parse provides a mock function with given fields: customImageName, deployImageName
func (_m *ContainerHandler) Parse(customImageName string, deployImageName string) error {
	ret := _m.Called(customImageName, deployImageName)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(customImageName, deployImageName)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pytest provides a mock function with given fields: pytestFile, customImageName, deployImageName, pytestArgsString
func (_m *ContainerHandler) Pytest(pytestFile string, customImageName string, deployImageName string, pytestArgsString string) (string, error) {
	ret := _m.Called(pytestFile, customImageName, deployImageName, pytestArgsString)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string, string) (string, error)); ok {
		return rf(pytestFile, customImageName, deployImageName, pytestArgsString)
	}
	if rf, ok := ret.Get(0).(func(string, string, string, string) string); ok {
		r0 = rf(pytestFile, customImageName, deployImageName, pytestArgsString)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, string, string) error); ok {
		r1 = rf(pytestFile, customImageName, deployImageName, pytestArgsString)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Run provides a mock function with given fields: args, user
func (_m *ContainerHandler) Run(args []string, user string) error {
	ret := _m.Called(args, user)

	var r0 error
	if rf, ok := ret.Get(0).(func([]string, string) error); ok {
		r0 = rf(args, user)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RunDAG provides a mock function with given fields: dagID, settingsFile, dagFile, noCache, taskLogs
func (_m *ContainerHandler) RunDAG(dagID string, settingsFile string, dagFile string, noCache bool, taskLogs bool) error {
	ret := _m.Called(dagID, settingsFile, dagFile, noCache, taskLogs)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, bool, bool) error); ok {
		r0 = rf(dagID, settingsFile, dagFile, noCache, taskLogs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Start provides a mock function with given fields: imageName, settingsFile, composeFile, noCache, noBrowser, waitTime
func (_m *ContainerHandler) Start(imageName string, settingsFile string, composeFile string, noCache bool, noBrowser bool, waitTime time.Duration) error {
	ret := _m.Called(imageName, settingsFile, composeFile, noCache, noBrowser, waitTime)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, bool, bool, time.Duration) error); ok {
		r0 = rf(imageName, settingsFile, composeFile, noCache, noBrowser, waitTime)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Stop provides a mock function with given fields:
func (_m *ContainerHandler) Stop() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpgradeTest provides a mock function with given fields: runtimeVersion, deploymentID, newImageName, dependencyTest, versionTest, dagTest, client
func (_m *ContainerHandler) UpgradeTest(runtimeVersion string, deploymentID string, newImageName string, dependencyTest bool, versionTest bool, dagTest bool, client astro.Client) error {
	ret := _m.Called(runtimeVersion, deploymentID, newImageName, dependencyTest, versionTest, dagTest, client)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, bool, bool, bool, astro.Client) error); ok {
		r0 = rf(runtimeVersion, deploymentID, newImageName, dependencyTest, versionTest, dagTest, client)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewContainerHandler interface {
	mock.TestingT
	Cleanup(func())
}

// NewContainerHandler creates a new instance of ContainerHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewContainerHandler(t mockConstructorTestingTNewContainerHandler) *ContainerHandler {
	mock := &ContainerHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
