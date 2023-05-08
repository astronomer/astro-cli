package deployment

import (
	"bytes"
	"os"

	astro "github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

func (s *Suite) TestVariableList() {
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

	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-key-1")
		s.Contains(buf.String(), "test-value-1")

		err = VariableList("test-id-1", "", ws, "", "", false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-key-1")
		s.Contains(buf.String(), "test-value-1")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid deployment", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableList("test-invalid-id", "test-key-1", ws, "", "", false, mockClient, buf)
		s.ErrorIs(err, errInvalidDeployment)

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = VariableList("", "test-key-1", ws, "", "", false, mockClient, buf)
		s.ErrorIs(err, ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid variable key", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-invalid-key", ws, "", "", false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "No variables found")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list deployment failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockClient, buf)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid file", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableList("test-id-1", "test-key-1", ws, "\000x", "", true, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "unable to write environment variables to file")
	})
}

func (s *Suite) TestVariableModify() {
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

	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockCreateResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, true, false, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-key-1")
		s.Contains(buf.String(), "test-key-2")
		s.Contains(buf.String(), "test-value-1")
		s.Contains(buf.String(), "test-value-2")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list deployment failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid deployment", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableModify("test-invalid-id", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.ErrorIs(err, errInvalidDeployment)

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = VariableModify("", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.ErrorIs(err, ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("missing var key or value", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Twice()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockCreateResponse, nil).Twice()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "", "test-value-2", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "You must provide a variable key")

		buf = new(bytes.Buffer)
		err = VariableModify("test-id-1", "test-key-2", "", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "You must provide a variable value")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("create env var failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("no env var for deployment", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return(mockListResponse, nil).Once()
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil).Once()

		buf := new(bytes.Buffer)
		err := VariableModify("test-id-2", "", "", ws, "", "", []string{}, false, false, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "No variables for this Deployment")
	})
}

func (s *Suite) TestContains() {
	resp, idx := contains([]string{"test-1", "test-2"}, "test-1")
	s.True(resp)
	s.Equal(0, idx)
}

func (s *Suite) TestReadLines() {
	resp, err := readLines("./testfiles/test-env-file")
	s.Contains(resp, "test-key-1=test-value-1")
	s.NoError(err)
}

func (s *Suite) TestAddVariableFromFile() {
	resp := addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-2"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-2", Value: "test-value-2"}},
		[]astro.EnvironmentVariable{{Key: "test-key-2", Value: "test-value-3"}}, true, false)

	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-2", Value: "test-value-3"}, {Key: "test-key-1", Value: "test-value-1"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-2"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, true, false)

	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-1"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-2"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, false, false)

	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	resp = addVariablesFromFile(
		"./testfiles/test-env-file-wrong", []string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-2"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, false, false)

	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)
}

func (s *Suite) TestWriteVarToFile() {
	testFile := "temp-test-env-file"
	defer func() { os.Remove(testFile) }()

	err := writeVarToFile([]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value1"}}, "test-key-1", testFile)
	s.NoError(err)

	err = writeVarToFile([]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value1"}}, "", testFile)
	s.NoError(err)
}

func (s *Suite) TestAddVariable() {
	buf := new(bytes.Buffer)
	resp := addVariable([]string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}},
		"test-key-1", "test-value-3", true, false, buf,
	)
	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	buf = new(bytes.Buffer)
	resp = addVariable([]string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}},
		"test-key-1", "test-value-3", false, false, buf,
	)
	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}}, resp)
}

func (s *Suite) TestAddVariablesFromArgs() {
	buf := new(bytes.Buffer)
	resp := addVariablesFromArgs([]string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}},
		[]string{"test-key-1=test-value-3"}, true, false, buf,
	)
	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-3"}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
		[]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}},
		[]string{"test-key-1=test-value-3"}, false, false, buf,
	)
	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-1", Value: "test-value-2"}}, resp)

	resp = addVariablesFromArgs([]string{"test-key-1"},
		[]astro.EnvironmentVariablesObject{},
		[]astro.EnvironmentVariable{},
		[]string{"test-key-2=test-value-3", "test-key-3=", "test-key-3"}, false, false, buf,
	)
	s.Equal([]astro.EnvironmentVariable{{Key: "test-key-2", Value: "test-value-3"}}, resp)
}
