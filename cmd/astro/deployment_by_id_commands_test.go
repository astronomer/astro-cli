package astro

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// byIDClient is a client that knows one Deployment, wqDeploymentID, in
// workspace-id, with the reads and writes these commands make. The Deployment
// is fetched by ID gets explicit counts: gets is how many times, so a second
// fetch shows. Each by-ID command costs the resolver's fetch, one list of the
// Deployment's own Workspace, and the library's own fetch again (more where the
// command re-reads the Deployment after a change).
func byIDClient(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	d := wqDeployment()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("GetDeploymentWithResponse", mock.Anything, "env-org", wqDeploymentID).Return(&astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &d}, nil).Maybe()
	m.On("ListDeploymentsWithResponse", mock.Anything, "env-org", listedInDeploymentsWorkspace("workspace-id")).Return(&astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{d}, TotalCount: 1, Limit: 1000},
	}, nil).Maybe()
	m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Maybe()
	return m
}

// orgTokenNoWorkspace is a run with an Organization token and no Workspace.
func orgTokenNoWorkspace(t *testing.T) {
	t.Helper()
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	resetTokenCommandFlags(t)
	deploymentName = ""
	t.Cleanup(func() { deploymentName = "" })
}

// getsOf counts the fetches of a Deployment by ID the client saw.
func getsOf(m *astrov1_mocks.ClientWithResponsesInterface) int {
	return callsTo(m, "GetDeploymentWithResponse")
}

// callsTo counts the calls the client saw of method.
func callsTo(m *astrov1_mocks.ClientWithResponsesInterface, method string) int {
	n := 0
	for i := range m.Calls {
		if m.Calls[i].Method == method {
			n++
		}
	}
	return n
}

// Every other command that takes a Deployment by ID works the same way under
// an Organization token that selects no Workspace: the Deployment is fetched by
// ID, then the command goes on in the Workspace it is in.
func TestMoreDeploymentCommandsAddressedByIDNeedNoWorkspace(t *testing.T) {
	updated := astrov1.UpdateDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: func() *astrov1.Deployment { d := wqDeployment(); return &d }()}
	hibernated := true
	override := astrov1.UpdateDeploymentHibernationOverrideResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentHibernationOverride{IsHibernating: &hibernated, IsActive: &hibernated},
	}
	for name, tc := range map[string]struct {
		args  []string
		gets  int
		lists int
		extra func(*astrov1_mocks.ClientWithResponsesInterface)
	}{
		"update": {
			args: []string{"update", wqDeploymentID, "--name", "renamed", "--yes"}, gets: 2, lists: 1,
			extra: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&updated, nil).Once()
			},
		},
		"hibernate": {
			args: []string{"hibernate", wqDeploymentID, "--yes"}, gets: 2, lists: 1,
			extra: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&override, nil).Once()
			},
		},
		"wake-up": {
			args: []string{"wake-up", wqDeploymentID, "--yes"}, gets: 2, lists: 1,
			extra: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&override, nil).Once()
			},
		},
		"inspect": {args: []string{"inspect", wqDeploymentID, "-o", "json"}, gets: 2, lists: 1},
		"open": {
			args: []string{"open", wqDeploymentID, "-o", "json"}, gets: 2, lists: 1,
		},
		"worker-queue create": {
			args: []string{"worker-queue", "create", "--deployment-id", wqDeploymentID, "--name", "reports", "--worker-type", "a5"}, gets: 3, lists: 2,
			extra: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&updated, nil).Once()
			},
		},
		"worker-queue delete": {
			args: []string{"worker-queue", "delete", "--deployment-id", wqDeploymentID, "--name", "etl-queue", "--yes"}, gets: 3, lists: 2,
			extra: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&updated, nil).Once()
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			orgTokenNoWorkspace(t)
			m := byIDClient(t)
			if tc.extra != nil {
				tc.extra(m)
			}
			astroV1Client = m
			prevOpen := openDeploymentUI
			openDeploymentUI = func(string) bool { return true }
			t.Cleanup(func() { openDeploymentUI = prevOpen })

			_, err := execDeploymentCmd(tc.args...)
			require.NoError(t, err)
			require.NotEmpty(t, m.Calls)
			assert.Equal(t, "GetDeploymentWithResponse", m.Calls[0].Method, "the ID is fetched first")
			assert.Equal(t, tc.gets, getsOf(m), "fetches of the Deployment")
			assert.Equal(t, tc.lists, callsTo(m, "ListDeploymentsWithResponse"), "lists of the Deployment's Workspace")
			m.AssertExpectations(t)
		})
	}
}

// The bundle commands take the Deployment by ID too.
func TestDeploymentBundleCommandsAddressedByIDNeedNoWorkspace(t *testing.T) {
	for name, tc := range map[string]struct {
		args  []string
		setup func(*astrov1alpha1_mocks.ClientWithResponsesInterface)
	}{
		"list": {
			args: []string{"bundle", "list", "--deployment-id", wqDeploymentID, "-o", "json"},
			setup: func(a *astrov1alpha1_mocks.ClientWithResponsesInterface) {
				a.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: 1, Bundles: []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}}},
				}, nil).Once()
			},
		},
		"update": {
			args: []string{"bundle", "update", "bundle-1", "--deployment-id", wqDeploymentID, "--description", "new"},
			setup: func(a *astrov1alpha1_mocks.ClientWithResponsesInterface) {
				a.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.UpdateBundleResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
				}, nil).Once()
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			orgTokenNoWorkspace(t)
			m := byIDClient(t)
			astroV1Client = m
			a := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
			prev := astroV1Alpha1Client
			astroV1Alpha1Client = a
			t.Cleanup(func() { astroV1Alpha1Client = prev })
			tc.setup(a)

			_, err := execDeploymentCmd(tc.args...)
			require.NoError(t, err)
			assert.Equal(t, "GetDeploymentWithResponse", m.Calls[0].Method)
			assert.Equal(t, 2, getsOf(m), "fetches of the Deployment")
			assert.Equal(t, 1, callsTo(m, "ListDeploymentsWithResponse"))
			m.AssertExpectations(t)
			a.AssertExpectations(t)
		})
	}
}

// delete, logs and variable list pin their counts too: the resolver's fetch,
// the list, and the library's fetch.
func TestByIDCommandsFetchTheDeploymentTwice(t *testing.T) {
	orgTokenNoWorkspace(t)
	m := byIDClient(t)
	astroV1Client = m
	deleted := astrov1.DeleteDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	m.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID).Return(&deleted, nil).Once()
	_, err := execDeploymentCmd("delete", wqDeploymentID, "--yes")
	require.NoError(t, err)
	assert.Equal(t, 2, getsOf(m))
	assert.Equal(t, 1, callsTo(m, "ListDeploymentsWithResponse"))
	m.AssertExpectations(t)
}

// A Deployment the platform will not give by ID says what failed and that
// --workspace avoids the fetch, and keeps the platform's own error under it.
func TestByIDFetchFailureSaysNoWorkspaceWasSelected(t *testing.T) {
	for name, status := range map[string]int{"not found": http.StatusNotFound, "forbidden": http.StatusForbidden} {
		t.Run(name, func(t *testing.T) {
			orgTokenNoWorkspace(t)
			m := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = m
			failed := astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(`{"message":"nope"}`)}
			m.On("GetDeploymentWithResponse", mock.Anything, "env-org", wqDeploymentID).Return(&failed, nil).Once()

			_, err := execDeploymentCmd("delete", wqDeploymentID, "--yes")
			require.Error(t, err)
			assert.ErrorContains(t, err, "no Workspace is selected, so Deployment "+wqDeploymentID+" was fetched by its id")
			assert.ErrorContains(t, err, "pass --workspace")
			assert.ErrorContains(t, err, "nope")
			require.NotNil(t, errors.Unwrap(err), "the platform's error is wrapped, not replaced")
			m.AssertExpectations(t)
		})
	}
}

// A saved login whose current Workspace is none, which v2 refused, takes a
// Deployment by ID the same way.
func TestByIDWithASavedLoginThatHasNoCurrentWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Setenv("ASTRO_API_TOKEN", "")
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	c.Workspace, c.Organization = "", "env-org"
	require.NoError(t, c.SetContext())
	require.NoError(t, c.SetContextKey("organization", "env-org"))
	resetTokenCommandFlags(t)
	withWorkspaces(t, "", "")
	m := byIDClient(t)
	astroV1Client = m
	deleted := astrov1.DeleteDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	m.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID).Return(&deleted, nil).Once()

	_, err = execDeploymentCmd("delete", wqDeploymentID, "--yes")
	require.NoError(t, err)
	m.AssertExpectations(t)

	// A name still says there is no Workspace.
	m2 := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = m2
	_, err = execDeploymentCmd("delete", "--deployment-name", "etl", "--yes")
	require.ErrorIs(t, err, workspace.ErrCurrentNotSet)
}

// The no-Workspace error of the Deployment commands keeps its prefix, the
// family over, as v2 had it, in text and in the -o json error string.
func TestDeploymentCommandsKeepTheirNoWorkspacePrefix(t *testing.T) {
	for name, tc := range map[string]struct {
		args   []string
		prefix string
	}{
		"delete":        {[]string{"delete", "--deployment-name", "etl", "--yes"}, "failed to find a valid Workspace: "},
		"logs":          {[]string{"logs", "--deployment-name", "etl", "--warn"}, "failed to find a valid Workspace: "},
		"update":        {[]string{"update", "--deployment-name", "etl", "--yes"}, "failed to find a valid Workspace: "},
		"variable list": {[]string{"variable", "list", "--deployment-name", "etl"}, "failed to find a valid workspace: "},
		"open":          {[]string{"open", "--deployment-name", "etl"}, "failed to find a valid workspace: "},
		"hibernate":     {[]string{"hibernate", "--deployment-name", "etl", "--yes"}, "failed to find a valid workspace: "},
		"list":          {[]string{"list"}, "failed to find a valid workspace: "},
		"create":        {[]string{"create", "--name", "etl"}, "failed to find a valid Workspace: "},
	} {
		t.Run(name, func(t *testing.T) {
			orgTokenNoWorkspace(t)
			astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
			_, err := execDeploymentCmd(tc.args...)
			require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
			assert.Contains(t, err.Error(), tc.prefix+"no Workspace is selected: the API token in ASTRO_API_TOKEN selects none")
		})
	}
}
