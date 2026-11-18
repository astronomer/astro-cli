package astro

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The note that says where a run's Workspace came from, on stderr, when that is
// a surprise: a token run took it from the token's claim. Not when the flag
// named it, not when the project link named what the token is for, and never
// under -o json.
func TestNoteSaysWhenATokenSuppliedTheWorkspace(t *testing.T) {
	list := func(t *testing.T, ws string) *astrov1_mocks.ClientWithResponsesInterface {
		t.Helper()
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p.WorkspaceId != nil && *p.WorkspaceId == ws
		})
		return mc
	}
	t.Run("a Workspace token's claim is named", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS)
		inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
		astroV1Client = list(t, claimWS)

		out, err := execEnvCmd("variable", "list")
		require.NoError(t, err)
		assert.Contains(t, out, "using workspace "+claimWS+" from ASTRO_API_TOKEN\n")
	})
	t.Run("a Deployment token's claim is named", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS, "deploymentId:cldeployment000000001")
		inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
		astroV1Client = list(t, claimWS)

		out, err := execEnvCmd("variable", "list")
		require.NoError(t, err)
		assert.Contains(t, out, "using workspace "+claimWS+" from ASTRO_API_TOKEN\n")
	})
	t.Run("not when the flag named it", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS)
		inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
		astroV1Client = list(t, claimWS)

		out, err := execEnvCmd("variable", "list", "--workspace", claimWS)
		require.NoError(t, err)
		assert.NotContains(t, out, "using workspace")
	})
	t.Run("not when the project link named what the token is for", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+projectWS)
		inProject(t, testProjectManifest)
		astroV1Client = list(t, projectWS)

		out, err := execEnvCmd("variable", "list")
		require.NoError(t, err)
		assert.NotContains(t, out, "using workspace")
	})
	t.Run("never under json", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS)
		inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
		astroV1Client = list(t, claimWS)

		out, err := execEnvCmd("variable", "list", "-o", "json")
		require.NoError(t, err)
		assert.NotContains(t, out, "using workspace")
	})
}

// The project's note, which names a Workspace the project chose over the
// context's, is for a person: a json run does not get it either.
func TestProjectNoteIsNotWrittenUnderJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool { return true })
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "-o", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "using workspace")
	assert.NotContains(t, out, "from pyproject.toml")
}

// projectWithoutHost is the test project with no host of its own, so it runs on
// the host the test's login is on.
var projectWithoutHost = strings.Replace(testProjectManifest, "domain = \"astronomer.io\"\n", "", 1)

// explainedDeploymentRoot is the deployment commands as the CLI mounts them:
// under the wrapper that names where a failed run's Workspace came from.
func explainedDeploymentRoot(out io.Writer) *cobra.Command {
	c := newDeploymentRootCmd(out)
	explainWorkspaceErrors(c)
	return c
}

// A lookup that fails in the Workspace the run was pointed at names where that
// Workspace came from, whichever of the flag, the project link and the token it
// was, and the error's kind and exit code are those of the error underneath.
func TestFailedLookupNamesWhereTheWorkspaceCameFrom(t *testing.T) {
	notFound := func(string) *astrov1_mocks.ClientWithResponsesInterface {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}, Body: []byte(`{"message":"workspace not found"}`),
		}, nil)
		// The project's note names its Workspace.
		mc.On("GetWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetWorkspaceResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		}, nil).Maybe()
		return mc
	}
	tests := []struct {
		name    string
		token   []string
		project string
		args    []string
		ws      string
		from    string
	}{
		{"flag", nil, "", []string{"inspect", "--deployment-name", "etl", "--workspace", "clflagworkspace00000001"}, "clflagworkspace00000001", "--workspace"},
		{"project link", []string{"organizationId:org"}, testProjectManifest, []string{"inspect", "--deployment-name", "etl"}, projectWS, "the project link"},
		{"token claim", []string{"organizationId:org", "workspaceId:" + claimWS}, "", []string{"inspect", "--deployment-name", "etl"}, claimWS, "ASTRO_API_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(newRoot func(io.Writer) *cobra.Command, extra ...string) tokenRun {
				testUtil.InitTestConfig(testUtil.CloudPlatform)
				if tt.token != nil {
					testUtil.UseAPIToken(t, tt.token...)
				}
				if tt.project != "" {
					inProject(t, tt.project)
				}
				mc := notFound(tt.ws)
				return execAstroCmd(t, mc, "", newRoot, append(append([]string{"deployment"}, tt.args...), extra...)...)
			}

			plain := run(newDeploymentRootCmd)
			explained := run(explainedDeploymentRoot)
			require.Error(t, plain.err)
			require.Error(t, explained.err)
			assert.NotContains(t, plain.err.Error(), "(from ")
			assert.Contains(t, explained.err.Error(), "workspace "+tt.ws+" (from "+tt.from+")")
			assert.Equal(t, plain.code, explained.code, "the exit code is the error underneath's")

			plainJSON := run(newDeploymentRootCmd, "-o", "json")
			explainedJSON := run(explainedDeploymentRoot, "-o", "json")
			var p, e errorJSON
			decodeOne(t, plainJSON.stdout, &p)
			decodeOne(t, explainedJSON.stdout, &e)
			assert.Equal(t, p.Kind, e.Kind, "the kind is the error underneath's")
			assert.Equal(t, p.Code, e.Code)
			assert.Contains(t, e.Error, "workspace "+tt.ws+" (from "+tt.from+")")
			assert.NotEqual(t, string(cliout.KindUsage), e.Kind)
		})
	}
}
