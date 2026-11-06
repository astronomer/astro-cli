package localdocker

import (
	"context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ErrStartTimedOut is a start whose containers did not finish starting in
// time. Unlike ErrHealthTimeout, nothing is left coming up: the start is
// handled as any failed up is.
var ErrStartTimedOut = rt.ErrStartTimedOut

// composeUp brings the project's containers up, under a deadline of the health
// timeout.
//
// The up needs one because it does not return when the containers are
// started. The Airflow services wait for the migration to complete, and the
// migration for postgres to be healthy, so compose waits inside the up. A
// postgres that never comes up is compose's own to report — its healthcheck
// fails, and the up exits with "dependency failed to start" — but a migration
// that hangs, or anything else compose waits on forever, held the up open for
// good, and the health timeout, applied only after it, never started.
//
// Pulling and creating the containers happen first and outside the deadline,
// because a cold pull of a 1.3 GB runtime image on a slow link is not the
// failure it is about.
func (e *Engine) composeUp(ctx context.Context, up *composeLine, cb rt.Callbacks) error {
	// `up --no-start` rather than `pull`: it pulls only what is missing — the
	// same policy the up always had — so an offline start with cached images
	// still works, and the locally built image is not looked for in a
	// registry.
	if err := e.runCompose(ctx, *up, cb, "up", "--no-start", "--quiet-pull"); err != nil {
		return err
	}
	upCtx, cancel := context.WithTimeout(ctx, e.healthTimeout)
	defer cancel()
	// --quiet-pull here too: nothing should be left to pull, but a pull_policy
	// in an override, or an image removed between the two steps, would
	// otherwise stream progress bars through cb.OnLine.
	err := e.runCompose(upCtx, *up, cb, "up", "--detach", "--quiet-pull")
	if err != nil && ctx.Err() == nil && errors.Is(upCtx.Err(), context.DeadlineExceeded) {
		// The deadline, not the caller's patience: a Ctrl-C keeps the plain
		// error. Either way the caller cleans up.
		return fmt.Errorf("%w after %s", ErrStartTimedOut, e.healthTimeout)
	}
	return err
}
