package cliout

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// ExitError carries a process exit code up to main, which is the only place
// that exits. The command has already rendered everything the user needs, so
// nothing prints it again: Execute reports it in neither text nor json mode,
// and main propagates the code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// JSONShown marks err as one whose command already wrote its own JSON object
// to stdout — a start blocked on missing env values publishes that list — so
// Execute does not add the generic error object on top. The process still
// exits non-zero.
func JSONShown(err error) error { return jsonShown{err} }

type jsonShown struct{ err error }

func (e jsonShown) Error() string { return e.err.Error() }
func (e jsonShown) Unwrap() error { return e.err }

// Usage marks err as a usage error: the command was invoked wrongly — an
// unknown flag or subcommand, a flag value it does not accept, the wrong
// number of arguments — and nothing ran. It exits 2 and publishes the kind
// `usage`.
func Usage(err error) error {
	if err == nil {
		return nil
	}
	return &usageError{err}
}

type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// cobraUsagePrefixes are the usage errors cobra returns untyped, from places
// no hook reaches: Find's "unknown command" at the root, and the required-flag
// and flag-group checks it runs after the pre-runs. Matching cobra's own
// wording is the only handle on them, and TestCobraUsageMessagesAreRecognized
// drives each one through the cobra this module pins, so an upgrade that
// rewords one fails there rather than quietly turning exit 2 back into 1.
var cobraUsagePrefixes = []string{
	"unknown command ",
	"required flag(s) ",
	"if any flags in the group ",
	"at least one of the flags in the group ",
}

// IsUsage reports whether err is a usage error.
func IsUsage(err error) bool {
	if err == nil {
		return false
	}
	var u *usageError
	if errors.As(err, &u) {
		return true
	}
	msg := err.Error()
	for _, p := range cobraUsagePrefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

// ErrorObject is the object a failed command writes in json mode. Named
// rather than anonymous so it is a declared type the schema pins can hold:
// this is the shape every `--output json` failure publishes.
type ErrorObject struct {
	Error string `json:"error"`
	// Code is the process exit status the failure ends with.
	Code int `json:"code"`
	// Kind is the stable name for WHICH failure this is, for a consumer that
	// wants to branch. Error is prose and will be reworded; Kind is contract.
	// Absent when the failure has no kind — see Kinds.Of, which does not
	// invent one.
	Kind ProblemKind `json:"kind,omitempty"`
}

// EmitError writes the single JSON error object a failed command reports in
// json mode.
//
// Through a Renderer rather than its own encoder, so that every published
// payload leaves by one door: a test can record what passes through Emit,
// and a payload that goes around it is one nothing can see.
//
// The message is plain text: the backticks its prose puts around a command
// are dropped (ansi.StripBackticks), as they are on a terminal, so a consumer
// shows it as it is.
//
// The text renderer writes nothing because there is nothing to write: in text
// mode Execute prints the error itself. Spelled out rather than passed as nil
// so that a caller in text mode gets silence by intent instead of a panic.
func EmitError(w io.Writer, err error, code int, kind ProblemKind) {
	//nolint:errcheck // the command already failed; a write error changes nothing
	Renderer{Format: FormatJSON, Out: w}.Emit(
		ErrorObject{Error: ansi.StripBackticks(err.Error()), Code: code, Kind: kind},
		func(io.Writer) error { return nil },
	)
}
