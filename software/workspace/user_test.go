package workspace

import (
	"bytes"
	"errors"
	"os"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
)

var (
	mockWsUserResponse = &houston.Workspace{
		ID: "ckc0eir8e01gj07608ajmvia1",
	}
	mockRoles = houston.WorkspaceUserRoleBindings{
		RoleBindings: []houston.RoleBinding{
			{
				Role:      houston.WorkspaceViewerRole,
				Workspace: houston.Workspace{ID: "ckoixo6o501496qemiwsja1tl"},
			},
			{
				Role:      "DEPLOYMENT_VIEWER",
				Workspace: houston.Workspace{ID: "ckg6sfddu30911pc0n1o0e97e"},
			},
		},
	}
	errMock = errors.New("api error")
)

func (s *Suite) TestAdd() {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", houston.AddWorkspaceUserRequest{WorkspaceID: id, Email: email, Role: role}).Return(mockWsUserResponse, nil)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	s.NoError(err)
	expected := ` NAME     WORKSPACE ID                  EMAIL             ROLE          
          ckc0eir8e01gj07608ajmvia1     test@test.com     test-role     
Successfully added test@test.com to 
`
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestAddError() {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", houston.AddWorkspaceUserRequest{WorkspaceID: id, Email: email, Role: role}).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestRemove() {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	userID := "ckc0eir8e01gj07608ajmvia1"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspaceUser", houston.DeleteWorkspaceUserRequest{WorkspaceID: id, UserID: mockWsUserResponse.ID}).Return(mockWsUserResponse, nil)

	buf := new(bytes.Buffer)
	err := Remove(id, userID, api, buf)
	s.NoError(err)
	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
                               ckc0eir8e01gj07608ajmvia1                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestRemoveError() {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspaceUser", houston.DeleteWorkspaceUserRequest{WorkspaceID: id, UserID: email}).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Remove(id, email, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListRoles() {
	wsID := "ck1qg6whg001r08691y117hub"

	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "ckbv7zpkh00og0760ki4mhl6r",
			Username: "test@test.com",
			FullName: "test",
			Emails:   []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBinding{
				{
					Role: houston.WorkspaceAdminRole,
					Workspace: houston.Workspace{
						ID: wsID,
					},
				},
			},
		},
	}

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	s.NoError(err)
	expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListRolesWithServiceAccounts() {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"
	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "ckbv7zpkh00og0760ki4mhl6r",
			Username: "test@test.com",
			FullName: "test",
			Emails:   []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBinding{
				{
					Role: houston.WorkspaceAdminRole,
					Workspace: houston.Workspace{
						ID: wsID,
					},
				},
			},
		},
	}

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	s.NoError(err)
	expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListRolesError() {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestPaginatedListRoles() {
	s.Run("user should not be prompted for pagination options if api returns less then page size", func() {
		wsID := "ck1qg6whg001r08691y117hub"
		paginationPageSize := 100

		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "ckbv7zpkh00og0760ki4mhl6r",
				Username: "test@test.com",
				FullName: "test",
				Emails:   []houston.Email{{Address: "test@test.com"}},
				RoleBindings: []houston.RoleBinding{
					{
						Role: houston.WorkspaceAdminRole,
						Workspace: houston.Workspace{
							ID: wsID,
						},
					},
				},
			},
		}

		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: "", Take: float64(paginationPageSize)}).Return(mockResponse, nil)

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

		buf := new(bytes.Buffer)
		err = PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
		s.NoError(err)
		expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
		s.Equal(expected, buf.String())
		api.AssertExpectations(s.T())
	})
	s.Run("user should be prompted for pagination options if return record is same as page size", func() {
		wsID := "ck1qg6whg001r08691y117hub"
		paginationPageSize := 1

		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "ckbv7zpkh00og0760ki4mhl6r",
				Username: "test@test.com",
				FullName: "test",
				Emails:   []houston.Email{{Address: "test@test.com"}},
				RoleBindings: []houston.RoleBinding{
					{
						Role: houston.WorkspaceAdminRole,
						Workspace: houston.Workspace{
							ID: wsID,
						},
					},
				},
			},
		}

		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: "", Take: float64(paginationPageSize)}).Return(mockResponse, nil)

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

		buf := new(bytes.Buffer)
		err = PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
		s.NoError(err)
		expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
		s.Equal(expected, buf.String())
		api.AssertExpectations(s.T())
	})
	s.Run("user should not see previous option if no record return if last action was next", func() {
		wsID := "ck1qg6whg001r08691y117hub"
		paginationPageSize := 10

		mockResponse := []houston.WorkspaceUserRoleBindings{}

		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: "", Take: float64(paginationPageSize)}).Return(mockResponse, nil)

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

		buf := new(bytes.Buffer)
		err = PaginatedListRoles(wsID, "", paginationPageSize, 10, api, buf)
		s.NoError(err)
		expected := ` USERNAME     ID     ROLE     
`
		s.Equal(expected, buf.String())
		api.AssertExpectations(s.T())
	})
	s.Run("user should not see next option if no record return if last action was previous", func() {
		wsID := "ck1qg6whg001r08691y117hub"
		paginationPageSize := -10

		mockResponse := []houston.WorkspaceUserRoleBindings{}

		api := new(mocks.ClientInterface)
		api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: "", Take: float64(paginationPageSize)}).Return(mockResponse, nil)

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

		buf := new(bytes.Buffer)
		err = PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
		s.NoError(err)
		expected := ` USERNAME     ID     ROLE     
`
		s.Equal(expected, buf.String())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestPaginatedListRolesError() {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"
	paginationPageSize := 100

	api := new(mocks.ClientInterface)
	api.On("ListWorkspacePaginatedUserAndRoles", houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: wsID, CursorID: "", Take: float64(paginationPageSize)}).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
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

		value := promptPaginatedOption(wsID, wsID, paginationPageSize, 10, 0, false)
		s.Equal(value.Quit, true)
	})
}

func (s *Suite) TestUpdateRole() {
	testUtil.InitTestConfig("software")

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", houston.UpdateWorkspaceUserRoleRequest{WorkspaceID: id, Email: email, Role: role}).Return(role, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	s.NoError(err)
	expected := `Role has been changed from WORKSPACE_VIEWER to test-role for user test@test.com`
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateRoleNoAccessDeploymentOnly() {
	testUtil.InitTestConfig("software")

	id := "ckg6sfddu30911pc0n1o0e97e"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	s.Equal("the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateRoleErrorGetRoles() {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(houston.WorkspaceUserRoleBindings{}, errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateRoleError() {
	testUtil.InitTestConfig("software")

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", houston.UpdateWorkspaceUserRoleRequest{WorkspaceID: id, Email: email, Role: role}).Return("", errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateRoleNoAccess() {
	testUtil.InitTestConfig("software")

	mockRoles.RoleBindings = []houston.RoleBinding{}

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: id, Email: email}).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	s.Equal("the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(s.T())
}
