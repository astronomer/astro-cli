// Package astrosession reads the current Astro login for the v2 tree: the
// bearer an `astro` instance proves itself with. It exists as its own package
// for one reason — it touches config/, which every other v2 package is barred
// from (docs/v2-architecture.md) — so the read stays in one place and the
// packages that need a token take it as a seam. internal/emenv holds the same
// posture for the same session.
//
// It never prompts, never logs in, and never refreshes: an unusable session is
// a named outage — what happened, and what to do about it.
package astrosession

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
)

// The two ways a session can be unusable. Both name the one way back, because
// there is only one.
var (
	errLoggedOut = errors.New("you are not logged in — log in with `astro login`")
	errExpired   = errors.New("your session expired — log in with `astro login`")
)

// Credential is the credential inside a stored context token, empty when there
// is none. A context that has been logged out of keeps its scheme and loses its
// token — it reads as "Bearer " with nothing after it (cmd/cloud/setup.go reads
// it the same way) — so "is anyone logged in" is a question about what follows
// the scheme, not about whether the field is set.
//
// It is exported so every v2 reader of the session answers that question the
// same way: `astro local start` deciding whether to consult Environment Manager
// and a query command deciding whether it has a bearer must not disagree.
func Credential(stored string) string {
	return strings.TrimSpace(strings.TrimPrefix(stored, "Bearer "))
}

// Bearer returns the current session's token exactly as the config stores it,
// scheme and all: airflowapi.BearerToken normalizes that away, and a second
// implementation of the same trimming here is one more place for the two to
// disagree. It matches the seam internal/instances takes for the astro auth
// method, context and all, though nothing about reading the local config
// blocks.
func Bearer(context.Context) (string, error) {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return "", errLoggedOut
	}
	if Credential(ctx.Token) == "" {
		return "", errLoggedOut
	}
	// The token carries an expiry the config records at login. Refreshing it on
	// this path is the known gap, so an expired session is reported
	// rather than renewed — and reported before the request, so the failure
	// names the cause instead of arriving as a 401 from Airflow.
	if expiry, err := ctx.GetExpiresIn(); err == nil && !expiry.IsZero() && time.Now().After(expiry) {
		return "", errExpired
	}
	return ctx.Token, nil
}
