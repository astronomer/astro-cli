package cloud

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func execUserCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newUserCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestUserRootCommand(t *testing.T) {
	expectedHelp := "Invite Users to your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newUserCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), expectedHelp)
}

func TestUserInvite(t *testing.T) {
	expectedHelp := "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints invite help", func(t *testing.T) {
		cmdArgs := []string{"invite", "-h"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("valid email with no role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"

		// TODO need to discuss the intent with the client in astro and the mock client as it feels duplicate
		mockInvite := astro.UserInvite{
			UserID:         "test-user-id",
			OrganizationID: "test-org-id",
			OauthInviteID:  "test-oauth-invite-id",
			ExpiresAt:      "now+7days",
		}
		mockWorkspace := astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace-label",
			OrganizationID: "test-org-id",
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(mockWorkspace, nil).Once()
		mockClient.On("CreateUserInvite", mock.Anything).Return(mockInvite, nil).Once()
		astroClient = mockClient

		cmdArgs := []string{"invite", "some@email.com"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})

	t.Run("valid email with valid role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role mytestvalidrole created"
		mockInvite := astro.UserInvite{
			UserID:         "test-user-id",
			OrganizationID: "test-org-id",
			OauthInviteID:  "test-oauth-invite-id",
			ExpiresAt:      "now+7days",
		}
		mockWorkspace := astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace-label",
			OrganizationID: "test-org-id",
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(mockWorkspace, nil).Once()
		mockClient.On("CreateUserInvite", mock.Anything).Return(mockInvite, nil).Once()
		astroClient = mockClient

		cmdArgs := []string{"invite", "some@email.com", "--role", "mytestvalidrole"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any error from api are returned and no invite gets created", func(t *testing.T) {
		mockError := errors.New("test-error") //nolint
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace",
			Description:    "",
			Users:          nil,
			OrganizationID: "test-organization-id",
			CreatedAt:      "",
			UpdatedAt:      "",
			RoleBindings:   nil,
		}, nil,
		)
		mockClient.On("CreateUserInvite", mock.Anything).Return(astro.UserInvite{},
			mockError).Once()
		astroClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "mytestrole"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, mockError.Error())
	})
}
