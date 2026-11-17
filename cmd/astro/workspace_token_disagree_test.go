package astro

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const claimWS = "clclaimworkspace0000001"

// A run whose API token is for one Workspace and whose --workspace or project
// link names another is refused before any request, with a usage error naming
// both and where each came from: exit 2, kind usage, in text and in json. No
// client is given, so a request would panic.
func TestTokenForAnotherWorkspaceIsRefusedBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		token   []string
		project string
		args    []string
		want    string
	}{
		{
			name:  "flag against a workspace token, cloud tree",
			token: []string{"organizationId:org", "workspaceId:" + claimWS},
			args:  []string{"deployment", "list", "--workspace", "clflagworkspace00000001"},
			want:  "ASTRO_API_TOKEN is a Workspace token for " + claimWS + ", but --workspace names clflagworkspace00000001: drop --workspace, or use a token for clflagworkspace00000001",
		},
		{
			name:  "flag against a deployment token",
			token: []string{"organizationId:org", "workspaceId:" + claimWS, "deploymentId:cldeployment000000001"},
			args:  []string{"deployment", "list", "--workspace", "clflagworkspace00000001"},
			want:  "ASTRO_API_TOKEN is a Deployment token for Workspace " + claimWS + ", but --workspace names clflagworkspace00000001: drop --workspace, or use a token for clflagworkspace00000001",
		},
		{
			name:  "flag against a workspace token, env tree",
			token: []string{"organizationId:org", "workspaceId:" + claimWS},
			args:  []string{"env", "variable", "list", "--workspace", "clflagworkspace00000001"},
			want:  "ASTRO_API_TOKEN is a Workspace token for " + claimWS + ", but --workspace names clflagworkspace00000001: drop --workspace, or use a token for clflagworkspace00000001",
		},
		{
			name:  "flag against a workspace token, workspace tree",
			token: []string{"organizationId:org", "workspaceId:" + claimWS},
			args:  []string{"workspace", "user", "list", "--workspace", "clflagworkspace00000001"},
			want:  "ASTRO_API_TOKEN is a Workspace token for " + claimWS + ", but --workspace names clflagworkspace00000001: drop --workspace, or use a token for clflagworkspace00000001",
		},
		{
			name:    "project link against a workspace token",
			token:   []string{"organizationId:org", "workspaceId:" + claimWS},
			project: testProjectManifest,
			args:    []string{"deployment", "list"},
			want:    "ASTRO_API_TOKEN is a Workspace token for " + claimWS + ", but the project's link names " + projectWS + ": run outside that project, or use a token for " + projectWS,
		},
		{
			name:    "project link against a deployment token, env tree",
			token:   []string{"organizationId:org", "workspaceId:" + claimWS, "deploymentId:cldeployment000000001"},
			project: testProjectManifest,
			args:    []string{"env", "variable", "list"},
			want:    "ASTRO_API_TOKEN is a Deployment token for Workspace " + claimWS + ", but the project's link names " + projectWS + ": run outside that project, or use a token for " + projectWS,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUtil.UseAPIToken(t, tt.token...)
			if tt.project != "" {
				inProject(t, tt.project)
			}
			newRoot := newDeploymentRootCmd
			switch tt.args[0] {
			case "env":
				newRoot = newEnvRootCmd
			case "workspace":
				newRoot = newWorkspaceCmd
			}

			text := execAstroCmd(t, nil, "", newRoot, tt.args...)
			require.Error(t, text.err)
			assert.Equal(t, tt.want, text.err.Error())
			assert.Equal(t, cliout.ExitUsage, text.code, "stderr: %s", text.stderr)
			assert.True(t, cliout.IsUsage(text.err))
			assert.Empty(t, text.stdout)

			js := execAstroCmd(t, nil, "", newRoot, append(append([]string{}, tt.args...), "-o", "json")...)
			require.Error(t, js.err)
			assert.Equal(t, cliout.ExitUsage, js.code)
			var got errorJSON
			decodeOne(t, js.stdout, &got)
			assert.Equal(t, cliout.ExitUsage, got.Code)
			assert.Equal(t, string(cliout.KindUsage), got.Kind)
			assert.Equal(t, tt.want, got.Error)
		})
	}
}

// The same Workspace passes, and an Organization token, which is for none, is
// unaffected by either the flag or the project link.
func TestTokenForTheSameWorkspaceOrNoneIsNotRefused(t *testing.T) {
	tests := []struct {
		name  string
		token []string
		ws    string
	}{
		{"workspace token for the project's workspace", []string{"organizationId:org", "workspaceId:" + projectWS}, projectWS},
		{"deployment token for the project's workspace", []string{"organizationId:org", "workspaceId:" + projectWS, "deploymentId:cldeployment000000001"}, projectWS},
		{"organization token", []string{"organizationId:org"}, projectWS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			testUtil.UseAPIToken(t, tt.token...)
			inProject(t, testProjectManifest)
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
				return p.WorkspaceId != nil && *p.WorkspaceId == tt.ws
			})
			// An Organization token is for no Workspace, so the project's link is
			// news and is named; a token for the project's own is not.
			mc.On("GetWorkspaceWithResponse", mock.Anything, mock.Anything, projectWS).Return(&astrov1.GetWorkspaceResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200:      &astrov1.Workspace{Id: projectWS, Name: "Example"},
			}, nil).Maybe()
			astroV1Client = mc

			_, err := execEnvCmd("variable", "list")
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
	}
}
