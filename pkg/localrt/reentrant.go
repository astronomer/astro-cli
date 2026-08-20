package localrt

import (
	"context"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localdocker"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstandalone/supervise"
)

// The engines re-invoke their own binary for two helper processes, so every
// consumer that embeds this runtime has to expose both as hidden subcommands —
// otherwise a session-tied docker start has nothing to spawn, and standalone
// Airflow runs unsupervised. The argv the engines build uses these exact names.
//
// This is the part of the contract that is easy to miss, because nothing fails at
// build time when a consumer forgets: the feature simply does not work.
const (
	// SessionWatchSubcommand watches the process that started a session-tied
	// docker Airflow and stops the compose stack when it exits.
	SessionWatchSubcommand = localdocker.SessionWatchSubcommand
	// SessionWatchParentPIDFlag and SessionWatchProjectFlag are the argv this
	// subcommand is spawned with. Exported for the same reason the supervisor's
	// are: the engine builds the command line from these constants, the consumer
	// parses it, and a rename with the strings hardcoded on the consumer side
	// breaks session cleanup silently.
	SessionWatchParentPIDFlag = localdocker.SessionParentPIDFlag
	SessionWatchProjectFlag   = localdocker.SessionProjectFlag
	// SuperviseSubcommand is the supervisor the standalone engine wraps every
	// Airflow launch in.
	SuperviseSubcommand = supervise.Subcommand

	// SuperviseParentPIDFlag arms the parent watch: the supervisor kills the
	// child when that PID exits. Omit it and the child is only stopped through
	// the normal path.
	SuperviseParentPIDFlag = supervise.ParentPIDFlag
	// SuperviseLogFileFlag makes the supervisor open, truncate, and cap a log
	// file of its own. Omit it and the child inherits the spawner's stdout and
	// stderr untouched — which is what a spawner that already owns the log file
	// needs, since the supervisor's copy would truncate it and its capper would
	// die with the supervisor.
	SuperviseLogFileFlag = supervise.LogFileFlag
)

// WatchAndStop is the body of the SessionWatchSubcommand process: wait for
// parentPID to exit, then stop the project's docker Airflow.
func (r *Runtime) WatchAndStop(ctx context.Context, projectPath string, parentPID int) error {
	return r.docker.WatchAndStop(ctx, projectPath, parentPID)
}

// RunSupervisor is the body of the SuperviseSubcommand process. It parses its own
// argv, which is why it takes the raw args.
func RunSupervisor(args []string) error { return supervise.Run(args) }
