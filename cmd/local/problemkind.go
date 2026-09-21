package local

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// ProblemKind is the stable name a failure is published under in json mode.
//
// The prose in "error" is for a person and will keep being reworded — this
// repo's plan says so, and says to assert on JSON rather than on text for
// exactly that reason. A script or an agent that wants to know WHICH failure
// happened had nothing to read but that prose, so the only way to branch was a
// regex over a sentence nobody promised to keep.
//
// A named type, like every other Kind vocabulary here (checks.Kind, pack.Kind,
// localenv.Kind, scaffold.Kind), so the compiler holds the vocabulary rather
// than trusting each literal.
type ProblemKind string

// The kinds. snake_case, matching the `kind` a check finding already publishes
// (import_error, duplicate_dag_id), because a consumer meeting both should not
// have to learn two spellings of one idea.
//
// These names are contract. Adding one is cheap; changing one breaks whatever
// was reading it, which is the whole point of having them.
const (
	// KindNoProject: neither this directory nor any above it is an astro
	// project. First for a consumer as well as first in the table — it is the
	// answer to "can I run anything here at all", and every other kind
	// presumes it is not this.
	KindNoProject ProblemKind = "no_project"
	// KindForeignMode: the project has a runtime record from the other mode,
	// which only that mode's engine can act on.
	KindForeignMode ProblemKind = "foreign_mode"
	// KindAlreadyRunning: a local Airflow is already live for this project.
	// Not transient; something has to stop first.
	KindAlreadyRunning ProblemKind = "already_running"
	// KindHealthTimeout: Airflow did not answer in the time allowed. The
	// runtime may still be coming up — the message says what its mode left
	// behind.
	KindHealthTimeout ProblemKind = "health_timeout"
	// KindLocked: another operation holds this project's lock. The one kind
	// here that says "try again" rather than "something is wrong".
	//
	// Unix only, and not by choice: the Windows lock takes no flock and so
	// never reports contention (localstate/lock_windows.go). A consumer's
	// retry policy is correct everywhere; it simply has nothing to fire on
	// there.
	KindLocked ProblemKind = "locked"
	// KindUnsupportedBase: the project's declared Dockerfile does not build on
	// an Astro Runtime image, so docker mode refuses before starting anything.
	// Not transient and not about the machine: the project has to change.
	KindUnsupportedBase ProblemKind = "unsupported_base"
	// KindNotRunning: no local Airflow is running for this project.
	KindNotRunning ProblemKind = "not_running"
)

// problemKinds maps a failure to the name it publishes under.
//
// A slice rather than a map because order decides: errors.Is walks every branch
// of a wrap — and of an errors.Join, which this tree uses in several places —
// so one error can match more than one row, and the first wins. The order is
// pinned by a test, so tidying this table cannot quietly change what a failure
// publishes.
//
// A predicate rather than a bare sentinel, because not every failure worth a
// name is one. "You are not in a project" arrives as a typed error carrying the
// directory it searched from, which is worth more to a reader than a sentinel
// would be.
//
// What earns a kind is that the CLI can recognize the failure reliably — a
// sentinel it wraps, or a type it can assert. NOT that an outside Go program
// could match the same error: the consumer of --output json reads JSON, and
// *project.NotFoundError lives in internal/, unimportable outside this module,
// which is fine and does not weaken the name at all. An earlier version of this
// rule claimed the opposite and was broken by its own first entry.
//
// What does NOT earn a kind is a failure nothing can currently emit.
// ErrImageNotBuilt and ErrNotImplemented were here and are gone: the first is
// returned only by RunInImage, the second only by HotInstall, Sync and
// Airflow.Env, and none of those is on cmd/local's Runtime interface. A
// documented name that can never appear is worse than no name, because a
// consumer writes a branch for it and never learns the branch is dead.
var problemKinds = []struct {
	kind  ProblemKind
	match func(error) bool
}{
	{KindNoProject, func(err error) bool {
		var notFound *project.NotFoundError
		return errors.As(err, &notFound)
	}},
	// Before already_running: a refusal carrying both is the more specific
	// complaint, since the mode is why stopping and starting again will not
	// help.
	{KindForeignMode, sentinel(localrt.ErrForeignMode)},
	{KindAlreadyRunning, sentinel(localrt.ErrAlreadyRunning)},
	{KindHealthTimeout, sentinel(localrt.ErrHealthTimeout)},
	{KindUnsupportedBase, sentinel(localrt.ErrUnsupportedBase)},
	// ErrStartInProgress, not a second name for it: pkg/localrt already
	// exports this sentinel, and adding another public alias to a module with
	// its own go.mod would mean one failure with two contracts.
	{KindLocked, sentinel(localrt.ErrStartInProgress)},
	{KindNotRunning, sentinel(localrt.ErrNotRunning)},
}

// sentinel matches an error that wraps target.
func sentinel(target error) func(error) bool {
	return func(err error) bool { return errors.Is(err, target) }
}

// problemKind returns the published name for err, or "" when this failure has
// no kind yet.
//
// Empty rather than an "unknown" catch-all, and the field is omitempty, so an
// unclassified failure publishes no kind at all. A catch-all would let a
// consumer believe it had branched on something when it had only been told the
// name of the default.
func problemKind(err error) ProblemKind {
	for _, p := range problemKinds {
		if p.match(err) {
			return p.kind
		}
	}
	return ""
}
