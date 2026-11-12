package input

import (
	"errors"
	"fmt"
	"io"
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
		if !errors.Is(err, io.EOF) {
			// A read that failed is not stdin ending: say what failed.
			return "", fmt.Errorf("reading the answer to %s: %w", Quote(promptText), err)
		}
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
			if !errors.Is(err, io.EOF) {
				return false, fmt.Errorf("reading the answer to %s: %w", Quote(promptText), err)
			}
			return false, endedRequired(promptText, opts)
		}
	}
	return strings.Trim(text, "\r\n") == "y", nil
}

// endedRequired is the refusal for a question whose input ended with no
// answer: nobody answered, so the answer has to come as the flag opts name.
func endedRequired(promptText string, opts []Option) error {
	e := &RequiredError{Prompt: promptText, Reason: EndedReason}
	for _, o := range opts {
		o(e)
	}
	return ApplyHint(e)
}

// Password requests a users passord, does not print out what they entered, and
// returns it. It returns a *RequiredError, and reads nothing, when this run may
// not ask (see SetGuard), and when stdin is not a terminal: a password is
// never read where it cannot be hidden.
func Password(promptText string, opts ...Option) (string, error) {
	if err := MayAsk(promptText, opts...); err != nil {
		return "", err
	}
	if !StdinIsTerminal() {
		// A hidden answer needs a terminal to hide it on: read from anything
		// else, it would be echoed by whatever is typing at the far end (ssh
		// without -t, docker exec -i) or taken from a pipe a script did not
		// mean as a password. So it is refused, naming the flag, not read, and
		// not left to fail with a terminal error.
		e := &RequiredError{Prompt: promptText, Reason: "stdin is not a terminal, so a hidden answer cannot be read"}
		for _, o := range opts {
			o(e)
		}
		return "", ApplyHint(e)
	}
	fmt.Fprint(os.Stderr, promptText)
	password, err := HiddenValue(os.Stdin, os.Stderr)
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		return "", err
	}
	return password, nil
}

// EndedReason is the reason a question gives when its input ended with no
// answer, as opposed to a run that may not ask at all.
const EndedReason = "stdin ended before an answer"
