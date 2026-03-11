//go:build no_agent

package agent

// Stub for builds without the embedded opencode binary.
// Build with -tags no_agent to use this instead of the real embed.

var opencodeCompressed []byte
var opencodeVersion = "none"
var skillsCompressed []byte
