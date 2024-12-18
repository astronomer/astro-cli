// Code generated by mockery v2.50.0. DO NOT EDIT.

package mocks

import (
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

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

	if len(ret) == 0 {
		panic("no return value specified for Bash")
	}

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

	if len(ret) == 0 {
		panic("no return value specified for ComposeExport")
	}

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

	if len(ret) == 0 {
		panic("no return value specified for ExportSettings")
	}

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

	if len(ret) == 0 {
		panic("no return value specified for ImportSettings")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, bool, bool, bool) error); ok {
		r0 = rf(settingsFile, envFile, connections, variables, pools)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Kill provides a mock function with no fields
func (_m *ContainerHandler) Kill() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Kill")
	}

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

	if len(ret) == 0 {
		panic("no return value specified for Logs")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool, ...string) error); ok {
		r0 = rf(follow, containerNames...)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PS provides a mock function with no fields
func (_m *ContainerHandler) PS() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for PS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Parse provides a mock function with given fields: customImageName, deployImageName, buildSecretString
func (_m *ContainerHandler) Parse(customImageName string, deployImageName string, buildSecretString string) error {
	ret := _m.Called(customImageName, deployImageName, buildSecretString)

	if len(ret) == 0 {
		panic("no return value specified for Parse")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string) error); ok {
		r0 = rf(customImageName, deployImageName, buildSecretString)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pytest provides a mock function with given fields: pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString
func (_m *ContainerHandler) Pytest(pytestFile string, customImageName string, deployImageName string, pytestArgsString string, buildSecretString string) (string, error) {
	ret := _m.Called(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString)

	if len(ret) == 0 {
		panic("no return value specified for Pytest")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string) (string, error)); ok {
		return rf(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString)
	}
	if rf, ok := ret.Get(0).(func(string, string, string, string, string) string); ok {
		r0 = rf(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, string, string, string) error); ok {
		r1 = rf(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Run provides a mock function with given fields: args, user
func (_m *ContainerHandler) Run(args []string, user string) error {
	ret := _m.Called(args, user)

	if len(ret) == 0 {
		panic("no return value specified for Run")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func([]string, string) error); ok {
		r0 = rf(args, user)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RunDAG provides a mock function with given fields: dagID, settingsFile, dagFile, executionDate, noCache, taskLogs
func (_m *ContainerHandler) RunDAG(dagID string, settingsFile string, dagFile string, executionDate string, noCache bool, taskLogs bool) error {
	ret := _m.Called(dagID, settingsFile, dagFile, executionDate, noCache, taskLogs)

	if len(ret) == 0 {
		panic("no return value specified for RunDAG")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, bool, bool) error); ok {
		r0 = rf(dagID, settingsFile, dagFile, executionDate, noCache, taskLogs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Start provides a mock function with given fields: imageName, settingsFile, composeFile, buildSecretString, noCache, noBrowser, waitTime, envConns
func (_m *ContainerHandler) Start(imageName string, settingsFile string, composeFile string, buildSecretString string, noCache bool, noBrowser bool, waitTime time.Duration, envConns map[string]astrocore.EnvironmentObjectConnection) error {
	ret := _m.Called(imageName, settingsFile, composeFile, buildSecretString, noCache, noBrowser, waitTime, envConns)

	if len(ret) == 0 {
		panic("no return value specified for Start")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, bool, bool, time.Duration, map[string]astrocore.EnvironmentObjectConnection) error); ok {
		r0 = rf(imageName, settingsFile, composeFile, buildSecretString, noCache, noBrowser, waitTime, envConns)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Stop provides a mock function with given fields: waitForExit
func (_m *ContainerHandler) Stop(waitForExit bool) error {
	ret := _m.Called(waitForExit)

	if len(ret) == 0 {
		panic("no return value specified for Stop")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(waitForExit)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpgradeTest provides a mock function with given fields: runtimeVersion, deploymentID, newImageName, customImageName, buildSecretString, dependencyTest, versionTest, dagTest, astroPlatformCore
func (_m *ContainerHandler) UpgradeTest(runtimeVersion string, deploymentID string, newImageName string, customImageName string, buildSecretString string, dependencyTest bool, versionTest bool, dagTest bool, astroPlatformCore astroplatformcore.ClientWithResponsesInterface) error {
	ret := _m.Called(runtimeVersion, deploymentID, newImageName, customImageName, buildSecretString, dependencyTest, versionTest, dagTest, astroPlatformCore)

	if len(ret) == 0 {
		panic("no return value specified for UpgradeTest")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string, bool, bool, bool, astroplatformcore.ClientWithResponsesInterface) error); ok {
		r0 = rf(runtimeVersion, deploymentID, newImageName, customImageName, buildSecretString, dependencyTest, versionTest, dagTest, astroPlatformCore)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewContainerHandler creates a new instance of ContainerHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewContainerHandler(t interface {
	mock.TestingT
	Cleanup(func())
}) *ContainerHandler {
	mock := &ContainerHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
