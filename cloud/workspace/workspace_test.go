package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock     = errors.New("mock error")
	description = "test workspace"
	workspace1  = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &description,
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

func TestList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

	buf := new(bytes.Buffer)
	err := List(mockClient, buf)
	assert.NoError(t, err)
	expected := ` NAME               ID               
 test-workspace     workspace-id     
`
	assert.Equal(t, buf.String(), expected)
}

func TestListError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

	buf := new(bytes.Buffer)
	err := List(mockClient, buf)
	assert.ErrorIs(t, err, errMock)
}

func TestGetWorkspaceSelection(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	t.Run("success", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

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

		buf := new(bytes.Buffer)
		resp, err := GetWorkspaceSelection(mockCoreClient, buf)
		assert.NoError(t, err)
		assert.Equal(t, "workspace-id", resp)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		buf := new(bytes.Buffer)
		_, err := GetWorkspaceSelection(mockCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("invalid selection", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		// mock os.Stdin
		input := []byte("0")
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

		buf := new(bytes.Buffer)
		_, err = GetWorkspaceSelection(mockCoreClient, buf)
		assert.ErrorIs(t, err, errInvalidWorkspaceKey)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("get current context failure", func(t *testing.T) {
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = GetWorkspaceSelection(mockCoreClient, buf)
		assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockCoreClient.AssertExpectations(t)
	})
}

func TestSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	t.Run("success", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		buf := new(bytes.Buffer)
		err := Switch("test-id-1", mockCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		buf := new(bytes.Buffer)
		err := Switch("test-id-1", mockCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with selection", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Twice()

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

		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "workspace-id")
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failure with invalid selection", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		// mock os.Stdin
		input := []byte("0")
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

		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, buf)
		assert.ErrorIs(t, err, errInvalidWorkspaceKey)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failure to get current context", func(t *testing.T) {
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		err = Switch("test-id-1", mockCoreClient, buf)
		assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockCoreClient.AssertExpectations(t)
	})
}

func TestGetCurrentWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	resp, err := GetCurrentWorkspace()
	assert.NoError(t, err)
	assert.Equal(t, resp, "ck05r3bor07h40d02y2hw4n4v")

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.Workspace = ""
	err = ctx.SetContext()
	assert.NoError(t, err)

	_, err = GetCurrentWorkspace()
	assert.EqualError(t, err, "current workspace context not set, you can switch to a workspace with \n\astro workspace switch WORKSPACEID")

	config.ResetCurrentContext()
	_, err = GetCurrentWorkspace()
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
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

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		expectedOutMessage := "Astro Workspace workspace-test was successfully created\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateWorkspaceWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateWorkspaceWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseError, nil).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		assert.EqualError(t, err, "failed to create workspace")
	})
	t.Run("error path when validateEnforceCD returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Create("workspace-test", "a test workspace", "on", out, mockClient)
		assert.ErrorIs(t, err, ErrWrongEnforceInput)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = Create("workspace-test", "a test workspace", "on", out, mockClient)
		assert.ErrorIs(t, err, user.ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Create("workspace-test", "a test workspace", "on", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
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

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Delete", func(t *testing.T) {
		expectedOutMessage := "Astro Workspace test-workspace was successfully deleted\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()
		err := Delete("workspace-id", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("print message if no workpaces found", func(t *testing.T) {
		ws := []astrocore.Workspace{}

		listWorkspacesResponseOK := astrocore.ListWorkspacesResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.WorkspacesPaginated{
				Limit:      2,
				Offset:     0,
				TotalCount: 0,
				Workspaces: ws,
			},
		}
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&listWorkspacesResponseOK, nil).Once()
		err := Delete("", out, mockClient)
		assert.ErrorIs(t, err, ErrNoWorkspaceExists)
	})

	t.Run("error path when DeleteWorkspaceWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Delete("workspace-id", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteWorkspaceWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		err := Delete("workspace-id", out, mockClient)
		assert.EqualError(t, err, "failed to delete workspace")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = Delete("workspace-id", out, mockClient)
		assert.ErrorIs(t, err, user.ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Delete("workspace-id", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("DeleteWorkspace no workspace id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
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

		err = Delete("", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
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

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update workspace",
	})

	UpdateWorkspaceResponseError = astrocore.UpdateWorkspaceResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
)

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := "Astro Workspace test-workspace was successfully updated\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "update-workspace-test", "updated workspace", "ON", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("print message if no workpaces found", func(t *testing.T) {
		ws := []astrocore.Workspace{}

		listWorkspacesResponseOK := astrocore.ListWorkspacesResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.WorkspacesPaginated{
				Limit:      2,
				Offset:     0,
				TotalCount: 0,
				Workspaces: ws,
			},
		}
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&listWorkspacesResponseOK, nil).Once()
		err := Update("", "update-workspace-test", "updated workspace", "ON", out, mockClient)
		assert.ErrorIs(t, err, ErrNoWorkspaceExists)
	})

	t.Run("ask to select the workspace if more than 1 exists", func(t *testing.T) {
		workspace2 := astrocore.Workspace{
			Name:                         "test-workspace-2",
			Description:                  &description,
			ApiKeyOnlyDeploymentsDefault: false,
			Id:                           "workspace-id-2",
		}

		workspaces = []astrocore.Workspace{
			workspace1,
			workspace2,
		}

		ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.WorkspacesPaginated{
				Limit:      2,
				Offset:     0,
				TotalCount: 2,
				Workspaces: workspaces,
			},
		}
		expectedOutMessage := "Astro Workspace test-workspace was successfully updated\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "update-workspace-test", "updated workspace", "ON", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when UpdateWorkspaceWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "", "", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateWorkspaceWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "", "", "", out, mockClient)
		assert.EqualError(t, err, "failed to update workspace")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = Update("workspace-id", "", "", "", out, mockClient)
		assert.ErrorIs(t, err, user.ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Update("workspace-id", "", "", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspace no workspace id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
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

		err = Update("", "", "", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}
