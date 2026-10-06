package astro

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// goldenInspectDeployment is the Deployment every golden below renders. It is
// built here rather than borrowed from deployment_test.go's fixtures, which
// other tests in this package share and could mutate under -shuffle, and it is
// a Standard Deployment because that is what a deploy-action preview is
// usually created from.
func goldenInspectDeployment() astrov1.Deployment {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	str := func(s string) *string { return &s }
	yes, no := true, false
	standard := astrov1.DeploymentTypeSTANDARD
	executor := astrov1.DeploymentExecutorASTRO
	provider := astrov1.DeploymentCloudProviderAWS
	size := astrov1.DeploymentSchedulerSizeSMALL
	return astrov1.Deployment{
		Id:                     "clgoldendeploy0001",
		Name:                   "golden-deployment",
		Description:            str("pinned by deployment_inspect_golden_test.go"),
		Namespace:              "golden-namespace-1234",
		WorkspaceId:            "ck05r3bor07h40d02y2hw4n4v",
		WorkspaceName:          str("golden-workspace"),
		RuntimeVersion:         "13.1.0",
		AirflowVersion:         "3.1.1",
		ImageTag:               "13.1.0",
		Status:                 "HEALTHY",
		Type:                   &standard,
		Executor:               &executor,
		CloudProvider:          &provider,
		Region:                 str("us-east-1"),
		SchedulerSize:          &size,
		SchedulerReplicas:      1,
		IsDagDeployEnabled:     true,
		IsCicdEnforced:         false,
		IsHighAvailability:     &no,
		IsDevelopmentMode:      &yes,
		DefaultTaskPodCpu:      str("0.25"),
		DefaultTaskPodMemory:   str("0.5Gi"),
		ResourceQuotaCpu:       str("10"),
		ResourceQuotaMemory:    str("20Gi"),
		WebServerUrl:           "golden.astronomer.run/d0001",
		WebServerAirflowApiUrl: "golden.astronomer.run/d0001/api/v2",
		ContactEmails:          &[]string{"alerts@example.com"},
		CreatedAt:              created,
		UpdatedAt:              updated,
		EnvironmentVariables: &[]astrov1.DeploymentEnvironmentVariable{
			{Key: "PLAIN_VAR", Value: str("plain-value"), UpdatedAt: updated},
			{Key: "SECRET_VAR", IsSecret: true, UpdatedAt: updated},
		},
		WorkerQueues: &[]astrov1.WorkerQueue{
			{
				Id:             "clgoldenqueue0001",
				Name:           "default",
				IsDefault:      true,
				AstroMachine:   str("A5"),
				MaxWorkerCount: 10,
				MinWorkerCount: 1,
				PodCpu:         "1",
				PodMemory:      "2Gi",
			},
		},
	}
}

// The bytes `astro deployment inspect` prints are a contract with
// astronomer/deploy-action, which parses them, and with every deployment-as-
// code pipeline that writes the --template YAML to a file and hands it to
// `astro deployment create --deployment-file`. Each case here is an invocation
// shape the action uses (see #414), plus the -o values; the goldens were
// written before `-o text` existed, so a case that renders the same golden as
// another is the claim that the two invocations print identical bytes.
//
// They are not schema goldens — they pin the rendered bytes, YAML and the
// --key values included, because deploy-action parses those bytes — but
// `make update-schemas` regenerates them too, so there is one command for
// every golden. Read the diff: a changed golden is a changed contract.
func TestDeploymentInspectPrintsPinnedBytes(t *testing.T) {
	const id = "clgoldendeploy0001"
	for _, tc := range []struct {
		name   string
		args   []string
		golden string
	}{
		// The default, and the two explicit spellings of it.
		{"default", []string{id}, "full.yaml"},
		{"output text", []string{id, "-o", "text"}, "full.yaml"},
		{"output yaml", []string{id, "-o", "yaml"}, "full.yaml"},
		{"output yaml long flag", []string{id, "--output", "yaml"}, "full.yaml"},
		{"clean output", []string{id, "--clean-output"}, "full.yaml"},
		{"output json", []string{id, "-o", "json"}, "full.json"},
		{"clean output json", []string{id, "--clean-output", "-o", "json"}, "full.json"},

		// The preview-create round trip: written to a file and read back by
		// `astro deployment create --deployment-file`.
		{"template", []string{id, "--template"}, "template.yaml"},
		{"clean output template", []string{id, "--clean-output", "--template"}, "template.yaml"},
		{"clean output template short", []string{id, "-c", "-t"}, "template.yaml"},
		{"text template", []string{id, "-o", "text", "--template"}, "template.yaml"},
		{"json template", []string{id, "-o", "json", "--template"}, "template.json"},

		// The single values deploy-action reads with --clean-output --key.
		{"key deployment_id", []string{id, "--clean-output", "--key", "metadata.deployment_id"}, "key-deployment_id.txt"},
		{"key name", []string{id, "--clean-output", "--key", "configuration.name"}, "key-name.txt"},
		{"key status", []string{id, "--clean-output", "--key", "metadata.status"}, "key-status.txt"},
		{"key dag_deploy_enabled", []string{id, "--clean-output", "--key", "configuration.dag_deploy_enabled"}, "key-dag_deploy_enabled.txt"},
		{"key deployment_type", []string{id, "--clean-output", "--key", "configuration.deployment_type"}, "key-deployment_type.txt"},
		{"key is_development_mode", []string{id, "--clean-output", "--key", "configuration.is_development_mode"}, "key-is_development_mode.txt"},
		// --key ignores --output; it prints the bare value either way.
		{"key with json", []string{id, "--clean-output", "-o", "json", "--key", "metadata.deployment_id"}, "key-deployment_id.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			dep := goldenInspectDeployment()
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{dep}},
			}, nil).Once()
			client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &dep,
			}, nil).Once()
			prev := astroV1Client
			astroV1Client = client
			t.Cleanup(func() { astroV1Client = prev })

			out, err := execDeploymentCmd(append([]string{"inspect"}, tc.args...)...)
			require.NoError(t, err)
			client.AssertExpectations(t)

			path := filepath.Join("testdata", "deployment_inspect", tc.golden)
			if cliouttest.Updating() {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(out), 0o600))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden; regenerate with make update-schemas")
			require.Equal(t, string(want), out, "`astro deployment inspect %s` changed its bytes", strings.Join(tc.args, " "))
		})
	}
}

// An unknown --output is refused before anything is fetched. It used to fall
// through to YAML, so `-o table` printed YAML and said nothing.
func TestDeploymentInspectRejectsUnknownOutput(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	prev := astroV1Client
	astroV1Client = client
	t.Cleanup(func() { astroV1Client = prev })

	_, err := execDeploymentCmd("inspect", "some-id", "-o", "table")
	require.EqualError(t, err, `unknown output format "table" (supported: text, json, yaml)`)
	require.True(t, cliout.IsUsage(err), "a bad --output is a usage error: exit 2")
	client.AssertExpectations(t) // no calls were expected, so none were made
}
