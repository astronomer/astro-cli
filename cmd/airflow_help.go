package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	cobra.AddTemplateFunc("filterFlags", filterFlagsByGroup)
	cobra.AddTemplateFunc("hasGroupFlags", hasGroupFlags)
}

// filterFlagsByGroup returns the FlagUsages string for flags matching the given
// group annotation. Flags with no "group" annotation belong to the "" (common) group.
func filterFlagsByGroup(flags *pflag.FlagSet, group string) string {
	filtered := pflag.NewFlagSet("filtered", pflag.ContinueOnError)
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flagGroup := ""
		if annotations, ok := f.Annotations["group"]; ok && len(annotations) > 0 {
			flagGroup = annotations[0]
		}
		if flagGroup == group {
			filtered.AddFlag(f)
		}
	})
	return filtered.FlagUsages()
}

// hasGroupFlags returns true if there are any visible flags in the given group.
func hasGroupFlags(flags *pflag.FlagSet, group string) bool {
	found := false
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || found {
			return
		}
		flagGroup := ""
		if annotations, ok := f.Annotations["group"]; ok && len(annotations) > 0 {
			flagGroup = annotations[0]
		}
		if flagGroup == group {
			found = true
		}
	})
	return found
}

// groupedFlagsUsageTemplate is a usage template that splits local flags into
// Common / Docker Mode / Standalone Mode sections based on flag annotations.
