package teams

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/software/utils"
	"github.com/stretchr/testify/assert"
)

var errMockHouston = errors.New("mock houston error")

func TestGet(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockTeamResp := &houston.Team{
			ID:   "test-id",
			Name: "test-name",
			RoleBindings: []houston.RoleBinding{
				{
					Role:      houston.WorkspaceAdminRole,
					Workspace: houston.Workspace{ID: "test-ws-id"},
				},
				{
					Role:       houston.DeploymentAdminRole,
					Deployment: houston.Deployment{ID: "test-deployment-id"},
				},
			},
		}
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(mockTeamResp, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{{ID: "user-id", Username: "username"}}, nil).Once()

		err := Get("test-id", true, true, false, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), "test-name")
		assert.Contains(t, buf.String(), "user-id")
		assert.Contains(t, buf.String(), "username")
		mockClient.AssertExpectations(t)
	})

	t.Run("getTeam error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Get("test-id", false, false, true, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("getTeamUsers error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", Name: "test-name"}, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{}, errMockHouston).Once()

		err := Get("test-id", false, false, true, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil)

		err := List(mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), "test-name")
		mockClient.AssertExpectations(t)
	})

	t.Run("listTeams error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston)

		err := List(mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestPaginatedList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		promptPaginatedOption = func(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) utils.PaginationOptions {
			return utils.PaginationOptions{Quit: true}
		}

		err := PaginatedList(mockClient, buf, ListTeamLimit, 0, "")
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		mockClient.AssertExpectations(t)
	})

	t.Run("with one recursion", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: 1}).Return(houston.ListTeamsResp{Count: 2, Teams: []houston.Team{{ID: "test-id-1", Name: "test-name-1"}}}, nil).Once()
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "test-id-1", Take: 1}).Return(houston.ListTeamsResp{Count: 2, Teams: []houston.Team{{ID: "test-id-2", Name: "test-name-2"}}}, nil).Once()

		try := 0
		promptPaginatedOption = func(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) utils.PaginationOptions {
			if try == 0 {
				try++
				return utils.PaginationOptions{Quit: false, PageSize: 1, PageNumber: 1, CursorID: "test-id-1"}
			}
			return utils.PaginationOptions{Quit: true}
		}

		err := PaginatedList(mockClient, buf, 1, 0, "")
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")
		mockClient.AssertExpectations(t)
	})

	t.Run("list team error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston).Once()

		err := PaginatedList(mockClient, buf, ListTeamLimit, 0, "")
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(t)
	})

	t.Run("success to set None", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid role", func(t *testing.T) {
		buf := new(bytes.Buffer)

		err := Update("test-id", "invalid-role-string", nil, buf)
		assert.Contains(t, err.Error(), "invalid role: invalid-role-string")
	})

	t.Run("CreateTeamSystemRoleBinding error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("GetTeam error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("No role set already", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Role for the team test-id already set to None, nothing to update")
		mockClient.AssertExpectations(t)
	})

	t.Run("DeleteTeamSystemRoleBinding error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestIsValidSystemRole(t *testing.T) {
	tests := []struct {
		role   string
		result bool
	}{
		{role: houston.SystemAdminRole, result: true},
		{role: houston.SystemEditorRole, result: true},
		{role: houston.SystemViewerRole, result: true},
		{role: houston.NoneTeamRole, result: true},
		{role: "invalid-role", result: false},
		{role: "", result: false},
	}

	for _, tt := range tests {
		resp := isValidSystemLevelRole(tt.role)
		assert.Equal(t, tt.result, resp, "expected: %v, actual: %v, for: %s", tt.result, resp, tt.role)
	}
}
