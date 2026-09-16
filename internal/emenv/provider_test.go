package emenv

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/emfetch"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const testWorkspace = "cmws123"

func envVarObj(key, val string, secret bool) astrov1.EnvironmentObject {
	return astrov1.EnvironmentObject{
		ObjectKey:           key,
		ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: val, IsSecret: secret},
	}
}

func airflowVarObj(key, val string, secret bool) astrov1.EnvironmentObject {
	return astrov1.EnvironmentObject{
		ObjectKey:       key,
		ObjectType:      astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE,
		AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: val, IsSecret: secret},
	}
}

func okResp(objs ...astrov1.EnvironmentObject) *astrov1.ListEnvironmentObjectsResponse {
	return &astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: objs, TotalCount: len(objs)},
	}
}

func errResp(status int, message string) *astrov1.ListEnvironmentObjectsResponse {
	return &astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: status},
		Body:         []byte(`{"message":"` + message + `"}`),
	}
}

// errRespBody is a failure whose body is not the JSON envelope the success path
// parses, which is what a gateway or proxy answering instead of the app sends.
func errRespBody(status int, body string) *astrov1.ListEnvironmentObjectsResponse {
	return &astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: status},
		Body:         []byte(body),
	}
}

// mockClient returns a mock returning resp for any list call.
func mockClient(resp *astrov1.ListEnvironmentObjectsResponse) *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(resp, nil)
	return mc
}

// loggedInProvider sets a logged-in context and builds the workspace provider.
func loggedInProvider(t *testing.T, client astrov1.APIClient, reveal bool) envresolve.Provider {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	return NewProvider(testWorkspace, client, reveal)
}

func TestLookupResolvesEnvVarAndAirflowVar(t *testing.T) {
	mc := mockClient(okResp(
		envVarObj("DATA_WAREHOUSE_URI", "postgres://db", false),
		airflowVarObj("AIRFLOW_VAR_REGION", "us-east", false),
	))
	p := loggedInProvider(t, mc, true)

	v, ok := p.Lookup("DATA_WAREHOUSE_URI")
	require.True(t, ok)
	require.Equal(t, "postgres://db", v)

	v, ok = p.Lookup("AIRFLOW_VAR_REGION")
	require.True(t, ok)
	require.Equal(t, "us-east", v)

	require.Equal(t, "workspace", p.Label())
}

// The read is scoped to the workspace: WorkspaceId set, no DeploymentId, and
// resolveLinked off (a deployment's runtime config stays off the laptop).
func TestFetchUsesWorkspaceScope(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil &&
				p.WorkspaceId != nil && *p.WorkspaceId == testWorkspace &&
				p.DeploymentId == nil &&
				p.ResolveLinked != nil && !*p.ResolveLinked
		}),
	).Return(okResp(envVarObj("K", "v", false)), nil)

	p := NewProvider(testWorkspace, mc, true)
	v, ok := p.Lookup("K")
	require.True(t, ok)
	require.Equal(t, "v", v)
	mc.AssertExpectations(t)
}

func TestLookupMissForUnknownKey(t *testing.T) {
	mc := mockClient(okResp(envVarObj("KNOWN", "v", false)))
	p := loggedInProvider(t, mc, true)

	_, ok := p.Lookup("UNKNOWN")
	require.False(t, ok)
	require.Equal(t, "workspace", p.Label())
	require.Equal(t, "Environment Manager holds no value for it in this workspace", p.(envresolve.Diagnoser).Diagnose("UNKNOWN"))
}

// One fetch serves every name in a run: repeated Lookups make one fetch (a call
// per object type), not one each.
func TestSharedFetchAcrossNames(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(okResp(envVarObj("A", "1", false), envVarObj("B", "2", false)), nil)

	p := NewProvider(testWorkspace, mc, true)

	v, ok := p.Lookup("A")
	require.True(t, ok)
	require.Equal(t, "1", v)
	v, ok = p.Lookup("B")
	require.True(t, ok)
	require.Equal(t, "2", v)

	// One call per env-var-keyed object type, shared across both lookups.
	mc.AssertNumberOfCalls(t, "ListEnvironmentObjectsWithResponse", len(envVarKeyedTypes))
}

func TestLoggedOutProviderAbsent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no cloud context
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	p := NewProvider(testWorkspace, mc, true)

	_, ok := p.Lookup("DATA_WAREHOUSE_URI")
	require.False(t, ok)
	require.Equal(t, "workspace (unavailable: logged out)", p.Label())
	require.Equal(t, "you are not logged in — log in with 'astro login'", p.(envresolve.Diagnoser).Diagnose("X"))
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// No top-level workspace in the manifest: the provider is unavailable and names
// the fix, without touching the API.
func TestNoWorkspaceUnavailable(t *testing.T) {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	p := NewProvider("", mc, true)

	_, ok := p.Lookup("X")
	require.False(t, ok)
	require.Contains(t, p.Label(), "unavailable: the manifest sets no `workspace`")
	require.Contains(t, p.(envresolve.Diagnoser).Diagnose("X"), "add `workspace =")
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// Docker start can't inject values without writing them to disk, so it wires an
// Unavailable provider whose reason names the fix.
func TestUnavailableProvider(t *testing.T) {
	p := Unavailable("run without --docker")

	_, ok := p.Lookup("X")
	require.False(t, ok)
	require.Equal(t, "workspace (unavailable: run without --docker)", p.Label())
	require.Equal(t, "run without --docker", p.(envresolve.Diagnoser).Diagnose("X"))
}

// Each HTTP failure mode gets its own named label and cause.
func TestFailureModeMessages(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		message   string
		wantLabel string
		wantCause string
	}{
		{"expired", http.StatusUnauthorized, "token expired", "workspace (unavailable: session expired)", "your session expired — log in again with 'astro login'"},
		{"revoked", http.StatusForbidden, "forbidden", "workspace (unavailable: access lost)", "you no longer have access to this workspace — ask an org admin to restore it"},
		{"deleted", http.StatusNotFound, "not found", "workspace (unavailable: workspace not found)", "this workspace no longer exists — check the `workspace` in your manifest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mc := mockClient(errResp(tc.status, tc.message))
			p := loggedInProvider(t, mc, true)

			_, ok := p.Lookup("X")
			require.False(t, ok)
			require.Equal(t, tc.wantLabel, p.Label())
			require.Equal(t, tc.wantCause, p.(envresolve.Diagnoser).Diagnose("X"))
		})
	}
}

func TestOfflineProviderAbsent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return((*astrov1.ListEnvironmentObjectsResponse)(nil), errors.New("dial tcp: no route to host"))
	p := NewProvider(testWorkspace, mc, true)

	_, ok := p.Lookup("X")
	require.False(t, ok)
	require.Equal(t, "workspace (unavailable: offline)", p.Label())
}

// A secret value the org will not release is a hard miss whose cause names the
// org toggle; non-secret values in the same workspace still resolve.
func TestSecretsDisabledHardMiss(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	// First call asks for secrets and is refused at the org level.
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ShowSecrets != nil && *p.ShowSecrets
		}),
	).Return(errResp(http.StatusForbidden, "showSecrets is not allowed for this organization"), nil).Once()
	// The retry without secrets returns the objects: the secret one empty.
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ShowSecrets != nil && !*p.ShowSecrets
		}),
	).Return(okResp(
		envVarObj("PLAIN", "visible", false),
		envVarObj("SECRET_TOKEN", "", true),
	), nil)

	p := NewProvider(testWorkspace, mc, true)

	v, ok := p.Lookup("PLAIN")
	require.True(t, ok, "non-secret value still resolves when secrets are disabled")
	require.Equal(t, "visible", v)

	_, ok = p.Lookup("SECRET_TOKEN")
	require.False(t, ok, "a withheld secret is a hard miss")
	require.Contains(t, p.(envresolve.Diagnoser).Diagnose("SECRET_TOKEN"), `enable "Environment Secrets Fetching"`)

	require.Equal(t, "workspace", p.Label())
	// The first typed call is refused; the retry re-fetches every type without
	// secrets: one refused call plus a full fetch.
	mc.AssertNumberOfCalls(t, "ListEnvironmentObjectsWithResponse", 1+len(envVarKeyedTypes))
}

// With the org policy on, a revealed secret returns its value.
func TestRevealedSecretValue(t *testing.T) {
	mc := mockClient(okResp(envVarObj("SECRET_TOKEN", "xoxb-real", true)))
	p := loggedInProvider(t, mc, true)

	v, ok := p.Lookup("SECRET_TOKEN")
	require.True(t, ok)
	require.Equal(t, "xoxb-real", v)
}

// In presence mode (list), a secret object resolves as a source without a
// value: list is value-free and never pulls the secret.
func TestPresenceModeSecretResolvesWithoutValue(t *testing.T) {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ShowSecrets != nil && !*p.ShowSecrets
		}),
	).Return(okResp(envVarObj("SECRET_TOKEN", "", true)), nil)

	p := loggedInProvider(t, mc, false) // presence mode, no secret pulled

	v, ok := p.Lookup("SECRET_TOKEN")
	require.True(t, ok, "list shows the source even for a secret")
	require.Equal(t, "", v)
	require.Equal(t, "workspace", p.Label())
}

// The refusal does not always arrive as the JSON envelope, and recognizing it
// from the response body rather than the decoded message field is what lets the
// fallback run at all. Read through the envelope only, this is an undecodable
// 405: the retry never happens and the whole workspace reads as unreachable.
func TestSecretsDisabledWhenTheRefusalIsNotAnEnvelope(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ShowSecrets != nil && *p.ShowSecrets
		}),
	).Return(errRespBody(http.StatusMethodNotAllowed,
		"showSecrets is not allowed for this organization"), nil).Once()
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ShowSecrets != nil && !*p.ShowSecrets
		}),
	).Return(okResp(
		envVarObj("PLAIN", "visible", false),
		envVarObj("SECRET_TOKEN", "", true),
	), nil)

	p := NewProvider(testWorkspace, mc, true)

	v, ok := p.Lookup("PLAIN")
	require.True(t, ok, "the fallback ran, so non-secret values still resolve")
	require.Equal(t, "visible", v)
	require.Equal(t, "workspace", p.Label(), "the workspace is available, not unreachable")
	require.Contains(t, p.(envresolve.Diagnoser).Diagnose("SECRET_TOKEN"), `enable "Environment Secrets Fetching"`)
}

// A secret the organization allowed but the platform returned empty is not the
// org toggle. Naming the toggle here would send someone to change a setting
// that is already on.
func TestSecretWithNoValueWhenSecretsWereAllowed(t *testing.T) {
	mc := mockClient(okResp(envVarObj("SECRET_TOKEN", "", true)))
	p := loggedInProvider(t, mc, true)

	_, ok := p.Lookup("SECRET_TOKEN")
	require.False(t, ok, "a secret with no value is a miss in reveal mode")

	cause := p.(envresolve.Diagnoser).Diagnose("SECRET_TOKEN")
	require.NotContains(t, cause, `enable "Environment Secrets Fetching"`)
	require.Contains(t, cause, "no value to resolve")
}

// A workspace larger than one window needs the offset to reach the request, or
// the second window re-reads the first and the rest of the workspace is never
// seen. The paging itself is pkg/emfetch's; what this holds is the wiring.
func TestFetchPagesThroughMoreThanOneWindow(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	first := make([]astrov1.EnvironmentObject, emfetch.PageLimit)
	for i := range first {
		first[i] = envVarObj("KEY_"+strconv.Itoa(i), "v", false)
	}
	total := emfetch.PageLimit + 1

	window := func(objs []astrov1.EnvironmentObject) *astrov1.ListEnvironmentObjectsResponse {
		return &astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1.EnvironmentObjectsPaginated{
				EnvironmentObjects: objs,
				TotalCount:         total,
			},
		}
	}
	atOffset := func(want int) func(*astrov1.ListEnvironmentObjectsParams) bool {
		return func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == want
		}
	}

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(atOffset(0))).Return(window(first), nil)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(atOffset(emfetch.PageLimit))).Return(
		window([]astrov1.EnvironmentObject{envVarObj("LAST_KEY", "found", false)}), nil)

	p := NewProvider(testWorkspace, mc, false)

	v, ok := p.Lookup("LAST_KEY")
	require.True(t, ok, "a key in the second window resolves")
	require.Equal(t, "found", v)
}
