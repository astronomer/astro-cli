package apitoken

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"

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
	deploymentAPIToken1                = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityId: "DEPLOYMENT", Role: role}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	errorNetwork                       = errors.New("network error")
	CreateDeploymentAPITokenResponseOK = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create apiToken",
	})
	CreateDeploymentAPITokenResponseError = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyCreate,
	}

	errorBodyDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete apiToken",
	})

	DeleteDeploymentAPITokenResponseOK = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentAPITokenResponseError = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyDelete,
	}

	paginatedAPITokens = astrocore.ListApiTokensPaginated{
		ApiTokens: []astrocore.ApiToken{
			deploymentAPIToken1,
		},
	}

	ListDeploymentAPITokensResponseOK = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &paginatedAPITokens,
	}

	paginatedAPITokensEmpty = astrocore.ListApiTokensPaginated{
		ApiTokens: []astrocore.ApiToken{},
	}

	ListDeploymentAPITokensResponseEmpty = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &paginatedAPITokensEmpty,
	}

	errorListDeploymentAPITokens, _ = json.Marshal(astrocore.Error{
		Message: "failed to list deployment apiTokens",
	})
	ListDeploymentAPITokensResponseError = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorListDeploymentAPITokens,
	}
	GetDeploymentAPITokenWithResponseOK = astrocore.GetDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
	errorGetDeploymentAPITokens, _ = json.Marshal(astrocore.Error{
		Message: "failed to get deployment apiToken",
	})
	GetDeploymentAPITokenWithResponseError = astrocore.GetDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorGetDeploymentAPITokens,
	}

	UpdateDeploymentAPITokenRoleResponseOK = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
	errorUpdateDeploymentAPIToken, _ = json.Marshal(astrocore.Error{
		Message: "failed to update deployment apiToken",
	})
	UpdateDeploymentAPITokenRoleResponseError = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorUpdateDeploymentAPIToken,
	}
)

func TestCreateDeploymentAPIToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("The apiToken was successfully created with the role %s\n", role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil).Once()
		err := CreateDeploymentAPIToken(deploymentAPIToken1.Name, role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path no name passed so user types one in when prompted", func(t *testing.T) {
		teamName := "Test ApiToken Name"
		expectedOutMessage := fmt.Sprintf("The apiToken was successfully created with the role %s\n", role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, teamName)()
		err := CreateDeploymentAPIToken("", role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := CreateDeploymentAPIToken(deploymentAPIToken1.Name, role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseError, nil).Once()
		err := CreateDeploymentAPIToken(deploymentAPIToken1.Name, role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to create apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateDeploymentAPIToken(deploymentAPIToken1.Name, role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path no name passed in and user doesn't type one in when prompted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateDeploymentAPIToken("", role, deploymentAPIToken1.Description, deploymentID, out, mockClient)
		assert.EqualError(t, err, "you must give your ApiToken a name")
	})
}

func TestUpdateDeploymentAPITokenRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateDeploymentApiTokenRole", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("The deployment apiToken %s role was successfully updated to %s\n", deploymentAPIToken1.Id, role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenRoleResponseOK, nil).Once()
		err := UpdateDeploymentAPITokenRole(deploymentAPIToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no deployment apiTokens found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseEmpty, nil).Twice()
		err := UpdateDeploymentAPITokenRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "no ApiTokens found in your deployment")
	})

	t.Run("error path when GetDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateDeploymentAPITokenRole(deploymentAPIToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateDeploymentAPITokenRole(deploymentAPIToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil).Twice()
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenRoleResponseError, nil).Once()
		err := UpdateDeploymentAPITokenRole(deploymentAPIToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to update deployment apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateDeploymentAPITokenRole(deploymentAPIToken1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateDeploymentApiTokenRole no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("The deployment apiToken %s role was successfully updated to %s\n", deploymentAPIToken1.Id, role)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenRoleResponseOK, nil).Once()

		err = UpdateDeploymentAPITokenRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestDeleteDeploymentAPIToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteDeploymentApiToken", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro ApiToken %s was successfully deleted from deployment %s\n", deploymentAPIToken1.Id, deploymentID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil).Once()
		err := DeleteDeploymentAPIToken(deploymentAPIToken1.Id, deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when DeleteDeploymentApiTokenWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := DeleteDeploymentAPIToken(deploymentAPIToken1.Id, "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteDeploymentApiTokenWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseError, nil).Once()
		err := DeleteDeploymentAPIToken(deploymentAPIToken1.Id, "", out, mockClient)
		assert.EqualError(t, err, "failed to delete apiToken")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := DeleteDeploymentAPIToken(deploymentAPIToken1.Id, "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("RemoveDeploymentApiToken no apiToken id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("Astro ApiToken %s was successfully deleted from deployment %s\n", deploymentAPIToken1.Id, deploymentID)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil).Once()

		err = DeleteDeploymentAPIToken("", deploymentID, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path - RemoveDeploymentApiToken no apiToken id passed error on ListDeploymentApiTokensWithResponse", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()

		err := DeleteDeploymentAPIToken("", deploymentID, out, mockClient)
		assert.EqualError(t, err, "failed to list deployment apiTokens")
	})
}

func TestListDeploymentAPITokens(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path TestListDeploymentApiTokens", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		err := ListDeploymentAPITokens(out, mockClient, deploymentID)
		assert.NoError(t, err)
	})

	t.Run("error path when ListDeploymentApiTokensWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListDeploymentAPITokens(out, mockClient, deploymentID)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListDeploymentApiTokensWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()
		err := ListDeploymentAPITokens(out, mockClient, deploymentID)
		assert.EqualError(t, err, "failed to list deployment apiTokens")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListDeploymentAPITokens(out, mockClient, deploymentID)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}
