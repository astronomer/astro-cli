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
			"`astro af` set the convention: `astro af runs get <DAG_ID> <RUN_ID>`.",
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
		name: "example-runs",
		why: "Every astro command line in an example names a command that exists and flags that command accepts: " +
			"an example is copied as it is, and one that fails teaches the wrong thing. Placeholders are fine as " +
			"argument and flag values.",
		check: checkExampleRuns,
	},
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
)

func checkUse(cmd *cobra.Command) string {
	fields := strings.Fields(cmd.Use)
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
		args, ok := astroArgs(line)
		if !ok {
			continue
		}
		if problem := resolveExample(cmd.Root(), args); problem != "" {
			return fmt.Sprintf("%q: %s", line, problem)
		}
	}
	return ""
}

// astroArgs returns the arguments an example line hands astro: the words
// after `astro` in the pipeline stage that runs it, up to whatever ends that
// command (a pipe, a redirect, a separator).
func astroArgs(line string) ([]string, bool) {
	words := shellWords(line)
	start := -1
	for i, w := range words {
		if w == "astro" && (i == 0 || words[i-1] == "|") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	var args []string
	for _, w := range words[start:] {
		if w == "|" || w == "&&" || w == "||" || w == ";" || strings.HasPrefix(w, ">") || strings.HasPrefix(w, "2>") || w == "<" {
			break
		}
		args = append(args, w)
	}
	return args, true
}

// shellWords splits a line the way a POSIX shell would for the cases examples
// use: whitespace, single and double quotes, a trailing # comment, and the
// operators astroArgs stops at. It does not expand anything.
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

func checkExampleStyle(cmd *cobra.Command) string {
	if strings.TrimSpace(cmd.Example) == "" {
		return ""
	}
	for _, line := range strings.Split(strings.Trim(cmd.Example, "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, "$"):
			return fmt.Sprintf("example line has a prompt: %q", line)
		case !strings.HasPrefix(line, "  "):
			return fmt.Sprintf("example line is not indented as a command or a # comment: %q", line)
		case !runsAstro(trimmed) && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(line, "    "):
			// A line indented further continues the command above it: a
			// heredoc, a wrapped argument, or output shown beneath.
			return fmt.Sprintf("example line is neither an astro command nor a # comment: %q", line)
		}
	}
	return ""
}

// runsAstro reports whether an example line is a shell command that runs the
// CLI: `astro ...` itself, or a pipeline with astro in it
// (`echo "$TOKEN" | astro local env variable set API_TOKEN --stdin`).
func runsAstro(line string) bool {
	if line == "astro" || strings.HasPrefix(line, "astro ") {
		return true
	}
	for _, stage := range strings.Split(line, "|")[1:] {
		if stage = strings.TrimSpace(stage); stage == "astro" || strings.HasPrefix(stage, "astro ") {
			return true
		}
	}
	return false
}

// lintedCommands is every command help is drawn for in either platform's
// tree, by path, each with the command it names on each platform. A path both
// trees build — `astro deployment create` — is linted on both, because they
// are different commands.
func lintedCommands(t *testing.T) map[string][]*cobra.Command {
	t.Helper()
	byPath := map[string][]*cobra.Command{}
	for _, root := range rootsUnderTest(t) {
		walkCmd(root, func(cmd *cobra.Command) {
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

// violations runs one rule over every command, keyed by path. A path both
// platforms build reports what is wrong on either, once each.
func violations(t *testing.T, rule helpRule) map[string]string {
	t.Helper()
	found := map[string]string{}
	for path, cmds := range lintedCommands(t) {
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
	for _, rule := range helpRules {
		t.Run(rule.name, func(t *testing.T) {
			excused := map[string]bool{}
			for _, path := range helpExceptions[rule.name] {
				excused[path] = true
			}
			found := violations(t, rule)
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
	for name, paths := range helpExceptions {
		rule, ok := rules[name]
		if !ok {
			t.Errorf("helpExceptions has a list for %q, which is not a rule in helpRules", name)
			continue
		}
		found := violations(t, rule)
		for _, path := range paths {
			if _, still := found[path]; !still {
				t.Errorf("helpExceptions[%q] lists %q, which now passes (or no longer exists); delete the line", name, path)
			}
		}
	}
}
