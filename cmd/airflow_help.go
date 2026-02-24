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
const groupedFlagsUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}{{if hasGroupFlags .LocalFlags ""}}

Flags:
{{filterFlags .LocalFlags "" | trimTrailingWhitespaces}}{{end}}{{if hasGroupFlags .LocalFlags "docker"}}

Docker Mode Flags:
{{filterFlags .LocalFlags "docker" | trimTrailingWhitespaces}}{{end}}{{if hasGroupFlags .LocalFlags "standalone"}}

Standalone Mode Flags:
{{filterFlags .LocalFlags "standalone" | trimTrailingWhitespaces}}{{end}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// annotateFlag sets a group annotation on a flag for grouped help display.
func annotateFlag(cmd *cobra.Command, name, group string) {
	cmd.Flags().SetAnnotation(name, "group", []string{group}) //nolint:errcheck
}
