package emenv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An unusable ASTRO_API_TOKEN is its own outage, named, and the saved login is
// not read in its place: no request is made.
func TestUnusableAPITokenIsAnOutageNotTheSavedLogin(t *testing.T) {
	mc := mockClient(okResp(envVarObj("X", "saved-login-value", false)))
	p := loggedInProvider(t, mc, true) // a saved login on localhost
	t.Setenv(astrosession.EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:org"))

	_, ok := p.Lookup("X")
	require.False(t, ok)
	require.Equal(t, "workspace (unavailable: unusable ASTRO_API_TOKEN)", p.Label())
	require.Equal(t, "API Token is expired", p.(envresolve.Diagnoser).Diagnose("X"))
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// The read goes out under the token's own login: its credential, and its
// organization when the manifest names none.
func TestReadUnderATokenUsesTheTokensLogin(t *testing.T) {
	token := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org-claim")
	mc := mockClient(okResp(envVarObj("X", "v", false)))
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Setenv(astrosession.EnvAPIToken, token)
	var got Login
	factory := func(l Login) astrov1.APIClient { got = l; return mc }
	p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain}, factory, true)

	_, ok := p.Lookup("X")
	require.True(t, ok)
	require.Equal(t, "Bearer "+token, got.Token)
	require.Equal(t, "org-claim", mc.Calls[0].Arguments.String(1))
}
