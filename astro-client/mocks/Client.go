// Code generated by mockery v2.14.0. DO NOT EDIT.

package astro_mocks

import (
	astro "github.com/astronomer/astro-cli/astro-client"
	mock "github.com/stretchr/testify/mock"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// CreateDeployment provides a mock function with given fields: input
func (_m *Client) CreateDeployment(input *astro.CreateDeploymentInput) (astro.Deployment, error) {
	ret := _m.Called(input)

	var r0 astro.Deployment
	if rf, ok := ret.Get(0).(func(*astro.CreateDeploymentInput) astro.Deployment); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.Deployment)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*astro.CreateDeploymentInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateImage provides a mock function with given fields: input
func (_m *Client) CreateImage(input astro.CreateImageInput) (*astro.Image, error) {
	ret := _m.Called(input)

	var r0 *astro.Image
	if rf, ok := ret.Get(0).(func(astro.CreateImageInput) *astro.Image); ok {
		r0 = rf(input)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astro.Image)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.CreateImageInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateUserInvite provides a mock function with given fields: input
func (_m *Client) CreateUserInvite(input astro.CreateUserInviteInput) (astro.UserInvite, error) {
	ret := _m.Called(input)

	var r0 astro.UserInvite
	if rf, ok := ret.Get(0).(func(astro.CreateUserInviteInput) astro.UserInvite); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.UserInvite)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.CreateUserInviteInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteDeployment provides a mock function with given fields: input
func (_m *Client) DeleteDeployment(input astro.DeleteDeploymentInput) (astro.Deployment, error) {
	ret := _m.Called(input)

	var r0 astro.Deployment
	if rf, ok := ret.Get(0).(func(astro.DeleteDeploymentInput) astro.Deployment); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.Deployment)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.DeleteDeploymentInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeployImage provides a mock function with given fields: input
func (_m *Client) DeployImage(input astro.DeployImageInput) (*astro.Image, error) {
	ret := _m.Called(input)

	var r0 *astro.Image
	if rf, ok := ret.Get(0).(func(astro.DeployImageInput) *astro.Image); ok {
		r0 = rf(input)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astro.Image)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.DeployImageInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetDeployment provides a mock function with given fields: deploymentID
func (_m *Client) GetDeployment(deploymentID string) (astro.Deployment, error) {
	ret := _m.Called(deploymentID)

	var r0 astro.Deployment
	if rf, ok := ret.Get(0).(func(string) astro.Deployment); ok {
		r0 = rf(deploymentID)
	} else {
		r0 = ret.Get(0).(astro.Deployment)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(deploymentID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetDeploymentConfig provides a mock function with given fields:
func (_m *Client) GetDeploymentConfig() (astro.DeploymentConfig, error) {
	ret := _m.Called()

	var r0 astro.DeploymentConfig
	if rf, ok := ret.Get(0).(func() astro.DeploymentConfig); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(astro.DeploymentConfig)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetDeploymentHistory provides a mock function with given fields: vars
func (_m *Client) GetDeploymentHistory(vars map[string]interface{}) (astro.DeploymentHistory, error) {
	ret := _m.Called(vars)

	var r0 astro.DeploymentHistory
	if rf, ok := ret.Get(0).(func(map[string]interface{}) astro.DeploymentHistory); ok {
		r0 = rf(vars)
	} else {
		r0 = ret.Get(0).(astro.DeploymentHistory)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(map[string]interface{}) error); ok {
		r1 = rf(vars)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetUserInfo provides a mock function with given fields:
func (_m *Client) GetUserInfo() (*astro.Self, error) {
	ret := _m.Called()

	var r0 *astro.Self
	if rf, ok := ret.Get(0).(func() *astro.Self); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astro.Self)
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

// GetWorkerQueueOptions provides a mock function with given fields:
func (_m *Client) GetWorkerQueueOptions() (astro.WorkerQueueDefaultOptions, error) {
	ret := _m.Called()

	var r0 astro.WorkerQueueDefaultOptions
	if rf, ok := ret.Get(0).(func() astro.WorkerQueueDefaultOptions); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(astro.WorkerQueueDefaultOptions)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetWorkspace provides a mock function with given fields: workspaceID
func (_m *Client) GetWorkspace(workspaceID string) (astro.Workspace, error) {
	ret := _m.Called(workspaceID)

	var r0 astro.Workspace
	if rf, ok := ret.Get(0).(func(string) astro.Workspace); ok {
		r0 = rf(workspaceID)
	} else {
		r0 = ret.Get(0).(astro.Workspace)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(workspaceID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// InitiateDagDeployment provides a mock function with given fields: input
func (_m *Client) InitiateDagDeployment(input astro.InitiateDagDeploymentInput) (astro.InitiateDagDeployment, error) {
	ret := _m.Called(input)

	var r0 astro.InitiateDagDeployment
	if rf, ok := ret.Get(0).(func(astro.InitiateDagDeploymentInput) astro.InitiateDagDeployment); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.InitiateDagDeployment)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.InitiateDagDeploymentInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListClusters provides a mock function with given fields: organizationID
func (_m *Client) ListClusters(organizationID string) ([]astro.Cluster, error) {
	ret := _m.Called(organizationID)

	var r0 []astro.Cluster
	if rf, ok := ret.Get(0).(func(string) []astro.Cluster); ok {
		r0 = rf(organizationID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]astro.Cluster)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(organizationID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListDeployments provides a mock function with given fields: organizationID, workspaceID
func (_m *Client) ListDeployments(organizationID string, workspaceID string) ([]astro.Deployment, error) {
	ret := _m.Called(organizationID, workspaceID)

	var r0 []astro.Deployment
	if rf, ok := ret.Get(0).(func(string, string) []astro.Deployment); ok {
		r0 = rf(organizationID, workspaceID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]astro.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(organizationID, workspaceID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListWorkspaces provides a mock function with given fields: organizationID
func (_m *Client) ListWorkspaces(organizationID string) ([]astro.Workspace, error) {
	ret := _m.Called(organizationID)

	var r0 []astro.Workspace
	if rf, ok := ret.Get(0).(func(string) []astro.Workspace); ok {
		r0 = rf(organizationID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]astro.Workspace)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(organizationID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ModifyDeploymentVariable provides a mock function with given fields: input
func (_m *Client) ModifyDeploymentVariable(input astro.EnvironmentVariablesInput) ([]astro.EnvironmentVariablesObject, error) {
	ret := _m.Called(input)

	var r0 []astro.EnvironmentVariablesObject
	if rf, ok := ret.Get(0).(func(astro.EnvironmentVariablesInput) []astro.EnvironmentVariablesObject); ok {
		r0 = rf(input)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]astro.EnvironmentVariablesObject)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(astro.EnvironmentVariablesInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ReportDagDeploymentStatus provides a mock function with given fields: input
func (_m *Client) ReportDagDeploymentStatus(input *astro.ReportDagDeploymentStatusInput) (astro.DagDeploymentStatus, error) {
	ret := _m.Called(input)

	var r0 astro.DagDeploymentStatus
	if rf, ok := ret.Get(0).(func(*astro.ReportDagDeploymentStatusInput) astro.DagDeploymentStatus); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.DagDeploymentStatus)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*astro.ReportDagDeploymentStatusInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateDeployment provides a mock function with given fields: input
func (_m *Client) UpdateDeployment(input *astro.UpdateDeploymentInput) (astro.Deployment, error) {
	ret := _m.Called(input)

	var r0 astro.Deployment
	if rf, ok := ret.Get(0).(func(*astro.UpdateDeploymentInput) astro.Deployment); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Get(0).(astro.Deployment)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*astro.UpdateDeploymentInput) error); ok {
		r1 = rf(input)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewClient interface {
	mock.TestingT
	Cleanup(func())
}

// NewClient creates a new instance of Client. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewClient(t mockConstructorTestingTNewClient) *Client {
	mock := &Client{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
