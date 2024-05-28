package software

import (
	"bytes"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/software/teams"
	"github.com/stretchr/testify/mock"
)

func execTeamCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newTeamCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func (s *Suite) TestNewGetTeamCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	team := &houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}

	api := new(mocks.ClientInterface)
	api.On("GetTeam", mock.Anything).Return(team, nil)
	houstonClient = api

	output, err := execTeamCmd("get", "test-id")
	s.NoError(err)
	s.Contains(output, "")
	api.AssertExpectations(s.T())
}

func (s *Suite) TestNewGetTeamUsersCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	team := &houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}
	users := []houston.User{
		{
			Username: "email@email.com",
			ID:       "test-id",
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetTeam", mock.Anything).Return(team, nil)
	api.On("GetTeamUsers", mock.Anything).Return(users, nil)
	houstonClient = api

	output, err := execTeamCmd("get", "-u", "test-id")
	s.NoError(err)
	s.Contains(output, "USERNAME            ID          \n email@email.com     test-id")
	api.AssertExpectations(s.T())
}

func (s *Suite) TestNewTeamListCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	team := houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}

	api := new(mocks.ClientInterface)
	api.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: teams.ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{team}}, nil)
	houstonClient = api

	output, err := execTeamCmd("list")
	s.NoError(err)
	s.Contains(output, "Everyone")
	s.Contains(output, "blah-id")
	api.AssertExpectations(s.T())
}

func (s *Suite) TestNewTeamUpdateCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("CreateTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "team-id", Role: houston.SystemAdminRole}).Return(houston.SystemAdminRole, nil)
	houstonClient = api

	output, err := execTeamCmd("update", "team-id", "--role", houston.SystemAdminRole)
	s.NoError(err)
	s.Contains(output, "team-id")
	s.Contains(output, houston.SystemAdminRole)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListTeam() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("success with page size more than the threshold", func() {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: teams.ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api
		defer testUtil.MockUserInput(s.T(), "q")()

		output, err := execTeamCmd("list", "-p", "-s=30")
		s.NoError(err)
		s.Contains(output, "test-id")
		api.AssertExpectations(s.T())
	})

	s.Run("success with negative page size", func() {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: teams.ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api
		defer testUtil.MockUserInput(s.T(), "q")()

		output, err := execTeamCmd("list", "-p", "-s=-2")
		s.NoError(err)
		s.Contains(output, "test-id")
		api.AssertExpectations(s.T())
	})

	s.Run("success without pagination", func() {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", houston.ListTeamsRequest{Cursor: "", Take: teams.ListTeamLimit}).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api

		output, err := execTeamCmd("list")
		s.NoError(err)
		s.Contains(output, "test-id")
		api.AssertExpectations(s.T())
	})
}
