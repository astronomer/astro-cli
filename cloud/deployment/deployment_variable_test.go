package deployment

import (
	"bytes"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockResponse := []astro.Deployment{
		{
			ID: "test-id-1",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
			},
		},
		{
			ID: "test-id-2",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-2", Value: "test-value-2"}},
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")

		err = VariableList("test-id-1", "", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-value-1")
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableList("test-invalid-id", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errInvalidDeployment)

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = VariableList("", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid variable key", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-invalid-key", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No variables found")
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid file", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "\000x", "", true, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "unable to write environment variables to file")
	})
}

func TestVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockListResponse := []astro.Deployment{
		{
			ID: "test-id-1",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
			},
		},
		{
			ID: "test-id-2",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{},
			},
		},
	}

	mockCreateResponse := []astro.EnvironmentVariablesObject{
		{
			Key:   "test-key-1",
			Value: "test-value-1",
		},
		{
			Key:   "test-key-2",
			Value: "test-value-2",
		},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockCreateResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, true, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-key-1")
		assert.Contains(t, buf.String(), "test-key-2")
		assert.Contains(t, buf.String(), "test-value-1")
		assert.Contains(t, buf.String(), "test-value-2")
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableModify("test-invalid-id", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errInvalidDeployment)

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = VariableModify("", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})

	t.Run("missing var key or value", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Twice()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockCreateResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, buf.String(), "You must provide a variable key")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")

		buf = new(bytes.Buffer)
		err = VariableModify("test-id-1", "test-key-2", "", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, buf.String(), "You must provide a variable value")
		assert.Contains(t, err.Error(), "there was an error while creating or updating one or more of the environment variables")

		mockClient.AssertExpectations(t)
	})

	t.Run("create env var failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("no env var for deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil).Once()

		buf := new(bytes.Buffer)
		_ = VariableModify("test-id-2", "", "", ws, "", "", []string{}, false, false, false, mockCoreClient, mockPlatformCoreClient, buf)

		assert.Contains(t, buf.String(), "No variables for this Deployment")
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

	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-2", Value: "test-value-3"}, {Key: "test-key-1", Value: "test-value-1"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, true, false)

	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-1"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file-wrong", []string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false)

	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)
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
	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	buf = new(bytes.Buffer)
	resp = addVariable([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", false, false, buf,
	)
	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}}, resp)
}

func TestAddVariablesFromArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	resp := addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, true, false, buf,
	)
	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, false, false, buf,
	)
	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astroplatformcore.DeploymentEnvironmentVariable{},
		[]astroplatformcore.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value-3", "test-key-3=", "test-key-3"}, false, false, buf,
	)
	assert.Equal(t, []astro.EnvironmentVariable{{Key: "test-key-2", Value: "test-value-3"}}, resp)
}
