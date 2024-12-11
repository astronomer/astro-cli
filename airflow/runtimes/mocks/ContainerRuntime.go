// Code generated by mockery v2.32.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// ContainerRuntime is an autogenerated mock type for the ContainerRuntime type
type ContainerRuntime struct {
	mock.Mock
}

// Configure provides a mock function with given fields:
func (_m *ContainerRuntime) Configure() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ConfigureOrKill provides a mock function with given fields:
func (_m *ContainerRuntime) ConfigureOrKill() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Initialize provides a mock function with given fields:
func (_m *ContainerRuntime) Initialize() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Kill provides a mock function with given fields:
func (_m *ContainerRuntime) Kill() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewContainerRuntime creates a new instance of ContainerRuntime. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewContainerRuntime(t interface {
	mock.TestingT
	Cleanup(func())
}) *ContainerRuntime {
	mock := &ContainerRuntime{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
