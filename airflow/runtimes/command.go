package runtimes

import (
	"bytes"
	"os/exec"
)

// Command represents a command to be executed.
type Command struct {
	Command string
	Args    []string
}

// Execute runs the Podman command and returns the output.
func (p *Command) Execute() (string, error) {
	cmd := exec.Command(p.Command, p.Args...) //nolint:gosec
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
