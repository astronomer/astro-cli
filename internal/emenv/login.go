package emenv

import (
	"errors"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/astroauth"
)

// errSessionExpired is a stored login whose access token has expired and could
// not be refreshed.
var errSessionExpired = errors.New("session expired")

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

// freshLogin returns the login stored for domain with a usable access token,
// refreshing a stale one first and saving the result under that domain's entry.
//
// The CLI refreshes only the current context's token on its own. The workspace
// a project links names its own host, which need not be the current one — a
// production-linked project read while the CLI is switched to dev — and
// without this its token would expire and every start would report the
// session expired, with `astro login` as the fix, which switches the CLI back.
// The refresh writes the domain's own context keys and never the current
// context pointer.
//
// A login with no refresh token is returned as it is; the platform's 401 then
// names the expired session.
func freshLogin(domain string) (config.Context, error) {
	c := config.Context{Domain: domain}
	ctx, err := c.GetContext()
	if err != nil || ctx.RefreshToken == "" {
		return ctx, err
	}
	if exp, _ := ctx.GetExpiresIn(); time.Now().Add(refreshThreshold).Before(exp) { //nolint:errcheck // a missing expiry reads as zero, which is expired: refresh
		return ctx, nil
	}
	tok, err := refreshLogin(domain, ctx.RefreshToken)
	if err != nil {
		return ctx, errSessionExpired
	}
	bearer := "Bearer " + tok.AccessToken
	if err := ctx.SetContextKey("token", bearer); err != nil {
		return ctx, err
	}
	if err := ctx.SetExpiresIn(tok.ExpiresIn); err != nil {
		return ctx, err
	}
	if tok.RefreshToken != "" && tok.RefreshToken != ctx.RefreshToken {
		if err := ctx.SetContextKey("refreshtoken", tok.RefreshToken); err != nil {
			return ctx, err
		}
		ctx.RefreshToken = tok.RefreshToken
	}
	ctx.Token = bearer
	return ctx, nil
}
