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
	if hasPipedStdin() {
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

// hasPipedStdin reports whether stdin appears to be a pipe rather than a TTY.
// Used by opt-in secret prompts to distinguish "user piped a value" from
// "user is at an interactive shell with no flag set".
func hasPipedStdin() bool {
	return !term.IsTerminal(int(os.Stdin.Fd()))
}
