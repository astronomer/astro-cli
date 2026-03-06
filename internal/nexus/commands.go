package nexus

import (
	"os"
	"strings"
	"sync"

	"github.com/astronomer/nexus/app"
	"github.com/astronomer/nexus/restish"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NexusCommandAnnotation is the cobra annotation key used to mark commands
// powered by nexus/restish. The pre-run hook checks this to skip cloud setup.
const NexusCommandAnnotation = "nexus.command"

type nexusCLI struct {
	ctx *app.Context
}

type commandEntry struct {
	flatName string // original restish name, e.g. "create-deployment"
	verb     string // e.g. "create"
	desc     string
}

// parseNounVerb splits a flat restish command name into (noun, verb).
// e.g. "create-deployment" -> ("deployment", "create"),
//
//	"list-deployments" -> ("deployment", "list").
func parseNounVerb(cmdName string) (noun, verb string, ok bool) {
	verb, noun, ok = strings.Cut(cmdName, "-")
	if !ok {
		return "", "", false
	}
	noun = strings.TrimSuffix(noun, "s")
	return noun, verb, true
}

// FindSubcommand returns the child of parent with the given Use name, or nil.
func FindSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// RegisterNounVerb registers restish API commands in noun-verb format directly
// on the root command (e.g. "astro deployment create" instead of
// "astro nexus create-deployment").
func RegisterNounVerb(rootCmd *cobra.Command, ctx *app.Context) {
	n := &nexusCLI{ctx: ctx}
	api := ctx.Config.GetDefaultAPI()
	if api == "" {
		return
	}

	if isCompletionMode() && !restish.IsHelpCached(api, "") {
		return
	}

	parser, err := restish.NewHelpParser(api, "")
	if err != nil {
		return
	}

	groups := make(map[string][]commandEntry)
	for _, cmd := range parser.ParseCommandsSection() {
		noun, verb, ok := parseNounVerb(cmd.Name)
		if !ok {
			continue
		}
		groups[noun] = append(groups[noun], commandEntry{
			flatName: cmd.Name,
			verb:     verb,
			desc:     cmd.Description,
		})
	}

	for noun, entries := range groups {
		nounCmd := FindSubcommand(rootCmd, noun)
		if nounCmd == nil {
			nounCmd = &cobra.Command{
				Use:   noun,
				Short: "Manage " + noun + "s",
			}
			rootCmd.AddCommand(nounCmd)
		}

		for _, e := range entries {
			flatName := e.flatName
			verbCmd := &cobra.Command{
				Use:                e.verb + " [args...]",
				Short:              e.desc,
				Annotations:        map[string]string{NexusCommandAnnotation: "true"},
				SilenceErrors:      true,
				SilenceUsage:       true,
				DisableFlagParsing: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					return n.runRestishCommand(flatName, args)
				},
			}

			verbCmd.SetHelpFunc(n.makeAPIHelpFunc(api, flatName))
			verbCmd.SetUsageFunc(n.makeAPIUsageFunc(api, flatName))
			verbCmd.ValidArgsFunction = n.makeLazyArgsCompleter(api, flatName)

			nounCmd.AddCommand(verbCmd)
		}
	}
}

func isCompletionMode() bool {
	return len(os.Args) >= 2 &&
		(os.Args[1] == "__complete" || os.Args[1] == "__completeNoDesc")
}

var (
	flagsLoaded   = make(map[string]bool)
	flagsLoadedMu sync.Mutex
)

func (n *nexusCLI) loadCommandFlags(api, cmdName string, cmd *cobra.Command) {
	flagsLoadedMu.Lock()
	key := api + ":" + cmdName
	if flagsLoaded[key] {
		flagsLoadedMu.Unlock()
		return
	}
	flagsLoaded[key] = true
	flagsLoadedMu.Unlock()

	if isCompletionMode() && !restish.IsHelpCached(api, cmdName) {
		return
	}

	registered := make(map[string]bool)
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		registered[f.Name] = true
	})

	schemaParser, err := restish.NewSchemaParser(api, cmdName)
	if err == nil {
		fields := schemaParser.GetFields(-1)
		for _, field := range fields {
			flagName := strcase.ToKebab(field.Name)
			if registered[flagName] {
				continue
			}
			registered[flagName] = true
			cmd.Flags().String(flagName, "", field.Description)

			if field.Enum != "" {
				enumValues := strings.Split(field.Enum, ",")
				for i := range enumValues {
					enumValues[i] = strings.TrimSpace(enumValues[i])
				}
				_ = cmd.Flags().SetAnnotation(flagName, "enum", enumValues)
			}
		}
	}

	helpParser, err := restish.NewHelpParser(api, cmdName)
	if err == nil {
		flags := helpParser.ParseFlagsSection()
		for _, flag := range flags {
			if registered[flag.Name] {
				continue
			}
			registered[flag.Name] = true
			switch flag.FlagType {
			case "boolean", "bool":
				cmd.Flags().Bool(flag.Name, false, flag.Description)
			default:
				cmd.Flags().String(flag.Name, "", flag.Description)
			}
		}
	}
}

func (n *nexusCLI) makeLazyArgsCompleter(api, cmdName string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		n.loadCommandFlags(api, cmdName, cmd)

		if strings.HasPrefix(toComplete, "-") {
			return n.completeFlagNames(cmd, toComplete)
		}

		if len(args) > 0 {
			prevArg := args[len(args)-1]
			if strings.HasPrefix(prevArg, "--") {
				return n.completeFlagValues(cmd, strings.TrimPrefix(prevArg, "--"))
			}
		}

		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func (n *nexusCLI) completeFlagNames(cmd *cobra.Command, prefix string) ([]string, cobra.ShellCompDirective) {
	search := strings.TrimLeft(prefix, "-")
	var completions []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.HasPrefix(f.Name, search) {
			if f.Usage != "" {
				completions = append(completions, "--"+f.Name+"\t"+f.Usage)
			} else {
				completions = append(completions, "--"+f.Name)
			}
		}
	})
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func (n *nexusCLI) completeFlagValues(cmd *cobra.Command, flagName string) ([]string, cobra.ShellCompDirective) {
	f := cmd.Flags().Lookup(flagName)
	if f == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if enums, ok := f.Annotations["enum"]; ok && len(enums) > 0 {
		return enums, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}

func (n *nexusCLI) makeAPIHelpFunc(api, cmdName string) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		n.showCommandHelp(api, cmdName)
	}
}

func (n *nexusCLI) makeAPIUsageFunc(api, cmdName string) func(*cobra.Command) error {
	return func(cmd *cobra.Command) error {
		n.showCommandHelp(api, cmdName)
		return nil
	}
}
