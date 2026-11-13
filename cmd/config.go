package cmd

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

const (
	configSetSuccessMsg = "Setting %s to %s successfully\n"
	// The refusals of a get or set without -g. CLI settings are global, so -g
	// is the only scope there is; it stays required because Astro CLI 1.x's
	// `astro config set` without it wrote the project's .astro/config.yaml,
	// and the same command must not now change every project on the machine
	// without saying so.
	configUseWithoutGlobalMsg = "CLI settings are global: one value for every project on this machine.\nTo %s the global value, run:\n  %s\n"
	configUseWithManifestMsg  = "This project keeps its settings in pyproject.toml under [tool.astro], so it has no per-project CLI settings such as %s. " + configUseWithoutGlobalMsg
	configUseIn1xProjectMsg   = "Astro CLI v2 reads no settings from a project's .astro/config.yaml, such as %s. Convert the project with astro init, or use Astro CLI 1.x for that file. " + configUseWithoutGlobalMsg
)

// unknownSetting fails a get or set of a name that is no setting the way an
// unknown flag fails: a usage error (exit 2), naming the settings the name is
// most likely a typo of, when any are close. It wraps errInvalidConfigPath.
func unknownSetting(name string) error {
	return cliout.Usage(fmt.Errorf("%w: %q.%s Run astro config list to see every setting.",
		errInvalidConfigPath, name, suggestSettings(name)))
}

// suggestSettings is the "Did you mean" for a name that is no setting, or ""
// when nothing is close. It suggests only what `astro config list` shows, so
// never contexts (a set of it would replace every saved login) nor the
// internal.* overrides. A typo of a renamed setting's 1.x name suggests the
// current name; a typo of a removed one says it was removed and what replaces
// it, since suggesting a name get and set refuse would only lead to a second
// error.
func suggestSettings(name string) string {
	current, removed := closestSettings(name)
	switch {
	case len(current) == 1:
		return " Did you mean " + current[0] + "?"
	case len(current) > 1:
		return " Did you mean one of: " + strings.Join(current, ", ") + "?"
	case len(removed) > 0:
		reason, _ := config.RemovedSetting(removed[0])
		return " Did you mean " + removed[0] + "? It was removed in Astro CLI v2: " + reason
	}
	return ""
}

// closestSettings are the names nearest to name by cliout.Closest: within
// cliout.DefaultSuggestionDistance edits, a swap of neighbors counting as one,
// or name a prefix of them. Every
// name at the nearest distance, not one picked by sort order, since a partial
// name such as "telemetry" is equally close to each setting under it. current
// holds listed settings, with a renamed setting's 1.x name mapped to its
// current one; removed holds the removed settings. Both sorted, and both
// empty for an empty name, which is a prefix of everything.
func closestSettings(name string) (current, removed []string) {
	var candidates []string
	for path, cfg := range config.CFGStrMap {
		if cfg.Listed() {
			candidates = append(candidates, path)
		}
	}
	candidates = append(candidates, config.RemovedSettingNames()...)
	matches := cliout.Closest(name, candidates, cliout.DefaultSuggestionDistance)
	for _, match := range matches {
		if renamed, ok := config.LegacySettingNames[match]; ok {
			current = append(current, renamed)
		} else if _, ok := config.CFGStrMap[match]; ok {
			current = append(current, match)
		} else {
			removed = append(removed, match)
		}
	}
	slices.Sort(current)
	slices.Sort(removed)
	return slices.Compact(current), removed
}

// refuseRemovedConfigKey fails a get or set of a 1.x setting v2 removed or
// renamed the way a removed 1.x flag fails: a usage error naming what
// replaces it.
func refuseRemovedConfigKey(key string) error {
	reason, ok := config.RemovedSetting(key)
	if !ok {
		return nil
	}
	return cliout.Usage(fmt.Errorf("%s was removed in Astro CLI v2: %s", key, reason))
}

var (
	globalFlag       bool
	configGetExample = `  # Get a setting
  astro config get upgrade_message -g`
	configSetExample = `  # Turn off the upgrade notice on this machine
  astro config set upgrade_message false -g`
)

// configOutput is the -o of `astro config get`, `set` and `list`, registered
// once on the group as -g is.
var configOutput cliout.Format

// configSetting is one setting as `astro config get`, `set` and `list`
// publish it: its key, its value, the scope the value was read from or
// written to, and whether the global config sets it. The scopes are global
// and, in a list, default for a value the global config does not set.
type configSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Scope string `json:"scope"`
	Set   bool   `json:"set"`
}

// globalScope is the scope of the home config.
const globalScope = "global"

// configSettings is `astro config list`.
type configSettings struct {
	Settings []configSetting `json:"settings"`
}

func newConfigRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "Manage CLI settings for this machine",
		Long:              "Manage CLI settings, stored globally: one value for every project on this machine. get and set take -g. A project's own settings are in its pyproject.toml. Run astro config list to see every setting, or see https://www.astronomer.io/docs/astro/cli/configure-cli#available-cli-configurations for what each one does",
		PersistentPreRunE: ensureGlobalFlag,
		Annotations:       map[string]string{astroCmd.NoLoginAnnotation: "true"},
	}
	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "View or modify global config")
	cliout.AddOutputFlag(cmd, &configOutput)
	cmd.AddCommand(
		newConfigGetCmd(out),
		newConfigSetCmd(out),
		newConfigListCmd(out),
	)
	return cmd
}

func newConfigGetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <SETTING_NAME>",
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
		Use:     "set <SETTING_NAME> <VALUE>",
		Short:   "Set a CLI setting",
		Long:    "Update or override a particular setting in your config.yaml file",
		Example: configSetExample,
		// Cobra checks Args before the group's PersistentPreRunE, so a wrong
		// count is refused here, as a usage error, before ensureGlobalFlag or
		// refuseRemovedConfigKey can read args[0] and fail some other way.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return cliout.Usage(errInvalidSetArgs)
			}
			return nil
		},
		RunE: configSet,
	}
	return cmd
}

func newConfigListCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List CLI settings",
		Long:  "List every CLI setting with its value and where the value comes from: global, or default when the global config does not set it. -g changes nothing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configList(cliout.Renderer{Format: configOutput, Out: out})
		},
		Example: `  # List every setting, with where its value comes from
  astro config list`,
	}
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) error {
	// Cobra has checked the subcommand's Args before this runs, so set
	// arrives with two arguments and get with one. list arrives with none,
	// and there is nothing here to check for it.
	if len(args) == 0 {
		return nil
	}
	// A removed key is refused by set and get alike. Ahead of the scope
	// checks, so the answer is the same in and out of a project and with or
	// without -g.
	if cmd.Name() == "set" || cmd.Name() == "get" {
		if err := refuseRemovedConfigKey(args[0]); err != nil {
			cmd.SilenceUsage = true
			return err
		}
		// An unknown name too, for the same reason: the scope checks below
		// would otherwise answer it with advice to run it with -g, which
		// then fails.
		if _, ok := config.CFGStrMap[args[0]]; !ok {
			cmd.SilenceUsage = true
			return unknownSetting(args[0])
		}
	}
	if globalFlag {
		return nil
	}
	// cmd.Name(), not cmd.Use: Use carries the argument placeholder, so the
	// suggested command read "astro config set [setting-name] project.name
	// -g", which nobody can run.
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = scaffold.ShellQuote(a)
	}
	c := "astro config " + cmd.Name() + " " + strings.Join(quoted, " ") + " -g"
	cmd.SilenceUsage = true
	switch {
	case project.HasManifest(config.WorkingPath):
		return fmt.Errorf(configUseWithManifestMsg, args[0], cmd.Name(), c)
	case is1xProject():
		return fmt.Errorf(configUseIn1xProjectMsg, args[0], cmd.Name(), c)
	default:
		return fmt.Errorf(configUseWithoutGlobalMsg, cmd.Name(), c)
	}
}

// is1xProject reports whether the working directory is a project in the
// Astro CLI 1.x layout, whose .astro/config.yaml a 1.x `astro config set`
// without -g would have written.
func is1xProject() bool {
	is, _ := config.IsProjectDir(config.WorkingPath) //nolint:errcheck // treated as absent on error
	return is
}

// configGet publishes one setting's global value, the built-in default when
// the home config does not set it; set says whether it does.
func configGet(cmd *cobra.Command, args []string) error {
	// Cobra has checked the argument count, so what can fail from here is
	// the name or the value, which the usage text does not help with.
	cmd.SilenceUsage = true
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return unknownSetting(args[0])
	}

	setting := configSetting{Key: cfg.Path, Value: cfg.GetHomeString(), Scope: globalScope, Set: cfg.HomeScope() == globalScope}
	return cliout.Renderer{Format: configOutput, Out: cmd.OutOrStdout()}.Emit(&setting, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "%s: %s\n", setting.Key, setting.Value)
	}))
}

func configSet(cmd *cobra.Command, args []string) error {
	// The pre-run refuses these first; this keeps the write itself from
	// ever storing one. Ahead of the lookup, which does not know them.
	if err := refuseRemovedConfigKey(args[0]); err != nil {
		return err
	}
	cmd.SilenceUsage = true
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return unknownSetting(args[0])
	}

	// Validate the value using the registered validator (if any)
	if err := cfg.Validate(args[1]); err != nil {
		return fmt.Errorf("invalid value for %s: %w", cfg.Path, err)
	}

	setting := configSetting{Key: cfg.Path, Value: args[1], Scope: globalScope, Set: true}
	if err := cfg.SetHomeString(args[1]); err != nil {
		return err
	}

	return cliout.Renderer{Format: configOutput, Out: cmd.OutOrStdout()}.Emit(&setting, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, configSetSuccessMsg+"\n", setting.Key, setting.Value)
	}))
}

// configList publishes every setting but the unlisted ones, by key, with the
// scope its value comes from; set is false for a default. In text, the table
// it always printed.
func configList(r cliout.Renderer) error {
	list := configSettings{Settings: []configSetting{}}
	for _, key := range slices.Sorted(maps.Keys(config.CFGStrMap)) {
		cfg := config.CFGStrMap[key]
		if !cfg.Listed() {
			continue
		}
		setting := configSetting{Key: key, Value: cfg.GetHomeString(), Scope: cfg.HomeScope()}
		setting.Set = setting.Scope != "default"
		list.Settings = append(list.Settings, setting)
	}
	return r.Emit(&list, func(w io.Writer) error {
		tab := printutil.Table{
			DynamicPadding: true,
			Header:         []string{"KEY", "VALUE", "SCOPE"},
		}
		for _, s := range list.Settings {
			tab.AddRow([]string{s.Key, s.Value, s.Scope}, false)
		}
		return tab.Print(w)
	})
}
