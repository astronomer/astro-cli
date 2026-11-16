package emenv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An unusable ASTRO_API_TOKEN is its own outage, named, and the saved login is
// not read in its place: no request is made.
func TestUnusableAPITokenIsAnOutageNotTheSavedLogin(t *testing.T) {
	testUtil.MockKeyring(t)
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
	testUtil.MockKeyring(t)
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

// The Workspace a run reads is the manifest's, else a Workspace or Deployment
// token's own, and never the saved login's: an Organization token, or no token,
// names none.
func TestWorkspaceIDIsTheManifestsThenTheTokens(t *testing.T) {
	linked := &manifest.Astro{Workspace: "manifest-ws"}
	unlinked := &manifest.Astro{}
	org := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org")
	ws := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org", "workspaceId:token-ws")
	dep := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org", "workspaceId:token-ws", "deploymentId:dep")
	expired := testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:org", "workspaceId:token-ws")
	for _, tt := range []struct {
		name, token string
		astro       *manifest.Astro
		want        string
	}{
		{"no token, linked", "", linked, "manifest-ws"},
		{"no token, unlinked", "", unlinked, ""},
		{"organization token, linked", org, linked, "manifest-ws"},
		{"organization token, unlinked", org, unlinked, ""},
		{"workspace token, linked", ws, linked, "manifest-ws"},
		{"workspace token, unlinked", ws, unlinked, "token-ws"},
		{"deployment token, unlinked", dep, unlinked, "token-ws"},
		{"unusable token, unlinked", expired, unlinked, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(astrosession.EnvAPIToken, tt.token)
			require.Equal(t, tt.want, WorkspaceID(tt.astro))
			require.Equal(t, tt.want, WorkspaceOf(tt.astro).ID)
		})
	}
}

// An unusable token with no Workspace to read is named, not blamed on the
// manifest.
func TestNewProviderWithAnUnusableTokenNamesTheToken(t *testing.T) {
	t.Setenv(astrosession.EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:org"))
	p := NewProvider(WorkspaceOf(&manifest.Astro{}), nil, false)
	require.Equal(t, "API Token is expired", p.(envresolve.Diagnoser).Diagnose("X"))
}
