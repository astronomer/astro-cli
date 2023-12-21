package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

//nolint:unparam
func execOrganizationCmd(args ...string) (string, error) {
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestOrganizationRootCommand(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "organization")
}

func TestOrganizationUserRootCommand(t *testing.T) {
	expectedHelp := "Manage users in your Astro Organization."
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmdArgs := []string{"user"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, expectedHelp)
}

func TestOrganizationTeamRootCommand(t *testing.T) {
	expectedHelp := "Manage teams in your Astro Organization."
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmdArgs := []string{"team"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, expectedHelp)
}

func TestOrganizationList(t *testing.T) {
	orgList = func(out io.Writer, platformCoreClient astroplatformcore.CoreClient) error {
		return nil
	}

	cmdArgs := []string{"list"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationSwitch(t *testing.T) {
	orgSwitch = func(orgName string, client astro.Client, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
		return nil
	}

	cmdArgs := []string{"switch"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationExportAuditLogs(t *testing.T) {
	// turn on audit logs
	config.CFG.AuditLogs.SetHomeString("true")
	orgExportAuditLogs = func(client astro.Client, out io.Writer, orgName string, earliest int) error {
		return nil
	}

	t.Run("Fails without organization name", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Contains(t, err.Error(), "required flag(s) \"organization-name\" not set")
	})

	t.Run("Without params", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("with auditLogsOutputFilePath param", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer", "--output-file", "test.json"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	// Delete audit logs exports
	currentDir, _ := os.Getwd()
	files, _ := os.ReadDir(currentDir)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "audit-logs-") {
			os.Remove(file.Name())
		}
	}
	os.Remove("test.json")
}

// test organization user commands

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
	user1   = astrocore.User{
		CreatedAt: time.Now(),
		FullName:  "user 1",
		Id:        "user1-id",
		OrgRole:   &orgRole,
		Username:  "user@1.com",
	}
	users = []astrocore.User{
		user1,
	}
	teamMembers = []astrocore.TeamMember{{
		UserId:   user1.Id,
		Username: user1.Username,
		FullName: &user1.FullName,
	}}
	team1 = astrocore.Team{
		CreatedAt:   time.Now(),
		Name:        "team 1",
		Description: &description,
		Id:          "team1-id",
		Members:     &teamMembers,
	}
	teams = []astrocore.Team{
		team1,
	}
	GetUserWithResponseOK = astrocore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	GetTeamWithResponseOK = astrocore.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
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
	ListOrgTeamsResponseOK = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      teams,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list users",
	})
	teamRequestErrorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list teams",
	})
	ListOrgUsersResponseError = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	ListOrgTeamsResponseError = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyList,
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
	UpdateTeamResponseOK = astrocore.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update user",
	})
	teamRequestErrorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update team",
	})
	MutateOrgUserRoleResponseError = astrocore.MutateOrgUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	MutateOrgTeamRoleResponseOK = astrocore.MutateOrgTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	MutateOrgTeamRoleResponseError = astrocore.MutateOrgTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: teamRequestErrorBodyUpdate,
	}
	UpdateTeamResponseError = astrocore.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyUpdate,
		JSON200: nil,
	}
	teamRequestErrorCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create team",
	})
	CreateTeamResponseError = astrocore.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorCreate,
		JSON200: nil,
	}
	CreateTeamResponseOK = astrocore.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}
	teamRequestErrorDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete team",
	})
	DeleteTeamResponseError = astrocore.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: teamRequestErrorDelete,
	}
	DeleteTeamResponseOK = astrocore.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	errorBodyUpdateTeamMembership, _ = json.Marshal(astrocore.Error{
		Message: "failed to update team membership",
	})
	AddTeamMemberResponseOK = astrocore.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	AddTeamMemberResponseError = astrocore.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
	RemoveTeamMemberResponseOK = astrocore.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	RemoveTeamMemberResponseError = astrocore.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
	getTokenErrorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to get token",
	})
	GetOrganizationAPITokenResponseError = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    getTokenErrorBodyList,
		JSON200: nil,
	}
	GetOrganizationAPITokenResponseOK = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
)

func TestUserInvite(t *testing.T) {
	expectedHelp := "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints invite help", func(t *testing.T) {
		cmdArgs := []string{"user", "invite", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email with no role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("valid email with valid role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and no invite gets created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "invalid"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidRole)
	})
	t.Run("any errors from api are returned and no invite gets created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create invite: test-error")
	})

	t.Run("any context errors from api are returned and no invite gets created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		cmdArgs := []string{"user", "invite"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("command returns an error when no email is provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		cmdArgs := []string{"user", "invite"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidEmail)
	})
}

func TestUserList(t *testing.T) {
	expectedHelp := "List all the users in your Astro Organization"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"user", "list", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and users are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list users")
	})
	t.Run("any context errors from api are returned and users are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestUserUpdate(t *testing.T) {
	expectedHelp := "astro user update [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"user", "update", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email with valid role updates user", func(t *testing.T) {
		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and role is not update", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "invalid"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidRole)
	})
	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

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

		cmdArgs := []string{"user", "update", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestTeamList(t *testing.T) {
	expectedHelp := "List all the teams in your Astro Organization"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"team", "list", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and teams are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list teams")
	})
	t.Run("any context errors from api are returned and teams are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestTeamUpdate(t *testing.T) {
	expectedHelp := "organization team update [team-id]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "update", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("valid id with valid name and description updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "update", "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid id with role should update role", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseOK, nil).Once()

		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "ORGANIZATION_OWNER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("will error on invalid role", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseOK, nil).Once()

		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "WORKSPACE_OPERATOR"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "requested role is invalid")
	})
}

func TestTeamCreate(t *testing.T) {
	expectedHelp := "organization team create"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "create", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid name and description updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully created\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "create", team1.Id, "--role", "ORGANIZATION_MEMBER", "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid id with valid name and description updates team no role passed in", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully created\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "create", team1.Id, "--name", team1.Name, "--description", *team1.Description}

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

		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("error no role passed in index out of range", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "create", team1.Id, "--name", team1.Name, "--description", *team1.Description}

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "invalid organization role selection")
	})

	t.Run("any errors from api are returned and role is not created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "create", team1.Id, "--role", "ORGANIZATION_MEMBER", "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create team")
	})

	t.Run("any context errors from api are returned and role is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", team1.Id, "--role", "ORGANIZATION_MEMBER", "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestTeamDelete(t *testing.T) {
	expectedHelp := "organization team delete"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "delete", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid name and description updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "delete", team1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not deleted", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "delete", team1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to delete team")
	})

	t.Run("any context errors from api are returned and role is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", team1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "delete"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestAddUser(t *testing.T) {
	expectedHelp := "organization team user add"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "user", "add", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid team-id with valid user-id updates team membership", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "add", "--team-id", team1.Id, "--user-id", user1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and membership is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "add", "--team-id", team1.Id, "--user-id", user1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team membership")
	})

	t.Run("any context errors from api are returned and membership is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "add", "--team-id", team1.Id, "--user-id", user1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("command asks for input when no team id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "user", "add", "--user-id", user1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("command asks for input when no user id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "user", "add", "--team-id", team1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestRemoveUser(t *testing.T) {
	expectedHelp := "organization team user remove"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "user", "remove", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid team-id with valid user-id updates team membership", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "remove", "--team-id", team1.Id, "--user-id", user1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and membership is not removed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "remove", "--team-id", team1.Id, "--user-id", user1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team membership")
	})

	t.Run("any context errors from api are returned and membership is not removed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "remove", "--team-id", team1.Id, "--user-id", user1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no team id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "user", "remove", "--user-id", user1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestTeamUserList(t *testing.T) {
	expectedHelp := "Lists users in an Astro Team\n"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"team", "user", "list", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and teams are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list teams")
	})
	t.Run("any context errors from api are returned and teams are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

// test organizatio  token commands

var (
	CreateOrganizationAPITokenResponseOK = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	CreateOrganizationAPITokenResponseError = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	RotateOrganizationAPITokenResponseOK = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateOrganizationAPITokenResponseError = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenRotate,
		JSON200: nil,
	}
	DeleteOrganizationAPITokenResponseOK = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteOrganizationAPITokenResponseError = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorTokenDelete,
	}
)

func TestOrganizationTokenRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"token", "-h"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationTokenList(t *testing.T) {
	expectedHelp := "List all the API tokens in an Astro Organization"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "list", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to list tokens")
	})
	t.Run("any context errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("tokens are listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestOrganizationTokenCreate(t *testing.T) {
	expectedHelp := "Create an API token in an Astro Organization"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "create", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to create workspace")
	})
	t.Run("any context errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("Token 1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--role", "ORGANIZATION_MEMBER"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no role provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)
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
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestOrganizationTokenUpdate(t *testing.T) {
	expectedHelp := "Update a Organization or Organaization API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "update", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to update token")
	})
	t.Run("any context errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestOrganizationTokenRotate(t *testing.T) {
	expectedHelp := "Rotate a Organization API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "rotate", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to rotate token")
	})
	t.Run("any context errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is rotated with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
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
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--force"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is rotated with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("y")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestOrganizationTokenDelete(t *testing.T) {
	expectedHelp := "Delete a Organization API token or remove an Organization API token from a Organization"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "delete", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to delete token")
	})
	t.Run("any context errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is deleted with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
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
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--force"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is delete with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("y")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestOrganizationTokenListRoles(t *testing.T) {
	expectedHelp := "List roles for an organization API token"
	mockTokenID := "mockTokenID"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "roles", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token roles are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseError, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "roles", mockTokenID}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to get token")
	})
	t.Run("any context errors from api are returned and token roles are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "roles", mockTokenID}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("token roles are listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "roles", mockTokenID}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}
