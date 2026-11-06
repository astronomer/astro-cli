// Package secretenv is the handoff format for Plan.SecretEnv: the values a
// local start gives Airflow without writing them into a file, a command line,
// or a container's configuration.
//
// Both engines deliver the same payload, a JSON object of string values, to a
// loader that adds it to the environment of the process that runs Airflow and
// nothing earlier. Only the transport differs. Standalone passes it on an
// inherited pipe to `astro __supervise --env-fd`, because the CLI is the
// supervisor's parent. Docker writes it into a tmpfs volume through `compose
// exec` stdin, because containers outlive the CLI and restart on their own.
// Either way the values never reach disk. In docker mode they also stay out of
// what `docker inspect` shows, which env set through compose cannot do: the
// daemon records a container's environment in its on-disk config.
package secretenv

import (
	"encoding/json"
	"fmt"
	"io"
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

// Encode renders values as the payload both loaders read, leaving out any
// pair Valid refuses. It returns nil when nothing is left, which callers read
// as "no handoff": an engine with no secret values to deliver changes nothing
// about how it starts.
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

// Decode reads a payload Encode wrote. It applies Valid again, so a payload
// from anywhere else cannot make the loader build an environment exec refuses.
func Decode(r io.Reader) (map[string]string, error) {
	var values map[string]string
	if err := json.NewDecoder(r).Decode(&values); err != nil {
		return nil, fmt.Errorf("reading secret values: %w", err)
	}
	for k, v := range values {
		if !Valid(k, v) {
			delete(values, k)
		}
	}
	return values, nil
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
