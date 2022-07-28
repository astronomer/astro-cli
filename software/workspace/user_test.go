package workspace

import (
	"bytes"
	"errors"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

var (
	mockWsUserResponse = &houston.Workspace{
		ID: "ckc0eir8e01gj07608ajmvia1",
	}
	mockRoles = houston.WorkspaceUserRoleBindings{
		RoleBindings: []houston.RoleBindingWorkspace{
			{
				Role: houston.WorkspaceViewerRole,
				Workspace: struct {
					ID string `json:"id"`
				}{ID: "ckoixo6o501496qemiwsja1tl"},
			},
			{
				Role: "DEPLOYMENT_VIEWER",
				Workspace: struct {
					ID string `json:"id"`
				}{ID: "ckg6sfddu30911pc0n1o0e97e"},
			},
		},
	}
	errMock = errors.New("api error")
)

func TestAdd(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", id, email, role).Return(mockWsUserResponse, nil)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     WORKSPACE ID                  EMAIL             ROLE          
          ckc0eir8e01gj07608ajmvia1     test@test.com     test-role     
Successfully added test@test.com to 
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestAddError(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", id, email, role).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestRemove(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	userID := "ckc0eir8e01gj07608ajmvia1"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspaceUser", id, mockWsUserResponse.ID).Return(mockWsUserResponse, nil)

	buf := new(bytes.Buffer)
	err := Remove(id, userID, api, buf)
	assert.NoError(t, err)
	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
                               ckc0eir8e01gj07608ajmvia1                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestRemoveError(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspaceUser", id, email).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Remove(id, email, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestListRoles(t *testing.T) {
	wsID := "ck1qg6whg001r08691y117hub"

	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "ckbv7zpkh00og0760ki4mhl6r",
			Username: "test@test.com",
			FullName: "test",
			Emails:   []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBindingWorkspace{
				{
					Role: houston.WorkspaceAdminRole,
					Workspace: struct {
						ID string `json:"id"`
					}{
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
	assert.NoError(t, err)
	expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestListRolesWithServiceAccounts(t *testing.T) {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"
	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "ckbv7zpkh00og0760ki4mhl6r",
			Username: "test@test.com",
			FullName: "test",
			Emails:   []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBindingWorkspace{
				{
					Role: houston.WorkspaceAdminRole,
					Workspace: struct {
						ID string `json:"id"`
					}{
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
	assert.NoError(t, err)
	expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestListRolesError(t *testing.T) {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestPaginatedListRoles(t *testing.T) {
	wsID := "ck1qg6whg001r08691y117hub"
	var paginationPageSize float64 = 100

	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "ckbv7zpkh00og0760ki4mhl6r",
			Username: "test@test.com",
			FullName: "test",
			Emails:   []houston.Email{{Address: "test@test.com"}},
			RoleBindings: []houston.RoleBindingWorkspace{
				{
					Role: houston.WorkspaceAdminRole,
					Workspace: struct {
						ID string `json:"id"`
					}{
						ID: wsID,
					},
				},
			},
		},
	}

	api := new(mocks.ClientInterface)
	api.On("ListWorkspacePaginatedUserAndRoles", wsID, "", paginationPageSize).Return(mockResponse, nil)

	// mock os.Stdin for when prompted by PromptPaginatedOption
	input := []byte("q")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	buf := new(bytes.Buffer)
	err = PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
	assert.NoError(t, err)
	expected := ` USERNAME          ID                            ROLE                
 test@test.com     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestPaginatedListRolesError(t *testing.T) {
	testUtil.InitTestConfig("software")

	wsID := "ck1qg6whg001r08691y117hub"
	var paginationPageSize float64 = 100

	api := new(mocks.ClientInterface)
	api.On("ListWorkspacePaginatedUserAndRoles", wsID, "", paginationPageSize).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := PaginatedListRoles(wsID, "", paginationPageSize, 0, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestShowListRolesPaginatedOption(t *testing.T) {
	wsID := "ck1qg6whg001r08691y117hub"
	var paginationPageSize float64 = 100

	t.Run("total record less then page size", func(t *testing.T) {
		// mock os.Stdin for when prompted by PromptPaginatedOption
		input := []byte("q")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		value := PromptPaginatedOption(wsID, wsID, paginationPageSize, 10, 0)
		assert.Equal(t, value.quit, true)
	})
}

func TestUpdateRole(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", id, email, role).Return(role, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.NoError(t, err)
	expected := `Role has been changed from WORKSPACE_VIEWER to test-role for user test@test.com`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestUpdateRoleNoAccessDeploymentOnly(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ckg6sfddu30911pc0n1o0e97e"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleErrorGetRoles(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(houston.WorkspaceUserRoleBindings{}, errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleError(t *testing.T) {
	testUtil.InitTestConfig("software")

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", id, email, role).Return("", errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleNoAccess(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockRoles.RoleBindings = []houston.RoleBindingWorkspace{}

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "test@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(t)
}
