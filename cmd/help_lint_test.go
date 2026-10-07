package cmd

import (
	"fmt"
	"regexp"
	"sort"
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
