package config

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// Legacy1xValue returns what key, a 1.x setting v2 removed (removedSettings),
// is set to for the project at projectDir, the file that sets it, and what
// replaces it, when the value is a choice rather than a default the setting
// shipped with. value "" means unset, set to a shipped default, or no config
// read; replacement is the same text `astro config` refuses the setting with.
//
// It resolves the way 1.x did: the project's .astro/config.yaml, where 1.x's
// `astro config set` wrote without -g, over the home config. A project file
// that sets the key decides it even at the default, since 1.x read that over
// any global value.
//
// It reads the raw value without registering the key, since `astro config
// set` and `get` refuse it. The value is only ever reported, so a user who
// relied on it in 1.x is told what replaces it; v2 applies none of these.
func Legacy1xValue(projectDir, key string) (value, file, replacement string) {
	replacement = removedSettings[key].replacement
	if projectDir != "" {
		path := filepath.Join(projectDir, ConfigDir, ConfigFileNameWithExt)
		v := viper.New()
		v.SetFs(configFs)
		v.SetConfigFile(path)
		if err := readConfigFile(v, configFs); err == nil && v.IsSet(key) {
			return legacyChoice(v, key), path, replacement
		}
	}
	if viperHome == nil {
		return "", HomeConfigFile, replacement
	}
	return legacyChoice(viperHome, key), HomeConfigFile, replacement
}

// legacyChoice is v's value for key, or "" when that is unset or one of the
// defaults the setting shipped with.
func legacyChoice(v *viper.Viper, key string) string {
	s := strings.TrimSpace(v.GetString(key))
	if s == "" || slices.Contains(removedSettings[key].defaults, s) {
		return ""
	}
	return s
}
