package config

import (
	"slices"
	"strings"
)

// Legacy1xHomeValue returns what the home config holds for key, one of the
// Astro CLI 1.x settings v2 does not register (shippedDefaults names them),
// when that is a choice rather than the default 1.x shipped. "" means unset,
// set to a shipped default, or no home config read.
//
// It reads the raw value without registering the key: a registered setting is
// one `astro config set` and `get` accept, and these are refused there
// (removedConfigKeys in cmd/config.go). The value is only ever reported, so a
// user who relied on it in 1.x is told what replaces it; v2 applies none of
// these.
func Legacy1xHomeValue(key string) string {
	if viperHome == nil {
		return ""
	}
	v := strings.TrimSpace(viperHome.GetString(key))
	if v == "" || slices.Contains(shippedDefaults[key], v) {
		return ""
	}
	return v
}
