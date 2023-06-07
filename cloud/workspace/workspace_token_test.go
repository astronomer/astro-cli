package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"
	apiToken1    = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: &description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiTokens    = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: &description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{{EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	ListWorkspaceApiTokensResponseOK = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens,
			Limit:     1,
			Offset:    0,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListWorkspaceApiTokensResponseError = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	CreateWorkspaceApiTokenResponseOK = astrocore.CreateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	CreateWorkspaceApiTokenResponseError = astrocore.CreateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateWorkspaceApiTokenResponseOK = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	UpdateWorkspaceApiTokenResponseError = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateWorkspaceApiTokenResponseOK = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateWorkspaceApiTokenResponseError = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteWorkspaceApiTokenResponseOK = astrocore.DeleteWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceApiTokenResponseError = astrocore.DeleteWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

func TestListTokens(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", out)
		assert.NoError(t, err)
	})

	t.Run("with specified workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherWorkspace", out)

		assert.NoError(t, err)
	})

	t.Run("error path when ListWorkspaceApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherWorkspace", out)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, "", out)

		assert.Error(t, err)
	})
}

func TestCreateToken(t *testing.T) {

	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceApiTokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, out, mockClient)

		assert.NoError(t, err)
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("invalid role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", "", 0, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("empty name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "WORKSPACE_MEMBER", "", 0, out, mockClient)

		assert.Equal(t, ErrInvalidName, err)
	})
}

func TestUpdateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceApiTokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when getWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseError, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when getWorkspaceToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		err := UpdateToken("token3", "", "", "", "", "", out, mockClient)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
	})

	t.Run("error path when UpdateWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceApiTokenResponseError, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		assert.Equal(t, "failed to update workspace", err.Error())
	})
}

func TestRotateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, "testOrg", "testWorkspace", "token1").Return(&RotateWorkspaceApiTokenResponseOK, nil)
		err := RotateToken("token1", "", "", false, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when getWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseError, nil)
		err := RotateToken("token1", "", "", false, out, mockClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when getWorkspaceToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		err := RotateToken("token3", "", "", false, out, mockClient)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
	})

	t.Run("error path when RotateWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceApiTokenResponseError, nil)
		err := RotateToken("token1", "", "", true, out, mockClient)
		assert.Equal(t, "failed to update workspace", err.Error())
	})
}

func TestDeleteToken(t *testing.T) {
	t.Run("happy path - delete workspace token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, "testOrg", "testWorkspace", "token1").Return(&DeleteWorkspaceApiTokenResponseOK, nil)
		err := DeleteToken("token1", "", "", false, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - remove organization token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, "testOrg", "testWorkspace", "token2").Return(&DeleteWorkspaceApiTokenResponseOK, nil)
		err := DeleteToken("token2", "", "", false, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when getWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseError, nil)
		err := DeleteToken("token1", "", "", false, out, mockClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when getWorkspaceToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		err := DeleteToken("token3", "", "", false, out, mockClient)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
	})

	t.Run("error path when DeleteWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceApiTokenResponseError, nil)
		err := DeleteToken("token1", "", "", true, out, mockClient)
		assert.Equal(t, "failed to update workspace", err.Error())
	})
}

func TestGetWorkspaceToken(t *testing.T) {
	// t.Run("select token by workspace when id and name are empty", func(t *testing.T) {
	// 	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	// 	mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
	// 	token, err := getWorkspaceToken("", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
	// 	assert.NoError(t, err)
	// 	assert.Equal(t, apiToken1, token)
	// })

	t.Run("select token by id when name is empty", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("token1", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiToken1, token)
	})

	t.Run("select token by name when id is empty and there is only one matching token", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("", "Token 2", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiTokens[1], token)
	})

	// t.Run("select token by name when id is empty and there are multiple matching tokens", func(t *testing.T) {
	// 	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	// 	mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
	// 	// Mock user input for selecting a token
	// 	mockInput := &inputTestUtil.MockInput{
	// 		Confirmed: false,
	// 		SelectedIndex: 1,
	// 	}
	// 	inputTestUtil.SetMockInput(mockInput)
	// 	token, err := getWorkspaceToken("", "Token 1", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
	// 	assert.NoError(t, err)
	// 	assert.Equal(t, apiToken1, token)
	// })

	t.Run("return error when token is not found by id", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("nonexistent", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
		assert.Equal(t, astrocore.ApiToken{}, token)
	})

	t.Run("return error when token is not found by name", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceApiTokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("", "Nonexistent Token", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
		assert.Equal(t, astrocore.ApiToken{}, token)
	})
}

func TestTimeAgo(t *testing.T) {
	currentTime := time.Now()

	t.Run("return 'Just now' for current time", func(t *testing.T) {
		result := TimeAgo(currentTime)
		assert.Equal(t, "Just now", result)
	})

	t.Run("return '30 minutes ago' for 30 minutes ago", func(t *testing.T) {
		pastTime := currentTime.Add(-30 * time.Minute)
		result := TimeAgo(pastTime)
		assert.Equal(t, "30 minutes ago", result)
	})

	t.Run("return '5 hours ago' for 5 hours ago", func(t *testing.T) {
		pastTime := currentTime.Add(-5 * time.Hour)
		result := TimeAgo(pastTime)
		assert.Equal(t, "5 hours ago", result)
	})

	t.Run("return '10 days ago' for 10 days ago", func(t *testing.T) {
		pastTime := currentTime.Add(-10 * 24 * time.Hour)
		result := TimeAgo(pastTime)
		assert.Equal(t, "10 days ago", result)
	})
}
