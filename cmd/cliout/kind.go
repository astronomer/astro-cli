package cliout

import (
	"errors"

	"github.com/astronomer/astro-cli/pkg/input"
)

// ProblemKind is the stable name a failure is published under in json mode.
//
// The prose in "error" is for a person and will keep being reworded. A script
// or an agent that wants to know WHICH failure happened had nothing to read but
// that prose, so the only way to branch was a regex over a sentence nobody
// promised to keep.
//
// A named type, like every other Kind vocabulary here (checks.Kind, pack.Kind,
// localenv.Kind, scaffold.Kind), so the compiler holds the vocabulary rather
// than trusting each literal. snake_case, matching the `kind` a check finding
// already publishes (import_error, duplicate_dag_id).
//
// These names are contract. Adding one is cheap; changing one breaks whatever
// was reading it, which is the whole point of having them.
type ProblemKind string

// KindUsage: the command was invoked wrongly and nothing ran. Exit status 2.
// It is cliout's own because cliout is what recognizes one (IsUsage), and it
// wins over every other kind: a usage error never reached the code that could
// fail any other way.
const KindUsage ProblemKind = "usage"

// KindInputRequired: the command needed an answer it would have asked for at
// a terminal, and this run could not ask — under --output json no command
// prompts (Execute's guard), and some refuse without a terminal too. The
// message names what was asked and, where the command knows it, the flag that
// answers it. Exit status 1, like any failure: the question can come after
// the command has started work (a v1 DAG deploy asks about an empty dags/
// after creating its deploy record), so "nothing ran" (usage's 2) is not a
// promise it can keep, and the same refusal without a terminal has always
// exited 1.
// Also cliout's own, because pkg/input is what recognizes one.
const KindInputRequired ProblemKind = "input_required"

// KindRule names one failure: Match recognizes it, Kind is what it publishes.
//
// A predicate rather than a bare sentinel, because not every failure worth a
// name is one: some arrive as a typed error carrying detail, or as a status on
// a typed error.
//
// What earns a rule is that the CLI can recognize the failure reliably — a
// sentinel it wraps, or a type it can assert. What does NOT earn one is a
// failure nothing can currently emit: a documented name that can never appear
// is worse than no name, because a consumer writes a branch for it and never
// learns the branch is dead.
type KindRule struct {
	Kind  ProblemKind
	Match func(error) bool
}

// Kinds is an ordered table of rules.
//
// A slice rather than a map because order decides: errors.Is walks every
// branch of a wrap — and of an errors.Join — so one error can match more than
// one rule, and the first wins. Each family pins the order of its own table by
// test, so tidying one cannot quietly change what a failure publishes.
type Kinds []KindRule

// Of returns the published name for err, or "" when this failure has no kind.
//
// Empty rather than an "unknown" catch-all, and the field is omitempty, so an
// unclassified failure publishes no kind at all. A catch-all would let a
// consumer believe it had branched on something when it had only been told the
// name of the default.
func (k Kinds) Of(err error) ProblemKind {
	if err == nil {
		return ""
	}
	if IsUsage(err) {
		return KindUsage
	}
	if input.IsRequired(err) {
		return KindInputRequired
	}
	for _, r := range k {
		if r.Match(err) {
			return r.Kind
		}
	}
	return ""
}

// Sentinel matches an error that wraps target.
func Sentinel(target error) func(error) bool {
	return func(err error) bool { return errors.Is(err, target) }
}
