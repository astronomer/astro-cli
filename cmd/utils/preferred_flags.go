package utils

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// aliasAnnotation names, on an alias flag, the flag it is another spelling of.
const aliasAnnotation = "astro.alias-of"

// nonEmptyAnnotation marks a flag ApplyAliases refuses to take as "".
const nonEmptyAnnotation = "astro.non-empty"

// RefuseEmpty marks f, a flag that names something (a Deployment, a
// workspace), so that ApplyAliases refuses it given as "": an empty name is a
// mistake, such as an unset variable in a script, and taking it as no name
// would quietly fall back to a default the run did not ask for.
func RefuseEmpty(f *pflag.Flag) {
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[nonEmptyAnnotation] = nil
}

func refusesEmpty(f *pflag.Flag) bool {
	_, ok := f.Annotations[nonEmptyAnnotation]
	return ok
}

// AddAlias registers --old (and the shorthand 1.x gave it) on fs, hidden, as
// another spelling of --preferred, which the caller has already put on fs.
// The alias holds nothing a command reads: ApplyAliases hands what it was
// given to --preferred, and the command reads --preferred alone, whichever
// spelling was typed. It returns the alias.
func AddAlias(fs *pflag.FlagSet, old, shorthand, preferred string) *pflag.Flag {
	fs.StringP(old, shorthand, "", "Another spelling of --"+preferred+", kept for scripts written for Astro CLI 1.x")
	f := fs.Lookup(old)
	f.Hidden = true
	f.Annotations = map[string][]string{aliasAnnotation: {preferred}}
	return f
}

// IsAlias reports whether f is a spelling AddAlias registered.
func IsAlias(f *pflag.Flag) bool {
	return aliasOf(f) != ""
}

func aliasOf(f *pflag.Flag) string {
	if p := f.Annotations[aliasAnnotation]; len(p) == 1 {
		return p[0]
	}
	return ""
}

// ApplyAliases hands each alias given on fs to the flag it stands for. Given
// both spellings, they must agree, or it is a usage error. Given only the
// alias, the preferred flag takes its value, so a required check on the
// preferred spelling is met by the alias too. Of two aliases of one flag
// given without it (1.x's --deployment-id and --deployment-name together),
// the first by name is handed on and they need not agree, as 1.x read both.
// A flag marked with RefuseEmpty given as "" is a usage error. An alias given
// as "" is not: 1.x took an empty --workspace-id as no workspace, and scripts
// pass one from a variable that may be unset, so it still means the default.
func ApplyAliases(fs *pflag.FlagSet) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err == nil && f.Changed && f.Value.String() == "" && refusesEmpty(f) {
			err = cliout.Usage(fmt.Errorf("--%s cannot be empty", f.Name))
		}
	})
	if err != nil {
		return err
	}
	typed := map[string]bool{}
	fs.VisitAll(func(f *pflag.Flag) {
		if p := aliasOf(f); p != "" && fs.Changed(p) {
			typed[p] = true
		}
	})
	fs.VisitAll(func(f *pflag.Flag) {
		p := aliasOf(f)
		if p == "" || !f.Changed || err != nil {
			return
		}
		pf := fs.Lookup(p)
		switch {
		case typed[p]:
			if v := pf.Value.String(); v != f.Value.String() {
				err = cliout.Usage(fmt.Errorf("--%s %q and --%s %q disagree: pass only --%s", p, v, f.Name, f.Value.String(), p))
			}
		case !pf.Changed:
			err = fs.Set(p, f.Value.String())
		}
	})
	return err
}

// BeforeArgs runs hook on every runnable command in tree once its flags are
// parsed, ahead of its argument check and of every pre-run: the one point
// cobra offers before a parent's persistent pre-run reads the flags. It wraps
// the command's Args, keeping the check it had.
func BeforeArgs(tree *cobra.Command, hook func(*cobra.Command, []string) error) {
	if tree.Runnable() {
		check := tree.Args
		tree.Args = func(cmd *cobra.Command, args []string) error {
			if err := hook(cmd, args); err != nil {
				return err
			}
			if check == nil {
				return cobra.ArbitraryArgs(cmd, args)
			}
			return check(cmd, args)
		}
	}
	for _, sub := range tree.Commands() {
		BeforeArgs(sub, hook)
	}
}

// Resolver is a tree's own work on what a command's flags hold once the 1.x
// spellings are handed on, such as reading a link name given as
// --deployment. typed reports whether this run typed the flag of that name
// itself, as distinct from a value an alias handed it.
type Resolver func(cmd *cobra.Command, args []string, typed func(name string) bool) error

// ApplyPreferredFlagsIn has every runnable command in tree reconcile its
// spellings once its flags are parsed (ApplyPreferredFlags), before its
// argument check and its pre-runs, which read what they resolve to. Each
// tree's top-level constructors call it on their own tree, with the tree's
// own resolution.
func ApplyPreferredFlagsIn(tree *cobra.Command, resolve Resolver) {
	BeforeArgs(tree, func(cmd *cobra.Command, args []string) error {
		return ApplyPreferredFlags(cmd, args, resolve)
	})
}

// ApplyPreferredFlags hands the 1.x spellings given to cmd to the flags they
// stand for (ApplyAliases), then runs resolve, nil for none.
func ApplyPreferredFlags(cmd *cobra.Command, args []string, resolve Resolver) error {
	fs := cmd.Flags()
	typed := map[string]bool{}
	fs.Visit(func(f *pflag.Flag) { typed[f.Name] = true })
	if err := ApplyAliases(fs); err != nil {
		return err
	}
	if resolve == nil {
		return nil
	}
	return resolve(cmd, args, func(name string) bool { return typed[name] })
}

// RefuseDisagreeingDeployment refuses a command line that names its
// Deployment twice, as the argument (args[0]) and as --deployment (flag, as
// typed), naming two different ones: it would act on one of them without a
// word. same are other values the argument agrees with, such as the
// Deployment id flag resolved to.
func RefuseDisagreeingDeployment(args []string, flag string, same ...string) error {
	if len(args) == 0 || flag == "" || args[0] == flag || slices.Contains(same, args[0]) {
		return nil
	}
	return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], flag))
}

// AddWorkspaceFlag registers --workspace on fs, read into to, with the
// shorthand given ("" for none), refusing "" (RefuseEmpty). With oneX, 1.x's
// --workspace-id is its hidden alias, with the shorthand 1.x gave it there
// (oneXShorthand, "" for none). It returns --workspace.
func AddWorkspaceFlag(fs *pflag.FlagSet, to *string, shorthand, usage string, oneX bool, oneXShorthand string) *pflag.Flag {
	fs.StringVarP(to, "workspace", shorthand, "", usage)
	f := fs.Lookup("workspace")
	RefuseEmpty(f)
	if oneX {
		AddAlias(fs, "workspace-id", oneXShorthand, "workspace")
	}
	return f
}

// OneXNoOpUsage is the help of a 1.x flag a command accepts and ignores.
const OneXNoOpUsage = "Has no effect; accepted so scripts written for Astro CLI 1.x keep working"

// AddOneXNoOp registers 1.x's --name on fs, a flag that took a value, with
// the shorthand 1.x gave it ("" for none), for a command that 1.x gave it to
// and that never read it: hidden and read by nothing, so a 1.x script
// passing it keeps working.
func AddOneXNoOp(fs *pflag.FlagSet, name, shorthand string) {
	fs.StringP(name, shorthand, "", OneXNoOpUsage)
	fs.MarkHidden(name) //nolint:errcheck // the flag is defined just above
}
