package workspace

import (
	"bytes"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
)

func (s *Suite) TestAddTeam() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddWorkspaceTeam", houston.AddWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: "role"}).Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		buf := new(bytes.Buffer)
		err := AddTeam("workspace-id", "team-id", "role", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "workspace-id")
		s.Contains(buf.String(), "team-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddWorkspaceTeam", houston.AddWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: "role"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := AddTeam("workspace-id", "team-id", "role", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestRemoveTeam() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("DeleteWorkspaceTeam", houston.DeleteWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		buf := new(bytes.Buffer)
		err := RemoveTeam("workspace-id", "team-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "workspace-id")
		s.Contains(buf.String(), "team-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("DeleteWorkspaceTeam", houston.DeleteWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveTeam("workspace-id", "team-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListTeamRoles() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "workspace-id"}}}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "workspace-id"}}}},
			}, nil)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("workspace-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "workspace-id")
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return([]houston.Team{}, errMock)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("workspace-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateTeamRole() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Workspace: houston.Workspace{ID: "workspace-id"}, Role: houston.WorkspaceAdminRole}}}, nil)
		mock.On("UpdateWorkspaceTeamRole", houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: "role-id"}).Return("role-id", nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "team-id")
		s.Contains(buf.String(), "role-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("teams not in workspace", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(nil, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		s.ErrorIs(err, errTeamNotInWorkspace)
		mock.AssertExpectations(s.T())
	})

	s.Run("GetWorkspaceTeamRole failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		s.ErrorIs(err, errTeamNotInWorkspace)
		mock.AssertExpectations(s.T())
	})

	s.Run("rolebinding not present", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		s.ErrorIs(err, errTeamNotInWorkspace)
		mock.AssertExpectations(s.T())
	})

	s.Run("UpdateWorkspaceTeamRole failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}).Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Workspace: houston.Workspace{ID: "workspace-id"}, Role: houston.WorkspaceAdminRole}}}, nil)
		mock.On("UpdateWorkspaceTeamRole", houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: "role-id"}).Return("", errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetWorkspaceLevelRole() {
	tests := []struct {
		roleBinding []houston.RoleBinding
		workspaceID string
		result      string
	}{
		{
			roleBinding: []houston.RoleBinding{
				{Role: houston.SystemAdminRole},
				{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "test-id-1"}},
				{Role: houston.WorkspaceEditorRole, Workspace: houston.Workspace{ID: "test-id-2"}},
			},
			workspaceID: "test-id-1",
			result:      houston.WorkspaceAdminRole,
		},
		{
			roleBinding: []houston.RoleBinding{
				{Role: houston.SystemAdminRole},
				{Role: houston.WorkspaceEditorRole, Workspace: houston.Workspace{ID: "test-id-2"}},
			},
			workspaceID: "test-id-1",
			result:      houston.NoneRole,
		},
	}

	for _, tt := range tests {
		resp := getWorkspaceLevelRole(tt.roleBinding, tt.workspaceID)
		s.Equal(tt.result, resp, "expected: %v, actual: %v", tt.result, resp)
	}
}
