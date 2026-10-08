package deployment

import (
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const userDeploymentID = "ckgqw2k2600081qc90nbage4h"

// A Deployment user as deploymentUsers returns them: their bindings narrowed
// to this Deployment, plus their Workspace binding, whose deployment is null.
var deploymentUser = houston.DeploymentUser{
	ID:       "ckgqw2k2600081qc90nbamgno",
	FullName: "Some Person",
	Username: "somebody@astronomer.io",
	RoleBindings: []houston.RoleBinding{
		{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "ws-1"}},
		{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: userDeploymentID}},
	},
}

func (s *Suite) TestUserList() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("the users with a role on the Deployment, and that role", func() {
		expectedRequest := houston.ListDeploymentUsersRequest{Email: "somebody@astronomer.io", DeploymentID: userDeploymentID}
		workspaceOnly := houston.DeploymentUser{
			ID: "u-2", Username: "other@astronomer.io",
			RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "ws-1"}}},
		}
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", expectedRequest).Return([]houston.DeploymentUser{deploymentUser, workspaceOnly}, nil)

		got, err := UserList(userDeploymentID, "somebody@astronomer.io", "", "", api)
		s.NoError(err)
		s.Equal([]UserRole{{ID: deploymentUser.ID, FullName: "Some Person", Username: "somebody@astronomer.io", Role: houston.DeploymentAdminRole}}, got)
		api.AssertExpectations(s.T())
	})

	s.Run("none", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", houston.ListDeploymentUsersRequest{DeploymentID: userDeploymentID}).Return([]houston.DeploymentUser{}, nil)

		_, err := UserList(userDeploymentID, "", "", "", api)
		s.ErrorIs(err, ErrNoDeploymentUsers)
	})

	// An unknown Deployment is an error, "Invalid deployment".
	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentUsers", houston.ListDeploymentUsersRequest{DeploymentID: userDeploymentID}).Return(nil, errMock)

		_, err := UserList(userDeploymentID, "", "", "", api)
		s.ErrorIs(err, errMock)
	})
}

// The add, update and remove mutations return the role binding they made,
// changed or deleted, its user resolved from its foreign key.
func boundTo(role string) *houston.RoleBinding {
	return &houston.RoleBinding{
		Role:       role,
		User:       houston.RoleBindingUser{ID: deploymentUser.ID, Username: deploymentUser.Username},
		Deployment: houston.Deployment{ID: userDeploymentID},
	}
}

func (s *Suite) TestAdd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	req := houston.UpdateDeploymentUserRequest{Email: "Somebody@astronomer.io", Role: houston.DeploymentEditorRole, DeploymentID: userDeploymentID}

	s.Run("returns the user Houston bound", func() {
		api := new(mocks.ClientInterface)
		api.On("AddDeploymentUser", req).Return(boundTo(houston.DeploymentEditorRole), nil)

		got, err := Add(userDeploymentID, "Somebody@astronomer.io", houston.DeploymentEditorRole, api)
		s.NoError(err)
		s.Equal(UserRole{ID: deploymentUser.ID, Username: "somebody@astronomer.io", Role: houston.DeploymentEditorRole}, got)
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("AddDeploymentUser", req).Return(nil, errMock)

		_, err := Add(userDeploymentID, "Somebody@astronomer.io", houston.DeploymentEditorRole, api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestDeleteUser() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	req := houston.DeleteDeploymentUserRequest{DeploymentID: userDeploymentID, Email: "Somebody@astronomer.io"}

	s.Run("returns the user, as Houston names them, and the role they held", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", req).Return(boundTo(houston.DeploymentViewerRole), nil)

		got, err := RemoveUser(userDeploymentID, "Somebody@astronomer.io", api)
		s.NoError(err)
		s.Equal(UserRole{ID: deploymentUser.ID, Username: "somebody@astronomer.io", Role: houston.DeploymentViewerRole}, got)
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentUser", req).Return(nil, errMock)

		_, err := RemoveUser(userDeploymentID, "Somebody@astronomer.io", api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestUpdateUser() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	req := houston.UpdateDeploymentUserRequest{Email: "somebody@astronomer.io", Role: houston.DeploymentAdminRole, DeploymentID: userDeploymentID}

	s.Run("returns the user Houston bound", func() {
		api := new(mocks.ClientInterface)
		api.On("UpdateDeploymentUser", req).Return(boundTo(houston.DeploymentAdminRole), nil)

		got, err := UpdateUser(userDeploymentID, "somebody@astronomer.io", houston.DeploymentAdminRole, api)
		s.NoError(err)
		s.Equal(UserRole{ID: deploymentUser.ID, Username: "somebody@astronomer.io", Role: houston.DeploymentAdminRole}, got)
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("UpdateDeploymentUser", req).Return(nil, errMock)

		_, err := UpdateUser(userDeploymentID, "somebody@astronomer.io", houston.DeploymentAdminRole, api)
		s.ErrorIs(err, errMock)
	})
}
