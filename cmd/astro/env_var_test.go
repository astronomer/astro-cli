package astro

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

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
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

// expectAbsent mocks the key lookup `set` makes before it decides whether to
// update or create. An empty list is what makes the update report ErrNotFound,
// which is what sends the upsert down the create path.
func expectAbsent(mc *astrov1_mocks.ClientWithResponsesInterface, key string) {
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ObjectKey != nil && *p.ObjectKey == key
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
	}, nil).Once()
}

func resetEnvFlags() {
	envWorkspaceID = ""
	envDeploymentID = ""
	envOutput = ""
	envIncludeSecrets = false
	envResolveLinked = false
	envYes = false

	envVarValue, envVarSecret, envVarNoCreate, envVarFromFile = "", false, false, ""

	envLinkVariableID, envLinkVariableKey = "", ""
	envLinkDeploymentID, envLinkValue, envLinkExclude = "", "", false

	envConnNoCreate = false
	envConnValue = ""
	envConnType, envConnHost, envConnLogin = "", "", ""
	envConnPassword, envConnSchema, envConnExtra = "", "", ""
	envConnPort = 0

	envMetricsNoCreate = false
	envMetricsEndpoint, envMetricsExporterType = "", ""
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
		}, TotalCount: 1},
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
		}, TotalCount: 2},
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

func TestEnvVarSetReadsValueFromStdin(t *testing.T) {
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
	expectAbsent(mc, "FOO")
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

	out, err := execEnvCmd("var", "set", "FOO", "--workspace-id", "ws-test")
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
		}, TotalCount: 1},
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

func TestEnvVarSetRequiresAnIDOrFromFile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "set", "--workspace-id", "ws-test", "--value", "bar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set requires <id-or-key> or --from-file")
	mc.AssertExpectations(t)
}

func TestEnvVarSetFromFileCreatesAbsentKeys(t *testing.T) {
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

	// Three keys, none of which exist: each looks itself up, misses, and is
	// created. Assert each carries IsSecret=true (from --secret).
	for _, key := range []string{"BAZ", "FOO", "WITH_QUOTE"} {
		k := key
		expectAbsent(mc, k)
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

	out, err := execEnvCmd("var", "set", "--workspace-id", "ws-test", "--from-file", envPath, "--secret")
	assert.NoError(t, err)
	assert.Contains(t, out, "Created BAZ")
	assert.Contains(t, out, "Created FOO")
	assert.Contains(t, out, "Created WITH_QUOTE")
	mc.AssertExpectations(t)
}

func TestEnvVarSetRejectsAnIDTogetherWithFromFile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "set", "FOO", "--workspace-id", "ws-test", "--from-file", "/tmp/whatever.env")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot pass an <id-or-key> together with --from-file")
	mc.AssertExpectations(t)
}

func TestEnvVarSetFromFileUpserts(t *testing.T) {
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
		}, TotalCount: 1},
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

	out, err := execEnvCmd("var", "set", "--workspace-id", "ws-test", "--from-file", envPath)
	assert.NoError(t, err)
	assert.Contains(t, out, "Updated EXISTS")
	assert.Contains(t, out, "Created MISSING")
	mc.AssertExpectations(t)
}

func TestEnvVarSetSaysWhenDeploymentsSeeTheChange(t *testing.T) {
	autoLink := true
	links := &[]astrov1.EnvironmentObjectLink{{ScopeEntityId: cuid.New()}}
	for name, tc := range map[string]struct {
		scopeFlag string
		updated   astrov1.EnvironmentObject
		wantNote  bool
	}{
		"unlinked workspace variable": {"--workspace-id", astrov1.EnvironmentObject{ObjectKey: "FOO"}, false},
		"linked workspace variable":   {"--workspace-id", astrov1.EnvironmentObject{ObjectKey: "FOO", Links: links}, true},
		"auto-linked":                 {"--workspace-id", astrov1.EnvironmentObject{ObjectKey: "FOO", AutoLinkDeployments: &autoLink}, true},
		"deployment variable":         {"--deployment-id", astrov1.EnvironmentObject{ObjectKey: "FOO"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			id := cuid.New()
			tc.updated.Id = &id
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
				Return(&astrov1.ListEnvironmentObjectsResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
						{Id: &id, ObjectKey: "FOO"},
					}, TotalCount: 1},
				}, nil).Once()
			mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id, mock.Anything).
				Return(&astrov1.UpdateEnvironmentObjectResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &tc.updated,
				}, nil).Once()
			astroV1Client = mc

			out, err := execEnvCmd("var", "set", "FOO", tc.scopeFlag, cuid.New(), "--value", "bar")
			assert.NoError(t, err)
			assert.Contains(t, out, "Updated FOO")
			if tc.wantNote {
				assert.Contains(t, out, deploymentPickupNote)
			} else {
				assert.NotContains(t, out, deploymentPickupNote)
			}
			mc.AssertExpectations(t)
		})
	}
}
