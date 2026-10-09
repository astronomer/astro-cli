package cmd

import (
	"errors"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// terraformProviderURL is where Deployments as code live now that the CLI no
// longer reads deployment files.
const terraformProviderURL = "https://registry.terraform.io/providers/astronomer/astro/latest"

// What a run passing a removed 1.x flag is told. Each one starts with the
// flag's own spelling and "was removed in Astro CLI v2", which
// TestRemovedFlagsSayWhatReplacedThem holds them to.
const (
	errJSONFlagRemoved     = "--json was removed in Astro CLI v2: use -o json"
	errTemplateFlagRemoved = "--template was removed in Astro CLI v2: use -o json, and a JSON tool such as jq to pick fields"
	errAPIURLFlagRemoved   = "--api-url was removed in Astro CLI v2: use --url"
	errDeploymentIDAPIFlag = "--deployment-id was removed in Astro CLI v2: use --deployment (-d), which takes a Deployment id or a link name from pyproject.toml"

	errDeploymentFileRemoved = "--deployment-file was removed in Astro CLI v2. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL + ". " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`"
	errInspectTemplateRemoved = "--template was removed in Astro CLI v2. " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL
	// A switch has not logged in again since 2023, so there is no login to
	// link to.
	errLoginLinkRemoved = "--login-link was removed in Astro CLI v2: switching organizations no longer re-authenticates. " +
		"To log in on another device, use `astro login --login-link`"
)

// removedFlag is a flag Astro CLI 1.x had and v2 dropped, and what a run
// passing it is told instead of cobra's bare "unknown flag".
type removedFlag struct {
	name      string
	shorthand string
	// under limits the entry to these commands and everything below them,
	// each a command path without the root's name ("api airflow"). Empty
	// means any command.
	under []string
	// needs reports whether what msg tells the user to use exists on cmd, so
	// that the message is true on every command it reaches, the ones that
	// never had the flag included. nil means msg is true anywhere under.
	needs func(cmd *cobra.Command) bool
	msg   func(cmd *cobra.Command) string
}

// removedFlags is every 1.x flag v2 removed without keeping a hidden alias,
// in one place: the list is defined by 1.x rather than by any one command, so
// it is reviewed against 1.x's tree (testdata/v1_flags.tsv) and deleted in v3
// as one.
//
// It is consulted only when cobra reports a flag the invoked command does not
// have (removedFlagError, from the root's flag error func), so a command that
// still has a flag of the same name (`astro deploy --force`) is untouched, and
// no command carries a hidden flag for it. An entry is keyed by the flag's
// name and shorthand, not by the commands that had it: 1.x's --force became
// --yes on every command that asks first, so the entry says so wherever a
// command has --yes. Where one name had different replacements on different
// commands, the narrower entry (with under) comes first, and the first entry
// that applies wins.
//
// A future removal belongs here too: TestEveryV1FlagStillWorksOrSaysWhatReplacedIt
// fails on a flag in testdata/v1_flags.tsv that the v2 tree neither has nor
// reports here.
var removedFlags = []removedFlag{
	// Deployments as code moved to Terraform.
	{
		name: "deployment-file", under: []string{"deployment create", "deployment update"},
		needs: treeCanClone, msg: says(errDeploymentFileRemoved),
	},
	{
		name: "template", shorthand: "t", under: []string{"deployment inspect"},
		needs: treeCanClone, msg: says(errInspectTemplateRemoved),
	},
	{
		name: "login-link", shorthand: "l", under: []string{"organization switch"},
		msg: says(errLoginLinkRemoved),
	},
	// `astro api airflow` names a Deployment and an Airflow the way every
	// other command does. They were persistent flags in 1.x, so ls and
	// describe took them too, and under covers every subcommand.
	{name: "api-url", under: []string{"api airflow"}, needs: has("url"), msg: says(errAPIURLFlagRemoved)},
	{name: "deployment-id", under: []string{"api airflow"}, needs: has("deployment"), msg: says(errDeploymentIDAPIFlag)},
	// 1.x's list commands shared --json, --template and -o
	// table|json|template (pkg/output's AddFlags), and api ls and describe
	// had --json.
	{name: "json", needs: offers(cliout.FormatJSON), msg: says(errJSONFlagRemoved)},
	{name: "template", needs: offers(cliout.FormatJSON), msg: says(errTemplateFlagRemoved)},
	// The env commands' --format took table, json and yaml (and dotenv on
	// env variable): -o takes json and dotenv where they are offered.
	{name: "format", needs: offers(cliout.FormatJSON), msg: formatRemoved},
	// --force skipped the confirmation that --yes skips now.
	{name: "force", shorthand: "f", needs: has("yes"), msg: forceRemoved},
}

// removedFlagError is what a run is told when it passed a removed 1.x flag
// that cmd does not have, or nil when err is not about one. It runs from the
// root's flag error func (flagError), while cobra parses flags, so it fails
// before any pre-run logs in or asks an API anything, and as a usage error
// (exit 2), which Execute reports under --output json as the error object.
func removedFlagError(cmd *cobra.Command, err error) error {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return nil
	}
	isShorthand := notExist.GetSpecifiedShortnames() != ""
	if f := findRemovedFlag(cmd, notExist.GetSpecifiedName(), isShorthand); f != nil {
		return cliout.Usage(errors.New(f.msg(cmd)))
	}
	return nil
}

// findRemovedFlag returns the entry for the flag typed as name (a shorthand's
// letter when isShorthand) that applies on cmd, or nil when none does.
func findRemovedFlag(cmd *cobra.Command, name string, isShorthand bool) *removedFlag {
	for i := range removedFlags {
		f := &removedFlags[i]
		typed := f.name
		if isShorthand {
			typed = f.shorthand
		}
		if typed == "" || typed != name {
			continue
		}
		// `astro local stop -f`: stop still has --force, without the -f, so
		// "--force was removed" would be false there.
		if cmd.Flag(f.name) != nil {
			continue
		}
		if len(f.under) > 0 && !slices.ContainsFunc(f.under, func(path string) bool { return isUnder(cmd, path) }) {
			continue
		}
		if f.needs != nil && !f.needs(cmd) {
			continue
		}
		return f
	}
	return nil
}

// isUnder reports whether cmd is the command at path below its root, or one
// of that command's subcommands, by cobra's own path for cmd.
func isUnder(cmd *cobra.Command, path string) bool {
	own := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
	return own == path || strings.HasPrefix(own, path+" ")
}

func says(msg string) func(*cobra.Command) string {
	return func(*cobra.Command) string { return msg }
}

func has(flag string) func(*cobra.Command) bool {
	return func(cmd *cobra.Command) bool { return cmd.Flag(flag) != nil }
}

func offers(format cliout.Format) func(*cobra.Command) bool {
	return func(cmd *cobra.Command) bool { return slices.Contains(cliout.Formats(cmd), format) }
}

// treeCanClone reports whether cmd's tree has `deployment create --clone`,
// which the Deployments-as-code messages point to. APC's has none.
func treeCanClone(cmd *cobra.Command) bool {
	create, _, err := cmd.Root().Find([]string{"deployment", "create"})
	return err == nil && create.Name() == "create" && create.Flag("clone") != nil
}

// forceRemoved names --yes with its shorthand when cmd gives it one.
func forceRemoved(cmd *cobra.Command) string {
	yes := "--yes"
	if s := cmd.Flag("yes").Shorthand; s != "" {
		yes += " (-" + s + ")"
	}
	return "--force was removed in Astro CLI v2: use " + yes
}

// formatRemoved names the formats cmd's -o takes, which differ by command
// (dotenv on `env variable get` and `list`), and says what became of yaml,
// which 1.x's --format took everywhere.
func formatRemoved(cmd *cobra.Command) string {
	var names []string
	for _, f := range cliout.Formats(cmd) {
		names = append(names, string(f))
	}
	msg := "--format was removed in Astro CLI v2: use -o (--output), which takes " +
		strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	if !slices.Contains(names, "yaml") {
		msg += "; there is no yaml, so use -o json"
	}
	return msg
}
