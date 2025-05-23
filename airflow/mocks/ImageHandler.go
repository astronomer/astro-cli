// Code generated by mockery v2.50.0. DO NOT EDIT.

package mocks

import (
	io "io"

	types "github.com/astronomer/astro-cli/airflow/types"
	mock "github.com/stretchr/testify/mock"
)

// ImageHandler is an autogenerated mock type for the ImageHandler type
type ImageHandler struct {
	mock.Mock
}

// Build provides a mock function with given fields: dockerfile, buildSecretString, config
func (_m *ImageHandler) Build(dockerfile string, buildSecretString string, config types.ImageBuildConfig) error {
	ret := _m.Called(dockerfile, buildSecretString, config)

	if len(ret) == 0 {
		panic("no return value specified for Build")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, types.ImageBuildConfig) error); ok {
		r0 = rf(dockerfile, buildSecretString, config)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreatePipFreeze provides a mock function with given fields: altImageName, pipFreezeFile
func (_m *ImageHandler) CreatePipFreeze(altImageName string, pipFreezeFile string) error {
	ret := _m.Called(altImageName, pipFreezeFile)

	if len(ret) == 0 {
		panic("no return value specified for CreatePipFreeze")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(altImageName, pipFreezeFile)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DoesImageExist provides a mock function with given fields: image
func (_m *ImageHandler) DoesImageExist(image string) error {
	ret := _m.Called(image)

	if len(ret) == 0 {
		panic("no return value specified for DoesImageExist")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(image)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetImageRepoSHA provides a mock function with given fields: registry
func (_m *ImageHandler) GetImageRepoSHA(registry string) (string, error) {
	ret := _m.Called(registry)

	if len(ret) == 0 {
		panic("no return value specified for GetImageRepoSHA")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (string, error)); ok {
		return rf(registry)
	}
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(registry)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(registry)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetLabel provides a mock function with given fields: altImageName, labelName
func (_m *ImageHandler) GetLabel(altImageName string, labelName string) (string, error) {
	ret := _m.Called(altImageName, labelName)

	if len(ret) == 0 {
		panic("no return value specified for GetLabel")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (string, error)); ok {
		return rf(altImageName, labelName)
	}
	if rf, ok := ret.Get(0).(func(string, string) string); ok {
		r0 = rf(altImageName, labelName)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(altImageName, labelName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListLabels provides a mock function with no fields
func (_m *ImageHandler) ListLabels() (map[string]string, error) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ListLabels")
	}

	var r0 map[string]string
	var r1 error
	if rf, ok := ret.Get(0).(func() (map[string]string, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() map[string]string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]string)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Pull provides a mock function with given fields: remoteImage, username, token
func (_m *ImageHandler) Pull(remoteImage string, username string, token string) error {
	ret := _m.Called(remoteImage, username, token)

	if len(ret) == 0 {
		panic("no return value specified for Pull")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string) error); ok {
		r0 = rf(remoteImage, username, token)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Push provides a mock function with given fields: remoteImage, username, token, getImageRepoSha
func (_m *ImageHandler) Push(remoteImage string, username string, token string, getImageRepoSha bool) (string, error) {
	ret := _m.Called(remoteImage, username, token, getImageRepoSha)

	if len(ret) == 0 {
		panic("no return value specified for Push")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string, bool) (string, error)); ok {
		return rf(remoteImage, username, token, getImageRepoSha)
	}
	if rf, ok := ret.Get(0).(func(string, string, string, bool) string); ok {
		r0 = rf(remoteImage, username, token, getImageRepoSha)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, string, bool) error); ok {
		r1 = rf(remoteImage, username, token, getImageRepoSha)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Pytest provides a mock function with given fields: pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config
func (_m *ImageHandler) Pytest(pytestFile string, airflowHome string, envFile string, testHomeDirectory string, pytestArgs []string, htmlReport bool, config types.ImageBuildConfig) (string, error) {
	ret := _m.Called(pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config)

	if len(ret) == 0 {
		panic("no return value specified for Pytest")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, []string, bool, types.ImageBuildConfig) (string, error)); ok {
		return rf(pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config)
	}
	if rf, ok := ret.Get(0).(func(string, string, string, string, []string, bool, types.ImageBuildConfig) string); ok {
		r0 = rf(pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, string, string, []string, bool, types.ImageBuildConfig) error); ok {
		r1 = rf(pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RunCommand provides a mock function with given fields: args, mountDirs, stdout, stderr
func (_m *ImageHandler) RunCommand(args []string, mountDirs map[string]string, stdout io.Writer, stderr io.Writer) error {
	ret := _m.Called(args, mountDirs, stdout, stderr)

	if len(ret) == 0 {
		panic("no return value specified for RunCommand")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func([]string, map[string]string, io.Writer, io.Writer) error); ok {
		r0 = rf(args, mountDirs, stdout, stderr)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RunDAG provides a mock function with given fields: dagID, envFile, settingsFile, containerName, dagFile, executionDate, taskLogs
func (_m *ImageHandler) RunDAG(dagID string, envFile string, settingsFile string, containerName string, dagFile string, executionDate string, taskLogs bool) error {
	ret := _m.Called(dagID, envFile, settingsFile, containerName, dagFile, executionDate, taskLogs)

	if len(ret) == 0 {
		panic("no return value specified for RunDAG")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string, string, bool) error); ok {
		r0 = rf(dagID, envFile, settingsFile, containerName, dagFile, executionDate, taskLogs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TagLocalImage provides a mock function with given fields: localImage
func (_m *ImageHandler) TagLocalImage(localImage string) error {
	ret := _m.Called(localImage)

	if len(ret) == 0 {
		panic("no return value specified for TagLocalImage")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(localImage)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewImageHandler creates a new instance of ImageHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewImageHandler(t interface {
	mock.TestingT
	Cleanup(func())
}) *ImageHandler {
	mock := &ImageHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
