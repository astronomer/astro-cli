package software

import (
	"bytes"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	mockWorkspaceTeamRole = &houston.RoleBinding{
		Role: houston.WorkspaceViewerRole,
		Team: houston.Team{
			ID:   "cl0evnxfl0120dxxu1s4nbnk7",
			Name: "test-team",
		},
		Workspace: houston.Workspace{
			ID:    "ck05r3bor07h40d02y2hw4n4v",
			Label: "airflow",
		},
	}
	mockWorkspaceTeam = &houston.Team{
		RoleBindings: []houston.RoleBinding{
			*mockWorkspaceTeamRole,
		},
	}
)

func (s *Suite) TestWorkspaceTeamAddCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` NAME        WORKSPACE ID                  TEAM ID                       ROLE                 
 airflow     ck05r3bor07h40d02y2hw4n4v     cl0evnxfl0120dxxu1s4nbnk7     WORKSPACE_VIEWER     
Successfully added cl0evnxfl0120dxxu1s4nbnk7 to airflow
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("AddWorkspaceTeam", houston.AddWorkspaceTeamRequest{WorkspaceID: mockWorkspace.ID, TeamID: mockWorkspaceTeamRole.Team.ID, Role: mockWorkspaceTeamRole.Role}).Return(mockWorkspace, nil)
	houstonClient = api

	output, err := execWorkspaceCmd(
		"team",
		"add",
		"--workspace-id="+mockWorkspace.ID,
		"--team-id="+mockWorkspaceTeamRole.Team.ID,
		"--role="+mockWorkspaceTeamRole.Role,
	)
	s.NoError(err)
	s.Equal(expectedOut, output)
}

func (s *Suite) TestWorkspaceTeamRm() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockTeamID := "ckc0eir8e01gj07608ajmvia1"

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("DeleteWorkspaceTeam", houston.DeleteWorkspaceTeamRequest{WorkspaceID: mockWorkspace.ID, TeamID: mockTeamID}).Return(mockWorkspace, nil)
	houstonClient = api

	expected := ` NAME                          WORKSPACE ID                                      TEAM ID                                           
 airflow                       ck05r3bor07h40d02y2hw4n4v                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed team from workspace
`

	buf := new(bytes.Buffer)
	cmd := newWorkspaceTeamRemoveCmd(buf)
	err := cmd.RunE(cmd, []string{mockTeamID})
	s.NoError(err)
	s.Equal(expected, buf.String())
}

func (s *Suite) TestWorkspaceTeamUpdateCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: mockWorkspace.ID, TeamID: mockWorkspaceTeamRole.Team.ID}).Return(mockWorkspaceTeam, nil)
	api.On("UpdateWorkspaceTeamRole", houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: mockWorkspace.ID, TeamID: mockWorkspaceTeamRole.Team.ID, Role: mockWorkspaceTeamRole.Role}).Return(mockWorkspace.Label, nil)
	houstonClient = api

	_, err := execWorkspaceCmd(
		"team",
		"update",
		mockWorkspaceTeamRole.Team.ID,
		"--workspace-id="+mockWorkspace.ID,
		"--role="+mockWorkspaceTeamRole.Role,
	)
	s.NoError(err)
}

func (s *Suite) TestWorkspaceTeamsListCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
		}
	})
	houstonClient = houston.NewClient(client)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceTeamsListCmd(buf)
	s.NotNil(cmd)
	s.Nil(cmd.Args)
}
