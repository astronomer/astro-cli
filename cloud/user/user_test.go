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

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
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

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
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
		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
		createInviteRequest := astrov1.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateUserInviteWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrov1.CreateUserInviteRequest{
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
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrov1.CreateUserInviteRequest{
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
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when email is blank returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidEmail)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when writing output returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOutMessage := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = UpdateUserRole("", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestListOrgUsersData(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("returns structured user data", func(t *testing.T) {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()

		data, err := ListOrgUsersData(mockClient)
		assert.NoError(t, err)
		assert.NotEmpty(t, data.Users)
		assert.Equal(t, "user 1", data.Users[0].FullName)
		assert.Equal(t, "user@1.com", data.Users[0].Email)
		assert.Equal(t, "ORGANIZATION_MEMBER", data.Users[0].OrgRole)
	})

	t.Run("returns error on failure", func(t *testing.T) {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(nil, errorNetwork).Once()

		_, err := ListOrgUsersData(mockClient)
		assert.Error(t, err)
	})
}

func TestListOrgUsersWithFormat(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("json output", func(t *testing.T) {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()

		buf := new(bytes.Buffer)
		err := ListOrgUsersWithFormat(mockClient, "json", "", buf)
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
		expectedOutMessage := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspaceUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = UpdateWorkspaceUserRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestAddWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddWorkspaceUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOutMessage := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = AddWorkspaceUser("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestDeleteWorkspaceUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteWorkspaceUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOutMessage := "The user user@1.com was successfully removed from the workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.WorkspaceId != nil })).Return(&ListWorkspaceUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("DeleteWorkspaceUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The user user@1.com was successfully removed from the workspace\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetWorkspaceUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = RemoveWorkspaceUser("", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestGetUser(t *testing.T) {
	t.Run("happy path GetUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		_, err := GetUser(mockClient, user1.Id)
		assert.NoError(t, err)
	})

	t.Run("error path when GetUserWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseError, nil).Once()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "failed to get user")
	})

	t.Run("error path when GetUserWithResponse returns a network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()

		_, err := GetUser(mockClient, user1.Id)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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
		expectedOutMessage := "The deployment user user@1.com role was successfully updated to DEPLOYMENT_ADMIN\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateDeploymentUserRole("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateDeploymentUserRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The deployment user user@1.com role was successfully updated to DEPLOYMENT_ADMIN\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = UpdateDeploymentUserRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})

	t.Run("error path UpdateDeploymentUserRole user not found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		err := UpdateDeploymentUserRole("notfound@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "no user was found for the email you provided")
	})
}

func TestAddDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddDeploymentUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOutMessage := "The user user@1.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, listUsersUnscoped()).Return(&ListOrgUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := AddDeploymentUser("user@1.com", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddDeploymentUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The user user@1.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = AddDeploymentUser("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestDeleteDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteDeploymentUser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOutMessage := "The user user@1.com was successfully removed from the deployment\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()
		err := RemoveDeploymentUser("user@1.com", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateUserRolesWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveDeploymentUser("user@1.com", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateUserRolesWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool { return p != nil && p.DeploymentId != nil })).Return(&ListDeploymentUsersResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseError, nil).Once()
		err := RemoveDeploymentUser("user@1.com", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := RemoveDeploymentUser("user@1.com", deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("DeleteDeploymentUser no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
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

		expectedOut := "The user user@1.com was successfully removed from the deployment\n"
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentUserResponseOK, nil).Once()
		mockClient.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateUserRolesResponseOK, nil).Once()

		err = RemoveDeploymentUser("", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})

	// ListWorkspaceUsersResponseError and ListDeploymentUsersResponseError are declared for
	// parity with prior tests even though individual error paths are not exercised here.
	_ = ListWorkspaceUsersResponseError
	_ = ListDeploymentUsersResponseError
}
