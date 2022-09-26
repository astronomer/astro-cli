package workspace

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
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
    assert.EqualError(t, err, "Cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions.\n\nDetails: Error processing GraphQL request: API error (500): Internal Server Error")
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
		resp, err := getWorkspaceSelection(mockClient, buf)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse[0].ID, resp)
		mockClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()

		buf := new(bytes.Buffer)
		_, err := getWorkspaceSelection(mockClient, buf)
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
		_, err = getWorkspaceSelection(mockClient, buf)
		assert.ErrorIs(t, err, errInvalidWorkspaceKey)
		mockClient.AssertExpectations(t)
	})

	t.Run("get current context failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = getWorkspaceSelection(mockClient, buf)
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
