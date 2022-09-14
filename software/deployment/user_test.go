package deployment

import (
	"bytes"
	"testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestUserList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	// Test that UserList returns a single deployment user correctly
	mockUser := houston.DeploymentUser{
		ID:       "ckgqw2k2600081qc90nbamgno",
		FullName: "Some Person",
		Username: "somebody",
		RoleBindings: []houston.RoleBinding{
			{Role: houston.SystemAdminRole},
			{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "ws-1"}},
			{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "ws-2"}},
			{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "deply-1"}},
			{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "ckgqw2k2600081qc90nbage4h"}},
		},
		Emails: []houston.Email{
			{Address: "somebody@astronomer.io"},
		},
	}

	t.Run("single deployment user", func(t *testing.T) {
		deploymentID := "ckgqw2k2600081qc90nbage4h"

		expectedRequest := houston.ListDeploymentUsersRequest{
			UserID:       mockUser.ID,
			Email:        mockUser.Emails[0].Address,
			FullName:     mockUser.FullName,
			DeploymentID: deploymentID,
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{mockUser}, nil)

		buf := new(bytes.Buffer)
		err := UserList(deploymentID, mockUser.Emails[0].Address, mockUser.ID, mockUser.FullName, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person     somebody     DEPLOYMENT_ADMIN`)
		api.AssertExpectations(t)
	})

	t.Run("multiple users", func(t *testing.T) {
		deploymentID := "ckgqw2k2600081qc90nbage4h"
		mockUsers := []houston.DeploymentUser{
			mockUser,
			{
				ID: "ckgqw2k2600081qc90nbamgni",
				Emails: []houston.Email{
					{Address: "anotherperson@astronomer.io"},
				},
				FullName: "Another Person",
				Username: "anotherperson",
				RoleBindings: []houston.RoleBinding{
					{Role: houston.WorkspaceViewerRole},
					{Role: houston.DeploymentEditorRole, Deployment: houston.Deployment{ID: deploymentID}},
				},
			},
		}

		expectedRequest := houston.ListDeploymentUsersRequest{
			DeploymentID: deploymentID,
		}

		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return(mockUsers, nil)
		buf := new(bytes.Buffer)
		err := UserList(deploymentID, "", "", "", api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person        somebody          DEPLOYMENT_ADMIN`)
		assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgni     Another Person     anotherperson     DEPLOYMENT_EDITOR`)
		api.AssertExpectations(t)
	})

	// Test that UserList returns an empty list when deployment does not exist
	t.Run("empty list when deployment does not exist", func(t *testing.T) {
		deploymentID := "ckgqw2k2600081qc90nbamgno"
		expectedRequest := houston.ListDeploymentUsersRequest{
			DeploymentID: deploymentID,
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{}, nil)

		buf := new(bytes.Buffer)
		err := UserList(deploymentID, "", "", "", api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), houstonInvalidDeploymentUsersMsg)
		api.AssertExpectations(t)
	})

	t.Run("api error", func(t *testing.T) {
		deploymentID := "ckgqw2k2600081qc90nbamgno"
		expectedRequest := houston.ListDeploymentUsersRequest{
			DeploymentID: deploymentID,
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{}, errMock)

		buf := new(bytes.Buffer)
		err := UserList(deploymentID, "", "", "", api, buf)
		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})
}

func TestAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("add user success", func(t *testing.T) {
		mockUserRole := &houston.RoleBinding{
			Role: houston.DeploymentAdminRole,
			User: houston.RoleBindingUser{
				Username: "somebody@astronomer.io",
			},
			Deployment: houston.Deployment{
				ID:          "ckggzqj5f4157qtc9lescmehm",
				ReleaseName: "prehistoric-gravity-9229",
			},
		}

		expectedRequest := houston.UpdateDeploymentUserRequest{
			Email:        mockUserRole.User.Username,
			Role:         mockUserRole.Role,
			DeploymentID: mockUserRole.Deployment.ID,
		}

		api := new(mocks.ClientInterface)
		api.On("AddDeploymentUser", expectedRequest).Return(mockUserRole, nil)

		buf := new(bytes.Buffer)
		err := Add(mockUserRole.Deployment.ID, mockUserRole.User.Username, mockUserRole.Role, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully added somebody@astronomer.io as a DEPLOYMENT_ADMIN")
		api.AssertExpectations(t)
	})
	t.Run("add user api error", func(t *testing.T) {
		deploymentID := "ckggzqj5f4157qtc9lescmehm"
		email := "somebody@astronomer.com"
		role := houston.DeploymentAdminRole

		expectedRequest := houston.UpdateDeploymentUserRequest{
			Email:        email,
			Role:         role,
			DeploymentID: deploymentID,
		}

		api := new(mocks.ClientInterface)
		api.On("AddDeploymentUser", expectedRequest).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := Add(deploymentID, email, role, api, buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), errMock.Error())
		api.AssertExpectations(t)
	})
}

func TestDeleteUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("delete user success", func(t *testing.T) {
		mockUserRole := &houston.RoleBinding{
			User: houston.RoleBindingUser{Username: "somebody@astronomer.com"},
			Role: houston.DeploymentAdminRole,
			Deployment: houston.Deployment{
				ID:          "deploymentid",
				ReleaseName: "prehistoric-gravity-9229",
			},
		}

		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", mockUserRole.Deployment.ID, mockUserRole.User.Username).Return(mockUserRole, nil)

		buf := new(bytes.Buffer)
		err := RemoveUser(mockUserRole.Deployment.ID, mockUserRole.User.Username, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully removed the DEPLOYMENT_ADMIN role for somebody@astronomer.com from deployment deploymentid")
		api.AssertExpectations(t)
	})
	t.Run("delete user api error", func(t *testing.T) {
		deploymentID := "deploymentid"
		email := "somebody@astronomer.com"

		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", deploymentID, email).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveUser(deploymentID, email, api, buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), errMock.Error())
		api.AssertExpectations(t)
	})
}

func TestUpdateUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("update user success", func(t *testing.T) {
		mockUserRole := &houston.RoleBinding{
			Role: houston.DeploymentEditorRole,
			User: houston.RoleBindingUser{
				Username: "somebody@astronomer.com",
			},
			Deployment: houston.Deployment{
				ID:          "deployment-id",
				ReleaseName: "prehistoric-gravity-9229",
			},
		}

		expectedRequest := houston.UpdateDeploymentUserRequest{
			Email:        mockUserRole.User.Username,
			Role:         houston.DeploymentEditorRole,
			DeploymentID: mockUserRole.Deployment.ID,
		}

		api := new(mocks.ClientInterface)
		api.On("UpdateDeploymentUser", expectedRequest).Return(mockUserRole, nil)

		buf := new(bytes.Buffer)
		err := UpdateUser(mockUserRole.Deployment.ID, mockUserRole.User.Username, houston.DeploymentEditorRole, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully updated somebody@astronomer.com to a DEPLOYMENT_EDITOR")
		api.AssertExpectations(t)
	})

	t.Run("update user api error", func(t *testing.T) {
		deploymentID := "ckggzqj5f4157qtc9lescmehm"
		email := "somebody@astronomer.com"
		role := "DEPLOYMENT_FAKE_ROLE"
		expectedRequest := houston.UpdateDeploymentUserRequest{
			Email:        email,
			Role:         role,
			DeploymentID: deploymentID,
		}

		api := new(mocks.ClientInterface)
		api.On("UpdateDeploymentUser", expectedRequest).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := UpdateUser(deploymentID, email, role, api, buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), errMock.Error())
		api.AssertExpectations(t)
	})
}
