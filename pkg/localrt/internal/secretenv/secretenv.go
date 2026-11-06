// Package secretenv is the payload format for Plan.SecretEnv in docker mode:
// the values a local start gives Airflow as a compose secret rather than as
// container environment, so they stay out of what `docker inspect` and the
// engine's GUIs show. A loader in each container reads the payload, a JSON
// object of string values, into the environment of the command it runs.
package secretenv

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Valid reports whether a key and value can travel as one environment
// variable. An empty key, or one containing '=', cannot survive KEY=VALUE: the
// OS would split "A=B" into key "A" holding "B=<value>". A NUL in either
// cannot reach an environment at all; exec refuses it.
func Valid(key, value string) bool {
	return key != "" && !strings.ContainsAny(key, "=\x00") && !strings.Contains(value, "\x00")
}

// Encode renders values as the payload the loader reads, leaving out any pair
// Valid refuses. It returns nil when nothing is left, which callers read as
// "no handoff": a start with no secret values to deliver changes nothing about
// how it starts.
func Encode(values map[string]string) ([]byte, error) {
	kept := make(map[string]string, len(values))
	for k, v := range values {
		if Valid(k, v) {
			kept[k] = v
		}
	}
	if len(kept) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("encoding secret values: %w", err)
	}
	return b, nil
}

// Environ renders values as sorted KEY=VALUE entries for a process
// environment, leaving out any pair Valid refuses.
func Environ(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for k, v := range values {
		if Valid(k, v) {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}
