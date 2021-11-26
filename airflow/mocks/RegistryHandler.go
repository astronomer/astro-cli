// Code generated by mockery v2.9.4. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// RegistryHandler is an autogenerated mock type for the RegistryHandler type
type RegistryHandler struct {
	mock.Mock
}

// Login provides a mock function with given fields: username, token
func (_m *RegistryHandler) Login(username string, token string) error {
	ret := _m.Called(username, token)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(username, token)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
