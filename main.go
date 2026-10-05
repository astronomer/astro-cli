package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --version
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./internal/platform/astro/clients/astrov1/api.cfg.yaml ../astro/apps/core/docs/versioned/v1.0/versionedapi_v1.0.yaml
//go:generate go run ./scripts/patch-v1-gen ./internal/platform/astro/clients/astrov1/api.gen.go
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./internal/platform/astro/clients/astrov1alpha1/api.cfg.yaml ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml

func main() {
	// TODO: Remove this when version logic is implemented
	fs := afero.NewOsFs()
	config.InitConfig(fs)

	ctx := signalContext()

	// The command has already reported its failure by the output contract
	// (cliout.Execute); what is left is the exit status. 130 for an interrupt —
	// the command has already unwound, and `astro local start` tears its
	// half-created containers back down on the way out — 2 for a usage error,
	// a command's own code when it carries one (`astro local check`), and 1
	// otherwise. cliout.ExitCode holds the table.
	if err := cmd.Execute(ctx); err != nil {
		os.Exit(cliout.ExitCode(ctx, err))
	}

	// platform specific terminal initialization:
	// this should run for all commands,
	// for most of the architectures there's no requirements:
	ansi.InitConsole()
}

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
