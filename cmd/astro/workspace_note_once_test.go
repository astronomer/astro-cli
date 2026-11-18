package astro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The token note is for a command that resolves and uses its Workspace: one
// that never does (a Deployment's tokens, listed by the Deployment's id) says
// nothing.
func TestTokenNoteIsOnlyForACommandThatUsesTheWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS)
	inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listTokensResp(), nil).Once()
	astroV1Client = mc

	out, err := execDeploymentCmd("token", "list", "--deployment", otherDep)
	require.NoError(t, err)
	assert.NotContains(t, out, "using workspace")
	assert.Equal(t, 0, strings.Count(out, "from ASTRO_API_TOKEN"))
}
