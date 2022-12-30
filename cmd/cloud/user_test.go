package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cloud/user"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	inviteUserID           = "user_cuid"
	createInviteResponseOK = astrocore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Invite{
			InviteId: "astro_invite_id",
			UserId:   &inviteUserID,
		},
	}
	errorBody, _ = json.Marshal(astrocore.Error{
		Message: "failed to create invite: test-error",
	})
	createInviteResponseError = astrocore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	orgRole = "ORGANIZATION_MEMBER"
	user1 = astrocore.User{
		CreatedAt: time.Now(),
		FullName: "user 1",
		Id: "user1-id",
		OrgRole: &orgRole,
		Username: "user@1.com",
	}
	users = []astrocore.User{
		user1,
	}
	ListOrgUsersResponseOK = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit: 1,
			Offset:   0,
			TotalCount: 1,
			Users: users,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list users",
	})
	ListOrgUsersResponseError = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateOrgUserRoleResponseOK = astrocore.MutateOrgUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UserRole{
			Role: "ORGANIZATION_MEMBER",
		},
	}
	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update user",
	})
	MutateOrgUserRoleResponseError = astrocore.MutateOrgUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"invite", "some@email.com"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("valid email with valid role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and no invite gets created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "invalid"}
		_, err := execUserCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidRole)
	})
	t.Run("any errors from api are returned and no invite gets created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create invite: test-error")
	})

	t.Run("any context errors from api are returned and no invite gets created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient

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

func TestUserList(t *testing.T) {
	expectedHelp := "List all the users in your Astro Organization"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"list", "-h"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("list all organization users with --limit", func(t *testing.T) {
		expectedOut := "user@1.com"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"list", "--limit", "1"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("any errors from api are returned and users are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"list"}
		_, err := execUserCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list users")
	})
	t.Run("any context errors from api are returned and users are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"list"}		
		_, err := execUserCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestUserUpdate(t *testing.T) {
	expectedHelp := "astro user update [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"update", "-h"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email with no role keeps the user the same", func(t *testing.T) {
		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("valid email with valid role updates user", func(t *testing.T) {
		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"		
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and role is not update", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "invalid"}
		_, err := execUserCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidRole)
	})
	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execUserCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"update"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		// mockClient.AssertExpectations(t)
	})
}
