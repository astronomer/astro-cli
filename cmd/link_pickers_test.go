package cmd

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// loginCalls counts the login steps a picker took.
type loginCalls struct{ ensures, refreshes int }

// stubLinkLogin replaces the login check and the forced refresh, which would
// otherwise reach an identity provider, and counts the calls.
func stubLinkLogin(t *testing.T, ensureErr, refreshErr error) *loginCalls {
	t.Helper()
	calls := &loginCalls{}
	origEnsure, origRefresh := ensureLinkLogin, refreshLinkLogin
	t.Cleanup(func() { ensureLinkLogin, refreshLinkLogin = origEnsure, origRefresh })
	ensureLinkLogin = func(astrov1.APIClient) error { calls.ensures++; return ensureErr }
	refreshLinkLogin = func() error { calls.refreshes++; return refreshErr }
	return calls
}

// stubSelectDeployment records which Deployments the deploy picker was shown
// and picks the first.
func stubSelectDeployment(t *testing.T) *[]astrov1.Deployment {
	t.Helper()
	var shown []astrov1.Deployment
	orig := deployment.SelectDeployment
	t.Cleanup(func() { deployment.SelectDeployment = orig })
	deployment.SelectDeployment = func(deployments []astrov1.Deployment, _ string) (astrov1.Deployment, error) {
		shown = deployments
		return deployments[0], nil
	}
	return &shown
}

// unauthorizedBody is how Astro answers a refused token: JSON with a message,
// which NormalizeAPIError turns into that message and nothing else in the text.
const unauthorizedBody = `{"message":"The access token is invalid or has expired"}`

func listResponse(status int, deps ...astrov1.Deployment) *astrov1.ListDeploymentsResponse {
	r := &astrov1.ListDeploymentsResponse{HTTPResponse: &http.Response{StatusCode: status}}
	if status == http.StatusUnauthorized {
		r.Body = []byte(unauthorizedBody)
	}
	if status == http.StatusOK {
		r.JSON200 = &astrov1.DeploymentsPaginated{Deployments: deps, TotalCount: len(deps)}
	}
	return r
}

func getResponse(dep *astrov1.Deployment) *astrov1.GetDeploymentResponse {
	return &astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: dep}
}

var (
	pickedDep = astrov1.Deployment{Id: "clx9", Name: "Orders Prod", WorkspaceId: "ws_A"}
	linkedDep = astrov1.Deployment{Id: "clx1", Name: "Already Linked", WorkspaceId: "ws_A"}
)

// The link pickers are wired to deploy's own Deployment picker, over the client
// given, with the create flow off, after deploy's login check. Deployments the
// project already links are not shown. This drives the wired function rather
// than the seam, so a root that forgot to wire it, or wired another client,
// fails.
func TestLinkPickersUseDeploysDeploymentPicker(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	calls := stubLinkLogin(t, nil, nil)
	shown := stubSelectDeployment(t)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusOK, linkedDep, pickedDep), nil).Once()
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(getResponse(&pickedDep), nil).Once()

	var d local.Deps
	withLinkPickers(&d, client, &bytes.Buffer{})
	require.NotNil(t, d.CurrentWorkspace)
	got, err := d.PickDeployment("ws_A", map[string]string{"clx1": "prod"})
	require.NoError(t, err)
	assert.Equal(t, local.PickedDeployment{ID: "clx9", Name: "Orders Prod", WorkspaceID: "ws_A"}, got)
	assert.Equal(t, []astrov1.Deployment{pickedDep}, *shown, "a linked Deployment was offered")
	assert.Equal(t, 1, calls.ensures, "the picker skipped deploy's login check")
	assert.Zero(t, calls.refreshes)
}

// A workspace whose every Deployment is linked is its own error, with no
// picker shown; one with no Deployments is an error, not an offer to create one.
func TestLinkPickersWithNothingToOffer(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	stubLinkLogin(t, nil, nil)
	shown := stubSelectDeployment(t)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusOK, linkedDep), nil).Once()
	var d local.Deps
	withLinkPickers(&d, client, &bytes.Buffer{})
	_, err := d.PickDeployment("ws_A", map[string]string{"clx1": "prod"})
	require.ErrorIs(t, err, local.ErrAllDeploymentsLinked)
	assert.Nil(t, *shown, "the picker was shown with nothing to pick")

	client = astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusOK), nil).Once()
	withLinkPickers(&d, client, &bytes.Buffer{})
	_, err = d.PickDeployment("ws_A", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, local.ErrAllDeploymentsLinked)
	assert.Contains(t, err.Error(), "no Deployments found in workspace ws_A")
}

// A 401 gets the login renewed and one more try, which then lists as usual.
func TestLinkPickersRefreshAfterA401(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	calls := stubLinkLogin(t, nil, nil)
	stubSelectDeployment(t)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusUnauthorized), nil).Once()
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusOK, pickedDep), nil).Once()
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(getResponse(&pickedDep), nil).Once()
	var d local.Deps
	withLinkPickers(&d, client, &bytes.Buffer{})
	got, err := d.PickDeployment("ws_A", nil)
	require.NoError(t, err)
	assert.Equal(t, "clx9", got.ID)
	assert.Equal(t, 1, calls.refreshes)
}

// A 401 that outlasts the refresh, a refresh the platform refuses, and a
// login check the platform refuses all say the session expired and how to log
// in, never that the Deployment was not found. The 401s carry a JSON body, so
// the status is all that says what they are.
func TestLinkPickersNameAnExpiredSession(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Setenv("ASTRO_API_TOKEN", "")
	const want = "your astronomer.io session expired. Log in again with `astro login astronomer.io`"
	refused := httputil.NormalizeAPIError(&http.Response{StatusCode: http.StatusUnauthorized}, []byte(unauthorizedBody))
	for name, tc := range map[string]struct {
		ensureErr, refreshErr error
		lists                 int
	}{
		"still 401":     {nil, nil, 2},
		"refresh fails": {nil, errors.New("refresh refused"), 1},
		"login check":   {refused, nil, 0},
	} {
		t.Run(name, func(t *testing.T) {
			stubLinkLogin(t, tc.ensureErr, tc.refreshErr)
			client := astrov1_mocks.NewClientWithResponsesInterface(t)
			if tc.lists > 0 {
				client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusUnauthorized), nil).Times(tc.lists)
			}
			var d local.Deps
			withLinkPickers(&d, client, &bytes.Buffer{})
			_, err := d.PickDeployment("ws_A", nil)
			require.EqualError(t, err, want)
		})
	}
}

// A login check that fails for any other reason keeps its own words: an
// unreachable host is the offline cause, an ASTRO_API_TOKEN failure is the
// token's own message, and anything else is named as the login check.
func TestLinkPickersPassOtherLoginFailuresThrough(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	unreachable := &url.Error{Op: "Get", URL: "https://auth.astronomer.io", Err: &net.OpError{Op: "dial", Err: errors.New("no route to host")}}
	for name, tc := range map[string]struct {
		token     string
		ensureErr error
		want      string
	}{
		"offline":      {"", unreachable, "could not reach astronomer.io. Check your connection"},
		"login flow":   {"", errors.New("login aborted"), "checking your astronomer.io login: login aborted"},
		"api token":    {"not-a-token", errors.New("the API token given has expired"), "the API token given has expired"},
		"offline, api": {"not-a-token", unreachable, unreachable.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ASTRO_API_TOKEN", tc.token)
			stubLinkLogin(t, tc.ensureErr, nil)
			var d local.Deps
			withLinkPickers(&d, astrov1_mocks.NewClientWithResponsesInterface(t), &bytes.Buffer{})
			_, err := d.PickDeployment("ws_A", nil)
			require.EqualError(t, err, tc.want)
		})
	}

	// A refresh that gets no response is offline, not an expired session.
	t.Setenv("ASTRO_API_TOKEN", "")
	stubLinkLogin(t, nil, unreachable)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listResponse(http.StatusUnauthorized), nil).Once()
	var d local.Deps
	withLinkPickers(&d, client, &bytes.Buffer{})
	_, err := d.PickDeployment("ws_A", nil)
	require.EqualError(t, err, "could not reach astronomer.io. Check your connection")
}

// The pickers are wired for an Astro context only.
func TestLinkPickersAreWiredForAstroOnly(t *testing.T) {
	var cloud, software local.Deps
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	wireLinkPickers(&cloud, cloudPlatform, client, &bytes.Buffer{})
	wireLinkPickers(&software, apcPlatform, client, &bytes.Buffer{})
	assert.NotNil(t, cloud.PickDeployment)
	assert.NotNil(t, cloud.PickWorkspace)
	assert.NotNil(t, cloud.CurrentWorkspace)
	assert.Nil(t, software.PickDeployment)
	assert.Nil(t, software.PickWorkspace)
	assert.Nil(t, software.CurrentWorkspace)
}

// The workspace picker is `astro workspace switch`'s, titled the way the
// Deployment picker is, behind the same login handling.
func TestLinkPickersUseTheWorkspacePicker(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	calls := stubLinkLogin(t, nil, nil)
	orig := workspace.GetWorkspaceSelection
	t.Cleanup(func() { workspace.GetWorkspaceSelection = orig })
	var out bytes.Buffer
	tries := 0
	workspace.GetWorkspaceSelection = func(_ astrov1.APIClient, w io.Writer) (string, error) {
		tries++
		assert.Same(t, &out, w)
		if tries == 1 {
			return "", httputil.NormalizeAPIError(&http.Response{StatusCode: http.StatusUnauthorized}, []byte(unauthorizedBody))
		}
		return "ws_Z", nil
	}
	var d local.Deps
	withLinkPickers(&d, astrov1_mocks.NewClientWithResponsesInterface(t), &out)
	id, err := d.PickWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "ws_Z", id)
	assert.Equal(t, 1, calls.refreshes)
	assert.Equal(t, 1, calls.ensures)
	assert.Contains(t, out.String(), "Select a Workspace\n")
}
