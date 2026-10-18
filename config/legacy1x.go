package config

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// Legacy1xValue returns what an Astro CLI 1.x setting v2 does not register
// (shippedDefaults names them) is set to for the project at projectDir, and
// the file that sets it, when that is a choice rather than the default 1.x
// shipped. "" means unset, set to a shipped default, or no config read.
//
// It resolves the way 1.x did: the project's .astro/config.yaml, where 1.x's
// `astro config set` wrote without -g, over the home config. A project file
// that sets the key decides it even at the default, since 1.x read that over
// any global value.
//
// It reads the raw value without registering the key: a registered setting is
// one `astro config set` and `get` accept, and these are refused there
// (removedConfigKeys in cmd/config.go). The value is only ever reported, so a
// user who relied on it in 1.x is told what replaces it; v2 applies none of
// these.
func Legacy1xValue(projectDir, key string) (value, file string) {
	if projectDir != "" {
		path := filepath.Join(projectDir, ConfigDir, ConfigFileNameWithExt)
		v := viper.New()
		v.SetFs(configFs)
		v.SetConfigFile(path)
		if err := readConfigFile(v, configFs); err == nil && v.IsSet(key) {
			return legacyChoice(v, key), path
		}
	}
	if viperHome == nil {
		return "", HomeConfigFile
	}
	return legacyChoice(viperHome, key), HomeConfigFile
}

// legacyChoice is v's value for key, or "" when that is unset or a default
// 1.x shipped.
func legacyChoice(v *viper.Viper, key string) string {
	s := strings.TrimSpace(v.GetString(key))
	if s == "" || slices.Contains(shippedDefaults[key], s) {
		return ""
	}
	return s
}
