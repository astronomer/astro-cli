package cmd

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The help style guide, enforced. Each rule is a function that names what is
// wrong with one command's help, or returns "" when nothing is. A command the
// rule already failed on when it was written is listed in helpExceptions under
// the rule's name; those lists may only shrink — fixing a command and deleting
// its line is the whole job of a cleanup PR, and TestHelpExceptionsOnlyShrink
// fails on a line that no longer excuses anything.

type helpRule struct {
	name string
	why  string
	// check names what is wrong with cmd's help, or returns "".
	check func(cmd *cobra.Command) string
}

var helpRules = []helpRule{
	{
		name: "short",
		why: "A command's Short is a sentence fragment in the imperative, read in a list beside its siblings: " +
			"it starts with a capital and does not end in a period.",
		check: checkShort,
	},
	{
		name: "root-row",
		why: "A top-level command's Short fits on its row of the root page at the widest help is drawn " +
			"(helpMaxWidth), beside the column the widest spellings size: the root page is the one everyone " +
			"reads first, and a row that wraps breaks its list.",
		check: checkRootRow,
	},
	{
		name: "flag-usage",
		why:  "A flag's description reads like a command's Short: it starts with a capital and does not end in a period.",
		check: func(cmd *cobra.Command) string {
			var bad []string
			cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
				if f.Hidden || f.Name == helpName {
					return
				}
				if problem := checkFragment(f.Usage); problem != "" {
					bad = append(bad, fmt.Sprintf("--%s %s", f.Name, problem))
				}
			})
			cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
				if f.Hidden {
					return
				}
				if problem := checkFragment(f.Usage); problem != "" {
					bad = append(bad, fmt.Sprintf("--%s %s", f.Name, problem))
				}
			})
			return strings.Join(bad, "; ")
		},
	},
	{
		name: "use",
		why: "A positional argument in Use is <UPPER_SNAKE> when required and [UPPER_SNAKE] when optional, " +
			"with ... inside or after the brackets of one that repeats, so every page spells an argument the same way. " +
			"`astro af` set the convention: `astro af runs get <DAG_ID> <RUN_ID>`. Arguments handed on after `--` " +
			"are spelled `[-- COMMAND...]`, last.",
		check: checkUse,
	},
	{
		name: "example",
		why: "A runnable command with no subcommands shows at least one example: help is where someone learns " +
			"a command, and an example is the fastest way to. A command that groups others is exempt; its " +
			"children carry the examples.",
		check: func(cmd *cobra.Command) string {
			if !cmd.Runnable() || cmd.HasAvailableSubCommands() || strings.TrimSpace(cmd.Example) != "" {
				return ""
			}
			return "has no Example"
		},
	},
	{
		name: "example-style",
		why: "Examples are command lines indented two spaces, with no prompt (`$ `) — so they paste as they are — " +
			"and any explanation as a `  # comment` line above the command it explains.",
		check: checkExampleStyle,
	},
	{
		name: "example-width",
		why: "An example line is at most helpMaxWidth columns, the widest help is drawn: examples are printed " +
			"as written, never wrapped, so a longer one runs off a terminal's edge. Continue a long command with " +
			"a trailing ` \\` onto a line indented further.",
		check: checkExampleWidth,
	},
	{
		name: "example-runs",
		why: "Every astro command line in an example names a command that exists and flags that command accepts: " +
			"an example is copied as it is, and one that fails teaches the wrong thing. Placeholders are fine as " +
			"argument and flag values.",
		check: checkExampleRuns,
	},
	{
		name: "long-unwrapped",
		why: "A paragraph of a Long is one line, separated from the next by a blank line: help wraps prose to the " +
			"terminal itself, so a paragraph broken by hand at a fixed width renders as a short line in mid-sentence " +
			"followed by the renderer's own wrapping. A list (`- ` items) or an indented literal starts after a blank line " +
			"or a heading ending in a colon; any other line directly after another is a paragraph broken by hand.",
		check: checkLongUnwrapped,
	},
}

// checkLongUnwrapped finds a Long laid out by hand. Each unindented line is a
// whole paragraph, a heading, or a list item, so the only lines that may
// follow one directly, with no blank line between, are:
//
//   - a list item after a heading (a line ending in a colon) or after another
//     list item;
//   - an indented line (a command or a literal) after a heading.
//
// Anything else is a paragraph broken by hand: in mid-sentence, after a
// sentence (one sentence per line renders as a run of short lines), after an
// abbreviation or a version number, inside a list item, or onto a line that a
// stray leading space makes look indented. Telling those apart from a real
// sentence end is guesswork, so the rule does not try: a new paragraph, list
// or literal starts after a blank line or a heading, and nowhere else.
func checkLongUnwrapped(cmd *cobra.Command) string {
	lines := strings.Split(strings.Trim(cmd.Long, "\n"), "\n")
	blank := func(line string) bool { return strings.TrimSpace(line) == "" }
	indented := func(line string) bool { return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") }
	item := func(line string) bool { return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") }
	heading := func(line string) bool { return !item(line) && strings.HasSuffix(strings.TrimSpace(line), ":") }
	for i := 0; i+1 < len(lines); i++ {
		line, next := lines[i], lines[i+1]
		if blank(line) || indented(line) || blank(next) {
			continue
		}
		switch {
		case item(next) && (heading(line) || item(line)):
		case indented(next) && heading(line):
		default:
			return fmt.Sprintf("Long breaks a paragraph by hand: %q is followed by %q", line, next)
		}
	}
	return ""
}

// checkRootRow finds cmd's row in the root page's command list, drawn by the
// renderer itself at helpMaxWidth, and reports a description that runs onto
// a second line. The column is whatever the renderer sizes it to for that
// root's commands, so the room follows it if helpNameColumnMax or the widest
// spellings change.
func checkRootRow(cmd *cobra.Command) string {
	parent := cmd.Parent()
	if parent == nil || parent.HasParent() {
		return ""
	}
	name := "  " + commandSpellings(cmd)
	for _, section := range commandSections(parent, helpMaxWidth) {
		lines := strings.Split(section, "\n")
		for i, line := range lines {
			if line != name && !strings.HasPrefix(line, name+" ") {
				continue
			}
			// Spellings wider than the column take a line of their own, with
			// the description on the next; otherwise it shares theirs. Either
			// way, any further line indented under it is the description
			// wrapping.
			ownLine := 0
			if line == name {
				ownLine = 1
			}
			var continued []string
			for _, next := range lines[i+1:] {
				if !strings.HasPrefix(next, "   ") {
					break
				}
				continued = append(continued, next)
			}
			// The renderer never breaks a word, so a description can also
			// overrun the width on a line of its own without wrapping.
			for _, drawn := range append([]string{line}, continued...) {
				if n := utf8.RuneCountInString(drawn); n > helpMaxWidth {
					return fmt.Sprintf("its row is %d columns, past the %d the root page wraps at: %q", n, helpMaxWidth, drawn)
				}
			}
			if len(continued) <= ownLine {
				return ""
			}
			hang := len(continued[0]) - len(strings.TrimLeft(continued[0], " "))
			return fmt.Sprintf("Short is %d columns and the root page has %d before it wraps at %d: %q",
				utf8.RuneCountInString(cmd.Short), helpMaxWidth-hang, helpMaxWidth, cmd.Short)
		}
	}
	return ""
}

func checkShort(cmd *cobra.Command) string {
	if strings.Contains(cmd.Short, "\n") {
		return "Short spans more than one line"
	}
	return checkFragment(cmd.Short)
}

// checkFragment is the rule a Short and a flag's description share.
func checkFragment(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "is empty"
	case !startsCapitalized(s):
		return fmt.Sprintf("does not start with a capital: %q", s)
	case strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "..."):
		return fmt.Sprintf("ends with a period: %q", s)
	}
	return ""
}

// startsCapitalized accepts an uppercase first letter, and a first word that is
// a literal — a flag, a code span, a number, a quoted value — whose case is not
// the writer's to choose.
func startsCapitalized(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r) || unicode.IsDigit(r) || strings.ContainsRune("-`\"'(", r)
}

var (
	useRequired = regexp.MustCompile(`^<[A-Z][A-Z0-9]*(_[A-Z0-9]+)*>(\.{3})?$`)
	useOptional = regexp.MustCompile(`^\[[A-Z][A-Z0-9]*(_[A-Z0-9]+)*(\.{3})?\](\.{3})?$`)
	// usePassthrough is the one spelling of "everything after -- is handed on
	// as it is": `[-- COMMAND...]`, an optional repeating argument behind the
	// `--` that ends flag parsing. It is the last thing in Use.
	usePassthrough = regexp.MustCompile(` \[-- [A-Z][A-Z0-9]*(_[A-Z0-9]+)*\.{3}\]$`)
)

func checkUse(cmd *cobra.Command) string {
	fields := strings.Fields(usePassthrough.ReplaceAllString(cmd.Use, ""))
	var bad []string
	for _, f := range fields[min(1, len(fields)):] {
		// [flags] and [command] are cobra's own, and it appends them itself.
		if f == "[flags]" || f == "[command]" {
			continue
		}
		if !useRequired.MatchString(f) && !useOptional.MatchString(f) {
			bad = append(bad, f)
		}
	}
	if len(bad) == 0 {
		return ""
	}
	return fmt.Sprintf("Use %q spells %s outside <ARG> / [ARG]", cmd.Use, strings.Join(bad, ", "))
}

// checkExampleRuns resolves each astro command line in cmd's examples against
// the tree cmd belongs to, as cobra would on that platform, and parses its
// flags there. Nothing runs. A line continued with a trailing backslash is
// joined with the next first.
func checkExampleRuns(cmd *cobra.Command) string {
	if strings.TrimSpace(cmd.Example) == "" {
		return ""
	}
	joined := strings.ReplaceAll(cmd.Example, "\\\n", " ")
	for _, line := range strings.Split(joined, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		for _, args := range astroCommands(line) {
			if problem := resolveExample(cmd.Root(), args); problem != "" {
				return fmt.Sprintf("%q: %s", line, problem)
			}
		}
	}
	return ""
}

// astroCommands returns the arguments each astro command on an example line
// is handed: for every command that runs astro (`astro local stop || astro
// local reset --yes` has two), the words after `astro` up to whatever ends
// that command (a pipe, a redirect, a separator).
func astroCommands(line string) [][]string {
	words := shellWords(line)
	var commands [][]string
	for _, start := range astroStarts(words) {
		args := []string{}
		for _, w := range words[start:] {
			if commandSeparator[w] || strings.HasPrefix(w, ">") || strings.HasPrefix(w, "2>") || w == "<" {
				break
			}
			args = append(args, w)
		}
		commands = append(commands, args)
	}
	return commands
}

// envAssignment is a shell word that sets an environment variable for the
// command after it: `ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start`.
var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// commandSeparator is a shell word after which a new command starts: a pipe,
// a list operator, or a sequence.
var commandSeparator = map[string]bool{"|": true, "||": true, "&&": true, ";": true}

// astroStarts is the index of the first argument astro is handed by each
// command in words that runs it. A command runs astro when it starts with
// `astro`, after any NAME=value words setting its environment, at the start
// of the line or after a separator (`stop || astro local reset`).
func astroStarts(words []string) []int {
	var starts []int
	for i := range words {
		if i > 0 && !commandSeparator[words[i-1]] {
			continue
		}
		j := i
		for j < len(words) && envAssignment.MatchString(words[j]) {
			j++
		}
		if j < len(words) && words[j] == "astro" {
			starts = append(starts, j+1)
		}
	}
	return starts
}

// shellWords splits a line the way a POSIX shell would for the cases examples
// use: whitespace, single and double quotes, a trailing # comment, and the
// operators astroCommands stops at. It does not expand anything.
func shellWords(line string) []string {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '#' && !inWord:
			flush()
			return words
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t':
			flush()
		case r == '|' || r == ';':
			flush()
			if n := len(words); r == '|' && n > 0 && words[n-1] == "|" {
				words[n-1] = "||"
			} else {
				words = append(words, string(r))
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	flush()
	return words
}

// resolveExample finds the command args name under root and parses args'
// flags with it, reporting what cobra would refuse.
func resolveExample(root *cobra.Command, args []string) string {
	target, rest, err := root.Find(args)
	if err != nil {
		return err.Error()
	}
	if target == root {
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			return fmt.Sprintf("no command %q", args[0])
		}
		return ""
	}
	if target.HasAvailableSubCommands() && !target.Runnable() {
		return fmt.Sprintf("%q is a group, not a command", target.CommandPath())
	}
	// Parse a copy of the flags, so linting one example leaves no values set
	// on the tree for the next. Help is not run, so -h needs no special case.
	flags := pflag.NewFlagSet(target.Name(), pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	target.Flags().VisitAll(func(f *pflag.Flag) { flags.AddFlag(copyFlag(f)) })
	target.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if flags.Lookup(f.Name) == nil {
			flags.AddFlag(copyFlag(f))
		}
	})
	if err := flags.Parse(rest); err != nil {
		return err.Error()
	}
	// Find stops at the deepest command it can match, so a mistyped
	// subcommand under a runnable group comes back as that group's argument.
	// A group that declares an Args validator takes positionals of its own
	// (`astro api airflow /dags`), and the validator below judges those.
	if positional := flags.Args(); len(positional) > 0 && target.HasAvailableSubCommands() && target.Args == nil {
		return fmt.Sprintf("%q has no subcommand %q", target.CommandPath(), positional[0])
	}
	if err := target.ValidateArgs(flags.Args()); err != nil {
		return err.Error()
	}
	return ""
}

// copyFlag is f with a value of its own of the same type, so parsing sets
// nothing on the command f came from. A value that cannot be told apart
// (a custom type) is reused as a string, which accepts anything — the flag's
// existence is what is checked, not its value.
func copyFlag(f *pflag.Flag) *pflag.Flag {
	c := *f
	if f.Value.Type() == "bool" {
		c.Value = new(boolFlag)
	} else {
		c.Value = new(stringFlag)
	}
	return &c
}

type stringFlag string

func (s *stringFlag) String() string     { return string(*s) }
func (s *stringFlag) Set(v string) error { *s = stringFlag(v); return nil }
func (s *stringFlag) Type() string       { return "string" }

type boolFlag bool

func (b *boolFlag) String() string { return fmt.Sprint(bool(*b)) }
func (b *boolFlag) Set(v string) error {
	parsed, err := strconv.ParseBool(v)
	*b = boolFlag(parsed)
	return err
}
func (b *boolFlag) Type() string     { return "bool" }
func (b *boolFlag) IsBoolFlag() bool { return true }

// checkExampleWidth measures each line of cmd's examples as help prints it.
func checkExampleWidth(cmd *cobra.Command) string {
	var bad []string
	for _, line := range strings.Split(strings.Trim(cmd.Example, "\n"), "\n") {
		if n := utf8.RuneCountInString(line); n > helpMaxWidth {
			bad = append(bad, fmt.Sprintf("%d columns: %q", n, line))
		}
	}
	return strings.Join(bad, "; ")
}

func checkExampleStyle(cmd *cobra.Command) string {
	if strings.TrimSpace(cmd.Example) == "" {
		return ""
	}
	continued := false // the line before ended in a continuation backslash
	for _, line := range strings.Split(strings.Trim(cmd.Example, "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		after := continued
		continued = strings.HasSuffix(line, "\\")
		switch {
		case strings.HasPrefix(line, "    ") && !after:
			// A continuation without the backslash before it pastes as two
			// commands, the second made only of flags, and example-runs
			// would check the first alone.
			return fmt.Sprintf("example line is indented as a continuation, but the line before does not end in ` \\`: %q", line)
		case after && !strings.HasPrefix(line, "    "):
			return fmt.Sprintf("example line follows a continuation backslash but is not indented four spaces: %q", line)
		case strings.HasSuffix(strings.TrimRight(line, " \t"), "\\") && strings.TrimRight(line, " \t") != line:
			// In a shell a backslash continues the line only as its last
			// character; with a space after it, the next line runs as a
			// command of its own, and example-runs checks neither half.
			return fmt.Sprintf("example line has whitespace after its continuation backslash: %q", line)
		case trimmed == "":
		case strings.HasPrefix(trimmed, "$"):
			return fmt.Sprintf("example line has a prompt: %q", line)
		case !strings.HasPrefix(line, "  "):
			return fmt.Sprintf("example line is not indented as a command or a # comment: %q", line)
		case !runsAstro(trimmed) && !strings.HasPrefix(trimmed, "#") && !after:
			return fmt.Sprintf("example line is neither an astro command nor a # comment: %q", line)
		}
	}
	return ""
}

// runsAstro reports whether an example line is a shell command that runs the
// CLI: `astro ...` itself, with any environment set before it
// (`ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start`), or a pipeline with
// astro in it (`echo "$TOKEN" | astro local env variable set API_TOKEN --stdin`).
func runsAstro(line string) bool {
	return len(astroStarts(shellWords(line))) > 0
}

// lintedCommands is every command help is drawn for in any tree
// rootsUnderTest builds, by path, each with the command it names in each
// tree. A path several trees build — `astro deployment create` — is linted in
// each, because they are different commands, or the same command with
// different flags and examples.
func lintedCommands(t *testing.T) map[string][]*cobra.Command {
	t.Helper()
	byPath := map[string][]*cobra.Command{}
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(cmd *cobra.Command) {
			if cmd.Hidden || cmd.Deprecated != "" || cmd.Name() == helpName || isCompletionCmd(cmd) {
				return
			}
			byPath[cmd.CommandPath()] = append(byPath[cmd.CommandPath()], cmd)
		})
	}
	return byPath
}

// isCompletionCmd reports whether cmd is cobra's generated completion command
// or under it: cobra writes those, not us.
func isCompletionCmd(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "completion" && c.HasParent() && !c.Parent().HasParent() {
			return true
		}
	}
	return false
}

// violations runs one rule over every command in linted (lintedCommands),
// keyed by path. A path several trees build reports what is wrong in any of
// them, once each. The trees are built once by the caller and shared across
// rules: building them is the slow part, and no rule changes them.
func violations(linted map[string][]*cobra.Command, rule helpRule) map[string]string {
	found := map[string]string{}
	for path, cmds := range linted {
		seen := map[string]bool{}
		var problems []string
		for _, cmd := range cmds {
			if problem := rule.check(cmd); problem != "" && !seen[problem] {
				seen[problem] = true
				problems = append(problems, problem)
			}
		}
		if len(problems) > 0 {
			sort.Strings(problems)
			found[path] = strings.Join(problems, " | ")
		}
	}
	return found
}

func TestHelpFollowsTheStyleGuide(t *testing.T) {
	linted := lintedCommands(t)
	for _, rule := range helpRules {
		t.Run(rule.name, func(t *testing.T) {
			excused := map[string]bool{}
			for _, path := range helpExceptions[rule.name] {
				excused[path] = true
			}
			found := violations(linted, rule)
			paths := make([]string, 0, len(found))
			for path := range found {
				paths = append(paths, path)
			}
			sort.Strings(paths)
			for _, path := range paths {
				if !excused[path] {
					t.Errorf("%s: %s\n\t%s", path, found[path], rule.why)
				}
			}
		})
	}
}

// TestHelpExceptionsOnlyShrink fails on an exception that no longer excuses
// anything: the command was fixed, renamed or removed, and its line goes with
// it, so the list says exactly what is left to clean up.
func TestHelpExceptionsOnlyShrink(t *testing.T) {
	rules := map[string]helpRule{}
	for _, rule := range helpRules {
		rules[rule.name] = rule
	}
	// The trees are built only when there is an exception to check them for;
	// every list is empty today, and building five roots to check nothing is
	// most of what this test would cost.
	var linted map[string][]*cobra.Command
	for name, paths := range helpExceptions {
		rule, ok := rules[name]
		if !ok {
			t.Errorf("helpExceptions has a list for %q, which is not a rule in helpRules", name)
			continue
		}
		if linted == nil {
			linted = lintedCommands(t)
		}
		found := violations(linted, rule)
		for _, path := range paths {
			if _, still := found[path]; !still {
				t.Errorf("helpExceptions[%q] lists %q, which now passes (or no longer exists); delete the line", name, path)
			}
		}
	}
}
