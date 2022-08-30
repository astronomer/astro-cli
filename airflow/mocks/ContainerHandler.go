// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

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

// Parse provides a mock function with given fields: imageName, buildImage
func (_m *ContainerHandler) Parse(imageName string, buildImage string) error {
	ret := _m.Called(imageName, buildImage)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(imageName, buildImage)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pytest provides a mock function with given fields: imageName, pytestFile, projectImageName
func (_m *ContainerHandler) Pytest(imageName string, pytestFile string, projectImageName string) (string, error) {
	ret := _m.Called(imageName, pytestFile, projectImageName)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string, string) string); ok {
		r0 = rf(imageName, pytestFile, projectImageName)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(imageName, pytestFile, projectImageName)
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

// Start provides a mock function with given fields: imageName, noCache, noBrowser
func (_m *ContainerHandler) Start(imageName string, noCache bool, noBrowser bool) error {
	ret := _m.Called(imageName, noCache, noBrowser)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, bool, bool) error); ok {
		r0 = rf(imageName, noCache, noBrowser)
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
