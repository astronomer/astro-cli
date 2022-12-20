// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import (
	types "github.com/astronomer/astro-cli/airflow/types"
	mock "github.com/stretchr/testify/mock"
)

// ImageHandler is an autogenerated mock type for the ImageHandler type
type ImageHandler struct {
	mock.Mock
}

// Build provides a mock function with given fields: config
func (_m *ImageHandler) Build(config types.ImageBuildConfig) error {
	ret := _m.Called(config)

	var r0 error
	if rf, ok := ret.Get(0).(func(types.ImageBuildConfig) error); ok {
		r0 = rf(config)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetLabel provides a mock function with given fields: labelName
func (_m *ImageHandler) GetLabel(labelName string) (string, error) {
	ret := _m.Called(labelName)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(labelName)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(labelName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListLabels provides a mock function with given fields:
func (_m *ImageHandler) ListLabels() (map[string]string, error) {
	ret := _m.Called()

	var r0 map[string]string
	if rf, ok := ret.Get(0).(func() map[string]string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
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

// Pytest provides a mock function with given fields: pytestFile, airflowHome, envFile, pytestArgs, config
func (_m *ImageHandler) Pytest(pytestFile string, airflowHome string, envFile string, pytestArgs []string, config types.ImageBuildConfig) (string, error) {
	ret := _m.Called(pytestFile, airflowHome, envFile, pytestArgs, config)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string, string, []string, types.ImageBuildConfig) string); ok {
		r0 = rf(pytestFile, airflowHome, envFile, pytestArgs, config)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, string, []string, types.ImageBuildConfig) error); ok {
		r1 = rf(pytestFile, airflowHome, envFile, pytestArgs, config)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Run provides a mock function with given fields: dagID, envFile, settingsFile, containerName, dagFile, taskLogs
func (_m *ImageHandler) Run(dagID string, envFile string, settingsFile string, containerName string, dagFile string, taskLogs bool) error {
	ret := _m.Called(dagID, envFile, settingsFile, containerName, dagFile, taskLogs)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string, bool) error); ok {
		r0 = rf(dagID, envFile, settingsFile, containerName, dagFile, taskLogs)
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

type mockConstructorTestingTNewImageHandler interface {
	mock.TestingT
	Cleanup(func())
}

// NewImageHandler creates a new instance of ImageHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewImageHandler(t mockConstructorTestingTNewImageHandler) *ImageHandler {
	mock := &ImageHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
