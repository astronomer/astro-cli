package cmd

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewGetTeamCmd(t *testing.T) {
	testUtil.InitTestConfig()

	team := &houston.Team{
		Name: "Everyone",
		ID:   "blah-id",
	}
	appConfig := &houston.AppConfig{
		NfsMountDagDeployment: false,
	}

	api := new(mocks.ClientInterface)
	api.On("GetTeam", mock.Anything).Return(team, nil)
	api.On("GetAppConfig").Return(appConfig, nil)

	output, err := executeCommandC(api, "team", "get", "test-id")
	assert.NoError(t, err)
	assert.Contains(t, output, "")
}

func TestNewGetTeamUsersCmd(t *testing.T) {
	testUtil.InitTestConfig()

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
	appConfig := &houston.AppConfig{
		NfsMountDagDeployment: false,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfig, nil)
	api.On("GetTeam", mock.Anything).Return(team, nil)
	api.On("GetTeamUsers", mock.Anything).Return(users, nil)

	output, err := executeCommandC(api, "team", "get", "-u", "test-id")
	assert.NoError(t, err)
	assert.Contains(t, output, "USERNAME            ID          \n email@email.com     test-id")
}
