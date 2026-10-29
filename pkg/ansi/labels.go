package ansi

import (
	"fmt"
	"io"
	"os"
)

// The labels that lead the CLI's warnings, errors and notes. They live here,
// below both cmd/ and internal/, so every part of the CLI that prints one
// spells and colors it the same way; cmd/cliout's helpers call these.

// Warn prints msg to w led by "Warning:", as the CLI's warnings are, so it is
// not read as the command's output. The label is yellow under the rule
// labelFor states.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, WarningLabel(w), msg)
}

// Note prints msg to w led by "Note:", for something worth knowing that is
// neither a warning nor the command's output. The label is dim under the rule
// labelFor states, so warnings and errors stand out beside it.
func Note(w io.Writer, msg string) {
	fmt.Fprintln(w, NoteLabel(w), msg)
}

// WarningLabel is the "Warning:" that leads a warning written to w, yellow
// under the rule labelFor states.
func WarningLabel(w io.Writer) string {
	return labelFor(w, "Warning:", Palette.Yellow)
}

// ErrorLabel is the "Error:" that leads the error a command fails with,
// written to w: red under the rule labelFor states, as Warning: is yellow.
func ErrorLabel(w io.Writer) string {
	return labelFor(w, "Error:", Palette.Red)
}

// NoteLabel is the "Note:" that leads a note written to w, dim under the
// rule labelFor states.
func NoteLabel(w io.Writer) string {
	return labelFor(w, "Note:", Palette.Faint)
}

// DangerLabel is the "WARNING:" that leads a confirmation of something that
// cannot be undone, written to w: red under the rule labelFor states, since
// it marks a danger rather than a warning the command goes ahead despite.
func DangerLabel(w io.Writer) string {
	return labelFor(w, "WARNING:", Palette.Red)
}

// labelFor colors a label only where color belongs: w is the process's
// stderr, stderr is a terminal (or color is forced, this package's rule), and
// NO_COLOR is unset or empty (no-color.org). Anywhere else (stdout, a pipe, a
// file, a test's buffer) it is plain text, since stderr's palette says nothing
// about another stream.
func labelFor(w io.Writer, text string, color func(Palette, string) string) string {
	f, ok := w.(*os.File)
	if !ok || f != os.Stderr || os.Getenv("NO_COLOR") != "" {
		return text
	}
	return color(ForStderr(), text)
}
