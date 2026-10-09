package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
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

// removedFlag is what a run passing a flag Astro CLI 1.x had and v2 dropped
// is told instead of cobra's bare "unknown flag".
type removedFlag struct {
	name string
	// under limits the entry to these commands and everything below them,
	// each a command path without the root's name ("api airflow"), for a
	// name whose replacement differs by command. Empty means any command.
	under []string
	// needs reports whether what the message tells the user to use exists on
	// cmd. nil means it always does.
	needs func(cmd *cobra.Command) bool
	// msg is the message; msgFor, when set, builds it instead, for one that
	// depends on cmd or on how the flag was typed ("-f" or "--force").
	msg    string
	msgFor func(cmd *cobra.Command, typed string) string
}

func (f *removedFlag) message(cmd *cobra.Command, typed string) string {
	if f.msgFor != nil {
		return f.msgFor(cmd, typed)
	}
	return f.msg
}

// removedFlags is every 1.x flag v2 removed without keeping a hidden alias,
// in one place: the list is defined by 1.x rather than by any one command, so
// it is reviewed against 1.x's tree (v1_flags.tsv) and deleted in v3 as one.
//
// It is consulted only when cobra reports a flag the invoked command does not
// have (removedFlagError, from the root's flag error func), and only for a
// flag v1_flags.tsv says 1.x had on that command, its own or inherited. So a
// command that still has a flag of the same name (`astro deploy --force`) is
// untouched, a command 1.x did not have, or had without the flag, gets
// cobra's error, and no command carries a hidden flag for it (except while
// a shell asks for completions: acceptRemovedFlags). An entry is keyed by the
// flag's name, and a shorthand 1.x gave it reaches it through v1_flags.tsv:
// 1.x's --force (-f) became --yes on every command that asks first, so the
// one entry says so on each. Where one name had different replacements on
// different commands, the narrower entry (with under) comes first, and the
// first entry that applies wins.
//
// A future removal belongs here too: TestEveryV1FlagStillWorksOrSaysWhatReplacedIt
// fails on a flag in v1_flags.tsv that the v2 tree neither has nor reports
// here, and on an entry no flag in it reaches.
var removedFlags = []removedFlag{
	// Deployments as code moved to Terraform.
	{name: "deployment-file", msg: errDeploymentFileRemoved},
	{name: "template", under: []string{"deployment inspect"}, msg: errInspectTemplateRemoved},
	{name: "login-link", msg: errLoginLinkRemoved},
	// `astro api airflow` names a Deployment and an Airflow the way every
	// other command does. They were persistent flags in 1.x, so ls and
	// describe took them too.
	{name: "api-url", msg: errAPIURLFlagRemoved},
	{name: "deployment-id", under: []string{"api airflow"}, needs: has("deployment"), msg: errDeploymentIDAPIFlag},
	// 1.x's list commands shared --json, --template and -o
	// table|json|template (pkg/output's AddFlags), and api ls and describe
	// had --json.
	{name: "json", needs: cliout.HasOutput, msg: errJSONFlagRemoved},
	{name: "template", needs: cliout.HasOutput, msg: errTemplateFlagRemoved},
	// The env commands' --format took table, json and yaml (and dotenv on
	// env variable): -o takes json and dotenv where they are offered.
	{name: "format", needs: cliout.HasOutput, msgFor: formatRemoved},
	// --force skipped the confirmation that --yes skips now.
	{name: "force", needs: has("yes"), msgFor: forceRemoved},
}

// removedFlagErr is a run refused for passing a removed 1.x flag.
type removedFlagErr struct{ msg string }

func (e *removedFlagErr) Error() string { return e.msg }

// removedFlagError is what a run is told when it passed a removed 1.x flag
// that cmd does not have, or nil when err is not about one. It runs from the
// root's flag error func (flagError), while cobra parses flags, so it fails
// before any pre-run logs in or asks an API anything, and as a usage error
// (exit 2), which Execute reports under --output json as the error object.
func removedFlagError(cmd *cobra.Command, err error) error {
	typed, isShorthand, spelling := unknownFlag(err)
	if spelling == "" {
		return nil
	}
	v1 := v1FlagOn(cmd, typed, isShorthand)
	if v1 == nil {
		return nil
	}
	f := findRemovedFlag(cmd, v1.name)
	if f == nil {
		return nil
	}
	return cliout.Usage(&removedFlagErr{f.message(cmd, spelling)})
}

// findRemovedFlag returns the entry for the flag called name that applies on
// cmd, or nil when none does.
func findRemovedFlag(cmd *cobra.Command, name string) *removedFlag {
	// `astro local stop -f`: stop still has --force, without the -f, so
	// "--force was removed" would be false there.
	if cmd.Flag(name) != nil {
		return nil
	}
	for i := range removedFlags {
		f := &removedFlags[i]
		if f.name != name {
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
	own := pathBelowRoot(cmd)
	return own == path || strings.HasPrefix(own, path+" ")
}

// pathBelowRoot is cmd's path without the root's name: "deployment list".
func pathBelowRoot(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}

func has(flag string) func(*cobra.Command) bool {
	return func(cmd *cobra.Command) bool { return cmd.Flag(flag) != nil }
}

// forceRemoved names --yes with its shorthand when cmd gives it one, and
// says so differently when the run passed it already (`-yf`).
func forceRemoved(cmd *cobra.Command, typed string) string {
	yes := cmd.Flag("yes")
	spelled := "--yes"
	if yes.Shorthand != "" {
		spelled += " (-" + yes.Shorthand + ")"
	}
	// Changed alone would include --yes=false, which skips nothing.
	if yes.Changed && yes.Value.String() == "true" {
		return "--force was removed in Astro CLI v2: " + spelled + ", which you passed, already skips the confirmation, so drop " + typed
	}
	return "--force was removed in Astro CLI v2: use " + spelled
}

// formatRemoved names the formats cmd's -o takes, which differ by command
// (dotenv on `env variable get` and `list`), and says what became of yaml,
// which 1.x's --format took everywhere.
func formatRemoved(cmd *cobra.Command, _ string) string {
	msg := "--format was removed in Astro CLI v2: use -o (--output), which takes " + cliout.FormatList(cmd)
	if !cliout.Offers(cmd, "yaml") {
		msg += "; there is no yaml, so use -o json"
	}
	return msg
}

// The trees v1_flags.tsv records, and the annotation on a root naming the one
// it is the v2 side of (markV1Tree).
const (
	v1TreeAstro       = "astro"
	v1TreeAstroHosted = "astro-hosted"
	v1TreeAPC         = "apc"
	v1TreeAnnotation  = "v1-tree"
)

// markV1Tree records on root which platform's 1.x tree a script on this
// machine was written against, APC's or Astro's. Whether Astro's is the one
// for a hosted organization is left to v1TreeOf, so that building a root reads
// no context: only a run that passes an unknown flag, or asks for
// completions, needs to know.
func markV1Tree(root *cobra.Command, platform string) {
	tree := v1TreeAstro
	if platform == apcPlatform {
		tree = v1TreeAPC
	}
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[v1TreeAnnotation] = tree
}

// v1TreeOf is the 1.x tree a script was written against on the machine root
// was built on: APC's, or Astro's for a hosted organization or not, decided
// as cmd/astro decides the hosted-only flags it registers. A run reads it at
// most a few times, when a flag fails to parse or a shell asks for
// completions, and the context does not change in between.
func v1TreeOf(root *cobra.Command) string {
	tree := root.Annotations[v1TreeAnnotation]
	if tree == v1TreeAstro && organization.IsOrgHosted() {
		return v1TreeAstroHosted
	}
	return tree
}

// v1FlagsFile is every flag 1.x had, on every command, in each tree.
//
//go:embed v1_flags.tsv
var v1FlagsFile string

// v1Flag is a line of v1_flags.tsv.
type v1Flag struct {
	trees                 []string
	path, name, shorthand string
	isBool                bool
}

// v1Flags is v1_flags.tsv, read the first time a run needs it: a run that
// passes no unknown flag never does. A file that does not parse is
// TestV1FlagsFileIsWellFormed's to catch; a run given one reports every flag
// it does not know as cobra does, rather than failing over a tombstone.
var v1Flags = sync.OnceValue(func() []v1Flag { return loadV1Flags(v1FlagsFile) })

// loadV1Flags is the flags in file, or none when it does not parse.
func loadV1Flags(file string) []v1Flag {
	flags, err := parseV1Flags(file)
	if err != nil {
		return nil
	}
	return flags
}

// parseV1Flags reads v1_flags.tsv. It takes CRLF line ends as well as LF:
// .gitattributes pins the file to LF, but a checkout that predates that, or
// a tool that rewrites it, embeds whatever is on disk.
func parseV1Flags(file string) ([]v1Flag, error) {
	var flags []v1Flag
	for line := range strings.Lines(file) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 || (f[4] != "bool" && f[4] != "string") {
			return nil, fmt.Errorf("v1_flags.tsv: malformed line %q", line)
		}
		flags = append(flags, v1Flag{trees: strings.Split(f[0], ","), path: f[1], name: f[2], shorthand: f[3], isBool: f[4] == "bool"})
	}
	return flags, nil
}

// v1FlagsOn is every flag 1.x had on the command at cmd's path, in the tree
// cmd's root was built for.
func v1FlagsOn(cmd *cobra.Command) []v1Flag {
	return v1FlagsAt(v1Flags(), v1TreeOf(cmd.Root()), pathSpellings(cmd), false)
}

// v1FlagsAt is every flag in flags that tree had on a command at one of paths,
// or, with below, on one of those commands or anything under it.
func v1FlagsAt(flags []v1Flag, tree string, paths []string, below bool) []v1Flag {
	var on []v1Flag
	for _, f := range flags {
		if !slices.Contains(f.trees, tree) {
			continue
		}
		if slices.ContainsFunc(paths, func(p string) bool {
			return f.path == p || (below && strings.HasPrefix(f.path, p+" "))
		}) {
			on = append(on, f)
		}
	}
	return on
}

// pathSpellings is every way to type cmd's path below the root, each word
// its command's name or one of its aliases, the names alone first: a command
// v2 renamed and kept the 1.x name of as an alias is at the 1.x path under
// one of these.
func pathSpellings(cmd *cobra.Command) []string {
	if !cmd.HasParent() {
		return nil
	}
	names := append([]string{cmd.Name()}, cmd.Aliases...)
	if !cmd.Parent().HasParent() {
		return names
	}
	var out []string
	for _, parent := range pathSpellings(cmd.Parent()) {
		for _, name := range names {
			out = append(out, parent+" "+name)
		}
	}
	return out
}

// v1FlagOn is the flag 1.x had on cmd typed as typed (a shorthand's letter
// when isShorthand), or nil when it had none.
func v1FlagOn(cmd *cobra.Command, typed string, isShorthand bool) *v1Flag {
	return findV1Flag(v1FlagsOn(cmd), typed, isShorthand)
}

func findV1Flag(flags []v1Flag, typed string, isShorthand bool) *v1Flag {
	for i := range flags {
		if f := &flags[i]; (isShorthand && f.shorthand == typed) || (!isShorthand && f.name == typed) {
			return f
		}
	}
	return nil
}

// isRemovedFlagErr reports whether err is a run refused for a removed flag.
func isRemovedFlagErr(err error) bool {
	var removed *removedFlagErr
	return errors.As(err, &removed)
}

// acceptRemovedFlags keeps a removed 1.x flag from standing in the way of a
// run that asks for help or for shell completions, which cobra answers only
// once the flags parse.
//
// A run that asks for help gets the help instead of the refusal: one that
// passes -h or --help as a word of its own before any "--", wherever it
// stands, or that pflag has already read help from when it reaches the
// removed flag (`-hf`). It does not matter that such a -h might really be a
// flag's value (`--name -h`): the worst that does is show help instead of an
// error. A group that reaches the removed letter first (`-fh`) is refused, as
// pflag stops there.
//
// A completion request (cobra's __complete) is never refused at all: cobra
// parses the command's flags itself there, without the flag error func, and
// fails the completion on any flag it does not know. So the command it
// completes gets each removed flag 1.x had on it as a hidden flag of the
// same arity, which offers nothing and runs nothing, since a completion
// request never runs the command (acceptForCompletion).
func acceptRemovedFlags(root *cobra.Command, args []string) {
	if isShellCompletion(args) {
		if len(args) > 1 {
			acceptForCompletion(root, args[1:len(args)-1])
		}
		return
	}
	askedForHelp := asksForHelp(args)
	refuse := root.FlagErrorFunc()
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		err = refuse(cmd, err)
		if isRemovedFlagErr(err) && (askedForHelp || cmd.Flags().Changed("help")) {
			return pflag.ErrHelp
		}
		return err
	})
}

// acceptForCompletion readies the tree for a completion request whose words
// before the one being completed are args.
//
// Cobra finds the command to complete as it finds one to run, and there it
// takes a flag it does not know on the command it has reached so far to have
// a value: `api airflow --json ls` would hide ls behind 1.x's boolean --json.
// So the walk here follows cobra's, and a flag the command reached so far does
// not have, that 1.x had on it or on something under it, is added to that
// command as a hidden flag of 1.x's arity first, which is what cobra then
// reads. The command the walk ends on then gets every removed flag 1.x had
// on it, as cobra parses them all there: but only when a flag typed is one it
// does not have, so a request that passes no unknown flag (nearly all) never
// reads the inventory.
func acceptForCompletion(root *cobra.Command, args []string) {
	cmd, descend := root, true
	var typed []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			typed = append(typed, arg)
			if descend {
				hasTypedFlag(cmd, arg, true)
				if consumesNextValue(mergedFlags(cmd), arg) {
					i++
				}
			}
			continue
		}
		if descend {
			if sub := subcommandNamed(cmd, arg); sub != nil {
				cmd = sub
			} else {
				descend = false
			}
		}
	}
	if cmd.DisableFlagParsing || !slices.ContainsFunc(typed, func(arg string) bool { return !hasTypedFlag(cmd, arg, false) }) {
		return
	}
	on := v1FlagsOn(cmd)
	for i := range on {
		if findRemovedFlag(cmd, on[i].name) != nil {
			addHiddenFlag(cmd, &on[i])
		}
	}
}

// hasTypedFlag reports whether cmd has every flag arg names that pflag would
// read: a long flag, or each letter of a shorthand group up to one that takes
// the rest as its value. With accept, a flag cmd lacks that 1.x had on cmd or
// below it is added to cmd first, hidden (addHiddenFlag).
func hasTypedFlag(cmd *cobra.Command, arg string, accept bool) bool {
	v1Below := func(typed string, isShorthand bool) *v1Flag {
		if !accept {
			return nil
		}
		return v1FlagBelow(cmd, typed, isShorthand)
	}
	if name, ok := strings.CutPrefix(arg, "--"); ok {
		name, _, _ = strings.Cut(name, "=")
		if cmd.Flag(name) != nil {
			return true
		}
		if f := v1Below(name, false); f != nil {
			addHiddenFlag(cmd, f)
		}
		return cmd.Flag(name) != nil
	}
	letters, _, _ := strings.Cut(arg[1:], "=")
	for i := range len(letters) {
		letter := letters[i : i+1]
		flag := shorthandFlag(cmd, letter)
		if flag == nil {
			if f := v1Below(letter, true); f != nil {
				addHiddenFlag(cmd, f)
				flag = shorthandFlag(cmd, letter)
			}
		}
		if flag == nil {
			return false
		}
		if flag.NoOptDefVal == "" {
			return true // the rest of the group is its value
		}
	}
	return true
}

// v1FlagBelow is the flag 1.x had typed as typed on cmd, or failing that on
// the commands under it, when those agree on whether it takes a value: -f
// was --force on some and --raw-field on others, and the arity is all a
// completion needs of it. nil when there is none, or they disagree.
func v1FlagBelow(cmd *cobra.Command, typed string, isShorthand bool) *v1Flag {
	if f := v1FlagOn(cmd, typed, isShorthand); f != nil {
		return f
	}
	var found *v1Flag
	for _, f := range v1FlagsAt(v1Flags(), v1TreeOf(cmd.Root()), pathSpellings(cmd), true) {
		if (isShorthand && f.shorthand != typed) || (!isShorthand && f.name != typed) {
			continue
		}
		if found != nil && found.isBool != f.isBool {
			return nil
		}
		found = &f
	}
	return found
}

// addHiddenFlag gives cmd 1.x's flag f as a hidden flag of the same arity,
// for a completion request to parse, unless cmd has a flag of that name. It
// drops a shorthand cmd uses already, as pflag panics on a second one.
func addHiddenFlag(cmd *cobra.Command, f *v1Flag) {
	if cmd.Flag(f.name) != nil {
		return
	}
	shorthand := f.shorthand
	if shorthand == "h" || shorthandFlag(cmd, shorthand) != nil {
		shorthand = ""
	}
	if f.isBool {
		cmd.Flags().BoolP(f.name, shorthand, false, "")
	} else {
		cmd.Flags().StringP(f.name, shorthand, "", "")
	}
	_ = cmd.Flags().MarkHidden(f.name) //nolint:errcheck // it was registered on the line above
}

func shorthandFlag(cmd *cobra.Command, letter string) *pflag.Flag {
	if f := cmd.LocalFlags().ShorthandLookup(letter); f != nil {
		return f
	}
	return cmd.InheritedFlags().ShorthandLookup(letter)
}

// mergedFlags is every flag cmd takes, its own and those it inherits, as
// cobra merges them before it parses.
func mergedFlags(cmd *cobra.Command) *pflag.FlagSet {
	flags := cmd.Flags()
	flags.AddFlagSet(cmd.InheritedFlags())
	return flags
}

// subcommandNamed is cmd's subcommand called name or aliased to it, as cobra
// matches one, or nil.
func subcommandNamed(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}

// asksForHelp reports whether args pass -h or --help as a word of their own
// before the "--" that ends flags.
func asksForHelp(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "-h", "--help":
			return true
		}
	}
	return false
}
