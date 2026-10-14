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
	configSetSuccessMsg           = "Setting %s to %s successfully\n"
	configUseOutsideProjectDirMsg = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s\n"
	configUseWithManifestMsg      = "This project keeps its settings in pyproject.toml under [tool.astro], so it has no per-project CLI settings such as %s. CLI settings are global: one value for every project on this machine.\nTo %s the global value, run:\n  %s\n"
)

// unlistedConfigs are settings `astro config list` leaves out: contexts is a
// map that `astro context` manages, and the other two are credentials.
var unlistedConfigs = map[string]bool{
	config.CFG.Contexts.Path:         true,
	config.CFG.CloudAPIToken.Path:    true,
	config.CFG.PostgresPassword.Path: true,
}

// removedConfigKeys are the 1.x settings nothing in v2 reads, with what
// replaces each. `astro config set` refuses them rather than writing a value
// that would change nothing; a 1.x CLI sharing the config file still reads
// what it wrote itself.
var removedConfigKeys = map[string]string{
	"dev.mode": "Local Airflow runs in standalone mode by default; start it in Docker with `astro local start --docker`. " +
		"`astro local restart` keeps the mode it is running in.",
	"proxy.port": "The local proxy listens on 6563, or on another free port when 6563 is taken, and has no setting.",
	"api-server.port": "Pick the local Airflow port with `astro local start --port <port>`; " +
		"without it a free port is chosen.",
	"webserver.port": "Pick the local Airflow port with `astro local start --port <port>`; " +
		"without it a free port is chosen.",
}

// refuseRemovedConfigKey fails a set of a removed key the way the removed
// commands fail: a usage error naming the replacement.
func refuseRemovedConfigKey(key string) error {
	replacement, ok := removedConfigKeys[key]
	if !ok {
		return nil
	}
	return cliout.Usage(fmt.Errorf("`%s` was removed in Astro CLI v2: nothing reads it. %s", key, replacement))
}

var (
	globalFlag       bool
	configGetExample = `  # Get a global setting
  astro config get show_warnings -g

  # Get a setting from a 1.x project's .astro/config.yaml
  astro config get project.name`
	configSetExample = `  # Turn off warnings for every project on this machine
  astro config set show_warnings false -g

  # Set a setting in a 1.x project's .astro/config.yaml
  astro config set postgres.user postgres`
)

// configOutput is the -o of `astro config get`, `set` and `list`, registered
// once on the group as -g is.
var configOutput cliout.Format

// configSetting is one setting as `astro config get`, `set` and `list`
// publish it: its key, its value, the scope the value was read from or
// written to, and whether a config file sets it there. The scopes are project
// (a 1.x project's .astro/config.yaml), global, and, in a list, default for a
// value no config file sets.
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

// projectScope is the scope a setting read from, or written to, a 1.x
// project's .astro/config.yaml has.
const projectScope = "project"

func newConfigRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "Manage CLI settings for this machine",
		Long:              "Manage CLI settings, stored globally with -g or in a 1.x project's .astro/config.yaml. Run `astro config list` to see every setting, or see https://www.astronomer.io/docs/astro/cli/configure-cli#available-cli-configurations for what each one does",
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
		Long:  "List every CLI setting with its value and where the value comes from: project (a 1.x project's .astro/config.yaml), global, or default. With -g, list the global values only",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configList(cliout.Renderer{Format: configOutput, Out: out})
		},
		Example: `  # List every setting, with where its value comes from
  astro config list

  # List the global values only
  astro config list -g`,
	}
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) error {
	// Cobra has checked the subcommand's Args before this runs, so set
	// arrives with two arguments and get with one. list arrives with none,
	// and there is nothing here to check for it.
	if len(args) == 0 {
		return nil
	}
	// Ahead of the scope checks, so the answer is the same in and out of a
	// project and with or without -g.
	if cmd.Name() == "set" {
		if err := refuseRemovedConfigKey(args[0]); err != nil {
			cmd.SilenceUsage = true
			return err
		}
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
		quoted[i] = scaffold.ShellQuote(a)
	}
	c := "astro config " + cmd.Name() + " " + strings.Join(quoted, " ") + " -g"
	cmd.SilenceUsage = true
	if project.HasManifest(config.WorkingPath) {
		return fmt.Errorf(configUseWithManifestMsg, args[0], cmd.Name(), c)
	}
	return fmt.Errorf(configUseOutsideProjectDirMsg, cmd.Name(), cmd.Name(), c)
}

// configGet publishes one setting from the scope asked for, as its text has
// always shown it: with -g the global value (the built-in default when the
// home config does not set it), and otherwise the 1.x project's own, which is
// "" when the project does not set it. It never falls back from one to the
// other; set says whether that scope sets the key. `config list` is where the
// value in effect, and the scope it comes from, are published.
func configGet(cmd *cobra.Command, args []string) error {
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errInvalidConfigPath
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	setting := configSetting{Key: cfg.Path, Value: cfg.GetProjectString(), Scope: projectScope, Set: cfg.Scope() == projectScope}
	if globalFlag {
		setting = configSetting{Key: cfg.Path, Value: cfg.GetHomeString(), Scope: globalScope, Set: cfg.HomeScope() == globalScope}
	}
	return cliout.Renderer{Format: configOutput, Out: cmd.OutOrStdout()}.Emit(&setting, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "%s: %s\n", setting.Key, setting.Value)
	}))
}

func configSet(cmd *cobra.Command, args []string) error {
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errInvalidConfigPath
	}
	// The pre-run refuses these first; this keeps the write itself from
	// ever storing one.
	if err := refuseRemovedConfigKey(args[0]); err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Validate the value using the registered validator (if any)
	if err := cfg.Validate(args[1]); err != nil {
		return fmt.Errorf("invalid value for %s: %w", cfg.Path, err)
	}

	setting := configSetting{Key: cfg.Path, Value: args[1], Scope: projectScope, Set: true}
	var err error
	if globalFlag {
		setting.Scope = globalScope
		err = cfg.SetHomeString(args[1])
	} else {
		err = cfg.SetProjectString(args[1])
	}
	if err != nil {
		return err
	}

	return cliout.Renderer{Format: configOutput, Out: cmd.OutOrStdout()}.Emit(&setting, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, configSetSuccessMsg+"\n", setting.Key, setting.Value)
	}))
}

// configList publishes every setting but the unlisted ones, by key: with -g
// the global values, and otherwise each value from where commands run here
// read it, with the scope it comes from; set is false for a default. In
// text, the table it always printed.
func configList(r cliout.Renderer) error {
	list := configSettings{Settings: []configSetting{}}
	for _, key := range slices.Sorted(maps.Keys(config.CFGStrMap)) {
		if unlistedConfigs[key] {
			continue
		}
		cfg := config.CFGStrMap[key]
		setting := configSetting{Key: key, Value: cfg.GetString(), Scope: cfg.Scope()}
		if globalFlag {
			setting = configSetting{Key: key, Value: cfg.GetHomeString(), Scope: cfg.HomeScope()}
		}
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
