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
// the caller must decide with --yes. Both are input_required.
//
// Without a terminal on stdin nobody is there to read the question, so it is
// not written ahead of the answer: a script that pipes one in gets the
// question and its answer as one line on stderr, a record of what it agreed
// to, and a script that pipes nothing gets the refusal alone rather than a
// question left hanging with the error run onto the end of its line.
func (c *cli) confirm(question string) error {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return err
	}
	prompt := question + " [y/N] "
	asked := c.interactive()
	if asked {
		fmt.Fprint(c.d.Stderr, prompt) //nolint:errcheck // a failed write to the terminal still reads the answer
	}
	line, err := input.Reader(c.d.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && line == "" {
		if asked {
			// The question is still open on the terminal's line (Ctrl-D);
			// end it so the error starts on a line of its own.
			fmt.Fprintln(c.d.Stderr)
		}
		if errors.Is(err, io.EOF) {
			return input.Required(errors.New("confirmation needed but stdin is not interactive; pass --yes to proceed"))
		}
		if !asked {
			// The question was never written, so the error carries it.
			return fmt.Errorf("reading the answer to %q: %w", question, err)
		}
		return err
	}
	if !asked {
		fmt.Fprintln(c.d.Stderr, prompt+line)
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return nil
	default:
		return errAborted
	}
}
