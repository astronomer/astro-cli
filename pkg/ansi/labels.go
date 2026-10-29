package ansi

import (
	"fmt"
	"io"
	"os"

	"github.com/logrusorgru/aurora"
)

// The labels that lead the CLI's warnings, errors, notes and next steps. They
// live here, below both cmd/ and internal/, so every part of the CLI that
// prints one spells and colors it the same way; cmd/cliout's helpers are
// these same functions under the names cmd/ uses.
//
// Every label follows one rule (labelFor): it is colored when it is written
// to the process's stdout (Output) or stderr (Messages) and that stream colors,
// by that stream's own palette, and NO_COLOR is unset or empty. Anywhere else
// (a buffer, a file, another writer) it is plain text.

// ErrorPrefix is the label that leads a command's error: cobra's own error
// prefix, which a command that sets none reports.
const ErrorPrefix = "Error:"

// Warn prints msg to w led by "Warning:", as the CLI's warnings are, so it is
// not read as the command's output.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, WarningLabel(w), msg)
}

// Note prints msg to w led by "Note:", for something worth knowing that is
// neither a warning nor the command's output.
func Note(w io.Writer, msg string) {
	fmt.Fprintln(w, NoteLabel(w), msg)
}

// WarningLabel is the yellow "Warning:" that leads a warning written to w.
func WarningLabel(w io.Writer) string {
	return labelFor(w, "Warning:", Palette.Yellow)
}

// ErrorLabel is the red "Error:" that leads the error a command fails with.
func ErrorLabel(w io.Writer) string {
	return labelFor(w, ErrorPrefix, Palette.Red)
}

// NoteLabel is the cyan "Note:" that leads a note written to w.
func NoteLabel(w io.Writer) string {
	return labelFor(w, "Note:", Palette.Cyan)
}

// DangerLabel is the red "WARNING:" that leads a confirmation of something
// that cannot be undone, since it marks a danger rather than a warning the
// command goes ahead despite.
func DangerLabel(w io.Writer) string {
	return labelFor(w, "WARNING:", Palette.Red)
}

// NextLabel is the cyan "Next:" that leads the command a run suggests next.
func NextLabel(w io.Writer) string {
	return labelFor(w, "Next:", Palette.Cyan)
}

// labelFor colors text by the palette of the stream w is, and leaves it plain
// on any other writer or under NO_COLOR (no-color.org).
func labelFor(w io.Writer, text string, color func(Palette, string) string) string {
	p, ok := paletteFor(w)
	if !ok || os.Getenv("NO_COLOR") != "" {
		return text
	}
	return color(p, text)
}

// paletteFor is the palette of the process stream w is: stderr's (ForStderr)
// for Messages, stdout's (ForStdout) for Output. ok is false for any other
// writer, whose terminal neither palette knows.
func paletteFor(w io.Writer) (p Palette, ok bool) {
	switch w {
	case io.Writer(Messages):
		return ForStderr(), true
	case io.Writer(Output):
		return ForStdout(), true
	}
	return Palette{}, false
}

// ForStdout colors text written to stdout (Output), by whether stdout is a
// terminal, as ForStderr does for stderr.
func ForStdout() Palette {
	return Palette{aurora.NewAurora(shouldColor(IsOutputTerminal))}
}
