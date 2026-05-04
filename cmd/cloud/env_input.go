package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/env"
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

// createFn matches the per-type CreateVar / CreateAirflowVar signature.
type createFn func(scope env.Scope, key, value string, isSecret bool, autoLink *bool, client astrocore.CoreClient) (*astrocore.EnvironmentObject, error)

// updateFn matches the per-type UpdateVar / UpdateAirflowVar signature.
type updateFn func(idOrKey string, scope env.Scope, value string, autoLink *bool, client astrocore.CoreClient) (*astrocore.EnvironmentObject, error)

// runFromFileCreate parses a dotenv file and calls create for each entry,
// printing per-entry status to out. Stops at the first error.
func runFromFileCreate(out io.Writer, scope env.Scope, autoLink *bool, isSecret bool, path string, create createFn) error {
	parsed, err := env.ParseDotenvFile(path)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		fmt.Fprintf(out, "no variables found in %s\n", displayPath(path))
		return nil
	}
	for _, k := range sortedKeys(parsed) {
		obj, err := create(scope, k, parsed[k], isSecret, autoLink, astroCoreClient)
		if err != nil {
			return fmt.Errorf("create %s: %w", k, err)
		}
		id := ""
		if obj.Id != nil {
			id = *obj.Id
		}
		fmt.Fprintf(out, "Created %s (id: %s)\n", obj.ObjectKey, id)
	}
	return nil
}

// runFromFileUpdate parses a dotenv file and upserts each entry. Honors the
// same `--strict` semantic as the single-key update path: when strict is true
// and a key does not exist, this aborts rather than creating.
func runFromFileUpdate(out io.Writer, scope env.Scope, autoLink *bool, isSecret, strict bool, path string, create createFn, update updateFn) error {
	parsed, err := env.ParseDotenvFile(path)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		fmt.Fprintf(out, "no variables found in %s\n", displayPath(path))
		return nil
	}
	for _, k := range sortedKeys(parsed) {
		obj, err := update(k, scope, parsed[k], autoLink, astroCoreClient)
		if err != nil {
			if errors.Is(err, env.ErrNotFound) && !strict {
				obj, err = create(scope, k, parsed[k], isSecret, autoLink, astroCoreClient)
				if err != nil {
					return fmt.Errorf("create %s: %w", k, err)
				}
				fmt.Fprintf(out, "Created %s\n", obj.ObjectKey)
				continue
			}
			return fmt.Errorf("update %s: %w", k, err)
		}
		fmt.Fprintf(out, "Updated %s\n", obj.ObjectKey)
	}
	return nil
}

// displayPath renders "-" as "<stdin>" for user-facing messages; otherwise
// returns the path unchanged.
func displayPath(path string) string {
	if path == "-" {
		return "<stdin>"
	}
	return path
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
