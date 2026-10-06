package astro

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The `--output json` contract of `astro deployment variable`. The objects
// below are pinned byte for byte: scripts, agents and Astro Desktop parse
// them, so a change to one of these strings is a change to that contract.

// variableRun is what one run of a `deployment variable` command left behind.
type variableRun struct {
	stdout, stderr string
	code           int
}

// runVariableCmd runs `deployment <args>` the way main does — through
// cliout.Execute, which reports failures and owns the exit code — with the
// process's real stdout and stderr swapped for files. The command's writer is
// that same stdout file, as it is in production, so anything that reaches
// stdout by any route, bare fmt.Print in the platform code included, lands in
// what the test reads.
func runVariableCmd(t *testing.T, args ...string) variableRun {
	t.Helper()
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)

	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	root := newDeploymentRootCmd(stdout)
	root.SetOut(stdout)
	root.SetErr(stderr)
	ctx := context.Background()
	runErr := cliout.Execute(ctx, root, args, stdout, nil)
	os.Stdout, os.Stderr = savedOut, savedErr
	require.NoError(t, stdout.Close())
	require.NoError(t, stderr.Close())

	outBytes, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	errBytes, err := os.ReadFile(stderr.Name())
	require.NoError(t, err)
	return variableRun{stdout: string(outBytes), stderr: string(errBytes), code: cliout.ExitCode(ctx, runErr)}
}

// withDeploymentVariables gives the shared mock Deployment these variables for
// the length of the test.
func withDeploymentVariables(t *testing.T, vars *[]astrov1.DeploymentEnvironmentVariable) {
	t.Helper()
	saved := deploymentResponse.JSON200.EnvironmentVariables
	deploymentResponse.JSON200.EnvironmentVariables = vars
	t.Cleanup(func() { deploymentResponse.JSON200.EnvironmentVariables = saved })
}

// twoVariables is a Deployment with one plain variable and one secret. The API
// returns a secret with its value withheld; the mock returns one anyway, to
// prove the json never carries it.
func twoVariables() *[]astrov1.DeploymentEnvironmentVariable {
	plain, secret := "test-value-1", "do-not-print"
	return &[]astrov1.DeploymentEnvironmentVariable{
		{Key: "test-key-1", Value: &plain},
		{Key: "SECRET_KEY", Value: &secret, IsSecret: true},
	}
}

// listMocks wires what `variable list` asks the API.
func listMocks() *astrov1_mocks.ClientWithResponsesInterface {
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
	return m
}

// modifyMocks wires what a create or update asks the API: find the
// Deployment, update it, read it back.
func modifyMocks() *astrov1_mocks.ClientWithResponsesInterface {
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
	m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
	m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Once()
	return m
}

func TestDeploymentVariableListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("variables", func(t *testing.T) {
		withDeploymentVariables(t, twoVariables())
		m := listMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t,
			`{"variables":[{"key":"test-key-1","value":"test-value-1","is_secret":false},{"key":"SECRET_KEY","value":null,"is_secret":true}]}`+"\n",
			run.stdout)
		assert.Empty(t, run.stderr)
		m.AssertExpectations(t)
	})

	// None is an empty array, not null and not a missing key.
	t.Run("none", func(t *testing.T) {
		withDeploymentVariables(t, nil)
		m := listMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t, `{"variables":[]}`+"\n", run.stdout)
		m.AssertExpectations(t)
	})

	// --save's notice is a note, not the result: under json it goes to stderr.
	t.Run("save", func(t *testing.T) {
		withDeploymentVariables(t, twoVariables())
		m := listMocks()
		astroV1Client = m
		envFile := filepath.Join(t.TempDir(), ".env")

		run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "--save", "--env", envFile, "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t,
			`{"variables":[{"key":"test-key-1","value":"test-value-1","is_secret":false},{"key":"SECRET_KEY","value":null,"is_secret":true}]}`+"\n",
			run.stdout)
		assert.Contains(t, run.stderr, "were saved to the file "+envFile)
		m.AssertExpectations(t)
	})

	// The platform code prints a note on bare stdout when both an id and a
	// name are given. Under json it must not reach stdout.
	t.Run("stray platform note goes to stderr", func(t *testing.T) {
		withDeploymentVariables(t, nil)
		m := listMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "--deployment-name", "test", "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t, `{"variables":[]}`+"\n", run.stdout)
		assert.Contains(t, run.stderr, "Both a Deployment ID and Deployment name have been supplied")
		m.AssertExpectations(t)
	})
}

// In text mode the same note stays on stdout, where it has always been, ahead
// of the table.
func TestDeploymentVariableListTextUnchanged(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	withDeploymentVariables(t, twoVariables())
	m := listMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "--deployment-name", "test")

	assert.Equal(t, 0, run.code)
	assert.Equal(t, "Both a Deployment ID and Deployment name have been supplied. The Deployment ID test-id-1 will be used\n"+
		" #     KEY            VALUE            SECRET     \n"+
		" 1     test-key-1     test-value-1     false      \n"+
		" 2     SECRET_KEY     ****             true       \n", run.stdout)
	assert.Empty(t, run.stderr)
	m.AssertExpectations(t)
}

func TestDeploymentVariableCreateJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// One input of each outcome a create can have. The run fails, and under
	// json the result is still the one object on stdout: it is what says which
	// inputs failed, and the root adds no error object after it.
	t.Run("partial failure", func(t *testing.T) {
		withDeploymentVariables(t, twoVariables())
		m := modifyMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "create", "test-key-1=again", "NEW=value", "MALFORMED", "=nokey",
			"--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 1, run.code)
		assert.Equal(t, `{"outcomes":[`+
			`{"kind":"skipped_exists","key":"test-key-1","reason":"already set; use the update command to change it"},`+
			`{"kind":"created","key":"NEW"},`+
			`{"kind":"invalid","input":"MALFORMED","reason":"not a key=value pair"},`+
			`{"kind":"invalid","input":"=nokey","reason":"blank key or value"}],`+
			`"variables":[{"key":"test-key-1","value":"test-value-1","is_secret":false},{"key":"SECRET_KEY","value":null,"is_secret":true}]}`+"\n",
			run.stdout)
		assert.Empty(t, run.stderr)
		m.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		withDeploymentVariables(t, twoVariables())
		m := modifyMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "create", "NEW=value", "--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t, `{"outcomes":[{"kind":"created","key":"NEW"}],`+
			`"variables":[{"key":"test-key-1","value":"test-value-1","is_secret":false},{"key":"SECRET_KEY","value":null,"is_secret":true}]}`+"\n",
			run.stdout)
		assert.Empty(t, run.stderr)
		m.AssertExpectations(t)
	})

	// A Deployment with no variables, asked to create nothing usable: both
	// lists are arrays, and the Deployment table the platform's Update prints
	// for an empty list does not reach stdout.
	t.Run("empty", func(t *testing.T) {
		withDeploymentVariables(t, nil)
		m := modifyMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "create", "--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 0, run.code)
		assert.Equal(t, `{"outcomes":[],"variables":[]}`+"\n", run.stdout)
		assert.Contains(t, run.stderr, "Successfully updated Deployment")
		m.AssertExpectations(t)
	})
}

// The same partial failure in text mode: the per-input lines and the table on
// stdout, as before, the error on stderr, and exit 1.
func TestDeploymentVariableCreateTextPartialFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	withDeploymentVariables(t, twoVariables())
	m := modifyMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "create", "test-key-1=again", "NEW=value", "MALFORMED", "=nokey",
		"--deployment-id", "test-id-1")

	assert.Equal(t, 1, run.code)
	assert.Equal(t, "key test-key-1 already exists, skipping creation. Use the update command to update existing variables\n"+
		"adding variable NEW\n"+
		"MALFORMED not created or updated: not a key=value pair\n"+
		"=nokey not created or updated: blank key or value\n"+
		"\nUpdated list of your Deployment's variables:\n"+
		" #     KEY            VALUE            SECRET     \n"+
		" 1     test-key-1     test-value-1     false      \n"+
		" 2     SECRET_KEY     ****             true       \n", run.stdout)
	assert.Equal(t, "Error: 2 variables were not created or updated: \"MALFORMED\", \"=nokey\"\n", run.stderr)
	m.AssertExpectations(t)
}

func TestDeploymentVariableUpdateJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	withDeploymentVariables(t, twoVariables())
	m := modifyMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "update", "test-key-1=updated", "NEW=value", "B=",
		"--deployment-id", "test-id-1", "-o", "json")

	assert.Equal(t, 1, run.code)
	assert.Equal(t, `{"outcomes":[`+
		`{"kind":"updated","key":"test-key-1"},`+
		`{"kind":"created","key":"NEW"},`+
		`{"kind":"invalid","key":"B","input":"B=","reason":"blank key or value"}],`+
		`"variables":[{"key":"test-key-1","value":"test-value-1","is_secret":false},{"key":"SECRET_KEY","value":null,"is_secret":true}]}`+"\n",
		run.stdout)
	assert.Empty(t, run.stderr)
	m.AssertExpectations(t)
}

// A bad --output is a usage error (exit 2) before anything asks the API. It is
// not json, so it is reported in text, with the usage, as any usage error is.
func TestDeploymentVariableOutputRejectsUnknownFormat(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = m

	for _, sub := range []string{"list", "create", "update"} {
		run := runVariableCmd(t, "variable", sub, "--deployment-id", "test-id-1", "-o", "yaml")
		assert.Equal(t, cliout.ExitUsage, run.code, sub)
		assert.Contains(t, run.stderr, `Error: unknown output format "yaml" (supported: text, json)`, sub)
	}
	m.AssertExpectations(t)
}
