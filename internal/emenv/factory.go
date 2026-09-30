package emenv

import (
	"errors"
	"time"

	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/emfetch"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// Login is the stored login a read uses: the one for the manifest's domain,
// whatever the CLI's current context names. Its own type, not config.Context,
// so the v2 packages that wire the provider never import config/.
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

// NewProvider builds the workspace Environment Manager provider for a run: it
// resolves `source = "workspace"` names against workspaceID's objects on
// domain, read with the login stored for domain. reveal asks for secret values
// (start and get); list passes reveal = false so it reads presence only and
// never pulls a secret value. One provider serves the whole run, so its fetch
// is shared across every workspace-source name.
//
// An empty workspaceID means the manifest sets no top-level `workspace`, which
// a workspace source needs: the provider is then unavailable and names the fix.
// domain is the manifest's, already defaulted (manifest.Astro.WorkspaceDomain).
func NewProvider(workspaceID, domain string, clientFor ClientFactory, reveal bool) envresolve.Provider {
	if workspaceID == "" {
		return Unavailable(emfetch.CauseNoWorkspace.Text(domain, ""))
	}
	return &provider{workspaceID: workspaceID, domain: domain, clientFor: clientFor, reveal: reveal}
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

// WorkspaceConnections is the native connections workspaceID holds on domain,
// read with secret values, for the warehouse file Otto's analyzing-data skill
// queries through: a connection without its credentials cannot be queried.
// A connection the org's secrets policy withheld is left out. The error is
// the read's outage cause when the workspace could not be read; an empty
// workspaceID reads nothing and returns neither. timeout bounds the read.
func WorkspaceConnections(workspaceID, domain string, clientFor ClientFactory, timeout time.Duration) ([]connmodel.Connection, error) {
	if workspaceID == "" {
		return nil, nil
	}
	p := &provider{workspaceID: workspaceID, domain: domain, clientFor: clientFor, reveal: true, timeout: timeout}
	conns := p.Connections()
	if _, cause := p.Outage(); cause != "" {
		return nil, errors.New(cause)
	}
	return conns, nil
}
