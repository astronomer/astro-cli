package config

import (
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// Legacy1xValue returns what key, a 1.x setting v2 removed (removedSettings),
// is set to in the home config, the file that sets it, and what replaces it,
// when the value is a choice rather than a default the setting shipped with.
// value "" means unset, set to a shipped default, or no config read;
// replacement is the same text `astro config` refuses the setting with.
//
// A 1.x project's own .astro/config.yaml is not read: astro init reports what
// it set when it converts the project, and deletes it.
//
// It reads the raw value without registering the key, since `astro config
// set` and `get` refuse it. The value is only ever reported, so a user who
// relied on it in 1.x is told what replaces it; v2 applies none of these.
func Legacy1xValue(key string) (value, file, replacement string) {
	replacement = removedSettings[key].replacement
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
