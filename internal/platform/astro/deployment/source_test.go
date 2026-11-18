package deployment

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// listing is a client whose Workspace listing answers with deployments, or with
// status when it is not 200.
func listing(status int, deployments ...astrov1.Deployment) astrov1.APIClient {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	resp := &astrov1.ListDeploymentsResponse{HTTPResponse: &http.Response{StatusCode: status}}
	if status == http.StatusOK {
		resp.JSON200 = &astrov1.DeploymentsPaginated{Deployments: deployments}
	} else {
		resp.Body = []byte(`{"message":"workspace not found"}`)
	}
	mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(resp, nil)
	return mc
}

// A lookup in a Workspace names where the run took it from, built into the
// error from the source the client was told, for each source a person can see;
// the saved login's is the run's own and is not named.
func TestLookupErrorsNameTheWorkspacesSource(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	for _, tt := range []struct {
		source astrosession.Source
		named  string
	}{
		{astrosession.SourceFlag, "workspace wsA (from --workspace)"},
		{astrosession.SourceProject, "workspace wsA (from the project link)"},
		{astrosession.SourceToken, "workspace wsA (from ASTRO_API_TOKEN)"},
		{astrosession.SourceContext, "workspace wsA"},
	} {
		t.Run(tt.named, func(t *testing.T) {
			sourced := func(c astrov1.APIClient) astrov1.APIClient { return WithWorkspaceSource(c, "wsA", tt.source) }
			visible := tt.source != astrosession.SourceContext

			// A Deployment id the Workspace does not have.
			_, err := GetDeployment("wsA", "nope", "", ToRead, nil, sourced(listing(http.StatusOK)))
			require.ErrorIs(t, err, errInvalidDeployment)
			var notFound *NotFoundError
			require.ErrorAs(t, err, &notFound)
			assert.Equal(t, "wsA", notFound.Workspace)
			assert.Equal(t, tt.source, notFound.Source)
			if visible {
				assert.Contains(t, err.Error(), tt.named)
			} else {
				assert.Contains(t, err.Error(), "not found in this workspace")
			}

			// A Workspace that cannot be listed keeps its cause.
			_, err = GetDeployment("wsA", "nope", "", ToRead, nil, sourced(listing(http.StatusNotFound)))
			require.ErrorAs(t, err, &notFound)
			require.ErrorIs(t, err, errInvalidDeployment)
			assert.NotNil(t, notFound.Err)
			if visible {
				assert.Contains(t, err.Error(), tt.named)
			}

			// A name that finds none.
			_, err = GetDeployment("wsA", "", "etl", ToRead, nil, sourced(listing(http.StatusOK)))
			var nameErr *NameError
			require.ErrorAs(t, err, &nameErr)
			assert.Equal(t, tt.source, nameErr.Source)
			assert.Contains(t, err.Error(), "in "+tt.named+" that")

			// A name that finds several.
			twins := []astrov1.Deployment{{Id: "a", Name: "etl", WorkspaceId: "wsA"}, {Id: "b", Name: "etl", WorkspaceId: "wsA"}}
			_, err = GetDeployment("wsA", "", "etl", ToRead, nil, sourced(listing(http.StatusOK, twins...)))
			require.ErrorAs(t, err, &nameErr)
			assert.Contains(t, err.Error(), "in "+tt.named+": pass")

			// A Workspace with nothing in it, and a pick that nothing fits.
			_, err = GetDeployment("wsA", "", "", ToChange, nil, sourced(listing(http.StatusOK)))
			var none *NoDeploymentsError
			require.ErrorAs(t, err, &none)
			assert.Contains(t, err.Error(), "found in "+tt.named+":")
			_, err = GetDeployment("wsA", "", "", ToChange, func(astrov1.Deployment) bool { return false }, sourced(listing(http.StatusOK, twins...)))
			var noneLeft *NoneEligibleError
			require.ErrorAs(t, err, &noneLeft)
			assert.Contains(t, err.Error(), "Deployments in "+tt.named+" is")
		})
	}
}

// The Deployment a token is scoped to, not in the Workspace listed, says which
// Workspace was listed and where it came from, and is still the token error.
func TestTokenDeploymentNotFoundNamesTheWorkspacesSource(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:wsA", "deploymentId:clgone")
	_, err := GetDeployment("wsA", "", "", ToRead, nil,
		WithWorkspaceSource(listing(http.StatusOK, astrov1.Deployment{Id: "other", Name: "x", WorkspaceId: "wsA"}), "wsA", astrosession.SourceToken))
	require.ErrorIs(t, err, ErrTokenDeploymentNotFound)
	var wsErr *astrosession.WorkspaceError
	require.ErrorAs(t, err, &wsErr)
	assert.Equal(t, astrosession.SourceToken, wsErr.Source)
	assert.Contains(t, err.Error(), "clgone is not in workspace wsA (from ASTRO_API_TOKEN). Pass --deployment")
}

// A message about another Workspace than the one the client was told about is
// not labeled with that Workspace's source, and neither is a client told
// nothing.
func TestSourceIsNotLentToAnotherWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := WithWorkspaceSource(listing(http.StatusOK), "wsA", astrosession.SourceFlag)
	_, err := GetDeployment("wsB", "nope", "", ToRead, nil, client)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, astrosession.SourceNone, notFound.Source)
	assert.NotContains(t, err.Error(), "(from ")

	_, err = GetDeployment("wsB", "", "etl", ToRead, nil, client)
	assert.NotContains(t, err.Error(), "(from ")

	_, err = GetDeployment("wsA", "nope", "", ToRead, nil, listing(http.StatusOK))
	assert.NotContains(t, err.Error(), "(from ")
}

// A Deployment fetched by id and a source told to the same client compose: the
// fetched Deployment still answers, and the source still reaches an error.
func TestSourceAndFetchedClientsCompose(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	d := astrov1.Deployment{Id: "dep", Name: "etl", WorkspaceId: "wsA"}
	client := WithWorkspaceSource(WithFetched(listing(http.StatusOK), d), "wsA", astrosession.SourceFlag)
	got, err := GetDeployment("wsA", "dep", "", ToRead, nil, client)
	require.NoError(t, err)
	assert.Equal(t, "dep", got.Id)
	_, err = GetDeployment("wsA", "other", "", ToRead, nil, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace wsA (from --workspace)")
}
