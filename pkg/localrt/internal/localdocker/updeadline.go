package localdocker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The health timeout is one budget for the whole of "Airflow comes up": the
// `compose up --detach` runs under it, and the health wait gets what the up
// left.
//
// The up needs bounding because it does not return when the containers are
// started. The Airflow services wait for the migration to complete, and the
// migration for postgres to be healthy, so compose waits inside the up. A
// postgres that never comes up is compose's own to report — its healthcheck
// fails, and the up exits with "dependency failed to start" — but a migration
// that hangs, or anything else compose waits on forever, held the up open for
// good, and the health timeout, applied only after it, never started. The
// budget is the safety net for that, and only that: what was pending is the
// rollback's captured output to show, not something this reconstructs.
//
// Pulling and creating the containers happen first and outside the budget,
// because a cold pull of a 1.3 GB runtime image on a slow link is not the
// failure the knob is about.
func (e *Engine) composeUp(ctx context.Context, up *composeLine, cb rt.Callbacks) (time.Duration, error) {
	// `up --no-start` rather than `pull`: it pulls only what is missing — the
	// same policy the up always had — so an offline start with cached images
	// still works, and the locally built image is not looked for in a
	// registry.
	if err := e.runCompose(ctx, *up, cb, "up", "--no-start", "--quiet-pull"); err != nil {
		return 0, err
	}
	began := e.now()
	upCtx, cancel := context.WithTimeout(ctx, e.healthTimeout)
	defer cancel()
	// --quiet-pull here too: nothing should be left to pull, but a pull_policy
	// in an override, or an image removed between the two steps, would
	// otherwise stream progress bars through cb.OnLine.
	err := e.runCompose(upCtx, *up, cb, "up", "--detach", "--quiet-pull")
	if err != nil && ctx.Err() == nil && errors.Is(upCtx.Err(), context.DeadlineExceeded) {
		// The budget ran out, not the caller's patience: a Ctrl-C keeps the
		// plain error. Either way the caller's rollback cleans up.
		return 0, fmt.Errorf("%w after %s, before the containers finished starting", ErrHealthTimeout, e.healthTimeout)
	}
	return e.healthTimeout - e.now().Sub(began), err
}

// minHealthWait is the least the health wait is given, so an up that finished
// at the very end of the budget still gets a probe rather than a timeout that
// never asked.
const minHealthWait = time.Second

// awaitHealth waits for Airflow to answer with what the up left of the budget.
//
// The wait is told the time it actually has, so its message reports that, and
// a timeout says where the rest went: "timed out after 12s" from a start given
// 45s otherwise reads as the setting not taking.
func (e *Engine) awaitHealth(ctx context.Context, urls []string, remaining time.Duration) error {
	wait := max(remaining, minHealthWait)
	if wait > time.Second {
		wait = wait.Round(time.Second)
	}
	err := e.health(ctx, urls, wait)
	if err != nil && errors.Is(err, ErrHealthTimeout) && wait < e.healthTimeout {
		return fmt.Errorf("%w (what was left of the %s budget: the containers took %s of it to start)",
			err, e.healthTimeout, (e.healthTimeout - remaining).Round(time.Second))
	}
	return err
}
