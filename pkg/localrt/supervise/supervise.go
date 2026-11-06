// Package supervise runs a child process (a standalone Airflow launch),
// owns its capped log file, and — when armed with a parent PID — kills the
// child when that parent exits. Ported from Astro Desktop's supervise
// package, with one adaptation: the desktop app is long-lived and can cap
// logs through an in-process pipe, but the CLI exits right after starting,
// so the supervisor owns the log file instead. Every standalone launch goes
// through it; the parent watch is armed only for Plan.StopWithSession,
// where the starting process may die without running its stop path
// (SIGKILL, a crashed IDE session) and Airflow must not outlive it.
//
// The engine spawns Airflow as `astro __supervise --log-file <path>
// [--parent-pid <pid>] -- <airflow-bin> standalone`. The supervisor is the
// process group leader (the engine launches it with Setpgid), so SIGTERM to
// the pgid in the normal stop path terminates supervisor and Airflow
// together; the parent watch only matters when that path never runs.
//
// The wake-up is event-driven where the platform allows: kqueue on darwin,
// pidfd on linux, a poll fallback elsewhere.
package supervise

// Subcommand is the hidden CLI subcommand that triggers supervisor mode.
const Subcommand = "__supervise"

const (
	parentPIDFlagName = "parent-pid"
	logFileFlagName   = "log-file"
	envFDFlagName     = "env-fd"
)

// ParentPIDFlag, LogFileFlag and EnvFDFlag are the flag tokens (with leading
// dashes) callers pass when building the supervisor command line. Derived from
// the flag names so the two forms cannot drift.
//
// EnvFDFlag names an inherited descriptor carrying the child's secret values
// (internal/secretenv): the supervisor reads it to end of file, closes it, and
// adds the values to the child's environment only, so they never sit on a
// command line or in the supervisor's own environment.
const (
	ParentPIDFlag = "--" + parentPIDFlagName
	LogFileFlag   = "--" + logFileFlagName
	EnvFDFlag     = "--" + envFDFlagName
)
