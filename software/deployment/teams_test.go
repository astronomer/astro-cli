package deployment

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

func TestAddTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddDeploymentTeam", "deployment-id", "team-id", "role").Return(&houston.RoleBinding{Deployment: houston.Deployment{ID: "deployment-id"}, Team: houston.Team{ID: "team-id"}, Role: "role"}, nil)

		buf := new(bytes.Buffer)
		err := AddTeam("deployment-id", "team-id", "role", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "deployment-id")
		assert.Contains(t, buf.String(), "team-id")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddDeploymentTeam", "deployment-id", "team-id", "role").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := AddTeam("deployment-id", "team-id", "role", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestRemoveTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", "deployment-id", "team-id").Return(&houston.RoleBinding{Deployment: houston.Deployment{ID: "deployment-id"}, Team: houston.Team{ID: "team-id"}, Role: "role"}, nil)

		buf := new(bytes.Buffer)
		err := RemoveTeam("deployment-id", "team-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "deployment-id")
		assert.Contains(t, buf.String(), "team-id")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("RemoveDeploymentTeam", "deployment-id", "team-id").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveTeam("deployment-id", "team-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestListTeamRoles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "deployment-id"}}}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "deployment-id"}}}},
			}, nil)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("deployment-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "deployment-id")
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListDeploymentTeamsAndRoles", "deployment-id").Return([]houston.Team{}, errMock)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("deployment-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestUpdateTeamRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", "deployment-id", "team-id", "role-id").Return(&houston.RoleBinding{}, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("deployment-id", "team-id", "role-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "team-id")
		assert.Contains(t, buf.String(), "role-id")
		mock.AssertExpectations(t)
	})

	t.Run("UpdateWorkspaceTeamRole failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("UpdateDeploymentTeamRole", "deployment-id", "team-id", "role-id").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("deployment-id", "team-id", "role-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestGetDeploymentLevelRole(t *testing.T) {
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
			result:       houston.NoneTeamRole,
		},
	}

	for _, tt := range tests {
		resp := getDeploymentLevelRole(tt.roleBinding, tt.deploymentID)
		assert.Equal(t, tt.result, resp, "expected: %v, actual: %v", tt.result, resp)
	}
}
