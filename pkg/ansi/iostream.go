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
