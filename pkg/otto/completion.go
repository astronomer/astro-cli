package otto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
)

// cacheSchemaVersion must match the value Otto writes in its completion-cache
// module. Bump in lockstep; mismatch falls back to baseline.
const cacheSchemaVersion = 1

// completionCache mirrors the JSON schema Otto writes to
// ~/.astro/otto/cache/completion.json on every invocation.
type completionCache struct {
	SchemaVersion int           `json:"schemaVersion"`
	OttoVersion   string        `json:"ottoVersion"`
	Flags         []flagSpec    `json:"flags"`
}

type flagSpec struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Desc        string   `json:"desc"`
	TakesValue  bool     `json:"takesValue"`
	Values      []string `json:"values,omitempty"`
	ValueSource string   `json:"valueSource,omitempty"`
}

func cachePath() string {
	return filepath.Join(config.HomeConfigPath, "otto", "cache", "completion.json")
}

func sessionsDir() string {
	return filepath.Join(config.HomeConfigPath, "otto", "sessions")
}

// loadCache returns the cached completion data when present and valid, or the
// embedded baseline. Never returns an error: completion must never break the
// shell.
func loadCache() completionCache {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return embeddedBaseline()
	}
	var c completionCache
	if err := json.Unmarshal(data, &c); err != nil || c.SchemaVersion != cacheSchemaVersion {
		return embeddedBaseline()
	}
	return c
}

// embeddedBaseline is the fallback when otto has never run (or the cache is
// unreadable / schema-mismatched). Keep it tiny: just the flags that have
// stayed stable across releases.
func embeddedBaseline() completionCache {
	return completionCache{
		SchemaVersion: cacheSchemaVersion,
		Flags: []flagSpec{
			{Name: "--help", Short: "-h", Desc: "Show help and exit"},
			{Name: "--version", Short: "-v", Desc: "Show version and exit"},
			{Name: "--mode", Desc: "Output mode", TakesValue: true,
				Values: []string{"interactive", "json", "text", "rpc"}},
			{Name: "--continue", Short: "-c", Desc: "Resume the most recent session"},
			{Name: "--resume", Short: "-r", Desc: "Open the session picker"},
		},
	}
}

// Complete is the Cobra ValidArgsFunction for `astro otto`. It inspects the
// previously-typed args to decide whether the user is mid-value (e.g. after
// `--mode`) or starting a new word, then returns matching candidates.
func Complete(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cache := loadCache()

	// If the previous arg is a flag that takes a value AND it wasn't given
	// inline (`--mode=json`), we're completing that flag's value.
	if len(args) > 0 {
		prev := args[len(args)-1]
		if spec := findFlag(cache.Flags, prev); spec != nil && spec.TakesValue {
			return completeValue(spec, toComplete)
		}
	}

	// `--flag=partial` form: split on the first `=`.
	if strings.HasPrefix(toComplete, "--") && strings.Contains(toComplete, "=") {
		eq := strings.IndexByte(toComplete, '=')
		flag, partial := toComplete[:eq], toComplete[eq+1:]
		if spec := findFlag(cache.Flags, flag); spec != nil && spec.TakesValue {
			vals, dir := completeValue(spec, partial)
			withPrefix := make([]string, 0, len(vals))
			for _, v := range vals {
				withPrefix = append(withPrefix, flag+"="+v)
			}
			return withPrefix, dir
		}
	}

	// New word: offer flag names when the user has typed a dash, otherwise
	// nothing (the positional slot is a free-text prompt, no candidates).
	if strings.HasPrefix(toComplete, "-") {
		return flagNames(cache.Flags), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func findFlag(flags []flagSpec, name string) *flagSpec {
	for i := range flags {
		if flags[i].Name == name || (flags[i].Short != "" && flags[i].Short == name) {
			return &flags[i]
		}
	}
	return nil
}

func flagNames(flags []flagSpec) []string {
	out := make([]string, 0, len(flags)*2)
	for _, f := range flags {
		out = append(out, f.Name+"\t"+f.Desc)
		if f.Short != "" {
			out = append(out, f.Short+"\t"+f.Desc)
		}
	}
	return out
}

func completeValue(spec *flagSpec, _ string) ([]string, cobra.ShellCompDirective) {
	if len(spec.Values) > 0 {
		return spec.Values, cobra.ShellCompDirectiveNoFileComp
	}
	switch spec.ValueSource {
	case "session":
		return sessionIDs(), cobra.ShellCompDirectiveNoFileComp
	case "file":
		return nil, cobra.ShellCompDirectiveDefault
	default:
		// model / skill / tool / unknown: no cheap source yet. Let the user
		// free-text; don't pollute the suggestion list with file paths.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func sessionIDs() []string {
	entries, err := os.ReadDir(sessionsDir())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".jsonl") {
			out = append(out, strings.TrimSuffix(name, ".jsonl"))
		}
	}
	return out
}
