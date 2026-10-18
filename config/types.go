package config

import (
	"path/filepath"
	"strconv"

	"github.com/spf13/viper"
)

// ValidatorFunc is a function that validates a configuration value
type ValidatorFunc func(value string) error

// cfg defines settings a single configuration setting can have
type cfg struct {
	Path string
	// Default is the value a getter returns when no config file sets Path.
	// It is never registered with viper, so it is never written to a file:
	// see initHome.
	Default string
	// Legacy are the names the setting had before v2 renamed it, which
	// Astro CLI 1.x still reads and writes. A file that sets only a legacy
	// name sets the setting; one that sets both reads the current name. A
	// write goes to the current name and leaves a legacy one where it is, for
	// 1.x.
	Legacy    []string
	validator ValidatorFunc
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudAPIProtocol        cfg
	CloudAPIPort            cfg
	CloudWSProtocol         cfg
	Context                 cfg
	Contexts                cfg
	DockerCommand           cfg
	LocalCore               cfg
	LocalHouston            cfg
	LocalPlatform           cfg
	ShowWarnings            cfg
	Verbosity               cfg
	HoustonDialTimeout      cfg
	HoustonSkipVerifyTLS    cfg
	Interactive             cfg
	PageSize                cfg
	UpgradeMessage          cfg
	AutoSelect              cfg
	ShaAsTag                cfg
	RemoteClientRegistry    cfg
	RemoteBaseImageRegistry cfg
	DeployGitMetadata       cfg
	TelemetryEnabled        cfg
	TelemetryAnonymousID    cfg
	TelemetryNoticeShown    cfg
	OttoAutoUpdate          cfg
	CosmosBoostPreDeploy    cfg
}

// Creates a new cfg struct
func newCfg(path, dflt string, legacy ...string) cfg {
	ncfg := cfg{
		Path:    path,
		Default: dflt,
		Legacy:  legacy,
	}
	CFGStrMap[path] = ncfg
	for _, name := range legacy {
		LegacySettingNames[name] = path
	}
	return ncfg
}

// keyIn is the name v sets c under: its path, else the first legacy name v
// sets, else "" when v does not set it (or there is no v).
func (c cfg) keyIn(v *viper.Viper) string {
	if v == nil {
		return ""
	}
	for _, name := range append([]string{c.Path}, c.Legacy...) {
		if v.IsSet(name) {
			return name
		}
	}
	return ""
}

// projectKey is keyIn for the 1.x project config, "" when there is none.
func (c cfg) projectKey() string {
	if !configExists(viperProject) {
		return ""
	}
	return c.keyIn(viperProject)
}

// SetHomeString sets a string value in home config
func (c cfg) SetHomeString(value string) error {
	if !configExists(viperHome) {
		return nil
	}
	viperHome.Set(c.Path, c.typed(value))
	err := saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}
	return nil
}

// SetProjectString sets a string value in project config
func (c cfg) SetProjectString(value string) error {
	if !configExists(viperProject) {
		return nil
	}
	viperProject.Set(c.Path, c.typed(value))
	err := saveConfig(viperProject, viperProject.ConfigFileUsed())
	if err != nil {
		return err
	}
	return nil
}

// GetString will return the requested config, check working dir and fallback to home
func (c cfg) GetString() string {
	if c.projectKey() != "" {
		return c.GetProjectString()
	}
	return c.GetHomeString()
}

// GetStringFor is GetString for the project at projectDir rather than the
// working directory: that project's .astro/config.yaml when it sets the value,
// the home config otherwise. A command that acts on a project it was handed a
// path to (astro local, Astro Desktop's mirror of it) resolves the project's
// own setting even when run from somewhere else.
func (c cfg) GetStringFor(projectDir string) string {
	if projectDir != "" {
		v := viper.New()
		v.SetFs(configFs)
		v.SetConfigFile(filepath.Join(projectDir, ConfigDir, ConfigFileNameWithExt))
		if err := readConfigFile(v, configFs); err == nil {
			if key := c.keyIn(v); key != "" {
				return v.GetString(key)
			}
		}
	}
	return c.GetHomeString()
}

// GetBool will return the requested config, check working dir and fallback to home
func (c cfg) GetBool() bool {
	if key := c.projectKey(); key != "" {
		return viperProject.GetBool(key)
	}
	if key := c.keyIn(viperHome); key != "" {
		return viperHome.GetBool(key)
	}
	b, _ := strconv.ParseBool(c.Default) //nolint:errcheck // an empty default is false
	return b
}

// GetInt will return the integer value of requested config, check working dir and fallback to home
func (c cfg) GetInt() int {
	if key := c.projectKey(); key != "" {
		return viperProject.GetInt(key)
	}
	if key := c.keyIn(viperHome); key != "" {
		return viperHome.GetInt(key)
	}
	i, _ := strconv.Atoi(c.Default) //nolint:errcheck // an empty default is 0
	return i
}

// typed is value as the YAML type of c's default: a bool or an int when the
// default is one and value parses as the same, the string otherwise. A config
// set from the command line arrives as text, and stored as text it reads back
// as `apc.show_warnings: "false"`. Either form reads the same, here and in Astro
// CLI 1.x, which shares the file.
//
// A bool is the words true and false only. strconv.ParseBool also takes 1, 0,
// t and f, which would make a setting that defaults to 0 a bool.
func (c cfg) typed(value string) any {
	// Only a number that reads back as what was typed: 0443 or +443 would be
	// stored as 443 while set echoed what the user typed, and get disagreed.
	if _, err := strconv.Atoi(c.Default); err == nil {
		if i, err := strconv.Atoi(value); err == nil && strconv.Itoa(i) == value {
			return i
		}
	}
	if (c.Default == "true" || c.Default == "false") && (value == "true" || value == "false") {
		return value == "true"
	}
	return value
}

// GetProjectString will return a project config
func (c cfg) GetProjectString() string {
	key := c.keyIn(viperProject)
	if key == "" {
		return ""
	}
	return viperProject.GetString(key)
}

// GetHomeString will return config from home string, its default when the
// home config does not set it, or "" before InitConfig has read one, so a
// caller like GetCurrentDomain reports no context (its ErrGetHomeString)
// rather than panicking on a config that is not there yet.
func (c cfg) GetHomeString() string {
	if viperHome == nil {
		return ""
	}
	key := c.keyIn(viperHome)
	if key == "" {
		return c.Default
	}
	return viperHome.GetString(key)
}

// Scope names where GetString finds this setting: "project", "global", or
// "default" when no config file sets it.
func (c cfg) Scope() string {
	if c.projectKey() != "" {
		return "project"
	}
	return c.HomeScope()
}

// HomeScope names where GetHomeString finds this setting: "global", or
// "default" when the home config does not set it.
func (c cfg) HomeScope() string {
	for _, name := range append([]string{c.Path}, c.Legacy...) {
		if viperHome.InConfig(name) {
			return "global"
		}
	}
	return "default"
}

// RegisterValidator registers a validation function for this config
func (c *cfg) RegisterValidator(fn ValidatorFunc) {
	c.validator = fn
}

// Validate validates a value using the registered validator
func (c cfg) Validate(value string) error {
	if c.validator != nil {
		return c.validator(value)
	}
	return nil
}
