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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

type Suite struct {
	suite.Suite
}

func TestWorkspaceToken(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", nil, out)
		s.NoError(err)
	})

	s.Run("with specified workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherWorkspace", nil, out)

		s.NoError(err)
	})

	s.Run("error path when ListWorkspaceApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherWorkspace", nil, out)
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
		mockClient.On("CreateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("invalid role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", "", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "WORKSPACE_MEMBER", "", 0, true, out, mockClient)

		s.Equal(workspace.ErrInvalidTokenName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
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
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("", "", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("", "invalid name", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := UpdateToken("tokenId", "", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateWorkspaceApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token3", "", "", "", "", "", out, mockClient, mockIamClient)
		s.Equal("failed to update workspace token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when workspace role is invalid returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token1", "", "", "", "Invalid Role", "", out, mockClient, mockIamClient)
		s.Equal(user.ErrInvalidWorkspaceRole.Error(), err.Error())
	})
	s.Run("Happy path when applying workspace role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("", apiToken1.Name, "", "", "WORKSPACE_MEMBER", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("token1", "", "", false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path with confirmation", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", true, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("", "", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := RotateToken("", "invalid name", "", false, false, out, mockClient, mockIamClient)
		s.Equal(ErrWorkspaceTokenNotFound, err)
	})

	s.Run("error path when getWorkspaceToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateWorkspaceApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient, mockIamClient)
		s.Equal("failed to update workspace token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete workspace token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - delete workspace token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - remove organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("", "invalid name", "", false, out, mockClient, mockIamClient)
		s.Equal(ErrWorkspaceTokenNotFound, err)
	})

	s.Run("error path when DeleteWorkspaceApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseError, nil)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient, mockIamClient)
		s.Equal("failed to update workspace token", err.Error())
	})
}

func (s *Suite) TestGetWorkspaceToken() {
	s.Run("select token by id when name is empty", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("token1", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("", "Token 2", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("nonexistent", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(ErrWorkspaceTokenNotFound, err)
		s.Equal(astrocore.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		token, err := getWorkspaceToken("", "Nonexistent Token", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(ErrWorkspaceTokenNotFound, err)
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

func (s *Suite) TestUpsertOrgTokenWorkspaceRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
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
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
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
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient, mockIamClient)
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRemoveOrgTokenWorkspaceRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
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
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateOrganizationApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully removed from the Workspace\n"
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient, mockIamClient)
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})
}
