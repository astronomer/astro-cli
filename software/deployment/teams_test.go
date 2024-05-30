package deployment

import (
	"bytes"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
)

func (s *Suite) TestAddTeam() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddDeploymentTeam", houston.AddDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: "role"}).Return(&houston.RoleBinding{Deployment: houston.Deployment{ID: "deployment-id"}, Team: houston.Team{ID: "team-id"}, Role: "role"}, nil)

		buf := new(bytes.Buffer)
		err := AddTeam("deployment-id", "team-id", "role", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "deployment-id")
		s.Contains(buf.String(), "team-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddDeploymentTeam", houston.AddDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: "role"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := AddTeam("deployment-id", "team-id", "role", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestRemoveTeam() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", houston.RemoveDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id"}).Return(&houston.RoleBinding{Deployment: houston.Deployment{ID: "deployment-id"}, Team: houston.Team{ID: "team-id"}, Role: "role"}, nil)

		buf := new(bytes.Buffer)
		err := RemoveTeam("deployment-id", "team-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "deployment-id")
		s.Contains(buf.String(), "team-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", houston.RemoveDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveTeam("deployment-id", "team-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListTeamRoles() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "deployment-id"}}}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "deployment-id"}}}},
			}, nil)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("deployment-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "deployment-id")
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")
		mock.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return([]houston.Team{}, errMock)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("deployment-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateTeamRole() {
	s.Run("success", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", houston.UpdateDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: "role-id"}).Return(&houston.RoleBinding{}, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("deployment-id", "team-id", "role-id", mock, buf)

		s.NoError(err)
		s.Contains(buf.String(), "team-id")
		s.Contains(buf.String(), "role-id")
		mock.AssertExpectations(s.T())
	})

	s.Run("UpdateWorkspaceTeamRole failure", func() {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", houston.UpdateDeploymentTeamRequest{DeploymentID: "deployment-id", TeamID: "team-id", Role: "role-id"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("deployment-id", "team-id", "role-id", mock, buf)

		s.ErrorIs(err, errMock)
		mock.AssertExpectations(s.T())
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
