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

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
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

	envVarKey, envVarValue, envVarSecret, envVarStrict = "", "", false, false

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
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}},
	}, nil).Once()
	astroCoreClient = mc

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
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
			{ObjectKey: "SHH", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "secret-value", IsSecret: true}},
		}},
	}, nil).Once()
	astroCoreClient = mc

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

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	astroCoreClient = mc

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
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(body astrocore.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "FOO" &&
			body.EnvironmentVariable != nil &&
			body.EnvironmentVariable.Value != nil &&
			*body.EnvironmentVariable.Value == "piped-value"
	})).Return(&astrocore.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()
	astroCoreClient = mc

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

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}},
	}, nil).Once()
	astroCoreClient = mc

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

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	astroCoreClient = mc

	_, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--value", "bar")
	assert.Error(t, err) // missing required --key
	mc.AssertExpectations(t)
}
