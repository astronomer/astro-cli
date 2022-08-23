package software

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/software/teams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func execTeamCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newTeamCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestNewGetTeamCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	team := &houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}

	api := new(mocks.ClientInterface)
	api.On("GetTeam", mock.Anything).Return(team, nil)
	houstonClient = api

	output, err := execTeamCmd("get", "test-id")
	assert.NoError(t, err)
	assert.Contains(t, output, "")
	api.AssertExpectations(t)
}

func TestNewGetTeamUsersCmd(t *testing.T) {
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
	assert.NoError(t, err)
	assert.Contains(t, output, "USERNAME            ID          \n email@email.com     test-id")
	api.AssertExpectations(t)
}

func TestNewTeamListCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	team := houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}

	api := new(mocks.ClientInterface)
	api.On("ListTeams", "", teams.ListTeamLimit).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{team}}, nil)
	houstonClient = api

	output, err := execTeamCmd("list")
	assert.NoError(t, err)
	assert.Contains(t, output, "Everyone")
	assert.Contains(t, output, "blah-id")
	api.AssertExpectations(t)
}

func TestNewTeamUpdateCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("CreateTeamSystemRoleBinding", "team-id", houston.SystemAdminRole).Return(houston.SystemAdminRole, nil)
	houstonClient = api

	output, err := execTeamCmd("update", "team-id", "--role", houston.SystemAdminRole)
	assert.NoError(t, err)
	assert.Contains(t, output, "team-id")
	assert.Contains(t, output, houston.SystemAdminRole)
	api.AssertExpectations(t)
}

func TestListTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("success with page size more than the threshold", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", "", teams.ListTeamLimit).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api
		defer testUtil.MockUserInput(t, "q")()

		output, err := execTeamCmd("list", "-p", "-s=30")
		assert.NoError(t, err)
		assert.Contains(t, output, "test-id")
		api.AssertExpectations(t)
	})

	t.Run("success with negative page size", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", "", teams.ListTeamLimit).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api
		defer testUtil.MockUserInput(t, "q")()

		output, err := execTeamCmd("list", "-p", "-s=-2")
		assert.NoError(t, err)
		assert.Contains(t, output, "test-id")
		api.AssertExpectations(t)
	})

	t.Run("success without pagination", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListTeams", "", teams.ListTeamLimit).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil).Once()
		houstonClient = api

		output, err := execTeamCmd("list")
		assert.NoError(t, err)
		assert.Contains(t, output, "test-id")
		api.AssertExpectations(t)
	})
}
