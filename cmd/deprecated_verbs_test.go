package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// A verb v2 moved or renamed keeps its old spelling for the scripts that type
// it: the same command, hidden, that says once per run what to type instead.
// These are the ones there are, by the path below the root of the old
// spelling, and what replaces it (also below the root). The token groups'
// own verbs moved up to the token command itself, and the links' delete is
// remove.
type deprecatedVerb struct{ instead, suffix string }

func (d deprecatedVerb) warning(path []string) string {
	return "astro " + strings.Join(path, " ") + " is deprecated: use astro " + d.instead + d.suffix
}

var deprecatedVerbs = func() map[string]deprecatedVerb {
	m := map[string]deprecatedVerb{}
	const (
		removeNote = " --yes (without --yes, remove asks for confirmation, which this spelling never did)"
		listNote   = ", which lists the tokens of every scope, not only %s tokens: tell them apart by the scope field of -o json"
	)
	for _, verb := range []string{"add", "update", "remove", "list"} {
		for _, g := range []struct{ old, new, kind string }{
			{"workspace token organization-token ", "workspace token ", "Organization"},
			{"deployment token workspace-token ", "deployment token ", "Workspace"},
			{"deployment token organization-token ", "deployment token ", "Organization"},
		} {
			d := deprecatedVerb{instead: g.new + verb}
			switch verb {
			case "remove":
				d.suffix = removeNote
			case "list":
				d.suffix = fmt.Sprintf(listNote, g.kind)
			}
			m[g.old+verb] = d
		}
	}
	for _, link := range []string{"variable", "connection", "airflow-variable"} {
		m["env "+link+" link delete"] = deprecatedVerb{instead: "env " + link + " link remove"}
	}
	return m
}()

// isDeprecatedVerb reports whether cmd is a deprecated spelling, as
// cliout.DeprecateCommand marks one.
func isDeprecatedVerb(cmd *cobra.Command) bool {
	return cmd.Annotations[cliout.DeprecatedCommandAnnotation] == "true"
}

// Every deprecated verb warns on stderr, once per run, naming the spelling
// typed and the one to type, whichever tree it is in; a second run says it
// again. The command still runs: the run past the warning is the one the tree
// has (disarmed here, as in the other spelling tests, so it ends in errRan).
func TestEveryDeprecatedVerbWarnsOncePerRun(t *testing.T) {
	var checked int
	for _, c := range treeConfigs {
		for path, verb := range deprecatedVerbs {
			words := strings.Fields(path)
			root, _ := disarmedTree(t, c)
			if cmd, rest, err := root.Find(words); err != nil || len(rest) > 0 || cmd.Name() != words[len(words)-1] {
				continue // a tree that does not have the command: APC
			}
			for range 2 {
				_, stderr, _ := executeRoot(root, words...)
				assert.Equal(t, []string{verb.warning(words)}, warningLines(stderr), "[%s] astro %s", c.name, path)
			}
			checked++
		}
	}
	assert.Equal(t, 2*len(deprecatedVerbs), checked, "both Astro trees have every deprecated verb")
}

// A deprecated verb is recorded as the hidden flag aliases are, one event with
// the old path beside the new, so we know when it can go; nothing else rides
// on it. Its own flag aliases ride in the same event.
func TestADeprecatedVerbIsRecordedForTelemetry(t *testing.T) {
	got := recordRenamedFlags(t)
	root, _ := disarmedTree(t, astroTree)
	executeRoot(root, "workspace", "token", "organization-token", "add", "--org-token-name", "x")
	executeRoot(root, "env", "variable", "link", "delete")
	require.Len(t, *got, 3, "the flag alias and the verb each record, and a link delete records once")
	assert.Equal(t, renamedFlagEvent{"astro workspace token organization-token add", []string{"--org-token-name"}, []string{"--name"}}, (*got)[0])
	assert.Equal(t, renamedFlagEvent{"astro workspace token organization-token add", []string{"workspace token organization-token add"}, []string{"workspace token add"}}, (*got)[1])
	assert.Equal(t, renamedFlagEvent{"astro env variable link delete", []string{"env variable link delete"}, []string{"env variable link remove"}}, (*got)[2])
}

// The alias is typed the way it was in 1.x, by its own short alias too, and
// the line names the verb, not the letters typed.
func TestADeprecatedVerbTypedByItsAliasNamesTheVerb(t *testing.T) {
	for _, c := range treeConfigs {
		root, _ := disarmedTree(t, c)
		if _, rest, err := root.Find([]string{"workspace", "token"}); err != nil || len(rest) > 0 {
			continue
		}
		// organization-token remove answers to rm, as every remove does.
		_, stderr, _ := executeRoot(root, "workspace", "token", "organization-token", "rm")
		assert.Equal(t, []string{"astro workspace token organization-token remove is deprecated: use astro workspace token remove --yes (without --yes, remove asks for confirmation, which this spelling never did)"}, warningLines(stderr), c.name)
	}
}

// The deprecated spellings are not in help, at any level: the group, the
// verbs and the old flags are hidden, and what replaces each is shown. Every
// command marked deprecated is in the table above, and every one in the
// table is marked.
func TestDeprecatedVerbsAreHiddenAndTheirReplacementsAreNot(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		marked := map[string]bool{}
		walkCmd(tree.root, func(cmd *cobra.Command) {
			if isDeprecatedVerb(cmd) {
				marked[pathBelowRoot(cmd)] = true
				assert.True(t, cmd.Hidden, "[%s] %s is a deprecated spelling and is hidden", tree.name, cmd.CommandPath())
			}
		})
		for path, verb := range deprecatedVerbs {
			words := strings.Fields(path)
			old, rest, err := tree.root.Find(words)
			if err != nil || len(rest) > 0 || old.Name() != words[len(words)-1] {
				assert.False(t, strings.HasPrefix(tree.name, "astro"), "[%s] has no %s", tree.name, path)
				continue
			}
			assert.True(t, marked[path], "[%s] astro %s is marked deprecated", tree.name, path)
			// The group that holds a verb's old spelling is hidden too.
			if group := old.Parent(); group.Name() == "organization-token" || group.Name() == "workspace-token" {
				assert.True(t, group.Hidden, "[%s] %s", tree.name, group.CommandPath())
			}
			now, rest, err := tree.root.Find(strings.Fields(verb.instead))
			require.NoError(t, err, verb.instead)
			require.Empty(t, rest, verb.instead)
			assert.False(t, now.Hidden, "[%s] astro %s is what the old spelling says to use, and shows in help", tree.name, verb.instead)
			assert.False(t, isDeprecatedVerb(now), "[%s] astro %s", tree.name, verb.instead)

			// Help of the command the old spelling hangs from does not offer
			// it: its group when it is in an old one, else the verb itself.
			holder, offered := old.Parent(), old.Name()
			if group := old.Parent(); group.Name() == "organization-token" || group.Name() == "workspace-token" {
				holder, offered = group.Parent(), group.Name()
			}
			helpOut, helpErr, _ := executeRoot(tree.root, append(strings.Fields(strings.TrimPrefix(holder.CommandPath(), tree.root.Name()+" ")), "--help")...)
			listing := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(offered) + `[\s,]`)
			assert.False(t, listing.MatchString(helpOut+helpErr), "[%s] help of %s offers %s", tree.name, holder.CommandPath(), offered)
		}
		for path := range marked {
			_, ok := deprecatedVerbs[path]
			assert.True(t, ok, "[%s] astro %s is marked deprecated and is not in deprecatedVerbs", tree.name, path)
		}
	}
}
