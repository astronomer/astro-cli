package emenv

import (
	"errors"
	"time"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/emfetch"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Login is the stored login a read uses: the one for the manifest's domain,
// whatever the CLI's current context names. Its own type, not config.Context,
// so the core packages that wire the provider never import config/.
type Login struct {
	Domain string
	Token  string
	// APIURL is the host's v1 base URL.
	APIURL string
}

// ClientFactory builds the v1 API client a read uses from the login it reads
// with. The provider picks that login from the manifest's domain, so it cannot
// take a client made for the current context.
type ClientFactory func(Login) astrov1.APIClient

// Clients is the factory every real read uses: one client per login, bound to
// that login's token and host.
func Clients(l Login) astrov1.APIClient {
	return astrov1.NewV1ClientForLogin(httputil.NewHTTPClient(), l.Token, l.APIURL)
}

// Workspace is the linked workspace a read is of, as the manifest states it.
type Workspace struct {
	// ID is [tool.astro] workspace; empty when the manifest sets none.
	ID string
	// Domain is the host it lives on, already defaulted
	// (manifest.Astro.WorkspaceDomain). It picks the login the read uses.
	Domain string
	// Organization is [tool.astro] organization, empty when the manifest names
	// none and the read asks under the login's organization.
	Organization string
}

// WorkspaceOf is the workspace a run reads for a's manifest: the one it links,
// else a token's own (see WorkspaceID).
//
// The host is the manifest's domain, else ASTRO_DOMAIN, else astronomer.io, in a
// run with a token (astrosession.TokenHost) and in one without alike; the saved
// context's host is in neither.
func WorkspaceOf(a *manifest.Astro) Workspace {
	domain := a.WorkspaceDomain()
	if astrosession.HasAPIToken() {
		domain = astrosession.TokenHost("", a.Domain)
	}
	return Workspace{ID: WorkspaceID(a), Domain: domain, Organization: a.Organization}
}

// WorkspaceID is the Workspace a run reads Environment Manager objects from:
// the manifest's `workspace` when it names one, else, in a run with
// ASTRO_API_TOKEN, the Workspace the token's claims name (a Workspace or
// Deployment token has one), and else "". An Organization token names none, so
// the manifest must; the saved login's last Workspace is never used, token or
// not, because the manifest is what links a project to a Workspace.
func WorkspaceID(a *manifest.Astro) string {
	if a.Workspace != "" {
		return a.Workspace
	}
	if id, ok, err := astrosession.TokenIdentity(); ok && err == nil {
		return id.Workspace
	}
	return ""
}

// NewProvider builds the workspace Environment Manager provider for a run: it
// resolves `source = "workspace"` names against ws's objects, read with the
// login stored for ws.Domain, under ws.Organization, else that login's
// organization. reveal asks for secret values (start and get); list passes
// reveal = false so it reads presence only and never pulls a secret value. One
// provider serves the whole run, so its fetch is shared across every
// workspace-source name.
//
// An empty ws.ID means the manifest sets no top-level `workspace` and the
// run has no token naming one (see WorkspaceID), which a workspace source
// needs: the provider is then unavailable and names the fix.
func NewProvider(ws Workspace, clientFor ClientFactory, reveal bool) envresolve.Provider {
	if ws.ID == "" {
		if _, ok, err := astrosession.TokenIdentity(); ok && err != nil {
			// An unusable token is the run's failure, not a missing key.
			return Unavailable(err.Error())
		}
		return Unavailable(emfetch.CauseNoWorkspace.Text(ws.Domain, ""))
	}
	return newProvider(ws, clientFor, reveal)
}

func newProvider(ws Workspace, clientFor ClientFactory, reveal bool) *provider {
	return &provider{workspaceID: ws.ID, domain: ws.Domain, organization: ws.Organization, clientFor: clientFor, reveal: reveal}
}

// Unavailable returns a provider that is absent for the given reason, such as a
// manifest with no workspace to read. A workspace-source name then resolves
// from nowhere and, if required, gates the start with the reason.
func Unavailable(reason string) envresolve.Provider {
	return &unavailable{reason: reason}
}

// unavailable is a provider that never resolves and says why.
type unavailable struct{ reason string }

func (u *unavailable) Lookup(string) (string, bool) { return "", false }
func (u *unavailable) Label() string                { return sourceLabel + " (unavailable: " + u.reason + ")" }
func (u *unavailable) Diagnose(string) string       { return u.reason }
func (u *unavailable) Keys() []string               { return nil }
func (u *unavailable) SkippedKeys() []string        { return nil }
func (u *unavailable) Outage() (short, cause string) {
	return u.reason, u.reason
}

// WorkspaceConnections is the native connections ws holds,
// read with secret values, for the warehouse file Otto's analyzing-data skill
// queries through: a connection without its credentials cannot be queried.
// A connection the org's secrets policy withheld is left out. The error is
// the read's outage cause when the workspace could not be read; an empty
// ws.ID reads nothing and returns neither. timeout bounds the read.
func WorkspaceConnections(ws Workspace, clientFor ClientFactory, timeout time.Duration) ([]connmodel.Connection, error) {
	if ws.ID == "" {
		return nil, nil
	}
	p := newProvider(ws, clientFor, true)
	p.timeout = timeout
	conns := p.Connections()
	if _, cause := p.Outage(); cause != "" {
		return nil, errors.New(cause)
	}
	return conns, nil
}
