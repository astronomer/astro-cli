package organization

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
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	description1           = "Description 1"
	description2           = "Description 2"
	fullName1              = "User 1"
	fullName2              = "User 2"
	token                  = "token"
	workspaceID            = "ck05r3bor07h40d02y2hw4n4v"
	iamAPIToken            = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSAPCE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
	GetAPITokensResponseOK = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIToken,
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

	apiToken1 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "ORGANIZATION_MEMBER"}, {EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiToken2 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}

	apiTokens = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	apiTokens2 = []astrocore.ApiToken{
		apiToken1,
		apiToken2,
	}
	ListOrganizationAPITokensResponseOK = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens,
			Limit:     1,
			Offset:    0,
		},
	}
	ListOrganizationAPITokensResponse2O0 = astrocore.ListOrganizationApiTokensResponse{
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
	ListOrganizationAPITokensResponseError = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}

	GetOrganizationAPITokenResponseOK = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken2,
	}
	getTokenErrorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to get token",
	})
	GetOrganizationAPITokenResponseError = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    getTokenErrorBodyList,
		JSON200: nil,
	}

	CreateOrganizationAPITokenResponseOK = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create token",
	})
	CreateOrganizationAPITokenResponseError = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateOrganizationAPITokenResponseOK = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update token",
	})
	UpdateOrganizationAPITokenResponseError = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateOrganizationAPITokenResponseOK = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateOrganizationAPITokenResponseError = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteOrganizationAPITokenResponseOK = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteOrganizationAPITokenResponseError = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

func TestAddOrgTokenToWorkspace(t *testing.T) {
	var (
		workspace          = workspaceID
		role               = "WORKSPACE_MEMBER"
		selectedTokenID    = "token1"
		selectedTokenName  = "Token 1"
		selectedTokenName2 = "Token 2"
	)

	t.Run("return error for invalid workspace role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, "INVALID_ROLE", workspace, nil, nil, nil)
		assert.Error(t, err)
		assert.Equal(t, user.ErrInvalidWorkspaceRole, err)
	})

	t.Run("return error for failed to get current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, role, workspace, nil, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("return error for failed to list organization tokens", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, nil, mockClient, mockIamClient)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("return error for failed to select organization token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace("", "Invalid name", role, workspace, nil, mockClient, mockIamClient)
		assert.Error(t, err)
		assert.Equal(t, errOrganizationTokenNotFound, err)
	})

	t.Run("return error for organization token already in workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, "WORKSPACE_AUTHOR", workspace, nil, mockClient, mockIamClient)
		assert.Error(t, err)
		assert.Equal(t, errOrgTokenInWorkspace, err)
	})

	t.Run("successfully add organization token to workspace using id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("return error for failed to get organization token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, nil, mockClient, mockIamClient)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("successfully add organization token to workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName2, role, workspace, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("successfully select and add organization token to workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

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
		err = AddOrgTokenToWorkspace("", "", role, workspace, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("successfully select and add organization token to workspace 2", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil)
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
		err = AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})
}

func TestListTokens(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, out)
		assert.NoError(t, err)
	})

	t.Run("error path when ListOrganizationApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, out)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, out)

		assert.Error(t, err)
	})
}

func TestCreateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		assert.NoError(t, err)
	})

	t.Run("error getting current context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("invalid role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", 0, false, out, mockClient)

		assert.Error(t, err)
	})

	t.Run("empty name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "ORGANIZATION_MEMBER", 0, true, out, mockClient)

		assert.Equal(t, ErrInvalidName, err)
	})
}

func TestUpdateToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path no id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path multiple name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "Token 1", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when listOrganizationTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listOrganizationToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := UpdateToken("", "invalid name", "", "", "", out, mockClient, mockIamClient)
		assert.Equal(t, errOrganizationTokenNotFound, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := UpdateToken("tokenId", "", "", "", "", out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when UpdateOrganizationApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil)
		err := UpdateToken("token3", "", "", "", "", out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when workspace role is invalid returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "Invalid Role", out, mockClient, mockIamClient)
		assert.Equal(t, user.ErrInvalidOrganizationRole.Error(), err.Error())
	})
	t.Run("Happy path when applying workspace role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("", apiToken1.Name, "", "", "ORGANIZATION_MEMBER", out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("Happy path when applying organization role using id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "ORGANIZATION_MEMBER", out, mockClient, mockIamClient)
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
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("token1", "", false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path with confirmation", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, true, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", false, false, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when listOrganizationTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := RotateToken("", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listOrganizationToken returns an not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", false, false, out, mockClient, mockIamClient)
		assert.Equal(t, errOrganizationTokenNotFound, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", false, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when RotateOrganizationApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseError, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})
}

func TestDeleteToken(t *testing.T) {
	t.Run("happy path - delete workspace token - by name", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - delete workspace token - by id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetOrganizationAPITokenResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("happy path - remove organization token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		assert.NoError(t, err)
	})

	t.Run("error path when there is no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		assert.Error(t, err)
	})

	t.Run("error path when getApiToken returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path when listOrganizationTokens returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		assert.ErrorContains(t, err, "failed to list tokens")
	})

	t.Run("error path when listOrganizationToken returns a not found error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", false, out, mockClient, mockIamClient)
		assert.Equal(t, errOrganizationTokenNotFound, err)
	})

	t.Run("error path when DeleteOrganizationApiTokenWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseError, nil)
		err := DeleteToken("", apiToken1.Name, true, out, mockClient, mockIamClient)
		assert.Equal(t, "failed to update token", err.Error())
	})
}

func TestListTokenRoles(t *testing.T) {
	t.Run("happy path - list token roles by id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		assert.NoError(t, err)
	})

	t.Run("error path - list token roles by id - api error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		assert.ErrorContains(t, err, "failed to get token")
	})

	t.Run("error path - list token roles by id - no context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		assert.Error(t, err)
	})

	t.Run("happy path - list token roles no id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
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
		err = ListTokenRoles("", mockClient, mockIamClient, out)
		assert.NoError(t, err)
	})

	t.Run("error path - list token roles no id - list tokens api error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Twice()
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
		err = ListTokenRoles("", mockClient, mockIamClient, out)
		assert.ErrorContains(t, err, "failed to list tokens")
	})
}

func TestGetOrganizationToken(t *testing.T) {
	t.Run("select token by id when name is empty", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("token1", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiToken1, token)
	})

	t.Run("select token by name when id is empty and there is only one matching token", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("", "Token 2", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.NoError(t, err)
		assert.Equal(t, apiTokens[1], token)
	})

	t.Run("return error when token is not found by id", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("nonexistent", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.Equal(t, errOrganizationTokenNotFound, err)
		assert.Equal(t, astrocore.ApiToken{}, token)
	})

	t.Run("return error when token is not found by name", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("", "Nonexistent Token", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		assert.Equal(t, errOrganizationTokenNotFound, err)
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
