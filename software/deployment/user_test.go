package deployment

import (
	"bytes"

	mocks "github.com/astronomer/astro-cli/houston/mocks"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestUserList() {
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

	s.Run("single deployment user", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person     somebody     DEPLOYMENT_ADMIN`)
		api.AssertExpectations(s.T())
	})

	s.Run("multiple users", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person        somebody          DEPLOYMENT_ADMIN`)
		s.Contains(buf.String(), `ckgqw2k2600081qc90nbamgni     Another Person     anotherperson     DEPLOYMENT_EDITOR`)
		api.AssertExpectations(s.T())
	})

	// Test that UserList returns an empty list when deployment does not exist
	s.Run("empty list when deployment does not exist", func() {
		deploymentID := "ckgqw2k2600081qc90nbamgno"
		expectedRequest := houston.ListDeploymentUsersRequest{
			DeploymentID: deploymentID,
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{}, nil)

		buf := new(bytes.Buffer)
		err := UserList(deploymentID, "", "", "", api, buf)
		s.NoError(err)
		s.Contains(buf.String(), houstonInvalidDeploymentUsersMsg)
		api.AssertExpectations(s.T())
	})

	s.Run("api error", func() {
		deploymentID := "ckgqw2k2600081qc90nbamgno"
		expectedRequest := houston.ListDeploymentUsersRequest{
			DeploymentID: deploymentID,
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{}, errMock)

		buf := new(bytes.Buffer)
		err := UserList(deploymentID, "", "", "", api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestAdd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("add user success", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), "Successfully added somebody@astronomer.io as a DEPLOYMENT_ADMIN")
		api.AssertExpectations(s.T())
	})
	s.Run("add user api error", func() {
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
		s.Error(err)
		s.Contains(err.Error(), errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDeleteUser() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("delete user success", func() {
		mockUserRole := &houston.RoleBinding{
			User: houston.RoleBindingUser{Username: "somebody@astronomer.com"},
			Role: houston.DeploymentAdminRole,
			Deployment: houston.Deployment{
				ID:          "deploymentid",
				ReleaseName: "prehistoric-gravity-9229",
			},
		}

		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", houston.DeleteDeploymentUserRequest{DeploymentID: mockUserRole.Deployment.ID, Email: mockUserRole.User.Username}).Return(mockUserRole, nil)

		buf := new(bytes.Buffer)
		err := RemoveUser(mockUserRole.Deployment.ID, mockUserRole.User.Username, api, buf)
		s.NoError(err)
		s.Contains(buf.String(), "Successfully removed the DEPLOYMENT_ADMIN role for somebody@astronomer.com from deployment deploymentid")
		api.AssertExpectations(s.T())
	})
	s.Run("delete user api error", func() {
		deploymentID := "deploymentid"
		email := "somebody@astronomer.com"

		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", houston.DeleteDeploymentUserRequest{DeploymentID: deploymentID, Email: email}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveUser(deploymentID, email, api, buf)
		s.Error(err)
		s.Contains(err.Error(), errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateUser() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("update user success", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), "Successfully updated somebody@astronomer.com to a DEPLOYMENT_EDITOR")
		api.AssertExpectations(s.T())
	})

	s.Run("update user api error", func() {
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
		s.Error(err)
		s.Contains(err.Error(), errMock.Error())
		api.AssertExpectations(s.T())
	})
}
