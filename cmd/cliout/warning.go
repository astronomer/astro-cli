package cliout

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// The CLI's warning, error, note and next-step labels. They are pkg/ansi's,
// which internal/ can import too, so the whole CLI spells and colors them one
// way; these are the same functions under the names cmd/ calls them by, so
// the two cannot drift. Each colors only on a terminal stdout or stderr, and
// is plain under NO_COLOR or anywhere else (ansi's labelFor rule).
var (
	// Warn prints msg to w led by a yellow "Warning:".
	Warn = ansi.Warn
	// Note prints msg to w led by a cyan "Note:".
	Note = ansi.Note
	// WarningLabel is the yellow "Warning:" that leads a warning.
	WarningLabel = ansi.WarningLabel
	// ErrorLabel is the red "Error:" that leads the error a command fails with.
	ErrorLabel = ansi.ErrorLabel
	// NoteLabel is the cyan "Note:" that leads a note.
	NoteLabel = ansi.NoteLabel
	// NextLabel is the cyan "Next:" that leads the command a run suggests next.
	NextLabel = ansi.NextLabel
	// DangerLabel is the red "WARNING:" that leads a confirmation of something
	// that cannot be undone.
	DangerLabel = ansi.DangerLabel
)

// errorPrefix is cobra's own error prefix, which a command that sets none
// reports (cobra.Command.ErrPrefix): ansi's, so it is spelled once.
const errorPrefix = ansi.ErrorPrefix

// errorPrefixFor is the prefix cmd's error is printed after: its own
// ErrPrefix as set, or, for cobra's default, the red Error: label.
func errorPrefixFor(prefix string, w io.Writer) string {
	if prefix != errorPrefix {
		return prefix
	}
	return ErrorLabel(w)
}
