package config

import (
	"strconv"
	"strings"
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
	// Astro CLI 1.x still reads and writes. `astro config` refuses them
	// (removed.go); a file that sets one still sets the setting (compat1x.go).
	Legacy    []string
	validator ValidatorFunc
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudAPIProtocol        cfg
	APCAPIPort              cfg
	APCWSProtocol           cfg
	Context                 cfg
	Contexts                cfg
	DockerCommand           cfg
	AstroLocalCoreURL       cfg
	APCHoustonURL           cfg
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

// SetHomeString sets a string value in home config
func (c cfg) SetHomeString(value string) error {
	if !configExists(viperHome) {
		return nil
	}
	c.set(viperHome, c.typed(value))
	err := saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}
	return nil
}

// GetString returns the setting's value: the home config's, or its default
// when the home config does not set it. No setting is read from a project's
// files: what a project decides is in its pyproject.toml.
func (c cfg) GetString() string {
	return c.GetHomeString()
}

// GetBool is GetString as a bool.
func (c cfg) GetBool() bool {
	if key := c.keyIn(viperHome); key != "" {
		return viperHome.GetBool(key)
	}
	b, _ := strconv.ParseBool(c.Default) //nolint:errcheck // an empty default is false
	return b
}

// GetInt is GetString as an int.
func (c cfg) GetInt() int {
	if key := c.keyIn(viperHome); key != "" {
		return viperHome.GetInt(key)
	}
	i, _ := strconv.Atoi(c.Default) //nolint:errcheck // an empty default is 0
	return i
}

// typed is value as the YAML type of c's default: a bool or an int when the
// default is one and value parses as the same, the string otherwise. A config
// set from the command line arrives as text, and stored as text it reads back
// as `show_warnings: "false"`. Either form reads the same, here and in Astro
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

// HomeScope names where GetHomeString finds this setting: "global", or
// "default" when the home config does not set it. The name it checks is the
// one GetHomeString reads, so the two never describe different keys.
func (c cfg) HomeScope() string {
	if key := c.keyIn(viperHome); key != "" && viperHome.InConfig(key) {
		return "global"
	}
	return "default"
}

// Listed reports whether `astro config list` shows the setting: not contexts,
// which `astro context` manages, nor the internal.* overrides for
// Astronomer's own development, which get and set still take.
func (c cfg) Listed() bool {
	return c.Path != contextsKey && !strings.HasPrefix(c.Path, "internal.")
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
