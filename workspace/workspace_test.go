package workspace

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var (
	mockWorkspace = &houston.Workspace{
		ID:           "ckc0j8y1101xo0760or02jdi7",
		Label:        "test",
		Description:  "description",
		Users:        nil,
		CreatedAt:    "",
		UpdatedAt:    "",
		RoleBindings: nil,
	}
	mockWorkspaceList = []houston.Workspace{
		{
			ID:           "ckbv7zvb100pe0760xp98qnh9",
			Label:        "w1",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckbv8pwbq00wk0760us7ktcgd",
			Label:        "wwww",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckc0j8y1101xo0760or02jdi7",
			Label:        "test",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
	}
)

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig()

	label := "test"
	description := "description"

	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", label, description).Return(mockWorkspace, nil)

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     ID                            
 test     ckc0j8y1101xo0760or02jdi7     

 Successfully created workspace
`
	assert.Equal(t, buf.String(), expected)
	api.AssertExpectations(t)
}

func TestCreateError(t *testing.T) {
	testUtil.InitTestConfig()

	label := "test"
	description := "description"

	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", label, description).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestList(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces").Return(mockWorkspaceList, nil)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.NoError(t, err)
	expected := ` NAME     ID                            
 w1       ckbv7zvb100pe0760xp98qnh9     
 wwww     ckbv8pwbq00wk0760us7ktcgd     
 test     ckc0j8y1101xo0760or02jdi7     
`
	assert.Equal(t, buf.String(), expected)
	api.AssertExpectations(t)
}

func TestListActiveWorkspace(t *testing.T) {
	testUtil.InitTestConfig()

	// Add active workspace to mocks
	mockWorkspaceList[0].ID = "ck05r3bor07h40d02y2hw4n4v"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces").Return(mockWorkspaceList, nil)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.NoError(t, err)
	expected := " NAME     ID                            \n\x1b[1;32m w1       ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n wwww     ckbv8pwbq00wk0760us7ktcgd     \n test     ckc0j8y1101xo0760or02jdi7     \n"
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestListError(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces").Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig()

	mockResponse := &houston.Workspace{
		ID: "ckc0j8y1101xo0760or02jdi7",
	}

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", mockResponse.ID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := Delete(mockResponse.ID, api, buf)
	assert.NoError(t, err)
	expected := "\n Successfully deleted workspace\n"
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestDeleteError(t *testing.T) {
	testUtil.InitTestConfig()

	wsID := "ckc0j8y1101xo0760or02jdi7"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Delete(wsID, api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestGetCurrentWorkspace(t *testing.T) {
	// we init default workspace to: ck05r3bor07h40d02y2hw4n4v
	testUtil.InitTestConfig()

	ws, err := GetCurrentWorkspace()
	assert.NoError(t, err)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", ws)
}

func TestGetCurrentWorkspaceError(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, []byte(""), 0o777)
	config.InitConfig(fs)
	_, err := GetCurrentWorkspace()
	assert.EqualError(t, err, "no context set, have you authenticated to a cluster")
}

func TestGetCurrentWorkspaceErrorNoCurrentContext(t *testing.T) {
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)
	_, err := GetCurrentWorkspace()
	assert.EqualError(t, err, "current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
}

func TestGetWorkspaceSelectionError(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces").Return(nil, errMock)

	buf := new(bytes.Buffer)
	_, err := getWorkspaceSelection(api, buf)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}

func TestSwitch(t *testing.T) {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	api := new(mocks.ClientInterface)
	api.On("GetWorkspace", mockWorkspace.ID).Return(mockWorkspace, nil)

	buf := new(bytes.Buffer)
	err := Switch(mockWorkspace.ID, api, buf)
	assert.NoError(t, err)
	expected := " CLUSTER                             WORKSPACE                           \n localhost                           ckc0j8y1101xo0760or02jdi7           \n"
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestSwitchHoustonError(t *testing.T) {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	wsID := "ckbv7zvb100pe0760xp98qnh9"

	api := new(mocks.ClientInterface)
	api.On("GetWorkspace", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Switch(wsID, api, buf)
	assert.EqualError(t, err, "workspace id is not valid: api error")
	api.AssertExpectations(t)
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig()

	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", id, args).Return(mockWorkspace, nil)

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	assert.NoError(t, err)
	expected := " NAME     ID                            \n test     ckc0j8y1101xo0760or02jdi7     \n\n Successfully updated workspace\n"
	assert.Equal(t, expected, buf.String())
	api.AssertExpectations(t)
}

func TestUpdateError(t *testing.T) {
	testUtil.InitTestConfig()

	// prepare houston-api fake response
	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", id, args).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	assert.EqualError(t, err, errMock.Error())
	api.AssertExpectations(t)
}
