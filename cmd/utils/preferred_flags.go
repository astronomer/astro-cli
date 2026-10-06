package utils

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// replacesAnnotation lists, on a preferred flag, the older spellings it
// stands for.
const replacesAnnotation = "astro.replaces"

// AddPreferredFlag registers --name as the spelling help shows for olds,
// flags already on fs that keep working exactly as before but are hidden.
// ApplyPreferredFlags moves a value given as --name onto one of them, so the
// command goes on reading the old flags and never needs to know which
// spelling was typed.
func AddPreferredFlag(fs *pflag.FlagSet, name, shorthand, usage string, olds ...string) {
	fs.StringP(name, shorthand, "", usage)
	MarkPreferredFlag(fs, name, olds...)
}

// MarkPreferredFlag makes --name, already on fs, the spelling help shows for
// olds, as AddPreferredFlag does for a flag it registers itself.
func MarkPreferredFlag(fs *pflag.FlagSet, name string, olds ...string) {
	f := fs.Lookup(name)
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[replacesAnnotation] = olds
	for _, old := range olds {
		_ = fs.MarkHidden(old) //nolint:errcheck // the caller registered old on fs; this only errors on an unknown flag name
	}
}

// Route names the older spelling a preferred flag's value goes to. ok false
// leaves the choice to ApplyPreferredFlags, which picks the first of the old
// spellings the command has; a target of "" with ok true sends it to none,
// for a route that delivered the value itself.
type Route func(fs *pflag.FlagSet, preferred, value string) (target string, ok bool)

// ApplyPreferredFlags reconciles each preferred flag on fs with the older
// spellings it stands for. Given both, they must agree, or it is a usage
// error. Given the preferred one, its value is set on the old flag route
// picks. Given only an old one, the preferred flag is set too, so a required
// check on the preferred spelling is met by the old.
func ApplyPreferredFlags(fs *pflag.FlagSet, route Route) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		olds, ok := f.Annotations[replacesAnnotation]
		if ok && err == nil {
			err = applyPreferredFlag(fs, f, olds, route)
		}
	})
	return err
}

func applyPreferredFlag(fs *pflag.FlagSet, f *pflag.Flag, olds []string, route Route) error {
	var given, present []*pflag.Flag
	for _, name := range olds {
		if o := fs.Lookup(name); o != nil {
			present = append(present, o)
			if o.Changed {
				given = append(given, o)
			}
		}
	}
	if !f.Changed {
		if len(given) == 0 {
			return nil
		}
		return fs.Set(f.Name, given[0].Value.String())
	}
	v := f.Value.String()
	for _, o := range given {
		if o.Value.String() != v {
			return cliout.Usage(fmt.Errorf("--%s %q and --%s %q disagree: pass only --%s", f.Name, v, o.Name, o.Value.String(), f.Name))
		}
	}
	target, ok := "", false
	if route != nil {
		target, ok = route(fs, f.Name, v)
	}
	if !ok && len(present) > 0 {
		target = present[0].Name
	}
	if target == "" {
		return nil
	}
	return fs.Set(target, v)
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
