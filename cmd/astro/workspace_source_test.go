package astro

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
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

// A lookup that fails in the Workspace the run was pointed at names where that
// Workspace came from, whichever of the flag, the project link and the token it
// was, and the error's kind and exit code are those of the error underneath.
// The words come from the error's own fields (deployment.NotFoundError), built
// with the Workspace's source, and not from a rewrite of its text.
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
			run := func(extra ...string) tokenRun {
				testUtil.InitTestConfig(testUtil.CloudPlatform)
				if tt.token != nil {
					testUtil.UseAPIToken(t, tt.token...)
				}
				if tt.project != "" {
					inProject(t, tt.project)
				}
				mc := notFound(tt.ws)
				return execAstroCmd(t, mc, "", newDeploymentRootCmd, append(append([]string{"deployment"}, tt.args...), extra...)...)
			}

			got := run()
			require.Error(t, got.err)
			assert.Contains(t, got.err.Error(), "workspace "+tt.ws+" (from "+tt.from+")")
			var nf *deployment.NotFoundError
			require.ErrorAs(t, got.err, &nf)
			assert.Equal(t, tt.ws, nf.Workspace)

			gotJSON := run("-o", "json")
			var e errorJSON
			decodeOne(t, gotJSON.stdout, &e)
			assert.Equal(t, got.code, gotJSON.code)
			assert.Contains(t, e.Error, "workspace "+tt.ws+" (from "+tt.from+")")
			assert.NotEqual(t, string(cliout.KindUsage), e.Kind)
		})
	}
}
