package workspace

import (
	"bytes"
	"errors"
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
				Role: "WORKSPACE_VIEWER",
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
	errMock = errors.New("api error") //nolint:goerr113
)

func TestAdd(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", id, email, role).Return(mockWsUserResponse, nil)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     WORKSPACE ID                  EMAIL               ROLE          
          ckc0eir8e01gj07608ajmvia1     andrii@test.com     test-role     
Successfully added andrii@test.com to 
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestAddError(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", id, email, role).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestRemove(t *testing.T) {
	testUtil.InitTestConfig()

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
	testUtil.InitTestConfig()

	id := "ck1qg6whg001r08691y117hub"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspaceUser", id, email).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Remove(id, email, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestListRoles(t *testing.T) {
	mockResponse := &houston.Workspace{
		ID:        "ckbv7zvb100pe0760xp98qnh9",
		Label:     "w1",
		CreatedAt: "2020-06-25T20:09:29.917Z",
		UpdatedAt: "2020-06-25T20:09:29.917Z",
		RoleBindings: []houston.RoleBinding{
			{
				Role: "WORKSPACE_ADMIN",
				User: houston.RoleBindingUser{
					ID:       "ckbv7zpkh00og0760ki4mhl6r",
					Username: "andrii@astronomer.io",
				},
			},
		},
	}

	wsID := "ck1qg6whg001r08691y117hub"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.NoError(t, err)
	expected := ` USERNAME                 ID                            ROLE                
 andrii@astronomer.io     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestListRolesWithServiceAccounts(t *testing.T) {
	testUtil.InitTestConfig()

	mockResponse := &houston.Workspace{
		ID:        "ckbv7zvb100pe0760xp98qnh9",
		Label:     "w1",
		CreatedAt: "2020-06-25T20:09:29.917Z",
		UpdatedAt: "2020-06-25T20:09:29.917Z",
		RoleBindings: []houston.RoleBinding{
			{
				Role: "WORKSPACE_ADMIN",
				User: houston.RoleBindingUser{
					ID:       "ckbv7zpkh00og0760ki4mhl6r",
					Username: "andrii@astronomer.io",
				},
			},
			{
				Role: "WORKSPACE_ADMIN",
				ServiceAccount: houston.WorkspaceServiceAccount{
					ID:    "ckxaolfky0822zsvcrgts3c6a",
					Label: "WA1",
				},
			},
		},
	}
	wsID := "ck1qg6whg001r08691y117hub"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.NoError(t, err)
	expected := ` USERNAME                 ID                            ROLE                
 andrii@astronomer.io     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
 WA1                      ckxaolfky0822zsvcrgts3c6a     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestListRolesError(t *testing.T) {
	testUtil.InitTestConfig()

	wsID := "ck1qg6whg001r08691y117hub"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaceUserAndRoles", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestUpdateRole(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", id, email, role).Return(role, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.NoError(t, err)
	expected := `Role has been changed from WORKSPACE_VIEWER to test-role for user andrii@test.com`
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestUpdateRoleNoAccessDeploymentOnly(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ckg6sfddu30911pc0n1o0e97e"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleErrorGetRoles(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(houston.WorkspaceUserRoleBindings{}, errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleError(t *testing.T) {
	testUtil.InitTestConfig()

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)
	api.On("UpdateWorkspaceUserRole", id, email, role).Return("", errMock)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestUpdateRoleNoAccess(t *testing.T) {
	testUtil.InitTestConfig()

	mockRoles.RoleBindings = []houston.RoleBindingWorkspace{}

	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "andrii@test.com"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspaceUserRole", id, email).Return(mockRoles, nil)

	buf := new(bytes.Buffer)
	err := UpdateRole(id, email, role, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
	api.AssertExpectations(t)
}
