package cliout

import (
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// Warn prints msg to w led by "Warning:", as the CLI's warnings are, so it is
// not read as the command's output. The label is yellow under the rule
// labelFor states.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, WarningLabel(w), msg)
}

// WarningLabel is the "Warning:" that leads a warning written to w, yellow
// under the rule labelFor states.
func WarningLabel(w io.Writer) string {
	return labelFor(w, "Warning:", ansi.Palette.Yellow)
}

// ErrorLabel is the "Error:" that leads the error a command fails with,
// written to w: red under the rule labelFor states, as Warning: is yellow.
func ErrorLabel(w io.Writer) string {
	return labelFor(w, errorPrefix, ansi.Palette.Red)
}

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

// labelFor colors a label only where color belongs: w is the process's
// stderr and stderr's palette colors (ansi.ForStderr: a terminal, the
// CLICOLOR variables, NO_COLOR). Anywhere else (stdout, a pipe, a file, a
// test's buffer) it is plain text, since stderr's palette says nothing about
// another stream.
func labelFor(w io.Writer, text string, color func(ansi.Palette, string) string) string {
	f, ok := w.(*os.File)
	if !ok || f != os.Stderr {
		return text
	}
	return color(ansi.ForStderr(), text)
}
