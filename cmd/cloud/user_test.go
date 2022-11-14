package cloud

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/cloud/user"

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
	expectedHelp := "Invite a user to your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newUserCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), expectedHelp)
}

var (
	errTest      = errors.New("test-error")
	errWorkspace = errors.New("test-workspace-error")
)

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
		astroGQLClient = mockClient

		cmdArgs := []string{"invite", "some@email.com"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("valid email with valid role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
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
		astroGQLClient = mockClient

		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and no invite gets created", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace",
			Description:    "",
			Users:          nil,
			OrganizationID: "test-org-id",
			CreatedAt:      "",
			UpdatedAt:      "",
			RoleBindings:   nil,
		}, nil,
		)
		mockClient.On("CreateUserInvite", mock.Anything).Return(astro.UserInvite{},
			errTest).Once()
		astroGQLClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "invalid"}
		_, err := execUserCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidRole)
	})
	t.Run("any errors from api are returned and no invite gets created", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace",
			Description:    "",
			Users:          nil,
			OrganizationID: "test-org-id",
			CreatedAt:      "",
			UpdatedAt:      "",
			RoleBindings:   nil,
		}, nil,
		)
		mockClient.On("CreateUserInvite", mock.Anything).Return(astro.UserInvite{},
			errTest).Once()
		astroGQLClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create invite: test-error")
	})
	t.Run("any workspace error from api are returned and no invite gets created", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(astro.Workspace{}, errWorkspace)
		mockClient.On("CreateUserInvite", mock.Anything).Return(astro.UserInvite{},
			nil).Once()
		astroGQLClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("any context errors from api are returned and no invite gets created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astro_mocks.Client)
		mockClient.On("CreateUserInvite", mock.Anything).Return(astro.UserInvite{},
			nil).Once()
		astroGQLClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		// mock os.Stdin
		expectedInput := []byte("test-email-input")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "invite for test-email-input with role ORGANIZATION_MEMBER created"

		mockInvite := astro.UserInvite{
			UserID:         "test-user-id",
			OrganizationID: "test-org-id",
			OauthInviteID:  "test-oauth-invite-id",
			ExpiresAt:      "now+lots-of-days",
		}
		mockWorkspace := astro.Workspace{
			ID:             "test-workspace-id",
			Label:          "test-workspace-label",
			OrganizationID: "test-org-id",
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkspace", mock.Anything).Return(mockWorkspace, nil).Once()
		mockClient.On("CreateUserInvite", mock.Anything).Return(mockInvite, nil).Once()
		astroGQLClient = mockClient

		cmdArgs := []string{"invite"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("command returns an error when no email is provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		// mock os.Stdin
		expectedInput := []byte("")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		cmdArgs := []string{"invite"}
		_, err = execUserCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidEmail)
	})
}
