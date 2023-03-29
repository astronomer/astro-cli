package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "workspace")
}

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1", Label: "test-label-1"}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"list"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-label-1")
	mockClient.AssertExpectations(t)
}

func TestWorkspaceSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1", Label: "test-label-1"}}, nil).Twice()
	astroClient = mockClient

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
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-label-1")
	mockClient.AssertExpectations(t)
}

func TestWorkspaceUserRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newUserCmd(os.Stdout)
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

func TestWorkspaceUserList(t *testing.T) {
	expectedHelp := "List all the users in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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

func TestWorkspacUserUpdate(t *testing.T) {
	expectedHelp := "astro workspace user update [email] --role [WORKSPACE_MEMBER, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	expectedHelp := "astro workspace user add [email] --role [WORKSPACE_MEMBER, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "add", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestWorkspacUserRemove(t *testing.T) {
	expectedHelp := "Remove a user from an Astro Workspace"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	errorNetwork              = errors.New("network error")
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		JSON200: &astrocore.Workspace{
			Name: "workspace-test",
		},
	}

	errorBodyDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete workspace",
	})

	DeleteWorkspaceResponseError = astrocore.DeleteWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyDelete,
		JSON200: nil,
	}
	workspaceTestDescription = "test workspace"
	workspace1               = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceTestDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
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

func TestWorkspaceDelete(t *testing.T) {
	expectedHelp := "workspace delete [workspace_id] [flags]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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
