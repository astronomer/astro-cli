package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/lucsky/cuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// org user variables
var (
	errorNetwork = errors.New("network error")
	errorInvite  = errors.New("test-inv-error")
	orgRole      = astroiamcore.ORGANIZATIONMEMBER
	user1        = astroiamcore.User{
		CreatedAt:        time.Now(),
		FullName:         "user 1",
		Id:               "user1-id",
		OrganizationRole: &orgRole,
		Username:         "user@1.com",
	}
	users = []astroiamcore.User{
		user1,
	}
	ListOrgUsersResponseOK = astroiamcore.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroiamcore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      users,
		},
	}
	errorBodyList, _ = json.Marshal(astroiamcore.Error{
		Message: "failed to list users",
	})
	ListOrgUsersResponseError = astroiamcore.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateOrgUserRoleResponseOK = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroiamcore.SubjectRoles{
			OrganizationRole: lo.ToPtr(astroiamcore.SubjectRolesOrganizationRoleORGANIZATIONMEMBER),
		},
	}
	errorBodyUpdate, _ = json.Marshal(astroiamcore.Error{
		Message: "failed to update user",
	})
	MutateOrgUserRoleResponseError = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	GetUserWithResponseOK = astroiamcore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	errorBodyGet, _ = json.Marshal(astroiamcore.Error{
		Message: "failed to get user",
	})
	GetUserWithResponseError = astroiamcore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyGet,
		JSON200: nil,
	}
)

// workspace users variables
var (
	workspaceID    = cuid.New()
	workspaceUser1 = astroiamcore.User{
		CreatedAt:        time.Now(),
		FullName:         "user 1",
		Id:               "user1-id",
		WorkspaceRoles:   &[]astroiamcore.WorkspaceRole{{Role: astroiamcore.WORKSPACEMEMBER, WorkspaceId: workspaceID}},
		OrganizationRole: &orgRole,
		Username:         "user@1.com",
	}
	workspaceUsers = []astroiamcore.User{
		workspaceUser1,
	}
	ListWorkspaceUsersResponseOK = astroiamcore.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroiamcore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      workspaceUsers,
		},
	}
	ListWorkspaceUsersResponseError = astroiamcore.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateWorkspaceUserRoleResponseOK = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroiamcore.SubjectRoles{
			OrganizationRole: lo.ToPtr(astroiamcore.SubjectRolesOrganizationRoleORGANIZATIONMEMBER),
			WorkspaceRoles:   &[]astroiamcore.WorkspaceRole{{Role: astroiamcore.WORKSPACEMEMBER, WorkspaceId: workspaceID}},
		},
	}
	MutateWorkspaceUserRoleResponseError = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteWorkspaceUserResponseOK = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceUserResponseError = astroiamcore.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	GetWorkspaceUserWithResponseOK = astroiamcore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &workspaceUser1,
	}
)

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
}

func TestCreateInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inviteUserID := "user_cuid"
	createInviteResponseOK := astroiamcore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroiamcore.Invite{
			InviteId: "",
			UserId:   &inviteUserID,
		},
	}
	errorBody, _ := json.Marshal(astroiamcore.Error{
		Message: "failed to create invite: test-inv-error",
	})
	createInviteResponseError := astroiamcore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	t.Run("happy path", func(t *testing.T) {
		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
		createInviteRequest := astroiamcore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateUserInviteWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astroiamcore.CreateUserInviteRequest{
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
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astroiamcore.CreateUserInviteRequest{
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
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err = CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when email is blank returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidEmail)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when writing output returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
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

func TestUpdateOrgRole(t *testing.T) {
	t.Run("happy path UpdateUserRole", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedOutMessage := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		err := UpdateOrgRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateOrgRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		err := UpdateOrgRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateOrgRole("user@1.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = UpdateOrgRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateOrgRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
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
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()

		err = UpdateOrgRole("", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestListOrgUser(t *testing.T) {
	t.Run("happy path TestListOrgUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when ListUsersWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgUsers(out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListUsersWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		assert.EqualError(t, err, "failed to list users")
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = ListOrgUsers(out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path TestListWorkspaceUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.NoError(t, err)
	})

	t.Run("error path when ListUsersWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListUsersWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.EqualError(t, err, "failed to list users")
	})

	t.Run("error path when no Workspaceanization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = ListWorkspaceUsers(out, mockClient, "")
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := ListWorkspaceUsers(out, mockClient, "")
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestUpdateWorkspaceUserRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path UpdateWorkspaceUserRole", func(t *testing.T) {
		expectedOutMessage := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "test-role", workspaceID, out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspaceUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
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
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = UpdateWorkspaceUserRole("", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestAddWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path AddWorkspaceUser", func(t *testing.T) {
		expectedOutMessage := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "test-role", workspaceID, out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
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
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = AddWorkspaceUser("", "WORKSPACE_MEMBER", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestDeleteWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path DeleteWorkspaceUser", func(t *testing.T) {
		expectedOutMessage := "The user user@1.com was successfully removed from the workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", workspaceID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseError, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", workspaceID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err = RemoveWorkspaceUser("user@1.com", workspaceID, out, mockClient)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceUser("user@1.com", workspaceID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("DeleteWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
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
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserWithResponseOK, nil).Once()

		err = RemoveWorkspaceUser("", workspaceID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestGetUser(t *testing.T) {
	t.Run("happy path GetUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		_, err := GetUser(mockClient, user1.Id)
		assert.NoError(t, err)
	})

	t.Run("error path when GetUserWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseError, nil).Twice()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "failed to get user")
	})

	t.Run("error path when GetUserWithResponse returns a network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when no organization id found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_id", "")
		assert.NoError(t, err)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		_, err = GetUser(mockClient, user1.Id)
		assert.ErrorIs(t, err, ErrNoOrganizationID)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
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
