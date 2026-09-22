package scaffold

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
		// v1 bulk-loaded connections and variables into a running Airflow and
		// dumped them back out. v2 declares them per name instead, so there is
		// There is no "object import" row: v1's import covered connections and
		// variables together, no single v2 command does, and the honest answer
		// — the tree — is what the "object" row below already gives. A second
		// row saying the same thing publishes a duplicate line in the
		// scaffolded AGENTS.md and in the stub's payload, and lookup is
		// longest-prefix-first, so removing it changes no answer.
		{"object export", "astro local env list"},
		{"object", "astro local env"},
	}
}
