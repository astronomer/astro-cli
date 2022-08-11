package workspace

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
		mock.On("AddWorkspaceTeam", "workspace-id", "team-id", "role").Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		buf := new(bytes.Buffer)
		err := AddTeam("workspace-id", "team-id", "role", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "workspace-id")
		assert.Contains(t, buf.String(), "team-id")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("AddWorkspaceTeam", "workspace-id", "team-id", "role").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := AddTeam("workspace-id", "team-id", "role", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestRemoveTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("DeleteWorkspaceTeam", "workspace-id", "team-id").Return(&houston.Workspace{ID: "workspace-id", Label: "label"}, nil)

		buf := new(bytes.Buffer)
		err := RemoveTeam("workspace-id", "team-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "workspace-id")
		assert.Contains(t, buf.String(), "team-id")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("DeleteWorkspaceTeam", "workspace-id", "team-id").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := RemoveTeam("workspace-id", "team-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestListTeamRoles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return(
			[]houston.Team{
				{ID: "test-id-1", Name: "test-name-1", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: "workspace-id"}}}},
				{ID: "test-id-2", Name: "test-name-2", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "workspace-id"}}}},
			}, nil)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("workspace-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "workspace-id")
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")
		mock.AssertExpectations(t)
	})

	t.Run("houston failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("ListWorkspaceTeamsAndRoles", "workspace-id").Return([]houston.Team{}, errMock)

		buf := new(bytes.Buffer)
		err := ListTeamRoles("workspace-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestUpdateTeamRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", "workspace-id", "team-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Workspace: houston.Workspace{ID: "workspace-id"}, Role: houston.WorkspaceAdminRole}}}, nil)
		mock.On("UpdateWorkspaceTeamRole", "workspace-id", "team-id", "role-id").Return("role-id", nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "team-id")
		assert.Contains(t, buf.String(), "role-id")
		mock.AssertExpectations(t)
	})

	t.Run("teams not in workspace", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", "workspace-id", "team-id").Return(nil, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		assert.ErrorIs(t, err, errTeamNotInWorkspace)
		mock.AssertExpectations(t)
	})

	t.Run("GetWorkspaceTeamRole failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", "workspace-id", "team-id").Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		assert.ErrorIs(t, err, errTeamNotInWorkspace)
		mock.AssertExpectations(t)
	})

	t.Run("rolebinding not present", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", "workspace-id", "team-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		assert.ErrorIs(t, err, errTeamNotInWorkspace)
		mock.AssertExpectations(t)
	})

	t.Run("UpdateWorkspaceTeamRole failure", func(t *testing.T) {
		mock := new(houston_mocks.ClientInterface)
		mock.On("GetWorkspaceTeamRole", "workspace-id", "team-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Workspace: houston.Workspace{ID: "workspace-id"}, Role: houston.WorkspaceAdminRole}}}, nil)
		mock.On("UpdateWorkspaceTeamRole", "workspace-id", "team-id", "role-id").Return("", errMock)

		buf := new(bytes.Buffer)
		err := UpdateTeamRole("workspace-id", "team-id", "role-id", mock, buf)

		assert.ErrorIs(t, err, errMock)
		mock.AssertExpectations(t)
	})
}

func TestGetWorkspaceLevelRole(t *testing.T) {
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
			result:      houston.NoneTeamRole,
		},
	}

	for _, tt := range tests {
		resp := getWorkspaceLevelRole(tt.roleBinding, tt.workspaceID)
		assert.Equal(t, tt.result, resp, "expected: %v, actual: %v", tt.result, resp)
	}
}
