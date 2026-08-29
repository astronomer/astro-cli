package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// claimant is who answers to one spelling under a parent, and how it claimed it.
type claimant struct {
	name string
	kind string
}

// spellings is every word a command answers to, its name first.
func spellings(cmd *cobra.Command) []string {
	return append([]string{cmd.Name()}, cmd.Aliases...)
}

// TestNoTwoSiblingsAnswerToTheSameWord fails when one word resolves to two
// commands under the same parent.
//
// This is not style. `astro deployment variable` and `astro deployment
// airflow-variable` both claimed the alias `var`, and cobra's Find returns the
// first match it walks past, so one of them was simply unreachable — no short
// spelling reached `airflow-variable` at all.
//
// The message deliberately does not say which one wins. Find iterates the raw
// command slice, but Commands() sorts that slice in place, so today's winner is
// whichever happened to be registered first and a single Commands() call
// anywhere earlier would reverse it. That instability is the argument for
// banning the duplicate rather than writing down a precedence rule.
//
// Hidden commands count. A hidden command still answers when typed, so it still
// shadows a sibling that shares its spelling.
func TestNoTwoSiblingsAnswerToTheSameWord(t *testing.T) {
	for platform, root := range rootsUnderTest(t) {
		walkCmd(root, func(parent *cobra.Command) {
			claims := map[string]claimant{}
			// Commands() is sorted, so a failure reports the same way every run.
			for _, sub := range parent.Commands() {
				for i, word := range spellings(sub) {
					kind := "an alias"
					if i == 0 {
						kind = "its name"
					}
					prior, seen := claims[word]
					if !seen {
						claims[word] = claimant{name: sub.Name(), kind: kind}
						continue
					}
					// Name comes first in spellings, so a repeat against the
					// same command is always the name echoed in its own alias list.
					if prior.name == sub.Name() {
						t.Errorf("[%s] %s: %q is both the name of %q and one of its own aliases; drop the alias",
							platform, parent.CommandPath(), word, sub.Name())
						continue
					}
					t.Errorf("[%s] %s: %q is claimed by both %q (%s) and %q (%s); cobra answers with whichever was registered first, so the other is unreachable",
						platform, parent.CommandPath(), word, prior.name, prior.kind, sub.Name(), kind)
				}
			}
		})
	}
}

// TestEveryVisibleTopLevelCommandIsClassified keeps the root help's groups
// complete. An unclassified command renders under "Additional Commands", which
// is a legible outcome but never a chosen one.
func TestEveryVisibleTopLevelCommandIsClassified(t *testing.T) {
	for platform, root := range rootsUnderTest(t) {
		for _, cmd := range root.Commands() {
			if cmd.Hidden {
				continue
			}
			if commandGroup[cmd.Name()] == "" {
				t.Errorf("[%s] %q has no commandGroup entry, so it renders under \"Additional Commands\"", platform, cmd.Name())
			}
		}
	}
}

// TestCommandGroupHasNoStaleEntries is the half the old
// NotContains("Additional Commands:") assertion could never do: it catches a
// table row naming a command that no longer exists, which is invisible in help
// output because nothing renders it.
func TestCommandGroupHasNoStaleEntries(t *testing.T) {
	existing := map[string]bool{}
	for _, root := range rootsUnderTest(t) {
		for _, cmd := range root.Commands() {
			existing[cmd.Name()] = true
		}
	}
	for name := range commandGroup {
		if !existing[name] {
			t.Errorf("commandGroup has %q, which is no longer a top-level command in either platform branch; drop the line", name)
		}
	}
}

// One gap this file cannot close: cobra injects `help` and `completion` at
// ExecuteC time, after the tree is built, so a command aliased to either never
// meets them in a walk over the constructed tree.
