package teams

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/software/utils"
	"github.com/stretchr/testify/suite"
)

var errMockHouston = errors.New("mock houston error")

type Suite struct {
	suite.Suite
}

func TestTeams(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestGet() {
	s.Run("success", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), "test-id")
		s.Contains(buf.String(), "test-name")
		s.Contains(buf.String(), "user-id")
		s.Contains(buf.String(), "username")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("getTeam error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Get("test-id", false, false, true, mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("getTeamUsers error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", Name: "test-name"}, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{}, errMockHouston).Once()

		err := Get("test-id", false, false, true, mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestList() {
	s.Run("success", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil)

		err := List(mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id")
		s.Contains(buf.String(), "test-name")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("listTeams error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston)

		err := List(mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestPaginatedList() {
	s.Run("success", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		promptPaginatedOption = func(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) utils.PaginationOptions {
			return utils.PaginationOptions{Quit: true}
		}

		err := PaginatedList(mockClient, buf, ListTeamLimit, 0, "")
		s.NoError(err)
		s.Contains(buf.String(), "test-id")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("with one recursion", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list team error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston).Once()

		err := PaginatedList(mockClient, buf, ListTeamLimit, 0, "")
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdate() {
	s.Run("success", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id")
		s.Contains(buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("success to set None", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.NoneRole, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id")
		s.Contains(buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid role", func() {
		buf := new(bytes.Buffer)

		err := Update("test-id", "invalid-role-string", nil, buf)
		s.Contains(err.Error(), "invalid role: invalid-role-string")
	})

	s.Run("CreateTeamSystemRoleBinding error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("GetTeam error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Update("test-id", houston.NoneRole, mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("No role set already", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil).Once()

		err := Update("test-id", houston.NoneRole, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "Role for the team test-id already set to None, nothing to update")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("DeleteTeamSystemRoleBinding error", func() {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		err := Update("test-id", houston.NoneRole, mockClient, buf)
		s.ErrorIs(err, errMockHouston)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsValidSystemRole() {
	tests := []struct {
		role   string
		result bool
	}{
		{role: houston.SystemAdminRole, result: true},
		{role: houston.SystemEditorRole, result: true},
		{role: houston.SystemViewerRole, result: true},
		{role: houston.NoneRole, result: true},
		{role: "invalid-role", result: false},
		{role: "", result: false},
	}

	for _, tt := range tests {
		resp := isValidSystemLevelRole(tt.role)
		s.Equal(tt.result, resp, "expected: %v, actual: %v, for: %s", tt.result, resp, tt.role)
	}
}
