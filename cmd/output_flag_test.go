package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The whole CLI answers to --output json: every runnable command can
// reach an --output flag that offers json, so a script or an agent can ask any
// of them for something it can parse, and a failure under it is one json
// object (cliout.Execute).
//
// The core tree has held to that since it landed (cmd/local's
// TestTreeInvariants). The shell trees did not, so the commands that still lack
// one are listed below, by platform and full command path. The list may only
// shrink: an entry that no longer exists, or that has gained the flag, fails
// TestOutputFlagAllowlistOnlyShrinks until it is deleted, so it cannot sit
// there excusing a later regression.

// outputExempt are the commands that have no --output by design.
var outputExempt = map[string]string{
	"astro login":       "an interactive browser flow; its result is the stored login, not data",
	"astro logout":      "removes the stored login; nothing to render",
	"astro auth login":  "the same flow as astro login",
	"astro auth logout": "the same as astro logout",
	"astro otto":        "launches an interactive agent session",
	// The requests `astro api` makes print the API's own response and shape
	// it with --jq and --template; an --output there would be a second answer
	// to that question. Their ls and describe are not here: those publish the
	// CLI's own listing and schemas, and take -o like every other command.
	"astro api airflow":      "prints the Airflow API's response; shaped with --jq and --template",
	"astro api cloud":        "prints the Astro API's response; shaped with --jq and --template",
	"astro api registry":     "prints the registry's response; shaped with --jq and --template",
	"astro api airflow spec": "prints the Airflow API's own OpenAPI document, always JSON",
}

// lacksOutputFlag lists, per platform, the runnable commands that cannot reach
// an --output json today. Delete an entry when its command gains one.
var lacksOutputFlag = map[string][]string{
	cloudPlatform: {},
	apcPlatform:   {},
}

// reachesJSONOutput reports whether cmd can reach an --output flag that offers
// json, on itself or inherited. By the flag's help text, because --output also
// names a destination file on some commands (`astro env ... --output FILE`),
// and that is not a format a consumer can ask for.
func reachesJSONOutput(cmd *cobra.Command) bool {
	f := cmd.Flag("output")
	return f != nil && strings.Contains(f.Usage, "json")
}

// visibleRunnable reports whether a user can run cmd and find it: runnable,
// and neither it nor an ancestor hidden.
func visibleRunnable(cmd *cobra.Command) bool {
	if !cmd.Runnable() {
		return false
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Hidden {
			return false
		}
	}
	return true
}

// exemptByDesign reports whether path is an outputExempt entry. Only the
// command itself: a subcommand added under an exempt one is not excused by it.
func exemptByDesign(path string) bool {
	return outputExempt[path] != ""
}

func TestEveryCommandCanReachOutputJSON(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		allowed := map[string]bool{}
		for _, p := range lacksOutputFlag[tree.platform] {
			allowed[p] = true
		}
		var missing []string
		walkCmd(tree.root, func(cmd *cobra.Command) {
			path := cmd.CommandPath()
			if !visibleRunnable(cmd) || exemptByDesign(path) || allowed[path] || reachesJSONOutput(cmd) {
				return
			}
			missing = append(missing, path)
		})
		sort.Strings(missing)
		for _, p := range missing {
			t.Errorf("[%s] %s has no --output json. Every command offers one (`cliout.AddOutputFlag`, "+
				"rendering through cliout.Renderer); do not add it to lacksOutputFlag", tree.name, p)
		}
	}
}

// The allowlist only shrinks: each entry must still name a visible, runnable
// command that still lacks the flag in some tree of its platform. A command
// one configuration builds and another does not (APC's version-gated
// deployment adopt) is excused by one entry for the platform, and stays real
// while any tree that builds it still lacks the flag.
func TestOutputFlagAllowlistOnlyShrinks(t *testing.T) {
	byPlatform := map[string][]map[string]*cobra.Command{}
	for _, tree := range rootsUnderTest(t) {
		byPath := map[string]*cobra.Command{}
		walkCmd(tree.root, func(cmd *cobra.Command) { byPath[cmd.CommandPath()] = cmd })
		byPlatform[tree.platform] = append(byPlatform[tree.platform], byPath)
	}
	for platform, trees := range byPlatform {
		seen := map[string]bool{}
		for _, p := range lacksOutputFlag[platform] {
			if seen[p] {
				t.Errorf("[%s] %q is listed twice in lacksOutputFlag", platform, p)
			}
			seen[p] = true
			// Why the entry is stale, in the first tree that has the
			// command; none if any tree still needs it.
			stale, exists := "", false
			for _, byPath := range trees {
				cmd, ok := byPath[p]
				if !ok {
					continue
				}
				exists = true
				switch {
				case !visibleRunnable(cmd):
					stale = firstNonEmpty(stale, "is no longer a visible runnable command; delete the entry")
				case exemptByDesign(p):
					stale = firstNonEmpty(stale, "is exempt by design (outputExempt); delete the entry")
				case reachesJSONOutput(cmd):
					stale = firstNonEmpty(stale, "has --output json now; delete it from lacksOutputFlag")
				default:
					stale = ""
				}
				if stale == "" {
					break
				}
			}
			switch {
			case !exists:
				t.Errorf("[%s] lacksOutputFlag lists %q, which no longer exists in any %s tree; delete the entry", platform, p, platform)
			case stale != "":
				t.Errorf("[%s] lacksOutputFlag lists %q, which %s", platform, p, stale)
			}
		}
	}
	for platform := range lacksOutputFlag {
		if platform != cloudPlatform && platform != apcPlatform {
			t.Errorf("lacksOutputFlag has an entry for unknown platform %q", platform)
		}
	}
}

// Each exemption names a command that exists in some platform and still has
// no --output json, so the list of reasons cannot outlive what it excuses.
func TestOutputExemptionsExist(t *testing.T) {
	exists, hasJSON := map[string]bool{}, map[string]bool{}
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(cmd *cobra.Command) {
			exists[cmd.CommandPath()] = true
			if reachesJSONOutput(cmd) {
				hasJSON[cmd.CommandPath()] = true
			}
		})
	}
	for p := range outputExempt {
		switch {
		case !exists[p]:
			t.Errorf("outputExempt names %q, which no longer exists; delete the entry", p)
		case hasJSON[p]:
			t.Errorf("outputExempt names %q, which has --output json now; delete the entry", p)
		}
	}
}
