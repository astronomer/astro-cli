package deployment

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

var (
	deploymentID = "ck05r3bor07h40d02y2hw4n4v"
	workspaceID  = "ck05r3bor07h40d02y2hw4n4w"
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"

	iamAPIOrgnaizationToken                 = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "ORGANIZATION", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}, {EntityType: "DEPLOYMENT", EntityId: deploymentID, Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
	GetAPITokensResponseOKOrganizationToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIOrgnaizationToken,
	}
	iamAPIWorkspaceToken = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "WORKSPACE", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}, {EntityType: "DEPLOYMENT", EntityId: deploymentID, Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKWorkspaceToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIWorkspaceToken,
	}

	iamAPIDeploymentToken = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "DEPLOYMENT", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKDeploymentToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIDeploymentToken,
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
		{Id: "token2", Name: "Token 2", Description: description2, Type: "ORGANIZATION", Roles: []astrocore.ApiTokenRole{{EntityId: "otherDeployment", EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{ApiTokenName: &fullName2}},
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
	UpdateOrganizationAPITokenResponseOK = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	UpdateOrganizationAPITokenResponseError = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}

	UpdateWorkspaceAPITokenResponseOK = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	UpdateWorkspaceAPITokenResponseError = astrocore.UpdateWorkspaceApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
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
	ListOrganizationAPITokensResponseError = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
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

	ListWorkspaceAPITokensResponseError = astrocore.ListWorkspaceApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
)

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", nil, out)
		s.NoError(err)
	})

	s.Run("with specified deployment", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherDeployment", nil, out)

		s.NoError(err)
	})

	s.Run("error path when ListDeploymentApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherDeployment", nil, out)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, "", nil, out)

		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 100, false, out, mockClient)

		s.NoError(err)
	})

	s.Run("happy path with clean output", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 100, true, out, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "DEPLOYMENT_MEMBER", "", 0, true, out, mockClient)

		s.Equal(ErrInvalidTokenName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path multiple name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpdateToken("", "Token 1", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "mockNewName", "mockDescription", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := UpdateToken("", "invalid name", "", "", "", "", out, mockClient, mockIamClient)
		s.Equal(ErrDeploymentTokenNotFound, err)
	})

	s.Run("error path when getDeploymentToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := UpdateToken("tokenId", "", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseError, nil)
		err := UpdateToken("token3", "", "", "", "", "", out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("Happy path when applying deployment role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		err := UpdateToken("", apiToken1.Name, "", "", "DEPLOYMENT_MEMBER", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path wrong token type provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil)

		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("token1", "", "", false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path with confirmation", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", false, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path with clean output", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, "", true, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := RotateToken("", "", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", "", false, false, out, mockClient, mockIamClient)
		s.Equal(ErrDeploymentTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseError, nil)
		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete deployment token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - delete deployment token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - delete deployment token - no force", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("y")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := DeleteToken("token1", "", "", true, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", "", true, out, mockClient, mockIamClient)
		s.Equal(ErrDeploymentTokenNotFound, err)
	})

	s.Run("error path when DeleteDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseError, nil)
		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestGetDeploymentToken() {
	s.Run("select token by id when name is empty", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("token1", "", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Token 2", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("nonexistent", "", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		s.Equal(ErrDeploymentTokenNotFound, err)
		s.Equal(astrocore.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Nonexistent Token", "testDeployment", "\nPlease select the deployment token you would like to delete or remove:", apiTokens)
		s.Equal(ErrDeploymentTokenNotFound, err)
		s.Equal(astrocore.ApiToken{}, token)
	})
}

func (s *Suite) TestTimeAgo() {
	currentTime := time.Now()

	s.Run("return 'Just now' for current time", func() {
		result := TimeAgo(currentTime)
		s.Equal("Just now", result)
	})

	s.Run("return '30 minutes ago' for 30 minutes ago", func() {
		pastTime := currentTime.Add(-30 * time.Minute)
		result := TimeAgo(pastTime)
		s.Equal("30 minutes ago", result)
	})

	s.Run("return '5 hours ago' for 5 hours ago", func() {
		pastTime := currentTime.Add(-5 * time.Hour)
		result := TimeAgo(pastTime)
		s.Equal("5 hours ago", result)
	})

	s.Run("return '10 days ago' for 10 days ago", func() {
		pastTime := currentTime.Add(-10 * 24 * time.Hour)
		result := TimeAgo(pastTime)
		s.Equal("10 days ago", result)
	})
}

func (s *Suite) TestRemoveOrgTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
		expectedOutMessage := "Astro Organization API token Token 1 was successfully removed from the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("", "", deploymentID, out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateOrganizationApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token Token 1 was successfully removed from the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("token-id", "", deploymentID, out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenDeploymentRole("token-id", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})
}

func (s *Suite) TestRemoveWorkspaceTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
		expectedOutMessage := "Astro Workspace API token Token 1 was successfully removed from the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateWorkspaceApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Workspace API token Token 1 was successfully removed from the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("token-id", "", "", deploymentID, out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveWorkspaceTokenDeploymentRole("token-id", "", "", deploymentID, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})
}

func (s *Suite) TestUpsertOrgTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
		expectedOutMessage := "Astro Organization API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListOrganizationApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
		expectedOutMessage := "Astro Organization API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "update", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "update", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateOrganizationApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path with token id passed in - wrong token type", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestUpsertWorkspaceTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
		expectedOutMessage := "Astro Workspace API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListWorkspaceApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
		expectedOutMessage := "Astro Workspace API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "update", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "update", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateWorkspaceApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Workspace API token Token 1 was successfully added/updated to the Deployment\n"
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path with token id passed in - wrong token type", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}
