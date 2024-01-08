// Code generated by mockery v2.32.0. DO NOT EDIT.

package mocks

import (
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

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, types.ImageBuildConfig) error); ok {
		r0 = rf(dockerfile, buildSecretString, config)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ConflictTest provides a mock function with given fields: workingDirectory, testHomeDirectory, buildConfig
func (_m *ImageHandler) ConflictTest(workingDirectory string, testHomeDirectory string, buildConfig types.ImageBuildConfig) (string, error) {
	ret := _m.Called(workingDirectory, testHomeDirectory, buildConfig)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, types.ImageBuildConfig) (string, error)); ok {
		return rf(workingDirectory, testHomeDirectory, buildConfig)
	}
	if rf, ok := ret.Get(0).(func(string, string, types.ImageBuildConfig) string); ok {
		r0 = rf(workingDirectory, testHomeDirectory, buildConfig)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, types.ImageBuildConfig) error); ok {
		r1 = rf(workingDirectory, testHomeDirectory, buildConfig)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreatePipFreeze provides a mock function with given fields: altImageName, pipFreezeFile
func (_m *ImageHandler) CreatePipFreeze(altImageName string, pipFreezeFile string) error {
	ret := _m.Called(altImageName, pipFreezeFile)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(image)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetLabel provides a mock function with given fields: altImageName, labelName
func (_m *ImageHandler) GetLabel(altImageName string, labelName string) (string, error) {
	ret := _m.Called(altImageName, labelName)

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

// ListLabels provides a mock function with given fields:
func (_m *ImageHandler) ListLabels() (map[string]string, error) {
	ret := _m.Called()

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

// Pull provides a mock function with given fields: registry, username, token, remoteImage
func (_m *ImageHandler) Pull(registry string, username string, token string, remoteImage string) error {
	ret := _m.Called(registry, username, token, remoteImage)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string) error); ok {
		r0 = rf(registry, username, token, remoteImage)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Push provides a mock function with given fields: registry, username, token, remoteImage
func (_m *ImageHandler) Push(registry string, username string, token string, remoteImage string) error {
	ret := _m.Called(registry, username, token, remoteImage)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string) error); ok {
		r0 = rf(registry, username, token, remoteImage)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pytest provides a mock function with given fields: pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config
func (_m *ImageHandler) Pytest(pytestFile string, airflowHome string, envFile string, testHomeDirectory string, pytestArgs []string, htmlReport bool, config types.ImageBuildConfig) (string, error) {
	ret := _m.Called(pytestFile, airflowHome, envFile, testHomeDirectory, pytestArgs, htmlReport, config)

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

// Run provides a mock function with given fields: dagID, envFile, settingsFile, containerName, dagFile, executionDate, taskLogs
func (_m *ImageHandler) Run(dagID string, envFile string, settingsFile string, containerName string, dagFile string, executionDate string, taskLogs bool) error {
	ret := _m.Called(dagID, envFile, settingsFile, containerName, dagFile, executionDate, taskLogs)

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
