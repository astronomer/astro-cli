package cmd

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	configSetSuccessMsg           = "Setting %s to %s successfully\n"
	configUseOutsideProjectDirMsg = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s\n"
	configUseInV2ProjectMsg       = "This project keeps its settings in pyproject.toml under [tool.astro], so it has no per-project CLI settings such as %s. CLI settings are global: one value for every project on this machine.\nTo %s the global value, run:\n  %s\n"
)

// unlistedConfigs are settings `astro config list` leaves out: contexts is a
// map that `astro context` manages, and the other two are credentials.
var unlistedConfigs = map[string]bool{
	config.CFG.Contexts.Path:         true,
	config.CFG.CloudAPIToken.Path:    true,
	config.CFG.PostgresPassword.Path: true,
}

var (
	globalFlag       bool
	configGetExample = `
		# Get your current project's name
		$ astro config get project.name
		`
	configSetExample = `
		# Set your current project's postgres user
		$ astro config set postgres.user postgres
		`
)

func newConfigRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "Manage CLI settings for this machine",
		Long:              "Manage CLI settings, stored globally with -g or in a 1.x project's .astro/config.yaml. Run `astro config list` to see every setting, or see https://www.astronomer.io/docs/astro/cli/configure-cli#available-cli-configurations for what each one does",
		PersistentPreRunE: ensureGlobalFlag,
	}
	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "view or modify global config")
	cmd.AddCommand(
		newConfigGetCmd(out),
		newConfigSetCmd(out),
		newConfigListCmd(out),
	)
	return cmd
}

func newConfigGetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get [setting-name]",
		Short:   "Get a CLI setting",
		Long:    "List the value for a particular setting in your config.yaml file",
		Args:    cobra.ExactArgs(1),
		Example: configGetExample,
		RunE:    configGet,
	}
	return cmd
}

func newConfigSetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set [setting-name]",
		Short:   "Set a CLI setting",
		Long:    "Update or override a particular setting in your config.yaml file",
		Example: configSetExample,
		RunE:    configSet,
	}
	return cmd
}

func newConfigListCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List CLI settings",
		Long:  "List every CLI setting with its value and where the value comes from: project (a 1.x project's .astro/config.yaml), global, or default. With -g, list the global values only",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configList(out)
		},
	}
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) error {
	// Cobra runs a PersistentPreRunE before the subcommand's own argument
	// check, so a bare `astro config set` arrives here with nothing to index
	// and used to panic. Let it through; configSet reports the arity.
	if len(args) == 0 {
		return nil
	}
	if globalFlag {
		return nil
	}
	if isProjectDir, _ := config.IsProjectDir(config.WorkingPath); isProjectDir { //nolint:errcheck // treated as absent on error
		return nil
	}
	// cmd.Name(), not cmd.Use: Use carries the argument placeholder, so the
	// suggested command read "astro config set [setting-name] project.name
	// -g", which nobody can run.
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuoteIfNeeded(a)
	}
	c := "astro config " + cmd.Name() + " " + strings.Join(quoted, " ") + " -g"
	cmd.SilenceUsage = true
	if project.HasManifest(config.WorkingPath) {
		return fmt.Errorf(configUseInV2ProjectMsg, args[0], cmd.Name(), c)
	}
	return fmt.Errorf(configUseOutsideProjectDirMsg, cmd.Name(), cmd.Name(), c)
}

func shellQuoteIfNeeded(s string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !isShellSafe(r) }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.,:/=@+%", r)
}

func configGet(cmd *cobra.Command, args []string) error {
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errInvalidConfigPath
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if globalFlag {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetHomeString())
	} else {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetProjectString())
	}

	return nil
}

func configSet(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errInvalidSetArgs
	}

	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errInvalidConfigPath
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Validate the value using the registered validator (if any)
	if err := cfg.Validate(args[1]); err != nil {
		return fmt.Errorf("invalid value for %s: %w", cfg.Path, err)
	}

	var err error
	if globalFlag {
		err = cfg.SetHomeString(args[1])
	} else {
		err = cfg.SetProjectString(args[1])
	}
	if err != nil {
		return err
	}

	fmt.Printf(configSetSuccessMsg+"\n", cfg.Path, args[1])
	return nil
}

func configList(out io.Writer) error {
	tab := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"KEY", "VALUE", "SCOPE"},
	}
	for _, key := range slices.Sorted(maps.Keys(config.CFGStrMap)) {
		if unlistedConfigs[key] {
			continue
		}
		cfg := config.CFGStrMap[key]
		if globalFlag {
			tab.AddRow([]string{key, cfg.GetHomeString(), cfg.HomeScope()}, false)
		} else {
			tab.AddRow([]string{key, cfg.GetString(), cfg.Scope()}, false)
		}
	}
	return tab.Print(out)
}
