package apitoken

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/stretchr/testify/mock"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

var (
	description1                       = "Description 1"
	description2                       = "Description 2"
	fullName1                          = "User 1"
	fullName2                          = "User 2"
	token                              = "token"
	workspaceID                        = "ck05r3bor07h40d02y2hw4n4v"
	deploymentID                       = "ck05r3bor07h40d02y2hw4n4d"
	role                               = "DEPLOYMENT_ADMIN"
	deploymentApiToken1                = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityId: "DEPLOYMENT", Role: role}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	errorNetwork                       = errors.New("network error")
	CreateDeploymentApiTokenResponseOK = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentApiToken1,
	}
	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create apiToken",
	})
	CreateDeploymentApiTokenResponseError = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyCreate,
	}

	errorBodyDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete apiToken",
	})

	DeleteDeploymentApiTokenResponseOK = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentApiTokenResponseError = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyDelete,
	}

	paginatedApitokens = astrocore.ListApiTokensPaginated{
		ApiTokens: []astrocore.ApiToken{
			deploymentApiToken1,
		},
	}

	ListDeploymentApiTokensResponseOK = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &paginatedApitokens,
	}

	paginatedApitokensEmpty = astrocore.ListApiTokensPaginated{
		ApiTokens: []astrocore.ApiToken{},
	}

	ListDeploymentApiTokensResponseEmpty = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &paginatedApitokensEmpty,
	}

	errorListDeploymentApiTokens, _ = json.Marshal(astrocore.Error{
		Message: "failed to list deployment apiTokens",
	})
	ListDeploymentApiTokensResponseError = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorListDeploymentApiTokens,
	}
	GetDeploymentApiTokenWithResponseOK = astrocore.GetDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentApiToken1,
	}
	errorGetDeploymentApiTokens, _ = json.Marshal(astrocore.Error{
		Message: "failed to get deployment apiToken",
	})
	GetDeploymentApiTokenWithResponseError = astrocore.GetDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorGetDeploymentApiTokens,
	}

	UpdateDeploymentApiTokenRoleResponseOK = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentApiToken1,
	}
	errorUpdateDeploymentApiToken, _ = json.Marshal(astrocore.Error{
		Message: "failed to update deployment apiToken",
	})
	UpdateDeploymentApiTokenRoleResponseError = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorUpdateDeploymentApiToken,
	}
)

func TestCreateDeploymentApiToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("The apiToken was successfully created with the role %s\n", role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentApiTokenResponseOK, nil).Once()
		err := CreateDeploymentApiToken(deploymentApiToken1.Name, role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path no name passed so user types one in when prompted", func(t *testing.T) {
		teamName := "Test ApiToken Name"
		expectedOutMessage := fmt.Sprintf("The apiToken was successfully created with the role %s\n", role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentApiTokenResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, teamName)()
		err := CreateDeploymentApiToken("", role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := CreateDeploymentApiToken(deploymentApiToken1.Name, role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentApiTokenResponseError, nil).Once()
		err := CreateDeploymentApiToken(deploymentApiToken1.Name, role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to create apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateDeploymentApiToken(deploymentApiToken1.Name, role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path no name passed in and user doesn't type one in when prompted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateDeploymentApiToken("", role, deploymentApiToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "you must give your ApiToken a name")
	})
}

func TestUpdateDeploymentApiTokenRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateDeploymentApiTokenRole", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("The deployment apiToken %s role was successfully updated to %s\n", deploymentApiToken1.Id, role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentApiTokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentApiTokenRoleResponseOK, nil).Once()
		err := UpdateDeploymentApiTokenRole(deploymentApiToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no deployment apiTokens found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseEmpty, nil).Twice()
		err := UpdateDeploymentApiTokenRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "no ApiTokens found in your deployment")
	})

	t.Run("error path when GetDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateDeploymentApiTokenRole(deploymentApiToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentApiTokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateDeploymentApiTokenRole(deploymentApiToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentApiTokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentApiTokenRoleResponseError, nil).Once()
		err := UpdateDeploymentApiTokenRole(deploymentApiToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to update deployment apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateDeploymentApiTokenRole(deploymentApiToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateDeploymentApiTokenRole no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("The deployment apiToken %s role was successfully updated to %s\n", deploymentApiToken1.Id, role)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentApiTokenRoleResponseOK, nil).Once()

		err = UpdateDeploymentApiTokenRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestRemoveDeploymentApiToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteDeploymentApiToken", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro ApiToken %s was successfully removed from deployment %s\n", deploymentApiToken1.Id, deploymentID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentApiTokenResponseOK, nil).Once()
		err := RemoveDeploymentApiToken(deploymentApiToken1.Id, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when DeleteDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveDeploymentApiToken(deploymentApiToken1.Id, "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentApiTokenResponseError, nil).Once()
		err := RemoveDeploymentApiToken(deploymentApiToken1.Id, "", out, mockClient)
		assert.EqualError(t, err, "failed to delete apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveDeploymentApiToken(deploymentApiToken1.Id, "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("RemoveDeploymentApiToken no apiToken id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("Astro ApiToken %s was successfully removed from deployment %s\n", deploymentApiToken1.Id, deploymentID)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentApiTokenResponseOK, nil).Once()

		err = RemoveDeploymentApiToken("", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path - RemoveDeploymentApiToken no apiToken id passed error on ListDeploymentApiTokensWithResponse", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseError, nil).Twice()

		err := RemoveDeploymentApiToken("", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to list deployment apiTokens")
	})
}

func TestListDeploymentApiTokens(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path TestListDeploymentApiTokens", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseOK, nil).Twice()
		err := ListDeploymentApiTokens(out, mockClient, deploymentID)
		assert.NoError(t, err)
	})

	t.Run("error path when ListDeploymentApiTokensWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListDeploymentApiTokens(out, mockClient, deploymentID)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListDeploymentApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentApiTokensResponseError, nil).Twice()
		err := ListDeploymentApiTokens(out, mockClient, deploymentID)
		assert.EqualError(t, err, "failed to list deployment apiTokens")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListDeploymentApiTokens(out, mockClient, deploymentID)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}
