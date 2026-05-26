package cloud

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execEnvCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newEnvRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	var verbosity string
	cmd.PersistentFlags().StringVar(&verbosity, "verbosity", "", "")
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func resetEnvFlags() {
	envWorkspaceID = ""
	envDeploymentID = ""
	envFormat = ""
	envOutputPath = ""
	envIncludeSecrets = false
	envResolveLinked = false
	envYes = false

	envVarKey, envVarValue, envVarSecret, envVarStrict, envVarFromFile = "", "", false, false, ""

	envConnKey, envConnType, envConnHost, envConnLogin = "", "", "", ""
	envConnPassword, envConnSchema, envConnExtra = "", "", ""
	envConnPort = 0

	envMetricsKey, envMetricsEndpoint, envMetricsExporterType = "", "", ""
	envMetricsAuthType, envMetricsBasicToken, envMetricsUsername = "", "", ""
	envMetricsPassword, envMetricsSigV4AssumeArn, envMetricsSigV4StsRegion = "", "", ""
	envMetricsHeaders, envMetricsLabels = nil, nil
}

func TestEnvVarList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "list", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	assert.Contains(t, out, "FOO")
	assert.Contains(t, out, "bar")
	mc.AssertExpectations(t)
}

func TestEnvVarExportDotenv(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
			{ObjectKey: "SHH", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "secret-value", IsSecret: true}},
		}},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "export", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	assert.Contains(t, out, "FOO=bar")
	assert.Contains(t, out, "SHH=") // present
	assert.NotContains(t, out, "secret-value")
	mc.AssertExpectations(t)
}

func TestEnvVarDeleteRequiresYes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "delete", "FOO", "--workspace-id", "ws-test")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "--yes"))
	mc.AssertExpectations(t)
}

func TestEnvVarCreateReadsValueFromStdin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	// Pipe a value into stdin so readSecretValue takes the piped path.
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	go func() {
		_, _ = w.WriteString("piped-value\n")
		_ = w.Close()
	}()

	createdID := "cabc12def0123456789012345"
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "FOO" &&
			body.EnvironmentVariable != nil &&
			body.EnvironmentVariable.Value != nil &&
			*body.EnvironmentVariable.Value == "piped-value"
	})).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--key", "FOO")
	assert.NoError(t, err)
	assert.Contains(t, out, "Created FOO")
	mc.AssertExpectations(t)
}

func TestEnvVarExportIncludeSecretsWarnsToStderr(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	// Capture os.Stderr so we can assert on the warning text without polluting test output.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("var", "list", "--workspace-id", "ws-test", "--include-secrets")
	assert.NoError(t, err)

	w.Close()
	stderrBytes, _ := io.ReadAll(r)
	assert.Contains(t, string(stderrBytes), "include-secrets")
	assert.Contains(t, string(stderrBytes), "sensitive")
	mc.AssertExpectations(t)
}

func TestEnvVarCreateRequiresKey(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--value", "bar")
	assert.Error(t, err) // missing --key (and no --from-file)
	mc.AssertExpectations(t)
}

func TestEnvVarCreateFromFile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	dir := t.TempDir()
	envPath := dir + "/.env"
	body := []byte("# header\nFOO=bar\nWITH_QUOTE=\"say \\\"hi\\\"\"\n\nBAZ=qux\n")
	if err := os.WriteFile(envPath, body, 0o600); err != nil {
		t.Fatal(err)
	}

	createdID := "cabc12def0123456789012345"
	mc := new(astrov1_mocks.ClientWithResponsesInterface)

	// Three creates in alphabetical order. Assert each carries IsSecret=true (from --secret).
	for _, key := range []string{"BAZ", "FOO", "WITH_QUOTE"} {
		k := key
		mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
				return body.ObjectKey == k &&
					body.EnvironmentVariable != nil &&
					body.EnvironmentVariable.IsSecret != nil &&
					*body.EnvironmentVariable.IsSecret
			}),
		).Return(&astrov1.CreateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
		}, nil).Once()
	}
	astroV1Client = mc

	out, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--from-file", envPath, "--secret")
	assert.NoError(t, err)
	assert.Contains(t, out, "Created BAZ")
	assert.Contains(t, out, "Created FOO")
	assert.Contains(t, out, "Created WITH_QUOTE")
	mc.AssertExpectations(t)
}

func TestEnvVarFromFileMutuallyExclusiveWithKey(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--key", "FOO", "--from-file", "/tmp/whatever.env")
	assert.Error(t, err)
	// cobra phrases mutual-exclusion as "none of the others can be"
	assert.Contains(t, err.Error(), "key from-file")
	mc.AssertExpectations(t)
}

func TestEnvVarUpdateFromFileUpserts(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	dir := t.TempDir()
	envPath := dir + "/.env"
	if err := os.WriteFile(envPath, []byte("EXISTS=updated\nMISSING=new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	id := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)

	// EXISTS: list lookup returns the row, then UPDATE.
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ObjectKey != nil && *p.ObjectKey == "EXISTS"
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "EXISTS"},
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "EXISTS"},
	}, nil).Once()

	// MISSING: list lookup is empty (404 path), then update returns ErrNotFound, then CREATE.
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ObjectKey != nil && *p.ObjectKey == "MISSING"
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
	}, nil).Once()
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool { return body.ObjectKey == "MISSING" }),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: cuid.New()},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "update", "--workspace-id", "ws-test", "--from-file", envPath)
	assert.NoError(t, err)
	assert.Contains(t, out, "Updated EXISTS")
	assert.Contains(t, out, "Created MISSING")
	mc.AssertExpectations(t)
}
