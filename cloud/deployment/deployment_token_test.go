package deployment

import (
	"bytes"
	"encoding/json"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	deploymentID = "ck05r3bor07h40d02y2hw4n4v"
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"

	iamApiToken            = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "DEPLOYMENT", Roles: &[]astroiamcore.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
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

	apiToken1 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "DEPLOYMENT", Roles: []astrocore.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiTokens = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "ORGANIZATION", Roles: []astrocore.ApiTokenRole{{EntityId: "otherDeployment", EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{ApiTokenName: &fullName2}},
	}
	apiTokens2 = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "DEPLOYMENT", Roles: []astrocore.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	ListDeploymentAPITokensResponseOK = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens,
			Limit:     1,
			Offset:    0,
		},
	}
	ListDeploymentAPITokensResponse2O0 = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens2,
			Limit:     1,
			Offset:    0,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListDeploymentAPITokensResponseError = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	CreateDeploymentAPITokenResponseOK = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create token",
	})
	CreateDeploymentAPITokenResponseError = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateDeploymentAPITokenResponseOK = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update token",
	})
	UpdateDeploymentAPITokenResponseError = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateDeploymentAPITokenResponseOK = astrocore.RotateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateDeploymentAPITokenResponseError = astrocore.RotateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentAPITokenResponseOK = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentAPITokenResponseError = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

func TestListTokens(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", nil, out)
		assert.NoError(t, err)
	})

	t.Run("with specified deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherDeployment", nil, out)

		assert.NoError(t, err)
	})

	t.Run("error path when ListDeploymentApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherDeployment", nil, out)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, "", nil, out)

		assert.Error(t, err)
	})
}

func TestCreateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 100, false, out, mockClient)

		assert.NoError(t, err)
	})

	t.Run("happy path with clean output", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 100, true, out, mockClient)

		assert.NoError(t, err)
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 0, false, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("empty name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "DEPLOYMENT_MEMBER", "", 0, true, out, mockClient)

		assert.Equal(t, ErrInvalidTokenName, err)
	})
}

func TestUpdateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path no id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
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
		err = UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path multiple name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
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
		err = UpdateToken("", "Token 1", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "mockNewName", "mockDescription", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when listDeploymentTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listDeploymentToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := UpdateToken("", "invalid name", "", "", "", "", out, mockClient, mockIamClient)
		assert.Equal(t, ErrDeploymentTokenNotFound, err)
	})

	t.Run("error path when getDeploymentToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := UpdateToken("tokenId", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when UpdateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseError, nil)
		err := UpdateToken("token3", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("Happy path when applying deployment role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		err := UpdateToken("", apiToken1.Name, "", "", "DEPLOYMENT_MEMBER", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})
}

func TestRotateToken(t *testing.T) {
	t.Run("happy path - id provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("token1", "", "", false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path with confirmation", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", false, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path with clean output", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", true, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when listDeploymentTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := RotateToken("", "", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listDeploymentToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", "", false, false, out, mockClient, mockIamClient)
		assert.Equal(t, ErrDeploymentTokenNotFound, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when RotateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseError, nil)
		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})
}

func TestDeleteToken(t *testing.T) {
	t.Run("happy path - delete deployment token - by name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - delete deployment token - by id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - delete deployment token - no force", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
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
		err = DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when listDeploymentTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listDeploymentToken returns a not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", "", true, out, mockClient, mockIamClient)
		assert.Equal(t, ErrDeploymentTokenNotFound, err)
	})

	t.Run("error path when DeleteDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseError, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})
}

func TestGetDeploymentToken(t *testing.T) {
	t.Run("select token by id when name is empty", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("token1", "", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiToken1, token)
	})

	t.Run("select token by name when id is empty and there is only one matching token", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Token 2", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiTokens[1], token)
	})

	t.Run("return error when token is not found by id", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("nonexistent", "", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		assert.Equal(t, ErrDeploymentTokenNotFound, err)
		assert.Equal(t, astrocore.ApiToken{}, token)
	})

	t.Run("return error when token is not found by name", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Nonexistent Token", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		assert.Equal(t, ErrDeploymentTokenNotFound, err)
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
