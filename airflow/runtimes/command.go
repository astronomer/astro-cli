package runtimes

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

func (p *Command) ExecuteWithProgress(progressHandler func(string)) error {
	cmd := exec.Command(p.Command, p.Args...) //nolint:gosec

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error setting up astro project: %w", err)
	}

	// Stream stdout and stderr concurrently
	doneCh := make(chan error, 2)
	go streamOutput(stdoutPipe, progressHandler, doneCh)
	go streamOutput(stderrPipe, progressHandler, doneCh)

	// Wait for both streams to finish
	for i := 0; i < 2; i++ {
		if err := <-doneCh; err != nil {
			return err
		}
	}
	// Wait for the command to complete
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("astro project execution failed: %w", err)
	}

	return nil
}

func streamOutput(pipe io.ReadCloser, handler func(string), doneCh chan<- error) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		handler(line)
	}
	// Notify completion or error
	if err := scanner.Err(); err != nil {
		doneCh <- fmt.Errorf("error reading output: %w", err)
		return
	}
	doneCh <- nil
}
