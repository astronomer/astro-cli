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
	"github.com/astronomer/astro-cli/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// org user variables
var (
	errorNetwork = errors.New("network error")
	errorInvite  = errors.New("test-inv-error")
	orgRole      = "ORGANIZATION_MEMBER"
	user1        = astrocore.User{
		CreatedAt: time.Now(),
		FullName:  "user 1",
		Id:        "user1-id",
		OrgRole:   &orgRole,
		Username:  "user@1.com",
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
)

// workspace users variables
var (
	workspaceRole  = "WORKSPACE_MEMBER"
	workspaceUser1 = astrocore.User{
		CreatedAt:     time.Now(),
		FullName:      "user 1",
		Id:            "user1-id",
		WorkspaceRole: &workspaceRole,
		Username:      "user@1.com",
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
		JSON200: &workspaceUser1,
	}
	DeleteWorkspaceUserResponseError = astrocore.DeleteWorkspaceUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
)

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
}

type Suite struct {
	suite.Suite
}

func TestCloudUserSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreateInvite() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
	s.Run("happy path", func() {
		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when CreateUserInviteWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(nil, errorNetwork).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when CreateUserInviteWithResponse returns an error", func() {
		expectedOutMessage := "failed to create invite: test-inv-error"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		createInviteRequest := astrocore.CreateUserInviteRequest{
			InviteeEmail: "test-email@test.com",
			Role:         "ORGANIZATION_MEMBER",
		}
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseError, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, expectedOutMessage)
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
		s.ErrorIs(err, ErrInvalidRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err = CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
	s.Run("error path when email is blank returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("", "test-role", out, mockClient)
		s.ErrorIs(err, ErrInvalidEmail)
		s.Equal(expectedOutMessage, out.String())
	})
	s.Run("error path when writing output returns an error", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", testWriter{Error: errorInvite}, mockClient)
		s.EqualError(err, "failed to create invite: test-inv-error")
	})
}

func (s *Suite) TestIsRoleValid() {
	var err error
	s.Run("happy path when role is ORGANIZATION_MEMBER", func() {
		err = IsRoleValid("ORGANIZATION_MEMBER")
		s.NoError(err)
	})
	s.Run("happy path when role is ORGANIZATION_BILLING_ADMIN", func() {
		err = IsRoleValid("ORGANIZATION_BILLING_ADMIN")
		s.NoError(err)
	})
	s.Run("happy path when role is ORGANIZATION_OWNER", func() {
		err = IsRoleValid("ORGANIZATION_OWNER")
		s.NoError(err)
	})
	s.Run("error path", func() {
		err = IsRoleValid("test")
		s.ErrorIs(err, ErrInvalidRole)
	})
}

func (s *Suite) TestUpdateUserRole() {
	s.Run("happy path UpdateUserRole", func() {
		expectedOutMessage := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when MutateOrgUserRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateOrgUserRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "failed to update user")
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "test-role", out, mockClient)
		s.ErrorIs(err, ErrInvalidRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateUserRole("user@1.com", "ORGANIZATION_MEMBER", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("UpdateUserRole no email passed", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER\n"
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()

		err = UpdateUserRole("", "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestListOrgUser() {
	s.Run("happy path TestListOrgUser", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when ListOrgUsersWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgUsers(out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListOrgUsersWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Twice()
		err := ListOrgUsers(out, mockClient)
		s.EqualError(err, "failed to list users")
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = ListOrgUsers(out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListOrgUsers(out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestIsWorkspaceRoleValid() {
	var err error
	s.Run("happy path when role is WORKSPACE_MEMBER", func() {
		err = IsWorkspaceRoleValid("WORKSPACE_MEMBER")
		s.NoError(err)
	})
	s.Run("happy path when role is WORKSPACE_OPERATOR", func() {
		err = IsWorkspaceRoleValid("WORKSPACE_OPERATOR")
		s.NoError(err)
	})
	s.Run("happy path when role is WORKSPACE_OWNER", func() {
		err = IsWorkspaceRoleValid("WORKSPACE_OWNER")
		s.NoError(err)
	})
	s.Run("error path", func() {
		err = IsRoleValid("test")
		s.ErrorIs(err, ErrInvalidRole)
	})
}

func (s *Suite) TestListWorkspaceUser() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("happy path TestListWorkspaceUser", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		s.NoError(err)
	})

	s.Run("error path when ListWorkspaceUsersWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceUsers(out, mockClient, "")
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListWorkspaceUsersWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Twice()
		err := ListWorkspaceUsers(out, mockClient, "")
		s.EqualError(err, "failed to list users")
	})

	s.Run("error path when no Workspaceanization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = ListWorkspaceUsers(out, mockClient, "")
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListWorkspaceUsers(out, mockClient, "")
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestUpdateWorkspaceUserRole() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("happy path UpdateWorkspaceUserRole", func() {
		expectedOutMessage := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when MutateWorkspaceUserRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceUserRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "failed to update user")
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "test-role", "", out, mockClient)
		s.ErrorIs(err, ErrInvalidWorkspaceRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceUserRole("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("UpdateWorkspaceUserRole no email passed", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = UpdateWorkspaceUserRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestAddWorkspaceUser() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("happy path AddWorkspaceUser", func() {
		expectedOutMessage := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when MutateWorkspaceUserRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceUserRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "failed to update user")
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "test-role", "", out, mockClient)
		s.ErrorIs(err, ErrInvalidWorkspaceRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceUser("user@1.com", "WORKSPACE_MEMBER", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("AddWorkspaceUser no email passed", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		err = AddWorkspaceUser("", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestDeleteWorkspaceUser() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("happy path DeleteWorkspaceUser", func() {
		expectedOutMessage := "The user user@1.com was successfully removed from the workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when DeleteWorkspaceUserWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteWorkspaceUserWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseError, nil).Once()
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		s.EqualError(err, "failed to update user")
	})

	s.Run("error path when no organization shortname found", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("organization_short_name", "")
		s.NoError(err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		s.ErrorIs(err, ErrNoShortName)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceUser("user@1.com", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("DeleteWorkspaceUser no email passed", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com was successfully removed from the workspace\n"
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()

		err = RemoveWorkspaceUser("", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}
