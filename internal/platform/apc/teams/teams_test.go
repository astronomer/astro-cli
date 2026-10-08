package teams

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/internal/platform/apc/utils"
)

var errMockHouston = errors.New("mock houston error")

type Suite struct {
	suite.Suite
}

func TestTeams(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TearDownSubTest() {
	promptPaginatedOption = utils.PromptPaginatedOption
}

func (s *Suite) TestGet() {
	// team returns all of the team's bindings, on every Workspace and
	// Deployment and on the platform.
	mockTeamResp := &houston.Team{
		ID:   "test-id",
		Name: "test-name",
		RoleBindings: []houston.RoleBinding{
			{Role: houston.SystemViewerRole},
			{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: "test-ws-id"}},
			{Role: houston.DeploymentAdminRole, Deployment: houston.Deployment{ID: "test-deployment-id"}},
		},
	}

	s.Run("the team with its users", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(mockTeamResp, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{{ID: "user-id", Username: "username"}}, nil).Once()

		got, err := Get("test-id", true, mockClient)
		s.NoError(err)
		s.Equal(Detail{Team: mockTeamResp, Users: []houston.User{{ID: "user-id", Username: "username"}}}, got)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("without its users, nothing more is asked", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(mockTeamResp, nil).Once()

		got, err := Get("test-id", false, mockClient)
		s.NoError(err)
		s.Nil(got.Users)
		mockClient.AssertNotCalled(s.T(), "GetTeamUsers", "test-id")
	})

	// teamUsers answers [] for a team with no users, and for an unknown one.
	s.Run("no users is empty, not nil", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(mockTeamResp, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return(nil, nil).Once()

		got, err := Get("test-id", true, mockClient)
		s.NoError(err)
		s.NotNil(got.Users)
	})

	// An unknown team is an error, "The requested resource was not found",
	// never a null answer.
	s.Run("getTeam error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		_, err := Get("test-id", true, mockClient)
		s.ErrorIs(err, errMockHouston)
	})

	s.Run("a null answer is refused rather than read", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, nil).Once()

		_, err := Get("test-id", false, mockClient)
		s.ErrorIs(err, errTeamNotFound)
	})

	s.Run("getTeamUsers error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", Name: "test-name"}, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{}, errMockHouston).Once()

		_, err := Get("test-id", true, mockClient)
		s.ErrorIs(err, errMockHouston)
	})

	s.Run("no ID", func() {
		_, err := Get("", false, nil)
		s.ErrorIs(err, errMissingTeamID)
	})
}

func (s *Suite) TestList() {
	// paginatedTeams skips the cursor row and counts every team on the
	// platform, not the page.
	s.Run("reads every page", func() {
		mockClient := new(houston_mocks.ClientInterface)
		first := make([]houston.Team, ListTeamLimit)
		for i := range first {
			first[i] = houston.Team{ID: "t-" + string(rune('a'+i))}
		}
		last := first[len(first)-1].ID
		mockClient.On("ListTeams", houston.ListTeamsRequest{Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: ListTeamLimit + 1, Teams: first}, nil).Once()
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: last, Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: ListTeamLimit + 1, Teams: []houston.Team{{ID: "z"}}}, nil).Once()

		got, err := List(mockClient)
		s.NoError(err)
		s.Len(got, ListTeamLimit+1)
		s.Equal("z", got[ListTeamLimit].ID)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("an empty page ends the list, whatever the count says", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 5, Teams: []houston.Team{{ID: "a"}}}, nil).Once()
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "a", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 5}, nil).Once()

		got, err := List(mockClient)
		s.NoError(err)
		s.Equal([]houston.Team{{ID: "a"}}, got)
	})

	s.Run("none is empty, not nil", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, nil).Once()

		got, err := List(mockClient)
		s.NoError(err)
		s.NotNil(got)
		s.Empty(got)
	})

	s.Run("listTeams error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston)

		_, err := List(mockClient)
		s.ErrorIs(err, errMockHouston)
	})
}

func (s *Suite) TestPaginatedList() {
	collect := func(pages *[][]houston.Team) func([]houston.Team) error {
		return func(ts []houston.Team) error {
			*pages = append(*pages, ts)
			return nil
		}
	}

	s.Run("success", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		promptPaginatedOption = func(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) (utils.PaginationOptions, error) {
			return utils.PaginationOptions{Quit: true}, nil
		}

		var pages [][]houston.Team
		err := PaginatedList(mockClient, ListTeamLimit, 0, "", collect(&pages))
		s.NoError(err)
		s.Equal([][]houston.Team{{{ID: "test-id", Name: "test-name"}}}, pages)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("with one recursion", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: 1}).Return(houston.ListTeamsResp{Count: 2, Teams: []houston.Team{{ID: "test-id-1", Name: "test-name-1"}}}, nil).Once()
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "test-id-1", Take: 1}).Return(houston.ListTeamsResp{Count: 2, Teams: []houston.Team{{ID: "test-id-2", Name: "test-name-2"}}}, nil).Once()

		try := 0
		promptPaginatedOption = func(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) (utils.PaginationOptions, error) {
			if try == 0 {
				try++
				return utils.PaginationOptions{Quit: false, PageSize: 1, PageNumber: 1, CursorID: "test-id-1"}, nil
			}
			return utils.PaginationOptions{Quit: true}, nil
		}

		var pages [][]houston.Team
		err := PaginatedList(mockClient, 1, 0, "", collect(&pages))
		s.NoError(err)
		s.Len(pages, 2)
		s.Equal("test-id-2", pages[1][0].ID)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list team error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: ListTeamLimit}).Return(houston.ListTeamsResp{}, errMockHouston).Once()

		err := PaginatedList(mockClient, ListTeamLimit, 0, "", func([]houston.Team) error { return nil })
		s.ErrorIs(err, errMockHouston)
	})
}

func (s *Suite) TestUpdate() {
	s.Run("success", func() {
		mockClient := new(houston_mocks.ClientInterface)
		// createTeamSystemRoleBinding answers with the binding, created or
		// changed.
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		got, err := Update("test-id", houston.SystemAdminRole, mockClient)
		s.NoError(err)
		s.Equal(RoleChange{TeamID: "test-id", Role: houston.SystemAdminRole, Changed: true}, got)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("success to set None", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil).Once()

		got, err := Update("test-id", houston.NoneRole, mockClient)
		s.NoError(err)
		s.Equal(RoleChange{TeamID: "test-id", Previous: houston.SystemAdminRole, Role: houston.NoneRole, Changed: true}, got)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid role", func() {
		_, err := Update("test-id", "invalid-role-string", nil)
		s.ErrorContains(err, "invalid role: invalid-role-string")
	})

	s.Run("CreateTeamSystemRoleBinding error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		_, err := Update("test-id", houston.SystemAdminRole, mockClient)
		s.ErrorIs(err, errMockHouston)
	})

	s.Run("GetTeam error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		_, err := Update("test-id", houston.NoneRole, mockClient)
		s.ErrorIs(err, errMockHouston)
	})

	s.Run("No role set already", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil).Once()

		got, err := Update("test-id", houston.NoneRole, mockClient)
		s.NoError(err)
		s.Equal(RoleChange{TeamID: "test-id", Previous: houston.NoneRole, Role: houston.NoneRole}, got)
		mockClient.AssertNotCalled(s.T(), "DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.NoneRole})
	})

	s.Run("DeleteTeamSystemRoleBinding error", func() {
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "test-id", Role: houston.SystemAdminRole}).Return("", errMockHouston).Once()

		_, err := Update("test-id", houston.NoneRole, mockClient)
		s.ErrorIs(err, errMockHouston)
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
