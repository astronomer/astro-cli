package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// org user variables
var (
	errorNetwork        = errors.New("network error")
	errorInvite         = errors.New("test-inv-error")
	orgRole             = astrov1.UserOrganizationRole("ORGANIZATION_MEMBER")
	workspaceRoleRole   = astrov1.WorkspaceRoleRole("WORKSPACE_MEMBER")
	workspaceIDForUsers = "test-workspace-id"
	user1               = astrov1.User{
		CreatedAt:        time.Now(),
		FullName:         "user 1",
		Id:               "user1-id",
		OrganizationRole: &orgRole,
		Username:         "user@1.com",
	}
	users = []astrov1.User{
		user1,
	}
	ListOrgUsersResponseOK = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      users,
		},
	}
	errorBodyList, _ = json.Marshal(astrov1.Error{
		Message: "failed to list users",
	})
	ListOrgUsersResponseError = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	UpdateUserRolesResponseOK = astrov1.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.SubjectRoles{},
	}
	errorBodyUpdate, _ = json.Marshal(astrov1.Error{
		Message: "failed to update user",
	})
	UpdateUserRolesResponseError = astrov1.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	GetUserWithResponseOK = astrov1.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	errorBodyGet, _ = json.Marshal(astrov1.Error{
		Message: "failed to get user",
	})
	GetUserWithResponseError = astrov1.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyGet,
		JSON200: nil,
	}
)

// workspace users variables
var (
	workspaceUser1 = astrov1.User{
		CreatedAt:        time.Now(),
		FullName:         "user 1",
		Id:               "user1-id",
		OrganizationRole: &orgRole,
		WorkspaceRoles: &[]astrov1.WorkspaceRole{
			{WorkspaceId: workspaceIDForUsers, Role: workspaceRoleRole},
		},
		Username: "user@1.com",
	}
	workspaceUsers = []astrov1.User{
		workspaceUser1,
	}
	ListWorkspaceUsersResponseOK = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      workspaceUsers,
		},
	}
	ListWorkspaceUsersResponseError = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	GetWorkspaceUserResponseOK = astrov1.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &workspaceUser1,
	}
)

// deployment users variables
var (
	deploymentID    = "ck05r3bor07h40d02y2hw4n4d"
	deploymentRole  = "DEPLOYMENT_ADMIN"
	deploymentUser1 = astrov1.User{
		CreatedAt:        time.Now(),
		FullName:         "user 1",
		Id:               "user1-id",
		OrganizationRole: &orgRole,
		DeploymentRoles: &[]astrov1.DeploymentRole{
			{DeploymentId: deploymentID, Role: deploymentRole},
		},
		Username: "user@1.com",
	}
	deploymentUsers = []astrov1.User{
		deploymentUser1,
	}
	ListDeploymentUsersResponseOK = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      deploymentUsers,
		},
	}
	ListDeploymentUsersResponseError = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	GetDeploymentUserResponseOK = astrov1.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentUser1,
	}
)

// listUsersUnscoped matches a ListUsersParams with no workspace or deployment scope filter.
func listUsersUnscoped() interface{} {
	return mock.MatchedBy(func(p *astrov1.ListUsersParams) bool {
		return p != nil && p.WorkspaceId == nil && p.DeploymentId == nil
	})
}

func TestCreateInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	inviteUserID := "user_cuid"
	createInviteResponseOK := astrov1.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Invite{
			InviteId: "",
			UserId:   &inviteUserID,
		},
	}
	errorBody, _ := json.Marshal(astrov1.Error{
		Message: "failed to create invite: test-inv-error",
	})
	createInviteResponseError := astrov1.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	t.Run("happy path", func(t *testing.T) {
		createInviteRequest := astrov1.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
		got, err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "test-email@test.com", got.Email)
		assert.Equal(t, "ORGANIZATION_MEMBER", got.Role)
		assert.Equal(t, "user_cuid", got.UserID)
		assert.NotEmpty(t, got.OrganizationID, "the current Organization, which the API did not echo")
		assert.Nil(t, got.ExpiresAt, "the API sent no expiry")
	})

	t.Run("error path when CreateUserInviteWithResponse return network error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		createInviteRequest := astrov1.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(nil, errorNetwork).Once()
		_, err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateUserInviteWithResponse returns an error", func(t *testing.T) {
		expectedOutMessage := "failed to create invite: test-inv-error"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		createInviteRequest := astrov1.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseError, nil).Once()
		_, err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", mockClient)
		assert.EqualError(t, err, expectedOutMessage)
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := CreateInvite("test-email@test.com", "test-role", mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", mockClient)
		assert.Error(t, err)
	})
	t.Run("error path when email is blank returns an error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := CreateInvite("", "test-role", mockClient)
		assert.ErrorIs(t, err, ErrInvalidEmail)
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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "ORGANIZATION_MEMBER", got.OrgRole)
		assert.Empty(t, got.WorkspaceRole)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := UpdateUserRole("user@1.com", "test-role", mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", mockClient)
		assert.Error(t, err)
	})

	t.Run("UpdateUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
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

		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := UpdateUserRole("", "ORGANIZATION_MEMBER", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "ORGANIZATION_MEMBER", got.OrgRole)
		assert.Empty(t, got.WorkspaceRole)
	})
}

func TestListOrgUsersData(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("returns structured user data", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()

		data, err := ListOrgUsersData(mockClient)
		assert.NoError(t, err)
		assert.NotEmpty(t, data.Users)
		assert.Equal(t, "user 1", data.Users[0].FullName)
		assert.Equal(t, "user@1.com", data.Users[0].Email)
		assert.Equal(t, "ORGANIZATION_MEMBER", data.Users[0].OrgRole)
	})

	t.Run("returns error on failure", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(nil, errorNetwork).Once()

		_, err := ListOrgUsersData(mockClient)
		assert.Error(t, err)
	})
}

func TestListOrgUsersWithFormat(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("json output", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()

		buf := new(bytes.Buffer)
		err := ListOrgUsersWithFormat(mockClient, testUtil.Renderer{JSON: true, Out: buf})
		assert.NoError(t, err)

		var result UserList
		assert.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		assert.NotEmpty(t, result.Users)
		assert.Equal(t, "user 1", result.Users[0].FullName)
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

func TestUpdateWorkspaceUserRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateWorkspaceUserRole", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "WORKSPACE_MEMBER", got.WorkspaceRole)
		assert.Empty(t, got.OrgRole)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := UpdateWorkspaceUserRole("user@1.com", "test-role", "", mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.Error(t, err)
	})

	t.Run("UpdateWorkspaceUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := UpdateWorkspaceUserRole("", "WORKSPACE_MEMBER", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "WORKSPACE_MEMBER", got.WorkspaceRole)
		assert.Empty(t, got.OrgRole)
	})
}

func TestAddWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddWorkspaceUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "WORKSPACE_MEMBER", got.WorkspaceRole)
		assert.Empty(t, got.OrgRole)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := AddWorkspaceUser("user@1.com", "test-role", "", mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", mockClient)
		assert.Error(t, err)
	})

	t.Run("AddWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := AddWorkspaceUser("", "WORKSPACE_MEMBER", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "WORKSPACE_MEMBER", got.WorkspaceRole)
		assert.Empty(t, got.OrgRole)
	})
}

func TestDeleteWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteWorkspaceUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := RemoveWorkspaceUser("user@1.com", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.NotEmpty(t, got.WorkspaceID, "the current Workspace")
		assert.Equal(t, Removed, got.Action)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveWorkspaceUser("user@1.com", "", mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := RemoveWorkspaceUser("user@1.com", "", mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := RemoveWorkspaceUser("user@1.com", "", mockClient)
		assert.Error(t, err)
	})

	t.Run("DeleteWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := RemoveWorkspaceUser("", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user1-id", got.ID)
		assert.Equal(t, "user@1.com", got.Email)
		assert.NotEmpty(t, got.WorkspaceID, "the current Workspace")
		assert.Equal(t, Removed, got.Action)
	})
}

func TestGetUser(t *testing.T) {
	t.Run("happy path GetUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		_, err := GetUser(mockClient, user1.Id)
		assert.NoError(t, err)
	})

	t.Run("error path when GetUserWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseError, nil).Once()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "failed to get user")
	})

	t.Run("error path when GetUserWithResponse returns a network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
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

func TestUpdateDeploymentUserRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateDeploymentUserRole", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "DEPLOYMENT_ADMIN", got.DeploymentRole)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.Error(t, err)
	})

	t.Run("UpdateDeploymentUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := UpdateDeploymentUserRole("", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "DEPLOYMENT_ADMIN", got.DeploymentRole)
	})

	t.Run("error path UpdateDeploymentUserRole user not found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		_, err := UpdateDeploymentUserRole("notfound@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.EqualError(t, err, "no user was found for the email you provided")
	})
}

func TestAddDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddDeploymentUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "DEPLOYMENT_ADMIN", got.DeploymentRole)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.Error(t, err)
	})

	t.Run("AddDeploymentUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := AddDeploymentUser("", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, "DEPLOYMENT_ADMIN", got.DeploymentRole)
	})
}

func TestDeleteDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteDeploymentUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		got, err := RemoveDeploymentUser("user@1.com", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, deploymentID, got.DeploymentID)
		assert.Equal(t, Removed, got.Action)
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveDeploymentUser("user@1.com", deploymentID, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		_, err := RemoveDeploymentUser("user@1.com", deploymentID, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		_, err := RemoveDeploymentUser("user@1.com", deploymentID, mockClient)
		assert.Error(t, err)
	})

	t.Run("DeleteDeploymentUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(t)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
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

		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		got, err := RemoveDeploymentUser("", deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "user@1.com", got.Email)
		assert.Equal(t, deploymentID, got.DeploymentID)
		assert.Equal(t, Removed, got.Action)
	})

	// ListWorkspaceUsersResponseError and ListDeploymentUsersResponseError are declared for
	// parity with prior tests even though individual error paths are not exercised here.
	_ = ListWorkspaceUsersResponseError
	_ = ListDeploymentUsersResponseError
}
