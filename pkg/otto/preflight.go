package otto

import (
	http_context "context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// ErrNotOrgMember signals that the signed-in user isn't a member of the
// organization `astro otto` is about to launch against. ottoRun intercepts it
// and exits silently (guidance is already printed), mirroring ErrNotLoggedIn.
var ErrNotOrgMember = errors.New("not a member of the target organization")

// effectiveOrganization returns the organization id BuildEnv will actually hand
// to Otto. The login context wins when it carries an org; otherwise a value
// inherited from the shell's ASTRO_ORGANIZATION passes through, because
// BuildEnv's set() only overwrites when the context value is non-empty. The
// preflight must validate whichever of the two Otto will really run under.
func effectiveOrganization(cfg *Config) string {
	if cfg.Organization != "" {
		return cfg.Organization
	}
	return os.Getenv("ASTRO_ORGANIZATION")
}

// verifyOrgMembership confirms the current token can access the organization
// Otto will run against, and fails fast with actionable guidance when it can't.
// This closes the gap where `astro otto` would spawn with a token/organization
// mismatch (a stale ASTRO_ORGANIZATION export, or an org selected under a
// different login) and only surface it much later as a confusing skills 403 —
// the LLM gateway isn't RBAC-gated, so the mismatch stays invisible until the
// first org-scoped, permission-checked call.
//
// It fails CLOSED only on a definitive 403/404 (the subject isn't a member of,
// or can't see, the org). Any other outcome — request error, 5xx, unexpected
// shape — is treated as inconclusive and lets the launch proceed. The preflight
// must never become a new way for `astro otto` to break on a transient blip.
func verifyOrgMembership(cfg *Config, client astrov1.APIClient, out io.Writer) error {
	org := effectiveOrganization(cfg)
	if org == "" || cfg.Token == "" {
		// Nothing to check: no target org (server-side/login handles resolution),
		// or not logged in (already gated by the caller).
		return nil
	}

	resp, err := client.GetOrganizationWithResponse(http_context.Background(), org, &astrov1.GetOrganizationParams{})
	if err != nil {
		logger.Debugf("otto: org membership preflight skipped (request error): %v", err)
		return nil
	}
	if resp.HTTPResponse == nil {
		return nil
	}

	switch resp.HTTPResponse.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		printOrgMismatch(org, client, out)
		return ErrNotOrgMember
	default:
		// 2xx (member) or anything inconclusive: don't block the launch.
		return nil
	}
}

// printOrgMismatch writes actionable guidance naming the signed-in user, so the
// failure reads as "wrong logged-in user for this org" rather than a mystery.
// The self-user lookup is best-effort: if it fails we fall back to a generic
// subject rather than swallowing the guidance.
func printOrgMismatch(org string, client astrov1.APIClient, out io.Writer) {
	who := "The Astro account you're signed in as"
	createIfNotExist := false
	if self, err := client.GetSelfUserWithResponse(http_context.Background(), &astrov1.GetSelfUserParams{CreateIfNotExist: &createIfNotExist}); err == nil &&
		self.JSON200 != nil && self.JSON200.Username != "" {
		who = self.JSON200.Username
	}

	fmt.Fprintf(out, "%s isn't a member of organization %s, so Otto can't run against it.\n", who, org)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "This usually means a stale ASTRO_ORGANIZATION is exported in your shell, or the")
	fmt.Fprintln(out, "org was last selected under a different login. To fix:")
	fmt.Fprintln(out, "  • Switch org:              astro organization switch")
	fmt.Fprintln(out, "  • Sign in as another user: astro login")
	fmt.Fprintln(out, "  • Clear a stale export:    unset ASTRO_ORGANIZATION")
}

// newV1Client builds a v1 public-API client bound to the current login context
// (token + REST base resolved by astrov1's request editor). Split out so the
// preflight is easy to exercise with a mock client in tests.
func newV1Client() astrov1.APIClient {
	return astrov1.NewV1Client(httputil.NewHTTPClient())
}
