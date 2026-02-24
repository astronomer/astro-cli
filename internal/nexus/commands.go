package nexus

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/astronomer/nexus/app"
	"github.com/astronomer/nexus/config"
	"github.com/astronomer/nexus/restish"
	"github.com/astronomer/nexus/shared"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type nexusCLI struct {
	ctx          *app.Context
	nexusCmd     *cobra.Command
	outputFormat string
}

// RegisterCommands adds a "nexus" subcommand to the root command.
func RegisterCommands(rootCmd *cobra.Command, ctx *app.Context) {
	n := &nexusCLI{ctx: ctx}

	nexusCmd := &cobra.Command{
		Use:   "nexus",
		Short: "Interact with Astronomer APIs via restish",
		Long:  n.buildLongDescription(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			n.maybeLoadFlagsForCompletion(cmd)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	n.nexusCmd = nexusCmd

	nexusCmd.PersistentFlags().StringVarP(&n.outputFormat, "output", "o", "text", "Output format: text or json")

	nexusCmd.AddGroup(
		&cobra.Group{ID: "api", Title: "API Commands:"},
		&cobra.Group{ID: "commands", Title: "Commands:"},
	)

	configCmd := n.configCmd()
	configCmd.GroupID = "commands"
	nexusCmd.AddCommand(configCmd)

	n.addRestishCommands(nexusCmd)

	rootCmd.AddCommand(nexusCmd)
}

func (n *nexusCLI) buildLongDescription() string {
	var sb strings.Builder

	sb.WriteString("Interact with Astronomer APIs via restish.\n\n")

	apis := n.ctx.APIs.List()
	maxAPIWidth := 0
	for _, meta := range apis {
		if len(meta.Name) > maxAPIWidth {
			maxAPIWidth = len(meta.Name)
		}
	}
	sb.WriteString("Available APIs:\n")
	for _, meta := range apis {
		paddedName := fmt.Sprintf("%-*s", maxAPIWidth, meta.Name)
		sb.WriteString(fmt.Sprintf("  %s  %s\n", paddedName, meta.Description))
	}

	api := n.ctx.Config.GetDefaultAPI()
	if api != "" {
		sb.WriteString(fmt.Sprintf("\nCurrent API: %s", api))
	} else {
		sb.WriteString("\nNo API selected. Use 'astro nexus config api set <name>' to select one.")
	}

	return sb.String()
}

func isCompletionMode() bool {
	return len(os.Args) >= 2 &&
		(os.Args[1] == "__complete" || os.Args[1] == "__completeNoDesc")
}

func (n *nexusCLI) addRestishCommands(parentCmd *cobra.Command) {
	api := n.ctx.Config.GetDefaultAPI()
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

	commands := parser.ParseCommandsSection()
	for _, cmd := range commands {
		cmdName := cmd.Name
		cmdDesc := cmd.Description
		apiName := api

		apiCmd := &cobra.Command{
			Use:                cmdName + " [args...]",
			Short:              cmdDesc,
			GroupID:            "api",
			SilenceErrors:      true,
			SilenceUsage:       true,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return n.runRestishCommand(cmdName, args)
			},
		}

		apiCmd.SetHelpFunc(n.makeAPIHelpFunc(apiName, cmdName))
		apiCmd.SetUsageFunc(n.makeAPIUsageFunc(apiName, cmdName))
		apiCmd.ValidArgsFunction = n.makeLazyArgsCompleter(apiName, cmdName)

		parentCmd.AddCommand(apiCmd)
	}
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

const minCompletionArgs = 4 // e.g. "astro __complete nexus <cmd>"

func (n *nexusCLI) maybeLoadFlagsForCompletion(_ *cobra.Command) {
	if len(os.Args) < 2 {
		return
	}
	if os.Args[1] != "__complete" && os.Args[1] != "__completeNoDesc" {
		return
	}

	if len(os.Args) < minCompletionArgs {
		return
	}
	targetCmdName := os.Args[3]

	api := n.ctx.Config.GetDefaultAPI()
	if api == "" {
		return
	}

	for _, subcmd := range n.nexusCmd.Commands() {
		if subcmd.Name() == targetCmdName && subcmd.GroupID == "api" {
			n.loadCommandFlags(api, targetCmdName, subcmd)
			return
		}
	}
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

func (n *nexusCLI) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage nexus configuration",
		Long:  `Show and manage nexus configuration. Without arguments, displays current settings.`,
		Run: func(cmd *cobra.Command, args []string) {
			n.showConfig()
		},
	}

	cmd.AddCommand(n.configShowCmd())
	cmd.AddCommand(n.configAPICmd())
	cmd.AddCommand(n.configContextCmd())
	cmd.AddCommand(n.configVariableCmd())

	return cmd
}

func (n *nexusCLI) configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			n.showConfig()
		},
	}
}

func (n *nexusCLI) configAPICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Manage default API profile",
		Long: `Manage the default API profile.

Without arguments, shows the current default API.`,
		Run: func(cmd *cobra.Command, args []string) {
			api := n.ctx.Config.GetDefaultAPI()
			if api != "" {
				fmt.Printf("api: %s\n", api)
			} else {
				fmt.Println("api: (not set)")
			}
		},
	}

	cmd.AddCommand(n.configAPISetCmd())
	cmd.AddCommand(n.configAPIUnsetCmd())

	return cmd
}

func (n *nexusCLI) configAPISetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <api-name>",
		Short: "Set the default API profile",
		Long: `Set the default API profile.

Examples:
  astro nexus config api set astro-dev
  astro nexus config api set astro-prod`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			value := args[0]
			if !n.ctx.APIs.IsKnownAPI(value) {
				fmt.Fprintf(os.Stderr, "Error: Unknown API '%s'. Run 'astro nexus config show' to see available APIs.\n", value)
				return
			}
			if err := n.ctx.Config.SetDefaultAPI(value); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}
			fmt.Printf("Set api = %s\n", value)
		},
	}
}

func (n *nexusCLI) configAPIUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset",
		Short: "Clear the default API profile",
		Run: func(cmd *cobra.Command, args []string) {
			if err := n.ctx.Config.SetDefaultAPI(""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}
			fmt.Println("Cleared api")
		},
	}
}

func (n *nexusCLI) configContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage default context (org, workspace, deployment)",
		Long: `Manage default context values for the current API.

Context values are API-specific defaults for organization, workspace, and deployment.
You must set a default API first with 'astro nexus config api set <api>'.

Without arguments, shows the current context.`,
		Run: func(cmd *cobra.Command, args []string) {
			n.showContext()
		},
	}

	cmd.AddCommand(n.configContextSetCmd())
	cmd.AddCommand(n.configContextUnsetCmd())

	return cmd
}

func (n *nexusCLI) configContextSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <type> <value>",
		Short: "Set a context value",
		Long: `Set a context value for the current API.

Types:
  org          Set the default organization ID
  workspace    Set the default workspace ID
  deployment   Set the default deployment ID

Examples:
  astro nexus config context set org clxxxxxxxx
  astro nexus config context set workspace clxxxxxxxx`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			n.setContext(args[0], args[1])
		},
	}
}

func (n *nexusCLI) configContextUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <type>",
		Short: "Clear a context value",
		Long: `Clear a context value for the current API.

Types:
  org          Clear the default organization ID
  workspace    Clear the default workspace ID
  deployment   Clear the default deployment ID

Examples:
  astro nexus config context unset org`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			n.clearContext(args[0])
		},
	}
}

func (n *nexusCLI) showConfig() {
	api := n.ctx.Config.GetDefaultAPI()
	cfg := n.ctx.Config.Config()

	fmt.Println("Current configuration:")
	fmt.Printf("  file: %s\n", config.GetLocalConfigPath())
	fmt.Println()

	if api != "" {
		fmt.Printf("  api: %s\n", api)
		fmt.Println()
		fmt.Printf("  Context for %s:\n", api)

		org := n.ctx.Config.GetDefaultOrganizationForAPI(api)
		ws := n.ctx.Config.GetDefaultWorkspaceForAPI(api)
		dep := n.ctx.Config.GetDefaultDeploymentForAPI(api)

		if org != "" {
			fmt.Printf("    organization: %s\n", org)
		} else {
			fmt.Println("    organization: (not set)")
		}

		if ws != "" {
			fmt.Printf("    workspace:    %s\n", ws)
		} else {
			fmt.Println("    workspace:    (not set)")
		}

		if dep != "" {
			fmt.Printf("    deployment:   %s\n", dep)
		} else {
			fmt.Println("    deployment:   (not set)")
		}
	} else {
		fmt.Println("  api: (not set)")
		fmt.Println()
		fmt.Println("  Set an API first with 'astro nexus config api set <api>'")
	}

	if cfg != nil && len(cfg.Variables) > 0 {
		fmt.Println()
		fmt.Println("  Variables:")
		for key, value := range cfg.Variables {
			fmt.Printf("    %s: %s\n", key, value)
		}
	}
}

func (n *nexusCLI) showContext() {
	api := n.ctx.Config.GetDefaultAPI()
	if api == "" {
		fmt.Fprintln(os.Stderr, "Error: No API set. Use 'astro nexus config api set <api>' first.")
		return
	}

	fmt.Printf("Context for %s:\n", api)

	org := n.ctx.Config.GetDefaultOrganizationForAPI(api)
	ws := n.ctx.Config.GetDefaultWorkspaceForAPI(api)
	dep := n.ctx.Config.GetDefaultDeploymentForAPI(api)

	if org != "" {
		fmt.Printf("  organization: %s\n", org)
	} else {
		fmt.Println("  organization: (not set)")
	}

	if ws != "" {
		fmt.Printf("  workspace:    %s\n", ws)
	} else {
		fmt.Println("  workspace:    (not set)")
	}

	if dep != "" {
		fmt.Printf("  deployment:   %s\n", dep)
	} else {
		fmt.Println("  deployment:   (not set)")
	}
}

func (n *nexusCLI) setContext(typ, value string) {
	api := n.ctx.Config.GetDefaultAPI()
	if api == "" {
		fmt.Fprintln(os.Stderr, "Error: No API set. Use 'astro nexus config api set <api>' first.")
		return
	}

	var err error
	switch shared.NormalizeResourceType(typ) {
	case shared.ResourceOrganization:
		err = n.ctx.Config.SetDefaultOrganizationForAPI(api, value)
	case shared.ResourceWorkspace:
		err = n.ctx.Config.SetDefaultWorkspaceForAPI(api, value)
	case shared.ResourceDeployment:
		err = n.ctx.Config.SetDefaultDeploymentForAPI(api, value)
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown type '%s'. Valid types: org, workspace, deployment\n", typ)
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	fmt.Printf("Set %s = %s\n", typ, value)
}

func (n *nexusCLI) clearContext(typ string) {
	api := n.ctx.Config.GetDefaultAPI()
	if api == "" {
		fmt.Fprintln(os.Stderr, "Error: No API set. Use 'astro nexus config api set <api>' first.")
		return
	}

	var err error
	switch shared.NormalizeResourceType(typ) {
	case shared.ResourceOrganization:
		err = n.ctx.Config.SetDefaultOrganizationForAPI(api, "")
	case shared.ResourceWorkspace:
		err = n.ctx.Config.SetDefaultWorkspaceForAPI(api, "")
	case shared.ResourceDeployment:
		err = n.ctx.Config.SetDefaultDeploymentForAPI(api, "")
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown type '%s'. Valid types: org, workspace, deployment\n", typ)
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	fmt.Printf("Cleared %s\n", typ)
}

func (n *nexusCLI) configVariableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable",
		Short: "Manage configuration variables",
		Long: `Manage configuration variables for nexus.

Variables are used to configure paths and other settings needed by certain APIs.
For example, the 'astro' variable is needed for the Admin API spec path.

Without arguments, lists all configured variables.`,
		Run: func(cmd *cobra.Command, args []string) {
			n.listVariables()
		},
	}

	cmd.AddCommand(n.configVariableSetCmd())
	cmd.AddCommand(n.configVariableUnsetCmd())
	cmd.AddCommand(n.configVariableListCmd())

	return cmd
}

func (n *nexusCLI) configVariableSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <value>",
		Short: "Set a configuration variable",
		Long: `Set a configuration variable value.

Variables:
  astro    Path to your local Astro monorepo (required for Admin API)

Examples:
  astro nexus config variable set astro /path/to/astro`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			n.setVariable(args[0], args[1])
		},
	}
}

func (n *nexusCLI) configVariableUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <name>",
		Short: "Clear a configuration variable",
		Long: `Clear a configuration variable.

Examples:
  astro nexus config variable unset astro`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			n.unsetVariable(args[0])
		},
	}
}

func (n *nexusCLI) configVariableListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration variables",
		Run: func(cmd *cobra.Command, args []string) {
			n.listVariables()
		},
	}
}

func (n *nexusCLI) setVariable(name, value string) {
	err := n.ctx.Config.SetVariables(map[string]string{name: value})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Printf("Set %s = %s\n", name, value)
}

func (n *nexusCLI) unsetVariable(name string) {
	cfg := n.ctx.Config.Config()
	if cfg == nil || cfg.Variables == nil || cfg.Variables[name] == "" {
		fmt.Fprintf(os.Stderr, "Error: Variable '%s' is not set.\n", name)
		return
	}

	err := n.ctx.Config.SetVariables(map[string]string{name: ""})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Printf("Cleared %s\n", name)
}

func (n *nexusCLI) listVariables() {
	cfg := n.ctx.Config.Config()

	fmt.Println("Configuration variables:")

	if cfg == nil || len(cfg.Variables) == 0 {
		fmt.Println("  No variables configured.")
		fmt.Println()
		fmt.Println("  Use 'astro nexus config variable set <name> <value>' to set a variable.")
		fmt.Println("  Example: astro nexus config variable set astro /path/to/astro")
		return
	}

	for name, value := range cfg.Variables {
		if value != "" {
			fmt.Printf("  %s: %s\n", name, value)
		}
	}
}
