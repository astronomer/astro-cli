package input

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Every prompt here is written to stderr: stdout carries a command's results
// only, and a question is not one.

// Text requests a user for input text and returns it. It returns a
// *RequiredError, and reads nothing, when this run may not ask (see SetGuard).
func Text(promptText string, opts ...Option) (string, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return "", err
	}
	reader := stdin()
	if promptText != "" {
		fmt.Fprint(os.Stderr, promptText)
	}
	text, _ := reader.ReadString('\n') //nolint:errcheck // error deliberately ignored in this shell code
	return strings.Trim(text, "\r\n"), nil
}

// Confirm requests a user to confirm their input. It returns a *RequiredError,
// and reads nothing, when this run may not ask (see SetGuard): a refused
// confirmation is neither a yes nor a no, so a caller must not read the bool
// as one.
func Confirm(promptText string, opts ...Option) (bool, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return false, err
	}
	reader := stdin()
	fmt.Fprintf(os.Stderr, "%s (y/n) ", promptText)

	text, _ := reader.ReadString('\n') //nolint:errcheck // error deliberately ignored in this shell code
	return strings.Trim(text, "\r\n") == "y", nil
}

// Password requests a users passord, does not print out what they entered, and
// returns it. It returns a *RequiredError, and reads nothing, when this run may
// not ask (see SetGuard).
func Password(promptText string, opts ...Option) (string, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, promptText)
	bytePassword, err := term.ReadPassword(stdinFD())
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "\n")
	return string(bytePassword), nil
}
