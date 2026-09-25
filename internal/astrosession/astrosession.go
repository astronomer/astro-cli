// Package astrosession reads the Astro login for the v2 tree: the bearer an
// `astro` instance proves itself with. It exists as its own package for one
// reason — it touches config/, which every other v2 package is barred from
// (docs/v2-architecture.md) — so the read stays in one place and the packages
// that need a token take it as a seam. internal/emenv reads the same logins
// through it.
//
// It never prompts and never logs in. A login named by domain is refreshed
// when its token is stale; the current context is not, and an unusable
// session is a named outage — what happened, and what to do about it.
package astrosession

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/astroauth"
)

// EnvAPIToken is the Astro API token CI supplies instead of a login on the
// machine. It is read here rather than only by the caller that wants a bearer,
// so "is there an identity" has one answer across the v2 tree.
const EnvAPIToken = "ASTRO_API_TOKEN" //nolint:gosec // the name of a variable, not a credential

// The two ways a session can be unusable. Both name both ways back, because a
// machine with no login has two.
//
// ErrLoggedOut is exported so every v2 reader of the session says the same
// sentence: a machine with no login should not get one answer from the
// credential path and a different one from the coordinate lookup.
var (
	ErrLoggedOut = errors.New("you are not logged in — log in with `astro login`, or set " + EnvAPIToken)
	errExpired   = errors.New("your session expired — log in with `astro login`, or set " + EnvAPIToken)
)

// ErrSessionExpired is a stored login whose access token has expired and could
// not be refreshed.
var ErrSessionExpired = errors.New("session expired")

// NotLoggedInTo is the outage for a domain with no stored login, worded as
// docs/v2-workspace-link.md words it, so a workspace read and a deployment link
// fail with the same sentence.
func NotLoggedInTo(domain string) error {
	return fmt.Errorf("not logged in to %s. Log in with `astro login %s`", domain, domain)
}

// ExpiredOn is the outage for a domain whose login has expired, worded the
// same way.
func ExpiredOn(domain string) error {
	return fmt.Errorf("your %s session expired. Log in again with `astro login %s`", domain, domain)
}

// Rejected is the outage for a 401 from domain. With EnvAPIToken set, that
// token is the one sent, so the fix is the variable, not a login.
func Rejected(domain string) error {
	if strings.TrimSpace(os.Getenv(EnvAPIToken)) != "" {
		host := cmp.Or(domain, "Astro")
		return fmt.Errorf("%s rejected the token in %s. Check it, or unset it to use your `astro login` session", host, EnvAPIToken)
	}
	if domain == "" {
		return errExpired
	}
	return ExpiredOn(domain)
}

// Credential is the credential inside a stored context token, empty when there
// is none. A context that has been logged out of keeps its scheme and loses its
// token — it reads as "Bearer " with nothing after it (cmd/astro/setup.go reads
// it the same way) — so "is anyone logged in" is a question about what follows
// the scheme, not about whether the field is set.
//
// It is exported so every v2 reader of the session answers that question the
// same way: `astro local start` deciding whether to consult Environment Manager
// and a query command deciding whether it has a bearer must not disagree.
func Credential(stored string) string {
	return strings.TrimSpace(strings.TrimPrefix(stored, "Bearer "))
}

// Bearer returns the current identity's token exactly as it is stored, scheme
// and all: airflowapi.BearerToken normalizes that away, and a second
// implementation of the same trimming here is one more place for the two to
// disagree. It matches the seam pkg/instances takes for the astro auth
// method, context and all, though nothing about reading the local config
// blocks.
//
// ASTRO_API_TOKEN wins when it is set. That is how CI supplies an identity with
// no login on the machine at all, and reading it here rather than only where a
// bearer is wanted is what lets a CI run reach an Astro Deployment it has to
// look up first, not just one whose URL it already holds.
func Bearer(context.Context) (string, error) {
	if token, ok := apiToken(); ok {
		return token, nil
	}
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return "", ErrLoggedOut
	}
	if Credential(ctx.Token) == "" {
		return "", ErrLoggedOut
	}
	// An expired current context is reported rather than renewed, and reported
	// before the request, so the failure names the cause instead of arriving
	// as a 401 from Airflow.
	if expired(&ctx) {
		return "", errExpired
	}
	return ctx.Token, nil
}

// ErrNoDomain reports a machine with no Astro host to name: no ASTRO_DOMAIN, and
// no login whose context would say which host it is for.
var ErrNoDomain = errors.New("no Astro login to take the domain from")

// Domain is the Astro host the current login is for, as `astro login` stored
// it: ASTRO_DOMAIN when that is set, else the host the current context names.
// Linking a workspace defaults its [tool.astro] domain to this, so a link made
// while logged in to a host is read back with that host's login. The session
// behind it is not checked: the host of an expired login is still its host.
func Domain() (string, error) {
	domain, err := config.GetCurrentDomain()
	if err != nil || strings.TrimSpace(domain) == "" {
		return "", ErrNoDomain
	}
	return domain, nil
}

// BearerFor is Bearer for a project whose Astro host is domain: after
// ASTRO_API_TOKEN, the login stored for that domain, refreshed when stale,
// whatever host the current context names. An empty domain is a project that
// names none, and reads the current context as Bearer does.
func BearerFor(ctx context.Context, domain string) (string, error) {
	if domain == "" {
		return Bearer(ctx)
	}
	if token, ok := apiToken(); ok {
		return token, nil
	}
	login, err := Login(domain)
	switch {
	case errors.Is(err, ErrSessionExpired):
		return "", ExpiredOn(domain)
	case err != nil && login.ContextExists():
		return "", err
	case err != nil || Credential(login.Token) == "":
		return "", NotLoggedInTo(domain)
	case expired(&login):
		return "", ExpiredOn(domain)
	}
	return login.Token, nil
}

func apiToken() (string, bool) {
	token := os.Getenv(EnvAPIToken)
	return token, strings.TrimSpace(token) != ""
}

// expired reads the expiry the config records at login. A login with none
// recorded counts as live, and the platform's 401 names it if it is not.
func expired(c *config.Context) bool {
	expiry, err := c.GetExpiresIn()
	return err == nil && !expiry.IsZero() && time.Now().After(expiry)
}

// refreshThreshold refreshes a token this close to expiry, the margin the CLI's
// own login check uses.
const refreshThreshold = 5 * time.Minute

// refreshLogin exchanges a refresh token for new tokens on domain. A var so a
// test can drive the refresh without an identity provider.
var refreshLogin = func(domain, refreshToken string) (*astroauth.TokenResponse, error) {
	cfg, err := astroauth.FetchAuthConfig(domain)
	if err != nil {
		return nil, err
	}
	return astroauth.RefreshToken(cfg, refreshToken)
}

// Login returns the login stored for domain with a usable access token,
// refreshing a stale one first and saving the result under that domain's entry.
//
// The CLI refreshes only the current context's token on its own. A project
// names its own host, which need not be the current one — a production-linked
// project used while the CLI is switched to dev — and without this its token
// would expire and every command would report the session expired, with
// `astro login` as the fix, which switches the CLI back. The refresh writes the
// domain's own context keys and never the current context pointer.
//
// A login with no refresh token is returned as it is, and so is one that has
// been logged out of: `astro logout` clears the token but keeps the refresh
// token, and refreshing it would log the user back in.
func Login(domain string) (config.Context, error) {
	c := config.Context{Domain: domain}
	ctx, err := c.GetContext()
	if err != nil || ctx.RefreshToken == "" || Credential(ctx.Token) == "" {
		return ctx, err
	}
	if exp, _ := ctx.GetExpiresIn(); time.Now().Add(refreshThreshold).Before(exp) { //nolint:errcheck // a missing expiry reads as zero, which is expired: refresh
		return ctx, nil
	}
	tok, err := refreshLogin(domain, ctx.RefreshToken)
	if err != nil {
		return ctx, ErrSessionExpired
	}
	if err := save(&ctx, tok); err != nil {
		return ctx, fmt.Errorf("saving the refreshed %s login: %w", domain, err)
	}
	return ctx, nil
}

// save persists a refreshed login. A login renewed from a refresh token works
// on every host on its identity provider tenant, so the whole login goes to
// them, as checkToken's refresh does: sharing the access token alone would
// leave a host calling as one user while it renews as another. A host that has
// not recorded its tenant shares with none.
func save(ctx *config.Context, tok *astroauth.TokenResponse) error {
	bearer := "Bearer " + tok.AccessToken
	refreshToken := ctx.RefreshToken
	if tok.RefreshToken != "" {
		refreshToken = tok.RefreshToken
	}
	if err := ctx.SetSharedContextKey("token", bearer); err != nil {
		return err
	}
	if err := ctx.SetSharedContextKey("refreshtoken", refreshToken); err != nil {
		return err
	}
	if err := ctx.SetSharedExpiresIn(tok.ExpiresIn); err != nil {
		return err
	}
	if err := ctx.SetSharedContextKey("user_email", ctx.UserEmail); err != nil {
		return err
	}
	ctx.Token = bearer
	ctx.RefreshToken = refreshToken
	return nil
}
