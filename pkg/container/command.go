package container

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// command represents an engine subcommand to execute (e.g. `podman machine ls`).
type command struct {
	binary string
	args   []string
}

// execute runs the command and returns its combined output.
func (c *command) execute() (string, error) {
	cmd := exec.Command(c.binary, c.args...) //nolint:gosec // G204: binary is a fixed runtime name, args are static
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// errorFromOutput extracts the meaningful "Error: ..." line from engine output,
// falling back to the whole output when none is found.
func errorFromOutput(prefix, output string) error {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Error: ") {
			msg := strings.TrimSpace(strings.TrimPrefix(line, "Error: "))
			return fmt.Errorf("%s%s", prefix, msg)
		}
	}
	return fmt.Errorf("%s%s", prefix, strings.TrimSpace(output))
}
