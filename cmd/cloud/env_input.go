package cloud

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/astronomer/astro-cli/pkg/input"
)

// readSecretValue resolves a secret value from one of three sources, in order:
//
//  1. The flag value (if explicitly provided).
//  2. Stdin, if it's piped (single line, trailing newline stripped).
//  3. An interactive prompt with echo disabled, if stdin is a TTY.
//
// Passing the value via flag is supported but discouraged for secret values,
// since it puts the value in shell history.
func readSecretValue(flagValue, prompt string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return input.Password(prompt + ": ")
}

// confirmTTY returns true if the user confirms y/Y at an interactive prompt.
// On a non-TTY it returns false without reading anything; callers must require
// an explicit --yes flag for non-interactive use.
func confirmTTY(prompt string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	ok, _ := input.Confirm(prompt)
	return ok
}
