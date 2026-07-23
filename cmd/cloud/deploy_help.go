package cloud

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	cobra.AddTemplateFunc("filterDeployFlags", filterDeployFlagsByGroup)
	cobra.AddTemplateFunc("hasDeployFlags", hasDeployGroupFlags)
}

// filterDeployFlagsByGroup returns the FlagUsages string for flags matching the
// given group annotation. Flags with no "group" annotation belong to the ""
// (common) group.
func filterDeployFlagsByGroup(flags *pflag.FlagSet, group string) string {
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

// hasDeployGroupFlags returns true if there are any visible flags in the given group.
func hasDeployGroupFlags(flags *pflag.FlagSet, group string) bool {
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

// deployFlagsUsageTemplate is a usage template that splits the deploy command's
// flags into sections by deploy type so it is clear which flags apply to which
// kind of deploy.
const deployFlagsUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableLocalFlags}}{{if hasDeployFlags .LocalFlags ""}}

Flags:
{{filterDeployFlags .LocalFlags "" | trimTrailingWhitespaces}}{{end}}{{if hasDeployFlags .LocalFlags "image"}}

Image Flags:
{{filterDeployFlags .LocalFlags "image" | trimTrailingWhitespaces}}{{end}}{{if hasDeployFlags .LocalFlags "dag"}}

DAG Flags:
{{filterDeployFlags .LocalFlags "dag" | trimTrailingWhitespaces}}{{end}}{{if hasDeployFlags .LocalFlags "test"}}

Test Flags:
{{filterDeployFlags .LocalFlags "test" | trimTrailingWhitespaces}}{{end}}{{if hasDeployFlags .LocalFlags "non-dags"}}

Non-DAG Bundle Flags:
{{filterDeployFlags .LocalFlags "non-dags" | trimTrailingWhitespaces}}{{end}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`

// annotateDeployFlag sets a group annotation on a flag for grouped help display.
func annotateDeployFlag(cmd *cobra.Command, name, group string) {
	cmd.Flags().SetAnnotation(name, "group", []string{group}) //nolint:errcheck
}
