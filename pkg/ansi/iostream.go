package ansi

import (
	"os"

	"github.com/mattn/go-isatty"
)

var (
	Input    = os.Stdin
	Output   = os.Stdout
	Messages = os.Stderr
)

func IsOutputTerminal() bool {
	return isatty.IsTerminal(Output.Fd())
}

// isMessagesTerminal reports whether Messages, stderr, is a terminal. A
// variable so a test can say so without one.
var isMessagesTerminal = func() bool {
	return isatty.IsTerminal(Messages.Fd())
}
