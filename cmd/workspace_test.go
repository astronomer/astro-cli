package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

var (
	mockWorkspace = &houston.Workspace{
		ID:           "ck05r3bor07h40d02y2hw4n4v",
		Label:        "airflow",
		Description:  "test description",
		Users:        nil,
		CreatedAt:    "2019-10-16T21:14:22.105Z",
		UpdatedAt:    "2019-10-16T21:14:22.105Z",
		RoleBindings: nil,
	}
	mockWorkspaceTeamRole = &houston.RoleBinding{
		Role: houston.WorkspaceViewerRole,
		Team: houston.Team{
			ID:   "cl0evnxfl0120dxxu1s4nbnk7",
			Name: "test-team",
		},
	}
)

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := " NAME           ID                            \n" +
		"\x1b[1;32m airflow        ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n " +
		"airflow123     XXXXXXXXXXXXXXX               \n"

	mockWorkspaces := []houston.Workspace{
		*mockWorkspace,
		{
			ID:          "XXXXXXXXXXXXXXX",
			Label:       "airflow123",
			Description: "test description 123",
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("ListWorkspaces").Return(mockWorkspaces, nil)

	output, err := executeCommandC(api, "workspace", "list")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output, err)
}

func TestWorkspaceSaRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("workspace", "service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro workspace service-account")
}

func TestNewWorkspaceUserListCmd(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
		}
	})
	houstonClient = houston.NewClient(client)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserListCmd(buf)
	assert.NotNil(t, cmd)
	assert.Nil(t, cmd.Args)
}

func TestWorkspaceUserRm(t *testing.T) {
	testUtil.InitTestConfig()

	mockUserID := "ckc0eir8e01gj07608ajmvia1"

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("DeleteWorkspaceUser", mockWorkspace.ID, mockUserID).Return(mockWorkspace, nil)
	houstonClient = api

	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
 airflow                       ck05r3bor07h40d02y2hw4n4v                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`

	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserRmCmd(buf)
	err := cmd.RunE(cmd, []string{mockUserID})
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())
}

func TestWorkspaceSAGetCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
	mockSA := houston.ServiceAccount{
		ID:         "ckqvfa2cu1468rn9hnr0bqqfk",
		APIKey:     "658b304f36eaaf19860a6d9eb73f7d8a",
		Label:      "yooo can u see me test",
		Category:   "",
		CreatedAt:  "2021-07-08T21:28:57.966Z",
		UpdatedAt:  "2021-07-08T21:28:57.966Z",
		LastUsedAt: "",
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("ListWorkspaceServiceAccounts", mockWorkspace.ID).Return([]houston.ServiceAccount{mockSA}, nil)

	output, err := executeCommandC(api, "workspace", "sa", "get", "-w="+mockWorkspace.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

// Workspace Teams

func TestWorkspaceTeamAddCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` NAME        WORKSPACE ID                  TEAM ID                       ROLE                 
 airflow     ck05r3bor07h40d02y2hw4n4v     cl0evnxfl0120dxxu1s4nbnk7     WORKSPACE_VIEWER     
Successfully added cl0evnxfl0120dxxu1s4nbnk7 to airflow
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("AddWorkspaceTeam", mockWorkspace.ID, mockWorkspaceTeamRole.Team.ID, mockWorkspaceTeamRole.Role).Return(mockWorkspace, nil)

	output, err := executeCommandC(api,
		"workspace",
		"team",
		"add",
		"--workspace-id="+mockWorkspace.ID,
		"--team-id="+mockWorkspaceTeamRole.Team.ID,
		"--role="+mockWorkspaceTeamRole.Role,
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestWorkspaceTeamRm(t *testing.T) {
	testUtil.InitTestConfig()

	mockTeamID := "ckc0eir8e01gj07608ajmvia1"

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("DeleteWorkspaceTeam", mockWorkspace.ID, mockTeamID).Return(mockWorkspace, nil)
	houstonClient = api

	expected := ` NAME                          WORKSPACE ID                                      TEAM ID                                           
 airflow                       ck05r3bor07h40d02y2hw4n4v                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed team from workspace
`

	buf := new(bytes.Buffer)
	cmd := newWorkspaceTeamRemoveCmd(buf)
	err := cmd.RunE(cmd, []string{mockTeamID})
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())
}

func TestWorkspaceTeamUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Role has been changed from WORKSPACE_VIEWER to WORKSPACE_EDITOR for team cl0evnxfl0120dxxu1s4nbnk7`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("UpdateWorkspaceTeam", mockWorkspace.ID, mockWorkspaceTeamRole.Team.ID, mockWorkspaceTeamRole.Role).Return(mockWorkspace.Label, nil)

	output, err := executeCommandC(api,
		"workspace",
		"team",
		"update",
		mockWorkspaceTeamRole.Team.ID,
		"--workspace-id="+mockWorkspace.ID,
		"--role="+mockWorkspaceTeamRole.Role,
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}
