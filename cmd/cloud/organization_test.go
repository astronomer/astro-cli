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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "organization")
}

func TestOrganizationUserRootCommand(t *testing.T) {
	expectedHelp := "Manage users in your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	cmdArgs := []string{"user"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, expectedHelp)
}

func TestOrganizationTeamRootCommand(t *testing.T) {
	expectedHelp := "Manage teams in your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	cmdArgs := []string{"team"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, expectedHelp)
}

func TestOrganizationList(t *testing.T) {
	orgList = func(out io.Writer, coreClient astrocore.CoreClient) error {
		return nil
	}

	cmdArgs := []string{"list"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationSwitch(t *testing.T) {
	orgSwitch = func(orgName string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
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
	team1 = astrocore.Team{
		CreatedAt:   time.Now(),
		Name:        "team 1",
		Description: &description,
		Id:          "team1-id",
	}
	teams = []astrocore.Team{
		team1,
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
		Body:    teamRequestErrorDelete,
		JSON200: nil,
	}
	DeleteTeamResponseOK = astrocore.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}
)

func TestUserInvite(t *testing.T) {
	expectedHelp := "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		cmdArgs := []string{"user", "invite"}
		resp, err := execOrganizationCmd(cmdArgs...)
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

		cmdArgs := []string{"user", "invite"}
		_, err = execOrganizationCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidEmail)
	})
}

func TestUserList(t *testing.T) {
	expectedHelp := "List all the users in your Astro Organization"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		cmdArgs := []string{"user", "update", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestTeamList(t *testing.T) {
	expectedHelp := "List all the teams in your Astro Organization"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	expectedHelp := "organization team update [team_id]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "update", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid name and description updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		cmdArgs := []string{"team", "update", "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestTeamCreate(t *testing.T) {
	expectedHelp := "organization team create"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		cmdArgs := []string{"team", "create", team1.Id, "--name", team1.Name, "--description", *team1.Description}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "create", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create team")
	})

	t.Run("any context errors from api are returned and role is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", team1.Id, "--name", team1.Name}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestTeamDelete(t *testing.T) {
	expectedHelp := "organization team delete"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "delete", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid name and description updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "delete", team1.Id}
		resp, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not deleted", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "delete", team1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to delete team")
	})

	t.Run("any context errors from api are returned and role is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", team1.Id}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Error(t, err)
	})
}
