// Code generated by mockery v2.32.0. DO NOT EDIT.

package airflow_mocks

import (
	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	mock "github.com/stretchr/testify/mock"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// CreateConnection provides a mock function with given fields: airflowURL, conn
func (_m *Client) CreateConnection(airflowURL string, conn *airflowclient.Connection) error {
	ret := _m.Called(airflowURL, conn)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, *airflowclient.Connection) error); ok {
		r0 = rf(airflowURL, conn)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreatePool provides a mock function with given fields: airflowURL, pool
func (_m *Client) CreatePool(airflowURL string, pool airflowclient.Pool) error {
	ret := _m.Called(airflowURL, pool)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, airflowclient.Pool) error); ok {
		r0 = rf(airflowURL, pool)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateVariable provides a mock function with given fields: airflowURL, variable
func (_m *Client) CreateVariable(airflowURL string, variable airflowclient.Variable) error {
	ret := _m.Called(airflowURL, variable)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, airflowclient.Variable) error); ok {
		r0 = rf(airflowURL, variable)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetConnections provides a mock function with given fields: airflowURL
func (_m *Client) GetConnections(airflowURL string) (airflowclient.Response, error) {
	ret := _m.Called(airflowURL)

	var r0 airflowclient.Response
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (airflowclient.Response, error)); ok {
		return rf(airflowURL)
	}
	if rf, ok := ret.Get(0).(func(string) airflowclient.Response); ok {
		r0 = rf(airflowURL)
	} else {
		r0 = ret.Get(0).(airflowclient.Response)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(airflowURL)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetPools provides a mock function with given fields: airflowURL
func (_m *Client) GetPools(airflowURL string) (airflowclient.Response, error) {
	ret := _m.Called(airflowURL)

	var r0 airflowclient.Response
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (airflowclient.Response, error)); ok {
		return rf(airflowURL)
	}
	if rf, ok := ret.Get(0).(func(string) airflowclient.Response); ok {
		r0 = rf(airflowURL)
	} else {
		r0 = ret.Get(0).(airflowclient.Response)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(airflowURL)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetVariables provides a mock function with given fields: airflowURL
func (_m *Client) GetVariables(airflowURL string) (airflowclient.Response, error) {
	ret := _m.Called(airflowURL)

	var r0 airflowclient.Response
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (airflowclient.Response, error)); ok {
		return rf(airflowURL)
	}
	if rf, ok := ret.Get(0).(func(string) airflowclient.Response); ok {
		r0 = rf(airflowURL)
	} else {
		r0 = ret.Get(0).(airflowclient.Response)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(airflowURL)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateConnection provides a mock function with given fields: airflowURL, conn
func (_m *Client) UpdateConnection(airflowURL string, conn *airflowclient.Connection) error {
	ret := _m.Called(airflowURL, conn)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, *airflowclient.Connection) error); ok {
		r0 = rf(airflowURL, conn)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdatePool provides a mock function with given fields: airflowURL, pool
func (_m *Client) UpdatePool(airflowURL string, pool airflowclient.Pool) error {
	ret := _m.Called(airflowURL, pool)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, airflowclient.Pool) error); ok {
		r0 = rf(airflowURL, pool)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateVariable provides a mock function with given fields: airflowURL, variable
func (_m *Client) UpdateVariable(airflowURL string, variable airflowclient.Variable) error {
	ret := _m.Called(airflowURL, variable)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, airflowclient.Variable) error); ok {
		r0 = rf(airflowURL, variable)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewClient creates a new instance of Client. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *Client {
	mock := &Client{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
