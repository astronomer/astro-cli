package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

const (
	errForceFlagRemoved    = "--force was removed in Astro CLI v2: use --yes (-y)"
	errTemplateFlagRemoved = "--template was removed in Astro CLI v2: use -o json, and a JSON tool such as jq to pick fields"
	errFormatFlagRemoved   = "--format was removed in Astro CLI v2: use -o (--output)"
	errAPIURLFlagRemoved   = "--api-url was removed in Astro CLI v2: use --url"
	errDeploymentIDAPIFlag = "--deployment-id was removed in Astro CLI v2: use --deployment (-d), which takes a Deployment id or a link name from pyproject.toml"
)

// removedV1Flag is a 1.x flag that a v2 command dropped, without a hidden
// alias and without a tombstone of its own next to the command.
type removedV1Flag struct {
	path      string // the command, without the leading "astro"
	name      string
	shorthand string
	isBool    bool
	msg       string
}

// removedV1Flags lists them in one place so a 1.x script that passes one is
// told what replaced it instead of cobra's bare "unknown flag". The 1.x `list`
// commands shared --json, --template and -o table|json|template (pkg/output's
// AddFlags); --force became --yes everywhere a command asks first; the env
// commands' --format became -o. A path the current tree does not mount (the
// cloud tree for an APC context, or the other way around) is skipped, and
// TestRemovedV1FlagsResolve keeps every entry pointing at a real command.
// Delete them in v3.
var removedV1Flags = func() []removedV1Flag {
	var flags []removedV1Flag
	for _, path := range []string{
		"context delete",
		"deployment create",
		"deployment delete",
		"deployment update",
		"deployment hibernate",
		"deployment wake-up",
		"deployment token delete",
		"deployment token rotate",
		"deployment worker-queue delete",
		"deployment worker-queue update",
		"organization team delete",
		"organization team update",
		"organization team user add",
		"organization team user remove",
		"organization token delete",
		"organization token rotate",
		"workspace token delete",
		"workspace token rotate",
	} {
		flags = append(flags, removedV1Flag{path: path, name: "force", shorthand: "f", isBool: true, msg: errForceFlagRemoved})
	}
	lists := []string{
		"deployment bundle list",
		"deployment team list",
		"deployment user list",
		"organization list",
		"organization team list",
		"organization user list",
		"workspace list",
		"workspace team list",
		"workspace user list",
	}
	for _, path := range lists {
		flags = append(flags, removedV1Flag{path: path, name: "json", isBool: true, msg: cliout.ErrJSONFlagRemoved})
	}
	// `deployment list` already tombstones --json itself
	for _, path := range append([]string{"deployment list"}, lists...) {
		flags = append(flags, removedV1Flag{path: path, name: "template", msg: errTemplateFlagRemoved})
	}
	for _, path := range []string{
		"env airflow-variable get",
		"env airflow-variable list",
		"env connection get",
		"env connection list",
		"env metrics-export get",
		"env metrics-export list",
		"env variable get",
		"env variable list",
		"env variable link list",
	} {
		flags = append(flags, removedV1Flag{path: path, name: "format", msg: errFormatFlagRemoved})
	}
	return append(flags,
		removedV1Flag{path: "api airflow", name: "api-url", msg: errAPIURLFlagRemoved},
		removedV1Flag{path: "api airflow", name: "deployment-id", msg: errDeploymentIDAPIFlag},
	)
}()

// tombstoneRemovedV1Flags registers removedV1Flags on the commands of root's
// tree that have them.
func tombstoneRemovedV1Flags(root *cobra.Command) {
	for _, f := range removedV1Flags {
		c := findCommand(root, f.path)
		if c == nil || c.Flags().Lookup(f.name) != nil {
			continue
		}
		shorthand := f.shorthand
		if shorthand != "" && (c.Flags().ShorthandLookup(shorthand) != nil || c.InheritedFlags().ShorthandLookup(shorthand) != nil) {
			shorthand = ""
		}
		cliout.AddRemovedFlag(c, f.name, shorthand, f.isBool, f.msg)
	}
}

// findCommand returns the command at path under root, or nil when root's
// tree has none there.
func findCommand(root *cobra.Command, path string) *cobra.Command {
	c := root
	for _, name := range strings.Fields(path) {
		var next *cobra.Command
		for _, sub := range c.Commands() {
			if sub.Name() == name {
				next = sub
				break
			}
		}
		if next == nil {
			return nil
		}
		c = next
	}
	return c
}
