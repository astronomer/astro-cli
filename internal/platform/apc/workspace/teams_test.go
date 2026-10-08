package workspace

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

func (s *Suite) TestAddTeam() {
	req := houston.AddWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: houston.WorkspaceEditorRole}
	s.Run("returns the Workspace", func() {
		mock := new(houston_mocks.ClientInterface)
		// workspaceAddTeam returns the whole Workspace row.
		mock.On("AddWorkspaceTeam", req).Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		w, err := AddTeam("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.NoError(err)
		s.Equal("label", w.Label)
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddWorkspaceTeam", req).Return(nil, errMock)

		_, err := AddTeam("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestRemoveTeam() {
	get := houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}
	req := houston.DeleteWorkspaceTeamRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}
	// The team query returns all of the team's bindings.
	member := &houston.Team{ID: "team-id", RoleBindings: []houston.RoleBinding{
		{Role: houston.SystemViewerRole},
		{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "workspace-id"}},
	}}

	s.Run("a member is removed, verified", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(member, nil)
		mock.On("DeleteWorkspaceTeam", req).Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		got, err := RemoveTeam("workspace-id", "team-id", mock)
		s.NoError(err)
		s.Equal(TeamRemoval{Workspace: &houston.Workspace{ID: "workspace-id", Label: "label"}, Verified: true}, got)
		mock.AssertExpectations(s.T())
	})

	// workspaceRemoveTeam deletes role bindings only, and removes nothing,
	// with no error, for a team that has none there. The lookup passes
	// Houston's shield for a team with any binding in the Workspace, a
	// Deployment's included; one with no Workspace role is refused before
	// anything is sent.
	s.Run("a team with no Workspace role is refused", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(&houston.Team{ID: "team-id", RoleBindings: []houston.RoleBinding{
			{Role: houston.DeploymentAdminRole, Workspace: houston.Workspace{ID: "workspace-id"}, Deployment: houston.Deployment{ID: "dep-1"}},
			{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "other"}},
		}}, nil)

		_, err := RemoveTeam("workspace-id", "team-id", mock)
		s.ErrorIs(err, errTeamNotInWorkspace)
		mock.AssertNotCalled(s.T(), "DeleteWorkspaceTeam", req)
	})

	// The lookup needs workspace.teams.get and the removal only
	// workspace.iam.update, and the shield's refusal reads the same for a
	// non-member: so it is sent anyway.
	s.Run("a refused lookup still sends the removal, unverified", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(nil, errors.New("Insufficient permissions."))
		mock.On("DeleteWorkspaceTeam", req).Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		got, err := RemoveTeam("workspace-id", "team-id", mock)
		s.NoError(err)
		s.False(got.Verified)
		mock.AssertExpectations(s.T())
	})

	s.Run("another lookup failure sends nothing", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(nil, errMock)

		_, err := RemoveTeam("workspace-id", "team-id", mock)
		s.ErrorIs(err, errMock)
		mock.AssertNotCalled(s.T(), "DeleteWorkspaceTeam", req)
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(member, nil)
		mock.On("DeleteWorkspaceTeam", req).Return(nil, errMock)

		_, err := RemoveTeam("workspace-id", "team-id", mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestListTeamRoles() {
	s.Run("the teams with a role on the Workspace, and that role", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "workspace-id"}}}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{
					{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "dep-1"}},
					{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "workspace-id"}},
				}},
			}, nil)

		got, err := ListTeamRoles("workspace-id", mock)
		s.NoError(err)
		s.Equal([]TeamRole{
			{ID: "test-id-1", Name: "test-name-1", Role: houston.WorkspaceViewerRole},
			{ID: "test-id-2", Name: "test-name-2", Role: houston.WorkspaceAdminRole},
		}, got)
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return([]houston.Team{}, errMock)

		_, err := ListTeamRoles("workspace-id", mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestUpdateTeamRole() {
	get := houston.GetWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id"}
	update := houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: "workspace-id", TeamID: "team-id", Role: houston.WorkspaceEditorRole}
	team := &houston.Team{ID: "team-id", Name: "Data", RoleBindings: []houston.RoleBinding{{Workspace: houston.Workspace{ID: "workspace-id"}, Role: houston.WorkspaceAdminRole}}}

	s.Run("returns the team, its role before and after", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(team, nil)
		// workspaceUpdateTeamRole answers with the new role.
		mock.On("UpdateWorkspaceTeamRole", update).Return(houston.WorkspaceEditorRole, nil)

		got, err := UpdateTeamRole("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.NoError(err)
		s.Equal(TeamRoleChange{Team: TeamRole{ID: "team-id", Name: "Data", Role: houston.WorkspaceEditorRole}, Previous: houston.WorkspaceAdminRole}, got)
	})

	// The team query with a workspaceUuid refuses a team with no binding in
	// that Workspace as "Insufficient permissions." rather than answering
	// null.
	s.Run("a team not in the Workspace", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(nil, errors.New("Insufficient permissions."))

		_, err := UpdateTeamRole("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.ErrorIs(err, errTeamNotInWorkspace)
		mock.AssertNotCalled(s.T(), "UpdateWorkspaceTeamRole", update)
	})

	s.Run("rolebinding not present", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(&houston.Team{ID: "team-id", RoleBindings: []houston.RoleBinding{}}, nil)

		_, err := UpdateTeamRole("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.ErrorIs(err, errTeamNotInWorkspace)
	})

	s.Run("UpdateWorkspaceTeamRole failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", get).Return(team, nil)
		mock.On("UpdateWorkspaceTeamRole", update).Return("", errMock)

		_, err := UpdateTeamRole("workspace-id", "team-id", houston.WorkspaceEditorRole, mock)
		s.ErrorIs(err, errMock)
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
