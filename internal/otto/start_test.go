package otto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// isHelpOrVersion lets an invocation past the login gate, so it must take
// exactly the flags Otto answers before it needs a login: a flag it misses
// says "not logged in" to a request Otto would have served, and one it adds
// lets through a launch that does need the login.
func TestIsHelpOrVersion(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"-h"},
		{"--version"},
		{"-v"},
		{"--model", "m", "--help"},
	} {
		assert.True(t, isHelpOrVersion(args), "%q", args)
	}
	for _, args := range [][]string{
		nil,
		{"explain this DAG"},
		// Otto refuses these as unknown flags rather than showing help.
		{"--help=true"},
		{"-h=true"},
		{"--version=true"},
		{"version"},
	} {
		assert.False(t, isHelpOrVersion(args), "%q", args)
	}
}
