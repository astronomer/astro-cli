package workspacetoken

import (
	"bytes"
	"encoding/json"
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

var (
	workspaceID                             = "ck05r3bor07h40d02y2hw4n4v"
	description1                            = "Description 1"
	description2                            = "Description 2"
	fullName1                               = "User 1"
	fullName2                               = "User 2"
	token                                   = "token"
	iamAPIOrgnaizationToken                 = astroiamcore.ApiToken{Id: "token1", Name: "token1", Token: &token, Description: description1, Type: "ORGANIZATION", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
	GetAPITokensResponseOKOrganizationToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIOrgnaizationToken,
	}
	iamAPIWorkspaceToken = astroiamcore.ApiToken{Id: "token1", Name: "token1", Token: &token, Description: description1, Type: "WORKSPACE", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKWorkspaceToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIWorkspaceToken,
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
	apiToken1 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "WORKSPACE", Roles: []astrocore.ApiTokenRole{{EntityId: workspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiTokens = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "ORGANIZATION", Roles: []astrocore.ApiTokenRole{{EntityId: workspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	apiTokens2 = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "WORKSPACE", Roles: []astrocore.ApiTokenRole{{EntityId: workspaceID, EntityType: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
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
	ListWorkspaceAPITokensResponse2O0 = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens2,
			Limit:     1,
			Offset:    0,
		},
	}
	errorBodyListTokens, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListWorkspaceAPITokensResponseError = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyListTokens,
		JSON200: nil,
	}
	CreateWorkspaceAPITokenResponseOK = astrocore.CreateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	orgAPIToken1 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "ORGANIZATION_MEMBER"}, {EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	orgAPIToken2 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}

	orgAPITokens = []astrocore.ApiToken{
		orgAPIToken1,
		orgAPIToken2,
	}

	ListOrganizationAPITokensResponseOK = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: orgAPITokens,
			Limit:     1,
			Offset:    0,
		},
	}

	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListOrganizationAPITokensResponseError = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}

	UpdateOrganizationAPITokenResponseOK = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &orgAPIToken1,
	}

	errorBodyUpdateToken, _ = json.Marshal(astrocore.Error{
		Message: "failed to update token",
	})
	UpdateOrganizationAPITokenResponseError = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdateToken,
		JSON200: nil,
	}

	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create workspace token",
	})

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

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update workspace token",
	})

	UpdateWorkspaceAPITokenResponseError = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateWorkspaceAPITokenResponseOK = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateWorkspaceAPITokenResponseError = astrocore.RotateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteWorkspaceAPITokenResponseOK = astrocore.DeleteWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceAPITokenResponseError = astrocore.DeleteWorkspaceApiTokenResponse{
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
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", nil, out)
		assert.NoError(t, err)
	})

	t.Run("with specified workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherWorkspace", nil, out)

		assert.NoError(t, err)
	})

	t.Run("error path when ListWorkspaceApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherWorkspace", nil, out)
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
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		assert.NoError(t, err)
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("invalid role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", "", 0, false, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("empty name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "WORKSPACE_MEMBER", "", 0, true, out, mockClient)

		assert.Equal(t, workspace.ErrInvalidTokenName, err)
	})
}

func TestUpdateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path no id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when listWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listWorkspaceToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("", "invalid name", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := UpdateToken("tokenId", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when UpdateWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token3", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update workspace token", err.Error())
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when workspace role is invalid returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token1", "", "", "", "Invalid Role", "", out, mockClient, mockIamClient)
		assert.Equal(t, user.ErrInvalidWorkspaceRole.Error(), err.Error())
	})
	t.Run("Happy path when applying workspace role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("", apiToken1.Name, "", "", "WORKSPACE_MEMBER", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})
}

func TestRotateToken(t *testing.T) {
	t.Run("happy path - id provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("token1", "", "", false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path with confirmation", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", true, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when listWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("", "", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listWorkspaceToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("", "invalid name", "", false, false, out, mockClient, mockIamClient)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
	})

	t.Run("error path when getWorkspaceToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when RotateWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update workspace token", err.Error())
	})
}

func TestDeleteToken(t *testing.T) {
	t.Run("happy path - delete workspace token - by name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - delete workspace token - by id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - remove organization token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when listWorkspaceTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listWorkspaceToken returns a not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("", "invalid name", "", false, out, mockClient, mockIamClient)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
	})

	t.Run("error path when DeleteWorkspaceApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update workspace token", err.Error())
	})
}

func TestGetWorkspaceToken(t *testing.T) {
	t.Run("select token by id when name is empty", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("token1", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiToken1, token)
	})

	t.Run("select token by name when id is empty and there is only one matching token", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("", "Token 2", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiTokens[1], token)
	})

	t.Run("return error when token is not found by id", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("nonexistent", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.Equal(t, ErrWorkspaceTokenNotFound, err)
		assert.Equal(t, astrocore.ApiToken{}, token)
	})

	t.Run("return error when token is not found by name", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
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

func TestUpsertOrgTokenWorkspaceRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error on ListOrganizationApiTokensWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient, mockIamClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error on ListWorkspaceApiTokensWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error on GetApiTokenWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error on UpdateOrganizationApiTokenWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to update token")
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient, mockIamClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error on GetApiTokenWithResponse with token id passed in", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})
}

func TestRemoveOrgTokenWorkspaceRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path", func(t *testing.T) {
		expectedOutMessage := "Astro Organization API token token1 was successfully removed from the Workspace\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error on ListWorkspaceApiTokensWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error on GetApiTokenWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error on UpdateOrganizationApiTokenWithResponse", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to update token")
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		expectedOutMessage := "Astro Organization API token token1 was successfully removed from the Workspace\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error on GetApiTokenWithResponse with token id passed in", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})
}
