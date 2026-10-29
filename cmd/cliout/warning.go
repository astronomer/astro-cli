package cliout

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// The CLI's warning, error and note labels. They are defined in pkg/ansi,
// which internal/ can import too, so the whole CLI spells and colors them one
// way; these are the names cmd/ calls them by. Each label is colored only on a
// terminal's stderr and is plain under NO_COLOR or anywhere else.

// Warn prints msg to w led by a yellow "Warning:" (ansi.Warn).
func Warn(w io.Writer, msg string) { ansi.Warn(w, msg) }

// Note prints msg to w led by a cyan "Note:" (ansi.Note).
func Note(w io.Writer, msg string) { ansi.Note(w, msg) }

// ErrorLabel is the red "Error:" that leads the error a command fails with.
func ErrorLabel(w io.Writer) string { return ansi.ErrorLabel(w) }

// NoteLabel is the cyan "Note:" that leads a note written to w.
func NoteLabel(w io.Writer) string { return ansi.NoteLabel(w) }

// NextLabel is the cyan "Next:" that leads the command a run suggests next.
// It colors on a terminal stdout as well as stderr (ansi.NextLabel).
func NextLabel(w io.Writer) string { return ansi.NextLabel(w) }

// DangerLabel is the red "WARNING:" that leads a confirmation of something
// that cannot be undone.
func DangerLabel(w io.Writer) string { return ansi.DangerLabel(w) }

// errorPrefix is cobra's own error prefix, which a command that sets none
// reports (cobra.Command.ErrPrefix).
const errorPrefix = "Error:"

// errorPrefixFor is the prefix cmd's error is printed after: its own
// ErrPrefix as set, or, for cobra's default, the red Error: label.
func errorPrefixFor(prefix string, w io.Writer) string {
	if prefix != errorPrefix {
		return prefix
	}
	return ErrorLabel(w)
}
