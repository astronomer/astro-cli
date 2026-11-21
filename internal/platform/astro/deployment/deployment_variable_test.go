package deployment

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
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

		got, err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, []VariableInfo{{Key: "test-key-1", Value: "test-value-1"}}, got.Variables)

		got, err = VariableList("test-id-1", "", ws, "", "", false, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, []VariableInfo{{Key: "test-key-1", Value: "test-value-1"}}, got.Variables)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "0")()

		_, err := VariableList("", "test-key-1", ws, "", "", false, mockV1Client)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid variable key", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "1")()

		got, err := VariableList("test-id-1", "test-invalid-key", ws, "", "", false, mockV1Client)
		assert.NoError(t, err)
		assert.Empty(t, got.Variables)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		_, err := VariableList("test-id-1", "test-key-1", ws, "", "", false, mockV1Client)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid file", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		_, err := VariableList("test-id-1", "test-key-1", ws, "\000x", "", true, mockV1Client)
		assert.ErrorContains(t, err, "could not write environment variables to")
	})
}

func TestVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		res, err := VariableModify(context.Background(), "test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.NoError(t, err)
		// test-key-2 is already on the Deployment and this call passes
		// updateVars=false, so it is skipped. The old test read as an update
		// because it asserted on the post-update list, which carries the key
		// either way.
		assert.Equal(t, []VariableOutcome{
			{Kind: VariableSkippedExists, Key: "test-key-2", Reason: "already set; use the update command to change it"},
		}, res.Outcomes)
		assert.Equal(t, []VariableInfo{
			{Key: "test-key-1", Value: "test-value-1"},
			{Key: "test-key-2", Value: "test-value-2"},
		}, res.Variables)
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

		res, err := VariableModify(context.Background(), "test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, true, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, []VariableOutcome{{Kind: VariableUpdated, Key: "test-key-2"}}, res.Outcomes)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list deployment failure", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		_, err := VariableModify(context.Background(), "test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid deployment", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		_, err := VariableModify(context.Background(), "test-invalid-id", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.ErrorIs(t, err, errInvalidDeployment)

		defer testUtil.MockUserInput(t, "0")()

		_, err = VariableModify(context.Background(), "", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("missing var key or value", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		res, err := VariableModify(context.Background(), "test-id-1", "", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(res.InvalidInputs()))
		assert.Equal(t, VariableInvalid, res.Outcomes[0].Kind)
		assert.Contains(t, res.Outcomes[0].Reason, "no key given")

		res, err = VariableModify(context.Background(), "test-id-1", "test-key-2", "", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(res.InvalidInputs()))
		assert.Equal(t, "test-key-2", res.Outcomes[0].Key)
		assert.Contains(t, res.Outcomes[0].Reason, "no value given")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("create env var failure", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		_, err := VariableModify(context.Background(), "test-id-1", "test-key-2", "test-value-2", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.ErrorIs(t, err, errMock)
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

		res, err := VariableModify(context.Background(), "test-id-2", "", "", ws, "", "", []string{}, false, false, false, mockV1Client)
		assert.NoError(t, err)
		assert.Empty(t, res.Variables)
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
	res := &VariableModifyResult{}
	resp := addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-2"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-2", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, true, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}, {Key: "test-key-1", Value: &testValue1}}, resp)
	assert.Equal(t, []VariableOutcome{{Kind: VariableCreated, Key: "test-key-1"}}, res.Outcomes)

	res = &VariableModifyResult{}
	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, true, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue1}}, resp)
	assert.Equal(t, []VariableOutcome{{Kind: VariableUpdated, Key: "test-key-1"}}, res.Outcomes)

	// Without --update an existing file key keeps its value and, as before
	// this shape, fails the run: it is an invalid outcome, which picks the
	// exit code, not a skip.
	res = &VariableModifyResult{}
	resp = addVariablesFromFile(
		"./testfiles/test-env-file", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)
	assert.Equal(t, VariableInvalid, res.Outcomes[0].Kind)
	assert.Equal(t, []string{"test-key-1"}, res.InvalidInputs())

	// A malformed file yields no variable and one invalid outcome naming the
	// line, which the old shape only printed.
	res = &VariableModifyResult{}
	resp = addVariablesFromFile(
		"./testfiles/test-env-file-wrong", []string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue2}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)
	assert.Positive(t, len(res.InvalidInputs()))
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
	res := &VariableModifyResult{}
	resp := addVariable(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", true, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)
	assert.Equal(t, []VariableOutcome{{Kind: VariableUpdated, Key: "test-key-1"}}, res.Outcomes)

	res = &VariableModifyResult{}
	resp = addVariable(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		"test-key-1", "test-value-3", false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)
	assert.Equal(t, VariableSkippedExists, res.Outcomes[0].Kind)
}

func TestAddVariablesFromArgs(t *testing.T) {
	res := &VariableModifyResult{}
	resp := addVariablesFromArgs(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, true, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue3}}, resp)

	res = &VariableModifyResult{}
	resp = addVariablesFromArgs(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &testValue1}},
		[]astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}},
		[]string{"test-key-1=test-value-3"}, false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-1", Value: &testValue2}}, resp)

	res = &VariableModifyResult{}
	resp = addVariablesFromArgs(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{},
		[]astrov1.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value-3", "test-key-3=", "test-key-3"}, false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue3}}, resp)

	res = &VariableModifyResult{}
	resp = addVariablesFromArgs(
		[]string{"test-key-1"},
		[]astrov1.DeploymentEnvironmentVariable{},
		[]astrov1.DeploymentEnvironmentVariableRequest{},
		[]string{"test-key-2=test-value=4", "test-key-3=", "test-key-3"}, false, false, res,
	)
	assert.Equal(t, []astrov1.DeploymentEnvironmentVariableRequest{{Key: "test-key-2", Value: &testValue4}}, resp)
}

// A rejected file line can carry a secret's value. The outcome, which the
// error and the rendered output both draw on, names the key or the line
// number, never the line itself.
func TestAddVariablesFromFileDoesNotEchoValues(t *testing.T) {
	envFile := t.TempDir() + "/.env"
	require.NoError(t, os.WriteFile(envFile, []byte("DB_PASSWORD=hunter2\nDB_PASSWORD=hunter2\n=hunter2\n"), 0o600))

	res := &VariableModifyResult{}
	addVariablesFromFile(envFile, nil, nil, nil, false, true, res)

	require.Len(t, res.Outcomes, 3)
	assert.Equal(t, VariableCreated, res.Outcomes[0].Kind)
	assert.Equal(t, []string{"DB_PASSWORD", envFile + " line 3"}, res.InvalidInputs())
	for _, o := range res.Outcomes {
		assert.NotContains(t, o.Input+o.Key+o.Reason, "hunter2")
	}
}

// An existing .env keeps what it held and gains the variables after it, and
// keeps the mode its owner gave it.
func TestWriteVarToFileKeepsAnExistingMode(t *testing.T) {
	path := t.TempDir() + "/.env"
	require.NoError(t, os.WriteFile(path, []byte("KEEP=1"), 0o644))
	err := writeVarToFile([]astrov1.DeploymentEnvironmentVariable{
		{Key: "A", Value: &testValue1},
		{Key: "S", IsSecret: true},
	}, path)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=1\nA="+testValue1+"\nS= # secret", string(got))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
}

// A .env this run creates is the user's alone: 0600.
func TestWriteVarToFileCreatesPrivately(t *testing.T) {
	path := t.TempDir() + "/.env"
	require.NoError(t, writeVarToFile([]astrov1.DeploymentEnvironmentVariable{{Key: "A", Value: &testValue1}}, path))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

// A symlinked .env is written through, not replaced by a file of its own.
func TestWriteVarToFileWritesThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	target := dir + "/real.env"
	link := dir + "/.env"
	require.NoError(t, os.WriteFile(target, []byte("KEEP=1"), 0o640))
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, writeVarToFile([]astrov1.DeploymentEnvironmentVariable{{Key: "A", Value: &testValue1}}, link))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=1\nA="+testValue1, string(got))
	tinfo, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), tinfo.Mode().Perm())
}

// A failure to publish leaves the earlier file as it was.
func TestWriteVarToFileLeavesTheFileOnFailure(t *testing.T) {
	// A directory where the file should go cannot be read as one.
	err := writeVarToFile([]astrov1.DeploymentEnvironmentVariable{{Key: "A", Value: &testValue1}}, t.TempDir())
	assert.Error(t, err)
}
