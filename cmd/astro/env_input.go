package astro

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
)

// readSecretValue resolves a secret value from one of three sources, in order:
//
//  1. The flag value (if explicitly provided).
//  2. Stdin, if it's piped (single line, trailing newline stripped).
//  3. An interactive prompt with echo disabled, if stdin is a TTY.
//
// Passing the value via flag is supported but discouraged for secret values,
// since it puts the value in shell history.
//
// opts describe the prompt for a run that may not ask (--output json): it is
// refused, naming the flag in opts, rather than read.
func readSecretValue(flagValue, prompt string, opts ...input.Option) (string, error) {
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
	return input.Password(prompt+": ", opts...)
}

// readSetValue resolves the value for a `set`, refusing to invent one.
//
// readSecretValue treats "no flag and nothing on stdin" as the empty string,
// which is right for an optional secret and wrong for the thing a `set` is
// setting: hasPipedStdin() is true whenever stdin is not a terminal, so in CI
// `astro env variable set API_TOKEN` with the --value expansion gone empty
// read zero bytes and wrote an explicit empty value over the stored token,
// printing "Updated API_TOKEN" and exiting 0. Now that `set` upserts, the same
// invocation against a mistyped key created an empty variable instead.
//
// An explicit --value is authoritative including when empty, since setting a
// variable to the empty string is legitimate; it is only the absence of any
// input that is now an error rather than a silent blanking.
func readSetValue(cmd *cobra.Command, flagName, flagValue, prompt string) (string, error) {
	if cmd.Flags().Changed(flagName) {
		return flagValue, nil
	}
	v, err := readSecretValue(flagValue, prompt, input.AnsweredBy("--"+flagName+", or pipe the value on stdin"))
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("no value supplied: pass --%s, or pipe one on stdin", flagName)
	}
	return v, nil
}

// errAbortedDelete is a delete that was not confirmed: answered no, or asked
// where nobody could answer.
var errAbortedDelete = errors.New("aborted: pass --yes (or confirm interactively) to delete")

// confirmTTY returns true if the user confirms y/Y at an interactive prompt.
// On a non-TTY it reads nothing and returns errAbortedDelete, marked as a
// question this run could not ask; callers must require an explicit --yes
// flag for non-interactive use. A run that may not ask at all (--output json)
// returns that refusal, naming --yes, rather than an answer.
func confirmTTY(prompt string) (bool, error) {
	yes := input.AnsweredBy("--yes")
	if err := input.MayAsk(prompt, yes); err != nil {
		return false, err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, input.Required(errAbortedDelete)
	}
	return input.Confirm(prompt, yes)
}

// hasPipedStdin reports whether stdin appears to be a pipe rather than a TTY.
// Used by opt-in secret prompts to distinguish "user piped a value" from
// "user is at an interactive shell with no flag set".
func hasPipedStdin() bool {
	return !term.IsTerminal(int(os.Stdin.Fd()))
}

// createFn matches the per-type CreateVar / CreateAirflowVar signature.
type createFn func(scope env.Scope, key, value string, isSecret bool, autoLink *bool, client astrov1.APIClient) (*astrov1.EnvironmentObject, error)

// updateFn matches the per-type UpdateVar / UpdateAirflowVar signature.
type updateFn func(idOrKey string, scope env.Scope, value string, autoLink *bool, client astrov1.APIClient) (*astrov1.EnvironmentObject, error)

// refuseCreateByID rejects creating an object addressed by an id.
//
// Mapping the id branch's 404 to ErrNotFound was needed so --no-create and
// the upsert could see a miss at all, but it also handed the create arm a
// CUID to use as the new object's key: `set <stale-cuid>` would have made an
// ENVIRONMENT_VARIABLE literally named cl9abc… Creating requires a key the
// caller chose; an id names something that was supposed to exist already.
func refuseCreateByID(noun, idOrKey string) error {
	if !util.IsCUID(idOrKey) {
		return nil
	}
	return fmt.Errorf("%s %q does not exist, and an ID cannot be created: "+
		"pass the key you want the new %s to have", noun, idOrKey, noun)
}

// setNotFound explains a lookup miss that --no-create turned into a failure.
//
// Without it the single-key paths returned a bare "environment object not
// found", indistinguishable from any other miss and silent about the flag
// that made it fatal — while the bulk path already said so.
func setNotFound(noun, idOrKey string, err error) error {
	return fmt.Errorf("%s %q does not exist and --no-create was passed: %w", noun, idOrKey, err)
}

// fromFileFns are one kind's calls, for `set --from-file`.
type fromFileFns struct {
	create createFn
	update updateFn
	get    getFn
}

// runFromFileSet parses a dotenv file and sets each entry. Honors the same
// --no-create semantic as the single-key path: when noCreate is true and a key
// does not exist, this aborts rather than creating.
//
// It renders once, at the end, what it did with every key. A failure part way
// through stops it there. Text prints a line for each key set before it and
// then the error, as it always has. Json publishes the outcomes so far,
// ending in the failed key and its error, and exits 1 with the error on
// stderr (failedAfterResult): the keys before it were set, and a script
// needs to know which.
func runFromFileSet(cmd *cobra.Command, r cliout.Renderer, scope env.Scope, autoLink *bool, isSecret, noCreate bool, path string, fns fromFileFns) error {
	parsed, err := env.ParseDotenvFile(path)
	if err != nil {
		return err
	}
	res := &env.SetFromFileResult{Outcomes: []env.SetOutcome{}}
	var skipped []string
	for _, k := range sortedKeys(parsed) {
		// An empty value in a dotenv file is very often not a value at all.
		// `astro env variable export` writes `KEY=  # secret, use
		// --include-secrets` for every secret it will not reveal, which parses
		// back as "" — so the round trip this command advertises would set each
		// of those to the empty string and report success. Skipping is the only
		// non-destructive reading: nothing here can tell the placeholder apart
		// from a deliberate empty, and one of the two silently destroys
		// credentials.
		if parsed[k] == "" {
			skipped = append(skipped, k)
			res.Outcomes = append(res.Outcomes, env.SetOutcome{Key: k, Kind: env.SetSkippedEmpty})
			continue
		}
		kind, obj, err := setFromFileEntry(cmd.ErrOrStderr(), r, scope, k, parsed[k], autoLink, isSecret, noCreate, fns)
		if err != nil {
			if r.Format != cliout.FormatJSON {
				writeSetLines(r.Out, res)
				return err
			}
			res.Outcomes = append(res.Outcomes, env.SetOutcome{Key: k, Kind: env.SetFailed, Error: err.Error()})
			if eerr := renderEnvSetFromFile(r, res, path); eerr != nil {
				return eerr
			}
			return failedAfterResult(cmd, r.Format, err)
		}
		info := setInfo(obj)
		res.Outcomes = append(res.Outcomes, env.SetOutcome{Key: k, Kind: kind, Object: &info})
	}
	if err := renderEnvSetFromFile(r, res, path); err != nil {
		return err
	}
	if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr,
			"skipped %d entr%s with an empty value: %s\n"+
				"  An export without --include-secrets writes secrets as empty placeholders;\n"+
				"  importing those would overwrite the stored values. Set them individually,\n"+
				"  or re-export with --include-secrets.\n",
			len(skipped), plural(len(skipped)), strings.Join(skipped, ", "))
	}
	return nil
}

// setFromFileEntry sets one key of the file: an update, or a create when the
// key is absent and creating is allowed. It returns which it was and the
// object it left.
func setFromFileEntry(warn io.Writer, r cliout.Renderer, scope env.Scope, k, value string, autoLink *bool, isSecret, noCreate bool, fns fromFileFns) (env.SetOutcomeKind, *astrov1.EnvironmentObject, error) {
	obj, err := fns.update(k, scope, value, autoLink, astroV1Client)
	switch {
	case err == nil:
		return env.SetUpdated, obj, nil
	case !errors.Is(err, env.ErrNotFound):
		return "", nil, fmt.Errorf("set %s: %w", k, err)
	case noCreate:
		return "", nil, fmt.Errorf("set %s: it does not exist and --no-create was passed: %w", k, err)
	}
	obj, err = fns.create(scope, k, value, isSecret, autoLink, astroV1Client)
	if err != nil {
		return "", nil, fmt.Errorf("set %s: creating it failed: %w", k, err)
	}
	return env.SetCreated, createdAsHeld(warn, r, obj, scope, fns.get), nil
}

// runEnvDelete deletes the noun idOrKey names, once confirmed: --yes, or a
// yes at a terminal. Under --output json it asks nothing, failing as
// input_required naming --yes.
func runEnvDelete(cmd *cobra.Command, out io.Writer, noun, idOrKey string, del func(string, env.Scope, astrov1.APIClient) (*astrov1.EnvironmentObject, error)) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	r := cliout.Renderer{Format: envOutput, Out: out}
	cmd.SilenceUsage = true

	if !envYes {
		ok, err := confirmTTY(fmt.Sprintf("Delete %s %q?", noun, idOrKey))
		if err != nil {
			return err
		}
		if !ok {
			return errAbortedDelete
		}
	}
	deleted, err := del(idOrKey, scope, astroV1Client)
	if err != nil {
		return err
	}
	return renderEnvDeleted(r, deleted, idOrKey)
}

// plural is the suffix for "entry"/"entries" in the skip notice.
func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
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
