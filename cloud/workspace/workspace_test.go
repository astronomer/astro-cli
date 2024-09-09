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
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

type Suite struct {
	suite.Suite
}

func (*Suite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
}

var _ suite.SetupTestSuite = (*Suite)(nil)

func TestWorkspace(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestList() {
	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

	buf := new(bytes.Buffer)
	err := List(mockClient, buf)
	s.NoError(err)
	expected := ` NAME               ID               
 test-workspace     workspace-id     
`
	s.Equal(buf.String(), expected)
}

func (s *Suite) TestListError() {
	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

	buf := new(bytes.Buffer)
	err := List(mockClient, buf)
	s.ErrorIs(err, errMock)
}

func (s *Suite) TestGetWorkspaceSelection() {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	s.Run("success", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

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

		buf := new(bytes.Buffer)
		resp, err := GetWorkspaceSelection(mockCoreClient, buf)
		s.NoError(err)
		s.Equal("workspace-id", resp)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("list workspace failure", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		buf := new(bytes.Buffer)
		_, err := GetWorkspaceSelection(mockCoreClient, buf)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("invalid selection", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = GetWorkspaceSelection(mockCoreClient, buf)
		s.ErrorIs(err, errInvalidWorkspaceKey)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("get current context failure", func() {
		err := config.ResetCurrentContext()
		s.NoError(err)

		buf := new(bytes.Buffer)
		_, err = GetWorkspaceSelection(mockCoreClient, buf)
		s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSwitch() {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

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
			Limit:      10,
			Offset:     0,
			TotalCount: 2,
			Workspaces: workspaces,
		},
	}

	s.Run("success", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		buf := new(bytes.Buffer)
		err := Switch("workspace-id-2", mockCoreClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "workspace-id-2")
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("list workspace failure", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		buf := new(bytes.Buffer)
		err := Switch("workspace-id-2", mockCoreClient, buf)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with selection", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

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

		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "workspace-id")
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failure with invalid selection", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, buf)
		s.ErrorIs(err, errInvalidWorkspaceKey)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failure to get current context", func() {
		err := config.ResetCurrentContext()
		s.NoError(err)

		buf := new(bytes.Buffer)
		err = Switch("test-id-1", mockCoreClient, buf)
		s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetCurrentWorkspace() {
	resp, err := GetCurrentWorkspace()
	s.NoError(err)
	s.Equal(resp, "ck05r3bor07h40d02y2hw4n4v")

	ctx, err := config.GetCurrentContext()
	s.NoError(err)
	ctx.Workspace = ""
	err = ctx.SetContext()
	s.NoError(err)

	_, err = GetCurrentWorkspace()
	s.EqualError(err, "current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")

	config.ResetCurrentContext()
	_, err = GetCurrentWorkspace()
	s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
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

func (s *Suite) TestCreate() {
	s.Run("happy path Create", func() {
		expectedOutMessage := "Astro Workspace workspace-test was successfully created\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseOK, nil).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when CreateWorkspaceWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when CreateWorkspaceWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceResponseError, nil).Once()
		err := Create("workspace-test", "a test workspace", "ON", out, mockClient)
		s.EqualError(err, "failed to create workspace")
	})
	s.Run("error path when validateEnforceCD returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Create("workspace-test", "a test workspace", "on", out, mockClient)
		s.ErrorIs(err, ErrWrongEnforceInput)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Create("workspace-test", "a test workspace", "on", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
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

func (s *Suite) TestDelete() {
	s.Run("happy path Delete", func() {
		expectedOutMessage := "Astro Workspace test-workspace was successfully deleted\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseOK, nil).Once()
		err := Delete("workspace-id", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("print message if no workpaces found", func() {
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
		s.ErrorIs(err, ErrNoWorkspaceExists)
	})

	s.Run("error path when DeleteWorkspaceWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Delete("workspace-id", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteWorkspaceWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockClient.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceResponseError, nil).Once()
		err := Delete("workspace-id", out, mockClient)
		s.EqualError(err, "failed to delete workspace")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Delete("workspace-id", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("DeleteWorkspace no workspace id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
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

		err = Delete("", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
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

func (s *Suite) TestUpdate() {
	s.Run("happy path Update", func() {
		expectedOutMessage := "Astro Workspace test-workspace was successfully updated\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseOK, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "update-workspace-test", "updated workspace", "ON", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("print message if no workpaces found", func() {
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
		s.ErrorIs(err, ErrNoWorkspaceExists)
	})

	s.Run("ask to select the workspace if more than 1 exists", func() {
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
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when UpdateWorkspaceWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "", "", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateWorkspaceWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceResponseError, nil).Once()
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Update("workspace-id", "", "", "", out, mockClient)
		s.EqualError(err, "failed to update workspace")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Update("workspace-id", "", "", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("UpdateWorkspace no workspace id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
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

		err = Update("", "", "", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}
