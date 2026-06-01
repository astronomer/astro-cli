package deployment

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	testValue1 = "test-value-1"
	testValue2 = "test-value-2"
	testValue3 = "test-value-3"
	testValue4 = "test-value=4"
)

func TestVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	MockResponseInit()
	variableValue := "test-value-1"
	t.Run("success", func(t *testing.T) {
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
			{
				Key:      "test-key-1",
				Value:    &variableValue,
				IsSecret: false,
			},
		}

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")

		err = VariableList("test-id-1", "", ws, "", "", false, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "0")()

		buf := new(bytes.Buffer)
		err := VariableList("", "test-key-1", ws, "", "", false, mockV1Client, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid variable key", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "1")()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-invalid-key", ws, "", "", false, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No variables found")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockV1Client, buf)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid file", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "\000x", "", true, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "unable to write environment variables to file")
	})
}

func TestVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	MockResponseInit()
	cloudProvider := astrov1.DeploymentCloudProviderGCP
	mockUpdateDeploymentResponse := astrov1.UpdateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			Region:        &cluster.Region,
			ClusterName:   &cluster.Name,
			EnvironmentVariables: &[]astrov1.DeploymentEnvironmentVariable{
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
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-key-2")
		assert.Contains(t, buf.String(), "test-value-1")
		assert.Contains(t, buf.String(), "test-value-2")
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("success with secret value", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, true, mockV1Client, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-key-2")
		assert.Contains(t, buf.String(), "****")
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		buf := new(bytes.Buffer)
		err := VariableModify("test-invalid-id", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.ErrorIs(t, err, errInvalidDeployment)

		defer testUtil.MockUserInput(t, "0")()

		err = VariableModify("", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("missing var key or value", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.Contains(t, buf.String(), "You must provide a variable key")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")

		buf = new(bytes.Buffer)
		err = VariableModify("test-id-1", "test-key-2", "", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.Contains(t, buf.String(), "You must provide a variable value")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("create env var failure", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("no env var for deployment", func(t *testing.T) {
		deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{}
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		buf := new(bytes.Buffer)
		_ = VariableModify("test-id-2", "", "", ws, "", "", []string{}, false, false, false, mockV1Client, buf)
		assert.Contains(t, buf.String(), "No variables for this Deployment")
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-2", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, true, false)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}, {Key: "test-key-1", Value: &testValue1}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, true, false)

	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue1}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file-wrong", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)
}

func TestWriteVarToFile(t *testing.T) {
	testFile := "temp-test-env-file"

	for _, tc := range []struct {
		Var      astrov1.DeploymentEnvironmentVariable
		Expected string
	}{
		{astrov1.DeploymentEnvironmentVariable{Key: "test-key-1", Value: &testValue1}, "test-key-1=" + testValue1},
		{astrov1.DeploymentEnvironmentVariable{Key: "test-key-1", Value: nil, IsSecret: true}, "test-key-1= # secret"},
	} {
		t.Run(tc.Var.Key, func(t *testing.T) {
			defer func() { os.Remove(testFile) }()
			err := writeVarToFile([]astrov1.DeploymentEnvironmentVariable{tc.Var}, testFile)
			assert.NoError(t, err)
			contents, err := os.ReadFile(testFile)
			require.NoError(t, err)
			assert.Equal(t, "\n"+tc.Expected, string(contents))
		})
	}
}

func TestAddVariable(t *testing.T) {
	buf := new(bytes.Buffer)
	resp := addVariable([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", true, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	buf = new(bytes.Buffer)
	resp = addVariable([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", false, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)
}

func TestAddVariablesFromArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	resp := addVariablesFromArgs([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, true, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, false, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{},
		[]astrov1.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value-3", "test-key-3=", "test-key-3"}, false, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{},
		[]astrov1.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value=4", "test-key-3=", "test-key-3"}, false, false, buf,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue4}}, resp)
}
