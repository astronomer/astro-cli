package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMock = errors.New("mock error")

func TestList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	okResponse := `{
  "data": {
    "workspaces": [
      {
        "id": "ckbv7zvb100pe0760xp98qnh9",
        "label": "w1",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "test@test.com"
            }
          },
          {
            "role": "WORKSPACE_VIEWER",
            "user": {
              "id": "ckc0eilr201fl07602i8gq4vo",
              "username": "test1@test.com"
            }
          }
        ]
      },
      {
        "id": "ckbv8pwbq00wk0760us7ktcgd",
        "label": "wwww",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "test@test.com"
            }
          }
        ]
      },
      {
        "id": "ckc0j8y1101xo0760or02jdi7",
        "label": "test",
        "description": "test",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "test@test.com"
            }
          }
        ]
      }
    ]
  }
}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	astroAPI := astro.NewAstroClient(client)

	buf := new(bytes.Buffer)
	err := List(astroAPI, buf)
	assert.NoError(t, err)
	expected := ` NAME     ID                            
 w1       ckbv7zvb100pe0760xp98qnh9     
 wwww     ckbv8pwbq00wk0760us7ktcgd     
 test     ckc0j8y1101xo0760or02jdi7     
`
	assert.Equal(t, buf.String(), expected)
}

func TestListError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	astroAPI := astro.NewAstroClient(client)

	buf := new(bytes.Buffer)
	err := List(astroAPI, buf)
	assert.EqualError(t, err, "cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions. If you are using an API Key or Token make sure your context is correct.\n\nDetails: Error processing GraphQL request: API error (500): Internal Server Error")
}

func TestGetWorkspaceSelection(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockResponse := []astro.Workspace{
		{
			ID:    "test-id-1",
			Label: "test-label",
		},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return(mockResponse, nil).Once()

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
		resp, err := GetWorkspaceSelection(mockClient, buf)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse[0].ID, resp)
		mockClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()
		buf := new(bytes.Buffer)
		_, err := GetWorkspaceSelection(mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return(mockResponse, nil).Once()

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
		_, err = GetWorkspaceSelection(mockClient, buf)
		assert.ErrorIs(t, err, errInvalidWorkspaceKey)
		mockClient.AssertExpectations(t)
	})

	t.Run("get current context failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = GetWorkspaceSelection(mockClient, buf)
		assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockClient.AssertExpectations(t)
	})
}

func TestSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockResponse := []astro.Workspace{
		{
			ID:    "test-id-1",
			Label: "test-label",
		},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return(mockResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := Switch("test-id-1", mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		mockClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := Switch("test-id-1", mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return(mockResponse, nil).Twice()

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
		err = Switch("", mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		mockClient.AssertExpectations(t)
	})

	t.Run("failure with invalid selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return(mockResponse, nil).Once()

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
		err = Switch("", mockClient, buf)
		assert.ErrorIs(t, err, errInvalidWorkspaceKey)
		mockClient.AssertExpectations(t)
	})

	t.Run("failure to get current context", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)

		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		err = Switch("test-id-1", mockClient, buf)
		assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockClient.AssertExpectations(t)
	})
}

func TestGetCurrentWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
