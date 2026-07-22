package scaffold

// DevMappingDoc is the published full mapping from `astro dev` to the v2
// surface.
const DevMappingDoc = "astro.sh/v2/dev-to-local"

// DevReplacement maps one v1 `astro dev` subcommand to what replaces it.
// One source, two surfaces: the `astro dev` removal stub renders this data
// as its error text, and the scaffold publishes it in AGENTS.md.
type DevReplacement struct {
	Command     string `json:"command"`
	Replacement string `json:"replacement"`
}

// DevReplacements returns the mapping in the order shown to the user.
// Multi-word entries must come before their one-word prefix so lookup finds
// the longest match.
func DevReplacements() []DevReplacement {
	return []DevReplacement{
		{"start", "astro local start"},
		{"stop", "astro local stop"},
		{"restart", "astro local restart"},
		{"ps", "astro local status"},
		{"logs", "astro local logs"},
		{"run", "astro local run"},
		{"bash", "astro local shell"},
		{"parse", "astro local check"},
		{"kill", "astro local stop --clean"},
		{"pytest", "uv run pytest"},
		{"init", "astro init"},
		{"object import", "astro local env schema"},
		{"object export", "astro local env schema"},
		{"object", "astro local env schema"},
	}
}
