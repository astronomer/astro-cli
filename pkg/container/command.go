package container

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

// executeWithProgress streams stdout+stderr line-by-line to progress while the
// command runs (used for `podman machine init`, which is slow and chatty).
func (c *command) executeWithProgress(progress func(string)) error {
	cmd := exec.Command(c.binary, c.args...) //nolint:gosec // G204: binary is a fixed runtime name, args are static

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	doneCh := make(chan error, 2)
	go streamOutput(stdoutPipe, progress, doneCh)
	go streamOutput(stderrPipe, progress, doneCh)

	for i := 0; i < 2; i++ {
		if err := <-doneCh; err != nil {
			return err
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}
	return nil
}

func streamOutput(pipe io.ReadCloser, handler func(string), doneCh chan<- error) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		handler(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		doneCh <- fmt.Errorf("error reading output: %w", err)
		return
	}
	doneCh <- nil
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
