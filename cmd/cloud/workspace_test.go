package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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

func (s *Suite) TestWorkspaceRootCommand() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	s.NoError(err)
	s.Contains(buf.String(), "workspace")
}

func (s *Suite) TestWorkspaceList() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1", Label: "test-label-1"}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"list"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	s.NoError(err)
	s.Contains(resp, "test-id-1")
	s.Contains(resp, "test-label-1")
	mockClient.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceSwitch() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1", Label: "test-label-1"}}, nil).Twice()
	astroClient = mockClient

	// mock os.Stdin
	input := []byte("1")
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	cmdArgs := []string{"switch"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	s.NoError(err)
	s.Contains(resp, "test-id-1")
	s.Contains(resp, "test-label-1")
	mockClient.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceUserRootCommand() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"user", "-h"}
	_, err := execWorkspaceCmd(cmdArgs...)
	s.NoError(err)
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

func (s *Suite) TestWorkspaceUserList() {
	expectedHelp := "List all the users in an Astro Workspace"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints list help", func() {
		cmdArgs := []string{"user", "list", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and users are not listed", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to list users")
	})
	s.Run("any context errors from api are returned and users are not listed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
	})
}

func (s *Suite) TestWorkspacUserUpdate() {
	expectedHelp := "astro workspace user update [email] --role [WORKSPACE_MEMBER, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints update help", func() {
		cmdArgs := []string{"user", "update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid email with valid role updates user", func() {
		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("valid email with invalid role returns an error and role is not update", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
	})
	s.Run("any errors from api are returned and role is not updated", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to update user")
	})

	s.Run("any context errors from api are returned and role is not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
	})
	s.Run("command asks for input when no email is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		expectedOut := "The workspace user user@1.com role was successfully updated to WORKSPACE_MEMBER"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "update", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
}

func (s *Suite) TestWorkspaceUserAdd() {
	expectedHelp := "astro workspace user add [email] --role [WORKSPACE_MEMBER, WORKSPACE_OPERATOR, WORKSPACE_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints add help", func() {
		cmdArgs := []string{"user", "add", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid email with valid role adds user", func() {
		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("valid email with invalid role returns an error and user is not added", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "invalid"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
	})
	s.Run("any errors from api are returned and user is not added", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to update user")
	})

	s.Run("any context errors from api are returned and role is not added", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "WORKSPACE_MEMBER"}
		_, err := execWorkspaceCmd(cmdArgs...)
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

		expectedOut := "The user user@1.com was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "add", "--role", "WORKSPACE_MEMBER"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
}

func (s *Suite) TestWorkspacUserRemove() {
	expectedHelp := "Remove a user from an Astro Workspace"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints remove help", func() {
		cmdArgs := []string{"user", "remove", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid email removes user", func() {
		expectedOut := "The user user@1.com was successfully removed from the workspace"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("any errors from api are returned and user is not removed", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to update user")
	})
	s.Run("any context errors from api are returned and the user is not removed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceUsersResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
	})
	s.Run("command asks for input when no email is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

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

		expectedOut := "The user user@1.com was successfully removed from the workspace"
		mockClient.On("DeleteWorkspaceUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceUserResponseOK, nil).Once()

		cmdArgs := []string{"user", "remove"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
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

func (s *Suite) TestWorkspaceCreate() {
	expectedHelp := "workspace create [flags]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints add help", func() {
		cmdArgs := []string{"create", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid name with valid enforce", func() {
		expectedOut := "Astro Workspace workspace-test was successfully created\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test", "--enforce-cicd", "ON"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("valid name with invalid enforce returns an error and workspace is not created", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test", "--enforce-cicd", "on"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.ErrorIs(err, workspace.ErrWrongEnforceInput)
	})
	s.Run("any errors from api are returned and workspace is not created", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to create workspace")
	})

	s.Run("any context errors from api are returned and workspace is not created", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"create", "--name", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
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

func (s *Suite) TestWorkspaceDelete() {
	expectedHelp := "workspace delete [workspace_id] [flags]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints add help", func() {
		cmdArgs := []string{"delete", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid id and successful delete", func() {
		expectedOut := "Astro Workspace test-workspace was successfully deleted\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("invalid id returns workspace not found", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.ErrorIs(err, workspace.ErrWorkspaceNotFound)
	})
	s.Run("any errors from api are returned and workspace is not deleted", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to delete workspace")
	})

	s.Run("any context errors from api are returned and workspace is not deleted", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"delete", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("command asks for input when no workspace id is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
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

		expectedOut := "Astro Workspace test-workspace was successfully deleted\n"
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()

		cmdArgs := []string{"delete"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
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

func (s *Suite) TestWorkspaceUpdate() {
	expectedHelp := "workspace update [workspace_id] [flags]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("-h prints add help", func() {
		cmdArgs := []string{"update", "-h"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("valid id and successful update", func() {
		expectedOut := "Astro Workspace test-workspace was successfully updated\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
	s.Run("invalid id returns workspace not found", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-test"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.ErrorIs(err, workspace.ErrWorkspaceNotFound)
	})
	s.Run("any errors from api are returned and workspace is not updated", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.EqualError(err, "failed to update workspace")
	})

	s.Run("any context errors from api are returned and workspace is not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"update", "workspace-id"}
		_, err := execWorkspaceCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("command asks for input when no workspace id is passed in as an arg", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		astroCoreClient = mockClient
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

		expectedOut := "Astro Workspace test-workspace was successfully updated\n"
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()

		cmdArgs := []string{"update"}
		resp, err := execWorkspaceCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOut)
	})
}
