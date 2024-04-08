package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func execWorkspaceCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestWorkspaceRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "workspace")
}

var (
	workspaceTestDescription = "test workspace"
	workspace1               = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceTestDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
		OrganizationId:               "test-org-id",
	}

	workspaces = []astrocore.Workspace{
		workspace1,
	}
	ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
)

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroCoreClient = mockClient

	cmdArgs := []string{"list"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "workspace-id")
	assert.Contains(t, resp, "test-workspace")
	mockClient.AssertExpectations(t)
}

func TestWorkspaceSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Twice()
	astroCoreClient = mockClient

	// mock os.Stdin
	input := []byte("1")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	cmdArgs := []string{"switch"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "workspace-id")
	assert.Contains(t, resp, "test-workspace")
	mockClient.AssertExpectations(t)
}

func TestWorkspaceUserRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"user", "-h"}
	_, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
}

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

	workspaceTeams = []astrocore.Team{
		team1,
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
	ListWorkspaceTeamsResponseOK = astrocore.ListWorkspaceTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      workspaceTeams,
		},
	}
	ListWorkspaceTeamsResponseError = astrocore.ListWorkspaceTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyList,
		JSON200: nil,
	}
	MutateWorkspaceTeamRoleResponseOK = astrocore.MutateWorkspaceTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamRole{
			Role: "WORKSPACE_MEMBER",
		},
	}
	MutateWorkspaceTeamRoleResponseError = astrocore.MutateWorkspaceTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyUpdate,
		JSON200: nil,
	}
	DeleteWorkspaceTeamResponseOK = astrocore.DeleteWorkspaceTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceTeamResponseError = astrocore.DeleteWorkspaceTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: teamRequestErrorDelete,
	}
)

func TestWorkspaceUserList(t *testing.T) {
	expectedHelp := "List all the users in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"user", "list", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and users are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list users")
	})
	t.Run("any context errors from api are returned and users are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestWorkspaceUserUpdate(t *testing.T) {
	expectedHelp := "astro workspace user update [email] --role [WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"user", "update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email with valid role updates user", func(t *testing.T) {
		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and role is not update", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
	})
	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

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

		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "update", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspaceUserAdd(t *testing.T) {
	expectedHelp := "astro workspace user add [email] --role [WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"user", "add", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email with valid role adds user", func(t *testing.T) {
		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid email with invalid role returns an error and user is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
	})
	t.Run("any errors from api are returned and user is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("any context errors from api are returned and role is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
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

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "add", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspaceUserRemove(t *testing.T) {
	expectedHelp := "Remove a user from an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints remove help", func(t *testing.T) {
		cmdArgs := []string{"user", "remove", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid email removes user", func(t *testing.T) {
		expectedOut := "The user user@1.com was successfully removed from the workspace"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and user is not removed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("any context errors from api are returned and the user is not removed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

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

		expectedOut := "The user user@1.com was successfully removed from the workspace"
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()

		cmdArgs := []string{"user", "remove"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

var (
	CreateWorkspaceResponseOK = astrocore.CreateWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Workspace{
			Name: "workspace-test",
		},
	}

	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create workspace",
	})

	CreateWorkspaceResponseError = astrocore.CreateWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
)

func TestWorkspaceCreate(t *testing.T) {
	expectedHelp := "workspace create [flags]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"create", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid name with valid enforce", func(t *testing.T) {
		expectedOut := "Astro Workspace workspace-test was successfully created\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test", "--enforce-cicd", "ON"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid name with invalid enforce returns an error and workspace is not created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test", "--enforce-cicd", "on"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, workspace.ErrWrongEnforceInput)
	})
	t.Run("any errors from api are returned and workspace is not created", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create workspace")
	})

	t.Run("any context errors from api are returned and workspace is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

var (
	DeleteWorkspaceResponseOK = astrocore.DeleteWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	errorBodyDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete workspace",
	})

	DeleteWorkspaceResponseError = astrocore.DeleteWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyDelete,
	}
)

func TestWorkspaceDelete(t *testing.T) {
	expectedHelp := "workspace delete [workspace_id] [flags]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"delete", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id and successful delete", func(t *testing.T) {
		expectedOut := "Astro Workspace test-workspace was successfully deleted\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("invalid id returns workspace not found", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, workspace.ErrWorkspaceNotFound)
	})
	t.Run("any errors from api are returned and workspace is not deleted", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to delete workspace")
	})

	t.Run("any context errors from api are returned and workspace is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("command asks for input when no workspace id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
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

		expectedOut := "Astro Workspace test-workspace was successfully deleted\n"
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()

		cmdArgs := []string{"delete"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

var (
	UpdateWorkspaceResponseOK = astrocore.UpdateWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Workspace{
			Name: "workspace-test",
		},
	}

	errorBodyWorkspaceUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update workspace",
	})

	UpdateWorkspaceResponseError = astrocore.UpdateWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyWorkspaceUpdate,
		JSON200: nil,
	}
)

func TestWorkspaceUpdate(t *testing.T) {
	expectedHelp := "workspace update [workspace_id] [flags]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id and successful update", func(t *testing.T) {
		expectedOut := "Astro Workspace test-workspace was successfully updated\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("invalid id returns workspace not found", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, workspace.ErrWorkspaceNotFound)
	})
	t.Run("any errors from api are returned and workspace is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update workspace")
	})

	t.Run("any context errors from api are returned and workspace is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("command asks for input when no workspace id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
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

		expectedOut := "Astro Workspace test-workspace was successfully updated\n"
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()

		cmdArgs := []string{"update"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspaceTeamList(t *testing.T) {
	expectedHelp := "List all the teams in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"team", "list", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and teams are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list teams")
	})
	t.Run("any context errors from api are returned and teams are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestWorkspaceTeamUpdate(t *testing.T) {
	expectedHelp := "astro workspace team update [id] --role [WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid role updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("The workspace team %s role was successfully updated to WORKSPACE_MEMBER", team1.Id)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("valid id with invalid role returns an error and role is not update", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
	})
	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("The workspace team %s role was successfully updated to WORKSPACE_MEMBER", team1.Id)
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "update", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspaceTeamAdd(t *testing.T) {
	expectedHelp := "astro workspace team add [id] --role [WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"team", "add", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id with valid role adds team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("The team %s was successfully added to the workspace with the role WORKSPACE_MEMBER\n", team1.Id)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("can add team with workspace-id flag", func(t *testing.T) {
		workspaceIDFromFlag := "mock-workspace-id"
		expectedOut := fmt.Sprintf("The team %s was successfully added to the workspace with the role WORKSPACE_MEMBER\n", team1.Id)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, workspaceIDFromFlag, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "WORKSPACE_MEMBER", "--workspace-id", workspaceIDFromFlag}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("valid email with invalid role returns an error and team is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
	})
	t.Run("any errors from api are returned and team is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
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

		expectedOut := fmt.Sprintf("The team %s was successfully added to the workspace with the role WORKSPACE_MEMBER\n", team1.Id)
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "add", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspaceTeamRemove(t *testing.T) {
	expectedHelp := "Remove a team from an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints remove help", func(t *testing.T) {
		cmdArgs := []string{"team", "remove", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("valid id removes team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully removed from workspace ck05r3bor07h40d02y2hw4n4v\n", team1.Name)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and team is not removed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to delete team")
	})
	t.Run("any context errors from api are returned and the team is not removed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
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

		expectedOut := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s", team1.Name, "ck05r3bor07h40d02y2hw4n4v")
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "remove"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

// test workspace token commands

var (
	mockWorkspaceID    = "ck05r3bor07h40d02y2hw4n4v"
	mockOrganizationID = "orgID"
	description1       = "Description 1"
	description2       = "Description 2"
	fullName1          = "User 1"
	fullName2          = "User 2"
	token              = "token"
	tokenName1         = "Token 1"
	apiToken1          = astrocore.ApiToken{Id: "token1", Name: tokenName1, Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityId: mockWorkspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiTokens          = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{{EntityId: mockWorkspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	iamApiToken            = astroiamcore.ApiToken{Id: "token1", Name: tokenName1, Token: &token, Description: description1, Type: "Type 1", Roles: &[]astroiamcore.ApiTokenRole{{EntityId: mockWorkspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_AUTHOR"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
	GetAPITokensResponseOK = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamApiToken,
	}
	errorTokenGet, _ = json.Marshal(astroiamcore.Error{
		Message: "failed to get token",
	})
	GetAPITokensResponseError = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenGet,
		JSON200: nil,
	}
	ListWorkspaceAPITokensResponseOK = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens,
			Limit:     1,
			Offset:    0,
		},
	}
	errorTokenList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListWorkspaceAPITokensResponseError = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenList,
		JSON200: nil,
	}
	CreateWorkspaceAPITokenResponseOK = astrocore.CreateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	CreateWorkspaceAPITokenResponseError = astrocore.CreateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateWorkspaceAPITokenResponseOK = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	errorTokenUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update token",
	})
	UpdateWorkspaceAPITokenResponseError = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenUpdate,
		JSON200: nil,
	}
	RotateWorkspaceAPITokenResponseOK = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	errorTokenRotate, _ = json.Marshal(astrocore.Error{
		Message: "failed to rotate token",
	})
	RotateWorkspaceAPITokenResponseError = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenRotate,
		JSON200: nil,
	}
	DeleteWorkspaceAPITokenResponseOK = astrocore.DeleteWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	errorTokenDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete token",
	})
	DeleteWorkspaceAPITokenResponseError = astrocore.DeleteWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorTokenDelete,
	}
)

func TestWorkspaceTokenRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"token", "-h"}
	_, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestWorkspaceTokenList(t *testing.T) {
	expectedHelp := "List all the API tokens in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "list", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to list tokens")
	})
	t.Run("any context errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("tokens are listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestWorkspaceTokenCreate(t *testing.T) {
	expectedHelp := "Create an API token in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "create", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to create workspace")
	})
	t.Run("any context errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)
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
		cmdArgs := []string{"token", "create", "--role", "WORKSPACE_MEMBER"}
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no role provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestWorkspaceTokenUpdate(t *testing.T) {
	expectedHelp := "Update a Workspace or Organaization API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	tokenID = ""

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil)
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to update token")
	})
	t.Run("any context errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestWorkspaceTokenRotate(t *testing.T) {
	expectedHelp := "Rotate a Workspace API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "rotate", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to rotate token")
	})
	t.Run("any context errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is rotated with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is rotated with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestWorkspaceTokenDelete(t *testing.T) {
	expectedHelp := "Delete a Workspace API token or remove an Organization API token from a Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "delete", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to delete token")
	})
	t.Run("any context errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is deleted with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is delete with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
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
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

var (
	tokenName2 = "Token 2"
	apiToken2  = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityId: mockWorkspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityId: mockOrganizationID, EntityType: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiTokens2 = []astrocore.ApiToken{
		apiToken2,
		{Id: "token2", Name: tokenName2, Description: description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{{EntityId: mockOrganizationID, EntityType: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	ListOrganizationAPITokensResponseOK = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens2,
			Limit:     1,
			Offset:    0,
		},
	}
	errorOrgTokenList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListOrganizationAPITokensResponseError = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorOrgTokenList,
		JSON200: nil,
	}
	UpdateOrganizationAPITokenResponseOK = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	errorOrgTokenUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update tokens",
	})
	UpdateOrganizationAPITokenResponseError = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorOrgTokenUpdate,
		JSON200: nil,
	}
)

func TestWorkspaceTokenAdd(t *testing.T) {
	expectedHelp := "Add an Organization API token to an Astro Workspace"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "add", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and token is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "add", "--org-token-name", tokenName1, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to list tokens")
	})
	t.Run("any context errors from api are returned and token is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "add", "--org-token-name", tokenName1, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		astroCoreIamClient = mockIamClient

		cmdArgs := []string{"token", "add", "--org-token-name", tokenName2, "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("2")
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
		cmdArgs := []string{"token", "add", "--role", "WORKSPACE_MEMBER"}
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is added with no role provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		astroCoreIamClient = mockIamClient

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
		cmdArgs := []string{"token", "add", "--org-token-name", tokenName2}
		_, err = execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}
