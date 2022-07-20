package user

import (
	"errors"
	"testing"

	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

var (
	errorWorkspace = errors.New("test-ws-error")
	errorInvite    = errors.New("test-inv-error")
)

func TestCreateInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedInput := astro.CreateUserInviteInput{
		InviteeEmail:   "test-email@test.com",
		Role:           "test-role",
		OrganizationID: "test-org-id",
	}
	expectedInvite := astro.UserInvite{
		UserID:         "test-user-id",
		OrganizationID: "test-org-id",
		OauthInviteID:  "test-oauth-invite-id",
		ExpiresAt:      "test-expiry",
	}
	expectedWorkspace := astro.Workspace{
		ID:             "test-workspace-id",
		Label:          "test-workspace-label",
		OrganizationID: "test-org-id",
	}
	t.Run("happy path", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(expectedWorkspace, nil).Once()
		mockClient.On("CreateUserInvite", expectedInput).Return(expectedInvite, nil).Once()
		invite, err := CreateInvite("test-email@test.com", "test-role", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, invite, expectedInvite)
	})
	t.Run("error path", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(astro.Workspace{}, errorWorkspace).Once()
		mockClient.On("CreateUserInvite", expectedInput).Return(astro.UserInvite{}, errorInvite).Once()
		invite, err := CreateInvite("test-email@test.com", "test-role", mockClient)
		assert.Error(t, err, errorWorkspace.Error())
		assert.Equal(t, invite, astro.UserInvite{})
	})
}
