package emenv

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// loginInOrg stubs the stored login as one in organization cllogin. The
// config is initialized because load builds the API URL from it.
func loginInOrg(t *testing.T) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	restore := login
	t.Cleanup(func() { login = restore })
	login = func(domain string) (config.Context, error) {
		return config.Context{Domain: domain, Token: "Bearer t", Organization: "cllogin"}, nil
	}
}

// orgClient answers every list call with resp and records the organization
// each one asked under.
func orgClient(resp *astrov1.ListEnvironmentObjectsResponse, asked *[]string) *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { *asked = append(*asked, args.String(1)) }).
		Return(resp, nil)
	return mc
}

// The read asks under the manifest's organization when it names one, and
// under the login's when it does not, for every object type.
func TestReadAsksUnderTheManifestsOrganization(t *testing.T) {
	for _, tc := range []struct {
		name, declared, want string
	}{
		{"declared", "clother", "clother"},
		{"fallback", "", "cllogin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loginInOrg(t)
			var asked []string
			mc := orgClient(okResp(envVarObj("X", "v", false)), &asked)
			p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain, Organization: tc.declared}, clientsOf(mc), true)
			v, ok := p.Lookup("X")
			require.True(t, ok)
			require.Equal(t, "v", v)
			require.Len(t, asked, len(fetchedTypes))
			for _, org := range asked {
				require.Equal(t, tc.want, org)
			}
		})
	}
}

// WorkspaceOf carries the manifest's organization, so every caller that builds
// a provider from a manifest asks where the manifest says.
func TestWorkspaceOfCarriesTheOrganization(t *testing.T) {
	t.Setenv("ASTRO_DOMAIN", "")
	a := manifest.Astro{Workspace: "cmws", Domain: "astronomer-dev.io", Organization: "clorg"}
	require.Equal(t, Workspace{ID: "cmws", Domain: "astronomer-dev.io", Organization: "clorg"}, WorkspaceOf(&a))
	a = manifest.Astro{Workspace: "cmws"}
	require.Equal(t, Workspace{ID: "cmws", Domain: manifest.DefaultWorkspaceDomain}, WorkspaceOf(&a))
}

// The warehouse feed reads under the manifest's organization too.
func TestWorkspaceConnectionsAsksUnderTheManifestsOrganization(t *testing.T) {
	loginInOrg(t)
	var asked []string
	mc := orgClient(okResp(), &asked)
	_, err := WorkspaceConnections(Workspace{ID: testWorkspace, Domain: testDomain, Organization: "clother"}, clientsOf(mc), ReadTimeout)
	require.NoError(t, err)
	require.NotEmpty(t, asked)
	for _, org := range asked {
		require.Equal(t, "clother", org)
	}
}

// A 403 or 404 under the organization the manifest names is that
// organization's cause, naming it and `astro organization list`. Under the
// login's own organization the causes stay the plain access and not-found
// ones, which point at `astro organization switch`.
func TestADeclaredOrganizationsRefusalNamesIt(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			loginInOrg(t)
			var asked []string
			mc := orgClient(errResp(status, "nope"), &asked)
			p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain, Organization: "clother"}, clientsOf(mc), true)
			cause := p.(envresolve.Diagnoser).Diagnose("X")
			require.Contains(t, cause, "organization clother")
			require.Contains(t, cause, "workspace "+testWorkspace)
			require.Contains(t, cause, "`astro organization list`")
			short, _ := envresolve.Outage(p)
			require.Equal(t, "no access to organization", short)

			loginInOrg(t)
			mc = orgClient(errResp(status, "nope"), &asked)
			p = NewProvider(Workspace{ID: testWorkspace, Domain: testDomain}, clientsOf(mc), true)
			cause = p.(envresolve.Diagnoser).Diagnose("X")
			require.NotContains(t, cause, "astro organization list")
			require.Contains(t, cause, "current organization")
		})
	}
}

// The organization's secrets refusal names the organization the read asked
// under, the manifest's when it names one.
func TestSecretsRefusalNamesTheDeclaredOrganization(t *testing.T) {
	loginInOrg(t)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool { return *p.ShowSecrets })).
		Return(errResp(http.StatusMethodNotAllowed, "showSecrets is not allowed for this organization"), nil)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool { return !*p.ShowSecrets })).
		Return(okResp(envVarObj("SECRET_TOKEN", "", true)), nil)
	p := NewProvider(Workspace{ID: testWorkspace, Domain: testDomain, Organization: "clother"}, clientsOf(mc), true)
	_, ok := p.Lookup("SECRET_TOKEN")
	require.False(t, ok)
	require.Contains(t, p.(envresolve.Diagnoser).Diagnose("SECRET_TOKEN"), "organization clother disables Environment Secrets Fetching")
}
