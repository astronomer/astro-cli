package deployment

import (
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

func (s *Suite) TestAddTeam() {
	req := houston.AddDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: houston.DeploymentEditorRole}
	s.Run("returns the role Houston bound", func() {
		mock := new(houston_mocks.ClientInterface)
		// deploymentAddTeamRole returns the binding it created
		//.
		mock.On("AddDeploymentTeam", req).Return(&houston.RoleBinding{Role: houston.DeploymentEditorRole}, nil)

		role, err := AddTeam("deployment-id", "team-id", houston.DeploymentEditorRole, mock)
		s.NoError(err)
		s.Equal(houston.DeploymentEditorRole, role)
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddDeploymentTeam", req).Return(nil, errMock)

		_, err := AddTeam("deployment-id", "team-id", houston.DeploymentEditorRole, mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestRemoveTeam() {
	req := houston.RemoveDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id"}
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", req).Return(&houston.RoleBinding{Role: houston.DeploymentViewerRole}, nil)

		s.NoError(RemoveTeam("deployment-id", "team-id", mock))
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", req).Return(nil, errMock)

		s.ErrorIs(RemoveTeam("deployment-id", "team-id", mock), errMock)
	})
}

func (s *Suite) TestListTeamRoles() {
	s.Run("the teams with a role on the Deployment, and that role", func() {
		mock := new(houston_mocks.ClientInterface)
		// Each team carries all of its bindings, on every Workspace and
		// Deployment, so the
		// list has to pick the one on this Deployment.
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{
					{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "ws-1"}},
					{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "deployment-id"}},
				}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "deployment-id"}}}},
				{ID: "test-id-3", Name: "elsewhere", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "other"}}}},
			}, nil)

		got, err := ListTeamRoles("deployment-id", mock)
		s.NoError(err)
		s.Equal([]TeamRole{
			{ID: "test-id-1", Name: "test-name-1", Role: houston.DeploymentViewerRole},
			{ID: "test-id-2", Name: "test-name-2", Role: houston.DeploymentAdminRole},
		}, got)
	})

	// deploymentTeams is [] for a Deployment with no team and for one that
	// does not exist alike: the resolver checks nothing
	//.
	s.Run("none", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return([]houston.Team{}, nil)

		_, err := ListTeamRoles("deployment-id", mock)
		s.ErrorIs(err, ErrNoDeploymentTeams)
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return(nil, errMock)

		_, err := ListTeamRoles("deployment-id", mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestUpdateTeamRole() {
	req := houston.UpdateDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: houston.DeploymentAdminRole}
	s.Run("returns the role Houston bound", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", req).Return(&houston.RoleBinding{Role: houston.DeploymentAdminRole}, nil)

		role, err := UpdateTeamRole("deployment-id", "team-id", houston.DeploymentAdminRole, mock)
		s.NoError(err)
		s.Equal(houston.DeploymentAdminRole, role)
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", req).Return(nil, errMock)

		_, err := UpdateTeamRole("deployment-id", "team-id", houston.DeploymentAdminRole, mock)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestGetDeploymentLevelRole() {
	tests := []struct {
		roleBinding  []houston.RoleBinding
		deploymentID string
		result       string
	}{
		{
			roleBinding: []houston.RoleBinding{
				{Role: houston.SystemAdminRole},
				{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "test-id-1"}},
				{Role: houston.DeploymentEditorRole, Deployment: houston.Deployment{ID: "test-id-2"}},
			},
			deploymentID: "test-id-1",
			result:       houston.DeploymentAdminRole,
		},
		{
			roleBinding: []houston.RoleBinding{
				{Role: houston.SystemAdminRole},
				{Role: houston.DeploymentEditorRole, Deployment: houston.Deployment{ID: "test-id-2"}},
			},
			deploymentID: "test-id-1",
			result:       houston.NoneRole,
		},
	}

	for _, tt := range tests {
		resp := getDeploymentLevelRole(tt.roleBinding, tt.deploymentID)
		s.Equal(tt.result, resp, "expected: %v, actual: %v", tt.result, resp)
	}
}
