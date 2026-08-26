package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --version
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-v1/api.cfg.yaml ../astro/apps/core/docs/versioned/v1.0/versionedapi_v1.0.yaml
//go:generate go run ./scripts/patch-v1-gen ./astro-client-v1/api.gen.go
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-v1alpha1/api.cfg.yaml ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml

func main() {
	// TODO: Remove this when version logic is implemented
	fs := afero.NewOsFs()
	config.InitConfig(fs)

	ctx := signalContext()

	if err := cmd.NewRootCmd().ExecuteContext(ctx); err != nil {
		// Interrupted rather than failed. The command has already unwound —
		// `astro local start` tears its half-created containers back down on the
		// way out — so there is nothing to report and nothing the user did wrong.
		// 130 is what a shell reports for a process ended by SIGINT.
		//
		// Keyed on the context alone, not on the error's identity. A canceled
		// run rarely surfaces context.Canceled: Ctrl-C reaches the whole
		// foreground process group, so what usually comes back is an exec error
		// from `docker compose` dying, and matching on that would print a stack
		// of noise for something the user asked for. If the context is done, the
		// error is a consequence of that.
		if ctx.Err() != nil {
			os.Exit(exitInterrupted)
		}
		// A command that carries its own exit code (e.g. `astro local check`)
		// has already rendered everything the user needs; propagate the code.
		var exit *local.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.Code)
		}
		os.Exit(1)
	}

	// platform specific terminal initialization:
	// this should run for all commands,
	// for most of the architectures there's no requirements:
	ansi.InitConsole()
}

// exitInterrupted is the conventional shell code for a process ended by SIGINT.
const exitInterrupted = 130

// signalContext returns a context canceled on the first interrupt, and restores
// default signal handling once that has happened.
//
// Canceling rather than exiting is the point. `astro local start` writes its
// state record only after `compose up` returns, so a start killed in between
// leaves containers that no later `astro local stop` can find — it looks up the
// project through that record. The teardown for exactly this already exists in
// pkg/localrt and runs on a failed up, under a context deliberately detached
// from the caller's so a cancellation cannot make the cleanup a no-op. It was
// simply unreachable from a Ctrl-C, because nothing here caught one.
//
// The stop after the first signal matters as much as the cancel. signal.Notify
// keeps the handler registered, so without it a second Ctrl-C would be swallowed
// and a user watching a slow teardown would have no way out. Restoring the
// default means the first interrupt asks for a clean stop and the second takes
// one.
func signalContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// No defer for stop: every failing path out of main ends in os.Exit, so it
	// would not run, and a defer that cannot fire reads as cleanup that happens.
	// The only moment it is needed is after the first signal, which is where it
	// is called; the process exiting takes care of the rest.
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx
}
