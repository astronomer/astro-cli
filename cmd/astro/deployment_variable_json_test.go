package astro

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// What `astro deployment variable` does, in both formats.
//
// The json shape is pinned once, by the goldens in testdata/schema
// (deployment-variable-*.json, `make update-schemas`). These tests decode what
// a run printed and assert what it means: the exit code, the outcome each
// input got, the variables listed, a secret's value withheld. In text they
// assert that the messages and the rows are there, not how the table pads
// them: the table is for a person, and nothing parses its spacing.

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

// The json as a consumer reads it. Value is a pointer because a secret's is
// null, and that is part of what the tests check.
type (
	variableJSON struct {
		Key      string  `json:"key"`
		Value    *string `json:"value"`
		IsSecret bool    `json:"is_secret"`
	}
	outcomeJSON struct {
		Kind   string `json:"kind"`
		Key    string `json:"key"`
		Input  string `json:"input"`
		Reason string `json:"reason"`
	}
	variableListJSON struct {
		Variables []variableJSON `json:"variables"`
	}
	variableModifyJSON struct {
		Outcomes  []outcomeJSON  `json:"outcomes"`
		Variables []variableJSON `json:"variables"`
	}
)

// decodeOne decodes stdout into v, failing unless it holds exactly one json
// object and nothing after it. It returns the object's fields undecoded, for
// a test that needs to tell [] from null.
func decodeOne(t *testing.T, stdout string, v any) map[string]json.RawMessage {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var fields map[string]json.RawMessage
	require.NoError(t, dec.Decode(&fields), "stdout is not a json object:\n%s", stdout)
	_, err := dec.Token()
	require.True(t, errors.Is(err, io.EOF), "stdout holds more than one json value:\n%s", stdout)
	require.NoError(t, json.Unmarshal([]byte(stdout), v))
	return fields
}

// twoVariablesJSON is what twoVariables publishes: the plain value, and null
// for the secret, whose value the mock returned and the json must not carry.
func twoVariablesJSON() []variableJSON {
	plain := "test-value-1"
	return []variableJSON{
		{Key: "test-key-1", Value: &plain},
		{Key: "SECRET_KEY", Value: nil, IsSecret: true},
	}
}

// requireRow fails unless some line of out is a table row holding exactly
// these cells, however the table pads them.
func requireRow(t *testing.T, out string, cells ...string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if assert.ObjectsAreEqual(cells, strings.Fields(line)) {
			return
		}
	}
	t.Fatalf("no row %q in:\n%s", cells, out)
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
// prove neither format ever prints it.
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
		var got variableListJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, twoVariablesJSON(), got.Variables)
		assert.NotContains(t, run.stdout, "do-not-print")
		assert.Empty(t, run.stderr)
		m.AssertExpectations(t)
	})

	// None is an empty array, not null and not a missing key. Byte for byte,
	// because this is the one thing about the encoding the golden cannot say:
	// it pins a populated value, indented, while Emit writes one compact line
	// that a script can read with a line-at-a-time reader.
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
		var got variableListJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, twoVariablesJSON(), got.Variables)
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
		var got variableListJSON
		decodeOne(t, run.stdout, &got)
		assert.Empty(t, got.Variables)
		assert.Contains(t, run.stderr, "Both a Deployment ID and Deployment name have been supplied")
		m.AssertExpectations(t)
	})
}

// In text mode the same note stays on stdout, where it has always been, ahead
// of the table, and a secret shows masked.
func TestDeploymentVariableListTextUnchanged(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	withDeploymentVariables(t, twoVariables())
	m := listMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1", "--deployment-name", "test")

	assert.Equal(t, 0, run.code)
	assert.True(t, strings.HasPrefix(run.stdout,
		"Both a Deployment ID and Deployment name have been supplied. The Deployment ID test-id-1 will be used\n"),
		"the note heads stdout:\n%s", run.stdout)
	requireRow(t, run.stdout, "#", "KEY", "VALUE", "SECRET")
	requireRow(t, run.stdout, "1", "test-key-1", "test-value-1", "false")
	requireRow(t, run.stdout, "2", "SECRET_KEY", maskedSecret, "true")
	assert.NotContains(t, run.stdout, "do-not-print")
	assert.Empty(t, run.stderr)
	m.AssertExpectations(t)
}

// A value holding a line break stays on its own row: the break prints as a
// space, so the cells after it are not pushed onto a line of their own.
func TestDeploymentVariableListTextKeepsAValueOnItsRow(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	multiline := "first\nsecond"
	withDeploymentVariables(t, &[]astrov1.DeploymentEnvironmentVariable{{Key: "PEM", Value: &multiline}})
	m := listMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "list", "--deployment-id", "test-id-1")

	assert.Equal(t, 0, run.code)
	requireRow(t, run.stdout, "1", "PEM", "first", "second", "false")
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
		var got variableModifyJSON
		fields := decodeOne(t, run.stdout, &got)
		assert.NotContains(t, fields, "error", "the result is the report; no error object follows it")
		assert.Equal(t, []outcomeJSON{
			{Kind: "skipped_exists", Key: "test-key-1", Reason: "already set; use the update command to change it"},
			{Kind: "created", Key: "NEW"},
			{Kind: "invalid", Input: "MALFORMED", Reason: "not a key=value pair"},
			{Kind: "invalid", Input: "=nokey", Reason: "blank key or value"},
		}, got.Outcomes, "one outcome per input, in the order given")
		assert.Equal(t, twoVariablesJSON(), got.Variables)
		assert.Empty(t, run.stderr)
		m.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		withDeploymentVariables(t, twoVariables())
		m := modifyMocks()
		astroV1Client = m

		run := runVariableCmd(t, "variable", "create", "NEW=value", "--deployment-id", "test-id-1", "-o", "json")

		assert.Equal(t, 0, run.code)
		var got variableModifyJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, []outcomeJSON{{Kind: "created", Key: "NEW"}}, got.Outcomes)
		assert.Equal(t, twoVariablesJSON(), got.Variables)
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
		var got variableModifyJSON
		fields := decodeOne(t, run.stdout, &got)
		assert.JSONEq(t, `[]`, string(fields["outcomes"]), "outcomes is an empty array, not null")
		assert.JSONEq(t, `[]`, string(fields["variables"]), "variables is an empty array, not null")
		assert.Contains(t, run.stderr, "Successfully updated Deployment")
		m.AssertExpectations(t)
	})
}

// The same partial failure in text mode: a line per input and the table on
// stdout, as before, the error on stderr, and exit 1.
func TestDeploymentVariableCreateTextPartialFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	withDeploymentVariables(t, twoVariables())
	m := modifyMocks()
	astroV1Client = m

	run := runVariableCmd(t, "variable", "create", "test-key-1=again", "NEW=value", "MALFORMED", "=nokey",
		"--deployment-id", "test-id-1")

	assert.Equal(t, 1, run.code)
	assert.True(t, strings.HasPrefix(run.stdout,
		"key test-key-1 already exists, skipping creation. Use the update command to update existing variables\n"+
			"adding variable NEW\n"+
			"MALFORMED not created or updated: not a key=value pair\n"+
			"=nokey not created or updated: blank key or value\n"),
		"one line per input, in the order given, ahead of the table:\n%s", run.stdout)
	assert.Contains(t, run.stdout, "Updated list of your Deployment's variables:")
	requireRow(t, run.stdout, "1", "test-key-1", "test-value-1", "false")
	requireRow(t, run.stdout, "2", "SECRET_KEY", maskedSecret, "true")
	assert.Contains(t, run.stderr, `Error: 2 variables were not created or updated: "MALFORMED", "=nokey"`)
	assert.NotContains(t, run.stdout, "Error:")
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
	var got variableModifyJSON
	decodeOne(t, run.stdout, &got)
	assert.Equal(t, []outcomeJSON{
		{Kind: "updated", Key: "test-key-1"},
		{Kind: "created", Key: "NEW"},
		{Kind: "invalid", Key: "B", Input: "B=", Reason: "blank key or value"},
	}, got.Outcomes, "one outcome per input, in the order given")
	assert.Equal(t, twoVariablesJSON(), got.Variables)
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
