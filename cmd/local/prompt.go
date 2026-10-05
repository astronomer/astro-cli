package local

import (
	"bufio"
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
func (c *cli) confirm(question string) error {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return err
	}
	fmt.Fprintf(c.d.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(c.d.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return input.Required(errors.New("confirmation needed but stdin is not interactive; pass --yes to proceed"))
		}
		return err
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return nil
	default:
		return errAborted
	}
}
