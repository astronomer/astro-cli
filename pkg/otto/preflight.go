package otto

import (
	http_context "context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// ErrNotOrgMember signals that the signed-in user isn't a member of the
// organization `astro otto` is about to launch against. ottoRun intercepts it
// and exits silently (guidance is already printed), mirroring ErrNotLoggedIn.
var ErrNotOrgMember = errors.New("not a member of the target organization")

// preflightTimeout bounds the membership preflight. A slow API must not hold
// up the launch for longer than this; a preflight that runs out lets it proceed.
const preflightTimeout = 5 * time.Second

// effectiveOrganization returns the organization id BuildEnv will actually hand
// to Otto, and whether it came from the shell. The login context wins when it
// carries an org; otherwise a value inherited from the shell's
// ASTRO_ORGANIZATION passes through, because BuildEnv's set() only overwrites
// when the context value is non-empty. The preflight must validate whichever
// of the two Otto will really run under.
func effectiveOrganization(cfg *Config) (org string, fromEnv bool) {
	if cfg.Organization != "" {
		return cfg.Organization, false
	}
	return os.Getenv("ASTRO_ORGANIZATION"), true
}

// verifyOrgMembership confirms the current token can access the organization
// Otto will run against, and fails fast with actionable guidance when it can't.
// This closes the gap where `astro otto` would spawn with a token/organization
// mismatch (a stale ASTRO_ORGANIZATION export, or an org selected under a
// different login) and only surface it much later as a confusing skills 403 —
// the LLM gateway isn't RBAC-gated, so the mismatch stays invisible until the
// first org-scoped, permission-checked call.
//
// It fails CLOSED only on a definitive 403/404 for a login that is a user —
// one the self-user lookup can name. An API token reaches an organization by
// its own grants rather than membership, so it is never blocked here. Any
// other outcome — request error, timeout, 5xx, unexpected shape, a self-user
// lookup that fails — is treated as inconclusive and lets the launch proceed.
// The preflight must never become a new way for `astro otto` to break.
func verifyOrgMembership(cfg *Config, client astrov1.APIClient, out io.Writer) error {
	org, fromEnv := effectiveOrganization(cfg)
	if org == "" || cfg.Token == "" {
		// Nothing to check: no target org (server-side/login handles resolution),
		// or not logged in (already gated by the caller).
		return nil
	}

	ctx, cancel := http_context.WithTimeout(http_context.Background(), preflightTimeout)
	defer cancel()

	resp, err := client.GetOrganizationWithResponse(ctx, org, &astrov1.GetOrganizationParams{})
	if err != nil {
		logger.Debugf("otto: org membership preflight skipped (request error): %v", err)
		return nil
	}
	if resp.HTTPResponse == nil {
		return nil
	}

	switch resp.HTTPResponse.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
	default:
		// 2xx (member) or anything inconclusive: don't block the launch.
		return nil
	}

	createIfNotExist := false
	self, err := client.GetSelfUserWithResponse(ctx, &astrov1.GetSelfUserParams{CreateIfNotExist: &createIfNotExist})
	if err != nil || self.JSON200 == nil || self.JSON200.Username == "" {
		// An API token, or a user we can't confirm: don't block the launch.
		logger.Debugf("otto: org membership preflight skipped (no user to name): %v", err)
		return nil
	}
	printOrgMismatch(self.JSON200.Username, org, fromEnv, out)
	return ErrNotOrgMember
}

// printOrgMismatch writes actionable guidance naming the signed-in user and
// where the organization came from, so the failure reads as "wrong logged-in
// user for this org" rather than a mystery.
func printOrgMismatch(who, org string, fromEnv bool, out io.Writer) {
	fmt.Fprintf(out, "%s isn't a member of organization %s, so Otto can't run against it.\n", who, org)
	fmt.Fprintln(out)
	if fromEnv {
		fmt.Fprintln(out, "The organization comes from ASTRO_ORGANIZATION in your environment. To fix:")
		fmt.Fprintln(out, "  • Clear the export:        unset ASTRO_ORGANIZATION")
	} else {
		fmt.Fprintln(out, "The org was selected for this login. To fix:")
		fmt.Fprintln(out, "  • Switch org:              astro organization switch")
	}
	fmt.Fprintln(out, "  • Sign in as another user: astro login")
}

// newV1Client builds a v1 public-API client bound to the current login context
// (token + REST base resolved by astrov1's request editor). Split out so the
// preflight is easy to exercise with a mock client in tests.
func newV1Client() astrov1.APIClient {
	return astrov1.NewV1Client(httputil.NewHTTPClient())
}
