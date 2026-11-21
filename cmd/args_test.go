package cmd

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// argsExceptions are the leaves that take a positional argument without
// refusing a stray one, each with why. Every other leaf refuses an argument it
// did not declare as a usage error before any pre-run, so a typo or a stray
// word is told apart from a command that was run right. A command that takes
// positionals declares them (cobra.ExactArgs and friends); only a command that
// hands its arguments on to something else belongs here.
var argsExceptions = map[string]string{
	"astro link add":                          "takes NAME and, after --, the command that mints a token: it validates both itself",
	"astro local run":                         "everything after the first word is the wrapped command's own argv",
	"astro deployment variable create":        "takes any number of KEY=VALUE pairs",
	"astro deployment variable update":        "takes any number of KEY=VALUE pairs",
	"astro local env airflow-variable link":   forEachDir,
	"astro local env airflow-variable unlink": forEachDir,
	"astro local env connection link":         forEachDir,
	"astro local env connection unlink":       forEachDir,
	"astro local env variable link":           forEachDir,
	"astro local env variable unlink":         forEachDir,
}

const forEachDir = "takes any number of DIR... arguments after its one declared name"

// TestEveryLeafRefusesAStrayArgument walks every tree and fails on a command
// that runs, has no subcommands, and accepts a word it never declared.
func TestEveryLeafRefusesAStrayArgument(t *testing.T) {
	var accepting []string
	seen := map[string]bool{}
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(c *cobra.Command) {
			if !c.Runnable() || c.HasSubCommands() || c.DisableFlagParsing {
				return
			}
			// The check a run applies: a command that declared none has the
			// default Execute gives it (cliout.UndeclaredArgs), and a tree
			// BeforeArgs wraps has it there too.
			check := c.Args
			if check == nil {
				accepting = append(accepting, fmt.Sprintf("[%s] %s (declares no Args)", tree.name, c.CommandPath()))
				return
			}
			path := c.CommandPath()
			if !acceptsStray(c, check) {
				return
			}
			if _, ok := argsExceptions[path]; ok {
				seen[path] = true
				return
			}
			accepting = append(accepting, fmt.Sprintf("[%s] %s", tree.name, path))
		})
	}
	sort.Strings(accepting)
	if len(accepting) > 0 {
		t.Errorf("leaves that accept a stray argument (declare cobra.NoArgs or the validator for the positionals it takes; an exception goes in argsExceptions with a reason):\n  %s", strings.Join(accepting, "\n  "))
	}
	for path, why := range argsExceptions {
		if why == "" {
			t.Errorf("argsExceptions[%q] has no reason", path)
		}
		if !seen[path] {
			t.Errorf("argsExceptions[%q] matches no leaf that accepts a stray argument; drop it", path)
		}
	}
}

// acceptsStray reports whether check takes 64 words, more than any command declares positionals for.
// A validator that reads its command's flags before they are parsed (the
// argument-or-flag selectors) panics after its own one-argument limit has
// passed: that is a declared positional, not a stray one.
func acceptsStray(c *cobra.Command, check cobra.PositionalArgs) (accepts bool) {
	defer func() {
		if recover() != nil {
			accepts = false
		}
	}()
	return check(c, make([]string, 64)) == nil
}
