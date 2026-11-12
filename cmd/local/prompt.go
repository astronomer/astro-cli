package local

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
)

// confirmUnless asks before something destructive, unless the caller already
// said yes. It is the whole guard: safe by default, one flag to skip, and a
// run that cannot be asked and did not pass the flag fails rather than
// proceeding.
func (c *cli) confirmUnless(yes bool, question string) error {
	if yes {
		return nil
	}
	return c.confirm(question)
}

// confirm asks a yes/no question, defaulting to no. A run that may not ask
// (--output json) is refused before anything is written or read, and a closed
// or empty stdin (EOF) is an error, not a silent "no": in scripts
// the caller must decide with --yes. Both are input_required. Anything but
// a yes is input.ErrAborted, and Ctrl-C ends the question at once with
// input.ErrInterrupted.
//
// The question is always written before the answer is read, whatever stdin
// is: a stdin that is not a terminal still often has a person behind it (ssh
// or exec without a tty, mintty), or a tool that waits for the question
// before it answers. So every way the read ends leaves its line whole: Ctrl-C
// and input that ended close it before the error, and an answer from a pipe,
// which nothing echoes, is written after the question as plain text, so a log
// reads "…? [y/N] n" on one line.
func (c *cli) confirm(question string) error {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return err
	}
	tty := c.interactive()
	fmt.Fprint(c.d.Stderr, question+" [y/N] ") //nolint:errcheck // a failed write to the terminal still reads the answer
	line, err := input.ReadLine(input.Reader(c.d.Stdin), c.d.Stderr)
	if errors.Is(err, input.ErrInterrupted) {
		return err // ReadLine ended the line
	}
	// The answer is decided on the text a log shows, so the line written
	// below never reads as a yes for a run that went on to abort, and input
	// that was nothing but escapes is no answer, not a no.
	answer := input.Plain(line)
	answered := err == nil || (errors.Is(err, io.EOF) && answer != "") // an empty line is an answer: a no; a read that failed is none
	switch {
	case answered && !tty:
		// A terminal echoed what was typed; a pipe did not.
		fmt.Fprintln(c.d.Stderr, answer)
	case err != nil:
		// Input that ended left the line open: end it, so what follows
		// starts on a fresh one.
		fmt.Fprintln(c.d.Stderr)
	}
	if !answered {
		switch {
		case errors.Is(err, io.EOF) && tty:
			return input.Required(errors.New("confirmation needed but input ended with no answer; pass --yes to proceed"))
		case errors.Is(err, io.EOF):
			return input.Required(errors.New("confirmation needed but stdin is not interactive; pass --yes to proceed"))
		case !tty:
			return fmt.Errorf("reading the answer to %s: %w", input.Quote(question), err)
		}
		return err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return nil
	default:
		return input.ErrAborted
	}
}
