package cliout

import (
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// Warn prints msg to w led by "Warning:", as the CLI's warnings are, so it is
// not read as the command's output. The label is yellow when w is stderr and
// stderr is a terminal; anywhere else (stdout, a pipe, a test's buffer) it is
// plain text, since stderr's palette says nothing about another stream.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, WarningLabel(w), msg)
}

// WarningLabel is the "Warning:" that leads a warning written to w, yellow
// under the rule Warn states.
func WarningLabel(w io.Writer) string {
	if f, ok := w.(*os.File); ok && f == os.Stderr {
		return ansi.ForStderr().Yellow("Warning:")
	}
	return "Warning:"
}
