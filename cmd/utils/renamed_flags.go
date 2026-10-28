package utils

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/internal/telemetry"
	sharedtel "github.com/astronomer/astro-cli/pkg/telemetry"
)

// RenameHintsDisabledEnv hides the hint noteRenamedFlags prints, for a
// script that still has to run on Astro CLI 1.x too, where the new spelling
// does not exist yet. It hides only the hint: the event still goes, unless
// telemetry is off.
const RenameHintsDisabledEnv = "ASTRO_RENAME_HINTS_DISABLED"

// noteRenamedFlags runs once ApplyPreferredFlags has accepted a run's flags.
// For the 1.x spellings it gave, which v2 keeps as hidden aliases (AddAlias),
// it says on stderr what each flag is called now, so that a script gets
// updated before the alias goes, and records them (RecordRenamedFlags), which
// is what tells us when an alias can go. Stdout is left alone, so -o json
// output stays one document.
//
// An alias given as "" is recorded but gets no hint: it still means the
// default, as in 1.x, and the new spelling refuses "" (RefuseEmpty), so the
// hint would break the script it means to fix.
func noteRenamedFlags(cmd *cobra.Command) {
	fs := cmd.Flags()
	var renamed, current, order []string
	hinted := map[string][]*pflag.Flag{}
	fs.Visit(func(f *pflag.Flag) {
		p := aliasOf(f)
		if p == "" {
			return
		}
		renamed, current = append(renamed, "--"+f.Name), append(current, "--"+p)
		if f.Value.String() == "" {
			return
		}
		if _, ok := hinted[p]; !ok {
			order = append(order, p)
		}
		hinted[p] = append(hinted[p], f)
	})
	if len(renamed) == 0 {
		return
	}
	if !sharedtel.EnvIsTrue(RenameHintsDisabledEnv) {
		for _, p := range order {
			fmt.Fprintln(cmd.ErrOrStderr(), renameHint(fs.Lookup(p), hinted[p]))
		}
	}
	RecordRenamedFlags(cmd, renamed, current)
}

// RecordRenamedFlags queues the event; a test swaps it to see what was sent.
var RecordRenamedFlags = telemetry.RecordRenamedFlags

// renameHint is the hint for the aliases of preferred a run gave, each named
// with its shorthand, which is what a script may spell it as.
func renameHint(preferred *pflag.Flag, aliases []*pflag.Flag) string {
	names := make([]string, len(aliases))
	for i, a := range aliases {
		names[i] = spelling(a)
	}
	var b strings.Builder
	if len(aliases) == 1 {
		fmt.Fprintf(&b, "Flag %s has been renamed to %s.", names[0], spelling(preferred))
	} else {
		fmt.Fprintf(&b, "Flags %s are replaced by %s, which takes one value.", strings.Join(names, " and "), spelling(preferred))
	}
	// 1.x's --deployment-id and --deployment-name keep the kind they were
	// given as; --deployment works it out, so the same value can name another
	// Deployment.
	if preferred.Name == "deployment" {
		b.WriteString(" --deployment also takes a link name from the project, and reads a value shaped like a Deployment id as an id, so check that the value names the same Deployment before switching.")
	}
	fmt.Fprintf(&b, " The old spelling will be removed in a future release. Set %s=1 to hide this message.", RenameHintsDisabledEnv)
	return b.String()
}

func spelling(f *pflag.Flag) string {
	if f.Shorthand == "" {
		return "--" + f.Name
	}
	return "--" + f.Name + " (-" + f.Shorthand + ")"
}
