package deployment

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	testValue1 = "test-value-1"
	testValue2 = "test-value-2"
	testValue3 = "test-value-3"
)

func TestVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	variableValue := "test-value-1"
	t.Run("success", func(t *testing.T) {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
			{
				Key:      "test-key-1",
				Value:    &variableValue,
				IsSecret: false,
			},
		}

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")

		err = VariableList("test-id-1", "", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "0")()

		buf := new(bytes.Buffer)
		err := VariableList("", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid variable key", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "1")()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-invalid-key", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No variables found")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid file", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "\000x", "", true, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "unable to write environment variables to file")
	})
}

func TestVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cloudProvider := "test-provider"
	mockUpdateDeploymentResponse := astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			Region:        &cluster.Region,
			ClusterName:   &cluster.Name,
			EnvironmentVariables: &[]astroplatformcore.DeploymentEnvironmentVariable{
				{
					Key:   "test-key-1",
					Value: &testValue1,
				},
				{
					Key:   "test-key-2",
					Value: &testValue2,
				},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	t.Run("success", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
			{
				Key:      "test-key-1",
				Value:    &testValue1,
				IsSecret: false,
			},
			{
				Key:      "test-key-2",
				Value:    &testValue2,
				IsSecret: false,
			},
		}

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-key-2")
		assert.Contains(t, buf.String(), "test-value-1")
		assert.Contains(t, buf.String(), "test-value-2")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with secret value", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
			{
				Key:      "test-key-1",
				Value:    nil,
				IsSecret: true,
			},
			{
				Key:      "test-key-2",
				Value:    nil,
				IsSecret: true,
			},
		}

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-key-2")
		assert.Contains(t, buf.String(), "****")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		buf := new(bytes.Buffer)
		err := VariableModify("test-invalid-id", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errInvalidDeployment)

		defer testUtil.MockUserInput(t, "0")()

		err = VariableModify("", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("missing var key or value", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, buf.String(), "You must provide a variable key")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")

		buf = new(bytes.Buffer)
		err = VariableModify("test-id-1", "test-key-2", "", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, buf.String(), "You must provide a variable value")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("create env var failure", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("no env var for deployment", func(t *testing.T) {
		deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{}
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		_ = VariableModify("test-id-2", "", "", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, buf.String(), "No variables for this Deployment")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestContains(t *testing.T) {
	resp, idx := contains([]string{"test-1", "test-2"}, "test-1")
	assert.True(t, resp)
	assert.Equal(t, 0, idx)
}

func TestReadLines(t *testing.T) {
	resp, err := readLines("./testfiles/test-env-file")
	assert.Contains(t, resp, "test-key-1=test-value-1")
	assert.NoError(t, err)
}

func TestAddVariableFromFile(t *testing.T) {
	resp := addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-2"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-2", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, true, false)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}, {Key: "test-key-1", Value: &testValue1}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, true, false)

	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue1}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file-wrong", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)
}

func TestWriteVarToFile(t *testing.T) {
	testFile := "temp-test-env-file"
	defer func() { os.Remove(testFile) }()

	err := writeVarToFile([]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}}, "test-key-1", testFile)
	assert.NoError(t, err)

	err = writeVarToFile([]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}}, "", testFile)
	assert.NoError(t, err)
}

func TestAddVariable(t *testing.T) {
	buf := new(bytes.Buffer)
	resp := addVariable([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", true, false, buf,
	)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	buf = new(bytes.Buffer)
	resp = addVariable([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", false, false, buf,
	)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)
}

func TestAddVariablesFromArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	resp := addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, true, false, buf,
	)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, false, false, buf,
	)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value-3", "test-key-3=", "test-key-3"}, false, false, buf,
	)
	assert.Equal(t, []astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, resp)
}
