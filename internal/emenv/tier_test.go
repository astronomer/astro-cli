package emenv

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// Keys lists what the workspace supplies: every kind, under the env key
// Airflow reads, sorted.
func TestKeysListsEveryKind(t *testing.T) {
	mc := typedClient(map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {envVarObj("TOKEN", "t", true)},
		astrov1.AIRFLOWVARIABLE:     {airflowVarObj("region", "eu", true)},
		astrov1.CONNECTION:          {connObj("db", &astrov1.EnvironmentObjectConnection{Type: "postgres", Password: ptr("pw")})},
	})
	p := loggedInProvider(t, mc, true)
	require.Equal(t, []string{"AIRFLOW_CONN_DB", "AIRFLOW_VAR_REGION", "TOKEN"}, envresolve.Keys(p))
	short, cause := envresolve.Outage(p)
	require.Empty(t, short)
	require.Empty(t, cause)
}

// With the org refusing secret values, a start's Keys leaves out the secrets
// and every native connection, which cannot be injected; the connections feed
// is empty and WorkspaceConnections returns nothing to write.
func TestKeysAndConnectionsLeaveOutWithheldValues(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool { return *p.ShowSecrets })).
		Return(errResp(http.StatusForbidden, "showSecrets is not allowed for this organization"), nil)
	for _, objectType := range fetchedTypes {
		var rows []astrov1.EnvironmentObject
		switch objectType {
		case astrov1.ENVIRONMENTVARIABLE:
			rows = []astrov1.EnvironmentObject{envVarObj("PLAIN", "v", false), envVarObj("SECRET", "", true)}
		case astrov1.CONNECTION:
			rows = []astrov1.EnvironmentObject{connObj("db", &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: ptr("h")})}
		case astrov1.AIRFLOWVARIABLE, astrov1.METRICSEXPORT:
		}
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
				return !*p.ShowSecrets && *p.ObjectType == objectType
			})).Return(okResp(rows...), nil)
	}
	p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), true)
	require.Equal(t, []string{"PLAIN"}, envresolve.Keys(p))

	conns, err := WorkspaceConnections(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), ReadTimeout)
	require.NoError(t, err)
	require.Empty(t, conns)
}

// The warehouse feed decodes native connections with their credentials.
func TestWorkspaceConnectionsDecodesNativeConnections(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := typedClient(map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {envVarObj("AIRFLOW_CONN_ENV_KEYED", `{"conn_type":"http"}`, false)},
		astrov1.CONNECTION:          {connObj("wh", &astrov1.EnvironmentObjectConnection{Type: "snowflake", Login: ptr("u"), Password: ptr("pw")})},
	})
	conns, err := WorkspaceConnections(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), ReadTimeout)
	require.NoError(t, err)
	require.Len(t, conns, 1, "only native connections feed the warehouses")
	require.Equal(t, "wh", conns[0].ConnID)
	require.Equal(t, "pw", conns[0].ConnPassword)

	none, err := WorkspaceConnections(Workspace{ID: "", Domain: testDomain}, clientsOf(mc), ReadTimeout)
	require.NoError(t, err)
	require.Nil(t, none)
}

// A read that outlasts the timeout is an outage, and the provider is absent.
func TestSlowReadTimesOut(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	restore := fetchTimeout
	fetchTimeout = 20 * time.Millisecond
	t.Cleanup(func() { fetchTimeout = restore })
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { <-args.Get(0).(context.Context).Done() }).
		Return(nil, context.DeadlineExceeded)
	p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), true)
	_, ok := p.Lookup("X")
	require.False(t, ok)
	short, cause := envresolve.Outage(p)
	require.Equal(t, "timed out", short)
	require.Contains(t, cause, "could not reach")
	require.Nil(t, envresolve.Keys(p))

	_, err := WorkspaceConnections(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), 20*time.Millisecond)
	require.ErrorContains(t, err, "could not reach")
}

// A key that cannot be an env-var name is not supplied, and is named, without
// its value, in the note start and list print.
func TestSkippedKeysAreNamed(t *testing.T) {
	mc := typedClient(map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {envVarObj("GOOD", "v", false), envVarObj("bad-key", "hidden-value", false)},
	})
	p := loggedInProvider(t, mc, true)
	require.Equal(t, []string{"GOOD"}, envresolve.Keys(p))
	require.Equal(t, []string{"bad-key"}, envresolve.SkippedKeys(p))
	note := envresolve.SkippedNote(p, testWorkspace)
	require.Contains(t, note, "workspace "+testWorkspace+" holds bad-key,")
	require.NotContains(t, note, "hidden-value")
}
