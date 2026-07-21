package local

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// confirm asks a yes/no question, defaulting to no. A closed or empty stdin
// (EOF) is an error, not a silent "no": in scripts the caller
// must decide with --yes.
func (c *cli) confirm(question string) error {
	fmt.Fprintf(c.d.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(c.d.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return errors.New("confirmation needed but stdin is not interactive; pass --yes to proceed")
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
