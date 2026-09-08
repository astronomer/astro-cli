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
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
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
	if token := os.Getenv(EnvAPIToken); strings.TrimSpace(token) != "" {
		return token, nil
	}
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return "", ErrLoggedOut
	}
	if Credential(ctx.Token) == "" {
		return "", ErrLoggedOut
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
