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
// Without a terminal on stdin nobody is there to read the question, so it is
// not written ahead of the answer: a piped answer is recorded with its
// question as one line on stderr, a record of what the script agreed to, and
// with nothing piped the refusal is all stderr gets. Only a question that was
// written has a line to end, so only then is one ended, on Ctrl-C (ReadLine)
// or on input that ended.
func (c *cli) confirm(question string) error {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return err
	}
	prompt := question + " [y/N] "
	asked := c.interactive()
	open := io.Discard // the line ReadLine ends on Ctrl-C, if one is open
	if asked {
		fmt.Fprint(c.d.Stderr, prompt) //nolint:errcheck // a failed write to the terminal still reads the answer
		open = c.d.Stderr
	}
	line, err := input.ReadLine(input.Reader(c.d.Stdin), open)
	if errors.Is(err, input.ErrInterrupted) {
		return err
	}
	line = strings.TrimSpace(line)
	answered := err == nil || line != "" // an empty line is an answer: a no
	switch {
	case asked && err != nil:
		// Input that ended left the prompt's line open: end it, so what
		// follows starts on a fresh one.
		fmt.Fprintln(c.d.Stderr)
	case !asked && answered:
		fmt.Fprintln(c.d.Stderr, strings.TrimRight(prompt+line, " "))
	}
	if !answered {
		switch {
		case asked && errors.Is(err, io.EOF):
			return input.Required(errors.New("confirmation needed but input ended with no answer; pass --yes to proceed"))
		case errors.Is(err, io.EOF):
			// The question was never written, so the refusal names it.
			return input.Required(fmt.Errorf("confirmation needed for %q but stdin is not interactive; pass --yes to proceed", question))
		case !asked:
			return fmt.Errorf("reading the answer to %q: %w", question, err)
		}
		return err
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return nil
	default:
		return input.ErrAborted
	}
}
