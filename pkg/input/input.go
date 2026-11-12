package input

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Every prompt here is written to stderr: stdout carries a command's results
// only, and a question is not one.

// ErrAborted is a confirmation answered no. A command returns it, rather than
// printing that it stopped and exiting 0: declining is not success, and the
// run did not do what it was asked. A declined confirmation fails with it
// (ConfirmOrAbort), so it reads "aborted" and exits 1 wherever it is used,
// and cliout publishes it as kind aborted.
var ErrAborted = errors.New("aborted")

// ConfirmOrAbort asks promptText as a y/n question and returns nil for a yes
// (y or yes, in any case) and ErrAborted for anything else. A run that may
// not ask is refused as Confirm refuses it. Input that ends before any answer
// (a script with stdin closed or from /dev/null) is not a no: nobody
// answered, so it is a *RequiredError naming the flag that answers it, as
// under --output json. It is the confirmation a command that changes
// something asks, so a decline is one failure everywhere.
func ConfirmOrAbort(promptText string, opts ...Option) error {
	return (&Confirmation{
		Question: promptText,
		In:       os.Stdin,
		Out:      os.Stderr,
		Terminal: StdinIsTerminal(),
		Options:  opts,
	}).Ask()
}

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
	// A read that ends is the answer so far, as it always was, unless it ends
	// with nothing: that is nobody answering, refused naming the flag like a
	// question this run could not ask. An interrupt is not an answer either.
	text, err := ReadLine(reader, os.Stderr)
	if errors.Is(err, ErrInterrupted) {
		return "", err
	}
	if err != nil && text == "" {
		fmt.Fprintln(os.Stderr)
		return "", endedRequired(promptText, opts)
	}
	return strings.Trim(text, "\r\n"), nil
}

// Confirm requests a user to confirm their input. It returns a *RequiredError,
// and reads nothing, when this run may not ask (see SetGuard): a refused
// confirmation is neither a yes nor a no, so a caller must not read the bool
// as one. Nor is an interrupt (ErrInterrupted).
func Confirm(promptText string, opts ...Option) (bool, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return false, err
	}
	reader := stdin()
	fmt.Fprintf(os.Stderr, "%s (y/n) ", promptText)

	text, err := ReadLine(reader, os.Stderr)
	if errors.Is(err, ErrInterrupted) {
		return false, err
	}
	if err != nil {
		// Input that ended left the prompt's line open.
		fmt.Fprintln(os.Stderr)
		if text == "" {
			return false, endedRequired(promptText, opts)
		}
	}
	return strings.Trim(text, "\r\n") == "y", nil
}

// endedRequired is the refusal for a question whose input ended with no
// answer: nobody answered, so the answer has to come as the flag opts name.
func endedRequired(promptText string, opts []Option) error {
	e := &RequiredError{Prompt: promptText, Reason: "stdin ended before an answer"}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Password requests a users passord, does not print out what they entered, and
// returns it. It returns a *RequiredError, and reads nothing, when this run may
// not ask (see SetGuard).
func Password(promptText string, opts ...Option) (string, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, promptText)
	password, err := HiddenValue(os.Stdin, os.Stderr)
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		return "", err
	}
	return password, nil
}
