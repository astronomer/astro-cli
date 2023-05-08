package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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

func (s *Suite) TestOrganizationRootCommand() {
	testUtil.SetupOSArgsForGinkgo()
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	s.NoError(err)
	s.Contains(buf.String(), "organization")
}

func (s *Suite) TestOrganizationUserRootCommand() {
	expectedHelp := "Manage users in your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	cmdArgs := []string{"user"}
	resp, err := execOrganizationCmd(cmdArgs...)
	s.NoError(err)
	s.Contains(resp, expectedHelp)
}

func (s *Suite) TestOrganizationList() {
	orgList = func(out io.Writer, coreClient astrocore.CoreClient) error {
		return nil
	}

	cmdArgs := []string{"list"}
	_, err := execOrganizationCmd(cmdArgs...)
	s.NoError(err)
}

func (s *Suite) TestOrganizationSwitch() {
	orgSwitch = func(orgName string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
		return nil
	}

	cmdArgs := []string{"switch"}
	_, err := execOrganizationCmd(cmdArgs...)
	s.NoError(err)
}

func (s *Suite) TestOrganizationExportAuditLogs() {
	// turn on audit logs
	config.CFG.AuditLogs.SetHomeString("true")
	orgExportAuditLogs = func(client astro.Client, out io.Writer, orgName string, earliest int) error {
		return nil
	}

	s.Run("Fails without organization name", func() {
		cmdArgs := []string{"audit-logs", "export"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.Contains(err.Error(), "required flag(s) \"organization-name\" not set")
	})

	s.Run("Without params", func() {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
	})

	s.Run("with auditLogsOutputFilePath param", func() {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer", "--output-file", "test.json"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
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

func (s *Suite) TestUserInvite() {
	expectedHelp := "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints invite help", func() {
		cmdArgs := []string{"user", "invite", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid email with no role creates an invite", func() {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("valid email with valid role creates an invite", func() {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("valid email with invalid role returns an error and no invite gets created", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "invalid"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.ErrorIs(err, user.ErrInvalidRole)
	})
	s.Run("any errors from api are returned and no invite gets created", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.EqualError(err, "failed to create invite: test-error")
	})

	s.Run("any context errors from api are returned and no invite gets created", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "invite", "some@email.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.Error(err)
	})
	s.Run("command asks for input when no email is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		// mock os.Stdin
		expectedInput := []byte("test-email-input")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
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
		s.NoError(err)
		s.Contains(resp, expectedOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("command returns an error when no email is provided", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		// mock os.Stdin
		expectedInput := []byte("")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		cmdArgs := []string{"user", "invite"}
		_, err = execOrganizationCmd(cmdArgs...)
		s.ErrorIs(err, user.ErrInvalidEmail)
	})
}

func (s *Suite) TestUserList() {
	expectedHelp := "List all the users in your Astro Organization"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints list help", func() {
		cmdArgs := []string{"user", "list", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and users are not listed", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.EqualError(err, "failed to list users")
	})
	s.Run("any context errors from api are returned and users are not listed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.Error(err)
	})
}

func (s *Suite) TestUserUpdate() {
	expectedHelp := "astro user update [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints update help", func() {
		cmdArgs := []string{"user", "update", "-h"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid email with valid role updates user", func() {
		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("valid email with invalid role returns an error and role is not update", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "invalid"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.ErrorIs(err, user.ErrInvalidRole)
	})
	s.Run("any errors from api are returned and role is not updated", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.EqualError(err, "failed to update user")
	})

	s.Run("any context errors from api are returned and role is not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "user@1.com", "--role", "ORGANIZATION_MEMBER"}
		_, err := execOrganizationCmd(cmdArgs...)
		s.Error(err)
	})
	s.Run("command asks for input when no email is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		expectedOut := "The user user@1.com role was successfully updated to ORGANIZATION_MEMBER"
		mockClient.On("MutateOrgUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "update", "--role", "ORGANIZATION_MEMBER"}
		resp, err := execOrganizationCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
}
