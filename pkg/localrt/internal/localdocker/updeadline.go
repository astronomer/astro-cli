package localdocker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

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
	if up.secretPayload != nil {
		if err := e.handOffSecretEnv(ctx, up, cb); err != nil {
			return err
		}
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

// handOffSecretEnv delivers a start's secret values before anything that reads
// them runs: it starts the service that holds the handoff volume, alone, and
// writes the values into the volume through `compose exec` stdin. The up that
// follows starts the Airflow containers, which compose holds back until that
// service reports the file present (secretEnvSpec).
//
// Stdin is the one channel here the daemon does not keep. The command line
// names only the file, and the write goes through a temporary name and a
// rename, so a reader never sees half a file. umask makes it owner-only.
func (e *Engine) handOffSecretEnv(ctx context.Context, up *composeLine, cb rt.Callbacks) error {
	if err := e.runCompose(ctx, *up, cb, "up", "--detach", "--no-deps", secretEnvService); err != nil {
		return fmt.Errorf("starting the %s service: %w", secretEnvService, err)
	}
	tmp := secretEnvFile + ".tmp"
	write := "umask 077 && cat > " + tmp + " && mv " + tmp + " " + secretEnvFile
	var stderr bytes.Buffer
	stdio := rt.Stdio{In: bytes.NewReader(up.secretPayload), Err: &stderr}
	args := up.argv("exec", "-T", secretEnvService, "sh", "-c", write)
	if err := e.cmd.Run(ctx, up.env(), stdio, up.conn.bin, args...); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("handing secret values to the containers: %w: %s", err, msg)
		}
		return fmt.Errorf("handing secret values to the containers: %w", err)
	}
	return nil
}
