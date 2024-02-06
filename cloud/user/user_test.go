package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/stretchr/testify/mock"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// org user variables
var (
	errorNetwork = errors.New("network error")
	errorInvite  = errors.New("test-inv-error")
	orgRole      = "ORGANIZATION_MEMBER"
	isIdpManaged = true
	user1        = astrocore.User{
		CreatedAt:                   time.Now(),
		FullName:                    "user 1",
		Id:                          "user1-id",
		OrgRole:                     &orgRole,
		OrgUserRelationIsIdpManaged: &isIdpManaged,
		Username:                    "user@1.com",
	}
	users = []astrocore.User{
		user1,
	}
	ListOrgUsersResponseOK = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      users,
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
	GetUserWithResponseOK = astrocore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	errorBodyGet, _ = json.Marshal(astrocore.Error{
		Message: "failed to get user",
	})
	GetUserWithResponseError = astrocore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyGet,
		JSON200: nil,
	}
)

// workspace users variables
var (
	workspaceRole  = "WORKSPACE_MEMBER"
	workspaceUser1 = astrocore.User{
		CreatedAt:                   time.Now(),
		FullName:                    "user 1",
		Id:                          "user1-id",
		WorkspaceRole:               &workspaceRole,
		OrgUserRelationIsIdpManaged: &isIdpManaged,
		Username:                    "user@1.com",
	}
	workspaceUsers = []astrocore.User{
		workspaceUser1,
	}
	ListWorkspaceUsersResponseOK = astrocore.ListWorkspaceUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      workspaceUsers,
		},
	}
	ListWorkspaceUsersResponseError = astrocore.ListWorkspaceUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateWorkspaceUserRoleResponseOK = astrocore.MutateWorkspaceUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UserRole{
			Role: "WORKSPACE_MEMBER",
		},
	}
	MutateWorkspaceUserRoleResponseError = astrocore.MutateWorkspaceUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteWorkspaceUserResponseOK = astrocore.DeleteWorkspaceUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceUserResponseError = astrocore.DeleteWorkspaceUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
}

func TestCreateInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	inviteUserID := "user_cuid"
	createInviteResponseOK := astrocore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Invite{
			InviteId: "",
			UserId:   &inviteUserID,
		},
	}
	errorBody, _ := json.Marshal(astrocore.Error{
		Message: "failed to create invite: test-inv-error",
	})
	createInviteResponseError := astrocore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	t.Run("happy path", func(t *testing.T) {
		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateUserInviteWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(nil, errorNetwork).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateUserInviteWithResponse returns an error", func(t *testing.T) {
		expectedOutMessage := "failed to create invite: test-inv-error"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseError, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, expectedOutMessage)
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when email is blank returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidEmail)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when writing output returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", testWriter{Error: errorInvite}, mockClient)
		assert.EqualError(t, err, "failed to create invite: test-inv-error")
	})
}

func TestIsRoleValid(t *testing.T) {
	var err error
	t.Run("happy path when role is ORGANIZATION_MEMBER", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_MEMBER")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is ORGANIZATION_BILLING_ADMIN", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_BILLING_ADMIN")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is ORGANIZATION_OWNER", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_OWNER")
		assert.NoError(t, err)
	})
	t.Run("error path", func(t *testing.T) {
		err = IsRoleValid("test")
		assert.ErrorIs(t, err, ErrInvalidRole)
	})
}

func TestUpdateUserRole(t *testing.T) {
	t.Run("happy path UpdateUserRole", func(t *testing.T) {
		expectedOutMessage := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when MutateOrgUserRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateOrgUserRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

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

		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()

		err = UpdateUserRole("", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestListOrgUser(t *testing.T) {
	t.Run("happy path TestListOrgUser", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when ListOrgUsersWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgUsers(out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListOrgUsersWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		assert.EqualError(t, err, "failed to list users")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListOrgUsers(out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestIsWorkspaceRoleValid(t *testing.T) {
	var err error
	t.Run("happy path when role is WORKSPACE_MEMBER", func(t *testing.T) {
		err = IsWorkspaceRoleValid("WORKSPACE_MEMBER")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is WORKSPACE_OPERATOR", func(t *testing.T) {
		err = IsWorkspaceRoleValid("WORKSPACE_OPERATOR")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is WORKSPACE_OWNER", func(t *testing.T) {
		err = IsWorkspaceRoleValid("WORKSPACE_OWNER")
		assert.NoError(t, err)
	})
	t.Run("error path", func(t *testing.T) {
		err = IsRoleValid("test")
		assert.ErrorIs(t, err, ErrInvalidRole)
	})
}

func TestListWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path TestListWorkspaceUser", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.NoError(t, err)
	})

	t.Run("error path when ListWorkspaceUsersWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListWorkspaceUsersWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.EqualError(t, err, "failed to list users")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestUpdateWorkspaceUserRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateWorkspaceUserRole", func(t *testing.T) {
		expectedOutMessage := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when MutateWorkspaceUserRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceUserRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspaceUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
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

		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = UpdateWorkspaceUserRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestAddWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddWorkspaceUser", func(t *testing.T) {
		expectedOutMessage := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when MutateWorkspaceUserRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceUserRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

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

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = AddWorkspaceUser("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestDeleteWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteWorkspaceUser", func(t *testing.T) {
		expectedOutMessage := "The user user@1.com was successfully removed from the workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when DeleteWorkspaceUserWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteWorkspaceUserWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseError, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("DeleteWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
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

		expectedOut := "The user user@1.com was successfully removed from the workspace\n"
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()

		err = RemoveWorkspaceUser("", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestGetUser(t *testing.T) {
	t.Run("happy path GetUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		_, err := GetUser(mockClient, user1.Id)
		assert.NoError(t, err)
	})

	t.Run("error path when GetUserWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseError, nil).Twice()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "failed to get user")
	})

	t.Run("error path when GetUserWithResponse returns a network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetUser(mockClient, user1.Id)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestIsOrganizationRoleValid(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		err := IsOrganizationRoleValid("ORGANIZATION_MEMBER")
		assert.NoError(t, err)
	})

	t.Run("error path", func(t *testing.T) {
		err := IsOrganizationRoleValid("Invalid Role")
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidOrganizationRole, err)
	})
}
