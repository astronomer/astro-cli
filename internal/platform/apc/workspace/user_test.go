package workspace

import (
	"errors"
	"os"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/internal/platform/apc/utils"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var errMock = errors.New("api error")

// mockRoles is a function, not a shared value: a test that changes what it
// returns must not change it for the next run of the suite (-count=N).
//
// workspaceUser returns the first active user with the email, whatever the
// Workspace, and narrows their bindings to the Workspace asked about
//: a user with no role there comes
// back with no bindings, not as an error.
func mockRoles() houston.WorkspaceUserRoleBindings {
	return houston.WorkspaceUserRoleBindings{
		ID:       "u-1",
		Username: "test@test.com",
		RoleBindings: []houston.RoleBinding{
			{
				Role:      houston.WorkspaceViewerRole,
				Workspace: houston.Workspace{ID: "ckoixo6o501496qemiwsja1tl"},
			},
		},
	}
}

func (s *Suite) TestAdd() {
	testUtil.InitTestConfig("software")
	id := "ck1qg6whg001r08691y117hub"
	req := houston.AddWorkspaceUserRequest{WorkspaceID: id, Email: "Test@test.com", Role: houston.WorkspaceEditorRole}

	s.Run("names the user Houston added", func() {
		// workspaceAddUser returns the Workspace as {id, label} and its
		// members, the new one among them under its username, the email in
		// lower case.
		ws := &houston.Workspace{ID: id, Label: "airflow", Users: []houston.User{
			{ID: "u-0", Username: "someone@test.com"},
			{ID: "u-1", Username: "test@test.com"},
		}}
		api := new(mocks.ClientInterface)
		api.On("AddWorkspaceUser", req).Return(ws, nil)

		w, added, err := Add(id, "Test@test.com", houston.WorkspaceEditorRole, api)
		s.NoError(err)
		s.Equal(ws, w)
		s.Equal(UserRole{ID: "u-1", Username: "test@test.com", Role: houston.WorkspaceEditorRole}, added)
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("AddWorkspaceUser", req).Return(nil, errMock)

		_, _, err := Add(id, "Test@test.com", houston.WorkspaceEditorRole, api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestRemove() {
	testUtil.InitTestConfig("software")
	id := "ck1qg6whg001r08691y117hub"
	req := houston.DeleteWorkspaceUserRequest{WorkspaceID: id, UserID: "u-1"}

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceUser", req).Return(&houston.Workspace{ID: id, Label: "airflow"}, nil)

		w, err := Remove(id, "u-1", api)
		s.NoError(err)
		s.Equal("airflow", w.Label)
	})

	// A user who is not a member is an error
	//.
	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceUser", req).Return(nil, errMock)

		_, err := Remove(id, "u-1", api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestListRoles() {
	wsID := "ck1qg6whg001r08691y117hub"
	// workspaceUsers narrows each user's bindings to the Workspace
	//.
	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:           "ckbv7zpkh00og0760ki4mhl6r",
			Username:     "test@test.com",
			FullName:     "test",
			Emails:       []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: wsID}}},
		},
	}

	s.Run("the users and their role", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceUserAndRoles", wsID).Return(mockResponse, nil)

		got, err := ListRoles(wsID, api)
		s.NoError(err)
		s.Equal([]UserRole{{ID: "ckbv7zpkh00og0760ki4mhl6r", Username: "test@test.com", FullName: "test", Role: houston.WorkspaceAdminRole}}, got)
	})

	s.Run("none is empty, not nil", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceUserAndRoles", wsID).Return([]houston.WorkspaceUserRoleBindings{}, nil)

		got, err := ListRoles(wsID, api)
		s.NoError(err)
		s.NotNil(got)
		s.Empty(got)
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceUserAndRoles", wsID).Return(nil, errMock)

		_, err := ListRoles(wsID, api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestPaginatedListRoles() {
	wsID := "ck1qg6whg001r08691y117hub"
	user := houston.WorkspaceUserRoleBindings{
		ID:           "ckbv7zpkh00og0760ki4mhl6r",
		Username:     "test@test.com",
		RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: wsID}}},
	}
	page := []UserRole{{ID: user.ID, Username: user.Username, Role: houston.WorkspaceAdminRole}}

	s.Run("a page shorter than the page size is the only one, so nothing is asked", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, Take: 100}).Return([]houston.WorkspaceUserRoleBindings{user}, nil)
		asked := false
		promptPaginatedOption = func(string, string, int, int, int, bool) (utils.PaginationOptions, error) {
			asked = true
			return utils.PaginationOptions{Quit: true}, nil
		}
		defer func() { promptPaginatedOption = utils.PromptPaginatedOption }()

		var pages [][]UserRole
		err := PaginatedListRoles(wsID, "", 100, 0, api, func(u []UserRole) error { pages = append(pages, u); return nil })
		s.NoError(err)
		s.Equal([][]UserRole{page}, pages)
		s.False(asked)
	})

	s.Run("a full page asks, and the answer picks the next", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, Take: 1}).Return([]houston.WorkspaceUserRoleBindings{user}, nil).Once()
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: user.ID, Take: 1}).Return([]houston.WorkspaceUserRoleBindings{}, nil).Once()
		answers := []utils.PaginationOptions{{CursorID: user.ID, PageSize: 1, PageNumber: 1}, {Quit: true}}
		promptPaginatedOption = func(string, string, int, int, int, bool) (utils.PaginationOptions, error) {
			a := answers[0]
			answers = answers[1:]
			return a, nil
		}
		defer func() { promptPaginatedOption = utils.PromptPaginatedOption }()

		var pages [][]UserRole
		err := PaginatedListRoles(wsID, "", 1, 0, api, func(u []UserRole) error { pages = append(pages, u); return nil })
		s.NoError(err)
		s.Equal([][]UserRole{page, {}}, pages)
		api.AssertExpectations(s.T())
	})

	s.Run("houston failure", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, Take: 100}).Return(nil, errMock)

		err := PaginatedListRoles(wsID, "", 100, 0, api, func([]UserRole) error { return nil })
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestShowListRolesPaginatedOption() {
	wsID := "ck1qg6whg001r08691y117hub"
	paginationPageSize := 100

	s.Run("total record less then page size", func() {
		// mock os.Stdin for when prompted by PromptPaginatedOption
		input := []byte("q")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		value, err := promptPaginatedOption(wsID, wsID, paginationPageSize, 10, 0, false)
		s.NoError(err)
		s.Equal(value.Quit, true)
	})
}

func (s *Suite) TestUpdateRole() {
	testUtil.InitTestConfig("software")
	id := "ckoixo6o501496qemiwsja1tl"
	email := "test@test.com"
	update := houston.UpdateWorkspaceUserRoleRequest{WorkspaceID: id, Email: email, Role: houston.WorkspaceAdminRole}

	s.Run("returns the role before and after", func() {
		api := new(mocks.ClientInterface)
		api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles(), nil)
		// workspaceUpsertUserRole answers with the role it set
		//.
		api.On("UpdateWorkspaceUserRole", update).Return(houston.WorkspaceAdminRole, nil)

		got, err := UpdateRole(id, email, houston.WorkspaceAdminRole, api)
		s.NoError(err)
		s.Equal(UserRoleChange{User: UserRole{ID: "u-1", Username: email, Role: houston.WorkspaceAdminRole}, Previous: houston.WorkspaceViewerRole}, got)
		api.AssertExpectations(s.T())
	})

	// It answers null when the user holds more than one binding, having set
	// the role all the same (workspace-upsert-user-role/index.js).
	s.Run("a null answer is the role asked for", func() {
		api := new(mocks.ClientInterface)
		api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles(), nil)
		api.On("UpdateWorkspaceUserRole", update).Return("", nil)

		got, err := UpdateRole(id, email, houston.WorkspaceAdminRole, api)
		s.NoError(err)
		s.Equal(houston.WorkspaceAdminRole, got.User.Role)
	})

	s.Run("a user with no role on the Workspace is refused before any change", func() {
		api := new(mocks.ClientInterface)
		api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(houston.WorkspaceUserRoleBindings{ID: "u-1", Username: email}, nil)

		_, err := UpdateRole(id, email, houston.WorkspaceAdminRole, api)
		s.ErrorIs(err, errUserNotInWorkspace)
		api.AssertNotCalled(s.T(), "UpdateWorkspaceUserRole", update)
	})

	s.Run("a lookup failure", func() {
		api := new(mocks.ClientInterface)
		api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(houston.WorkspaceUserRoleBindings{}, errMock)

		_, err := UpdateRole(id, email, houston.WorkspaceAdminRole, api)
		s.ErrorIs(err, errMock)
	})

	s.Run("an update failure", func() {
		api := new(mocks.ClientInterface)
		api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles(), nil)
		api.On("UpdateWorkspaceUserRole", update).Return("", errMock)

		_, err := UpdateRole(id, email, houston.WorkspaceAdminRole, api)
		s.ErrorIs(err, errMock)
	})
}
