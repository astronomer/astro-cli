package software

import (
	"bytes"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var mockDeploymentTeamRole = &houston.RoleBinding{
	Role: houston.DeploymentViewerRole,
	Team: houston.Team{
		ID:   "cl0evnxfl0120dxxu1s4nbnk7",
		Name: "test-team",
	},
	Deployment: houston.Deployment{
		ID:          "ck05r3bor07h40d02y2hw4n4v",
		Label:       "airflow",
		ReleaseName: "airflow",
	},
}

func (s *Suite) TestDeploymentTeamAddCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` DEPLOYMENT ID                 TEAM ID                       ROLE                  
 cknz133ra49758zr9w34b87ua     cl0evnxfl0120dxxu1s4nbnk7     DEPLOYMENT_VIEWER     

Successfully added team cl0evnxfl0120dxxu1s4nbnk7 to deployment cknz133ra49758zr9w34b87ua as a DEPLOYMENT_VIEWER
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("AddDeploymentTeam", houston.AddDeploymentTeamRequest{DeploymentID: mockDeployment.ID, TeamID: mockDeploymentTeamRole.Team.ID, Role: mockDeploymentTeamRole.Role}).Return(mockDeploymentTeamRole, nil)
	houstonClient = api

	output, err := execDeploymentCmd(
		"team",
		"add",
		"--deployment-id="+mockDeployment.ID,
		"--team-id="+mockDeploymentTeamRole.Team.ID,
		"--role="+mockDeploymentTeamRole.Role,
	)
	s.NoError(err)
	s.Equal(expectedOut, output)
}

func (s *Suite) TestDeploymentTeamRm() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` DEPLOYMENT ID                 TEAM ID                       
 cknz133ra49758zr9w34b87ua     cl0evnxfl0120dxxu1s4nbnk7     

 Successfully removed team cl0evnxfl0120dxxu1s4nbnk7 from deployment cknz133ra49758zr9w34b87ua
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("RemoveDeploymentTeam", houston.RemoveDeploymentTeamRequest{DeploymentID: mockDeployment.ID, TeamID: mockDeploymentTeamRole.Team.ID}).Return(mockDeploymentTeamRole, nil)
	houstonClient = api

	output, err := execDeploymentCmd(
		"team",
		"remove",
		mockDeploymentTeamRole.Team.ID,
		"--deployment-id="+mockDeployment.ID,
	)

	s.NoError(err)
	s.Equal(expectedOut, output)
}

func (s *Suite) TestDeploymentTeamUpdateCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("UpdateDeploymentTeamRole", houston.UpdateDeploymentTeamRequest{DeploymentID: mockDeployment.ID, TeamID: mockDeploymentTeamRole.Team.ID, Role: mockDeploymentTeamRole.Role}).Return(mockDeploymentTeamRole, nil)
	houstonClient = api

	_, err := execDeploymentCmd(
		"team",
		"update",
		mockDeploymentTeamRole.Team.ID,
		"--deployment-id="+mockDeployment.ID,
		"--role="+mockDeploymentTeamRole.Role,
	)
	s.NoError(err)
}

func (s *Suite) TestDeploymentTeamsListCmd() {
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
	cmd := newDeploymentTeamListCmd(buf)
	s.NotNil(cmd)
	s.Nil(cmd.Args)
}
