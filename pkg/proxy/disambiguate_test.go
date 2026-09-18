package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DeriveHostname is a function of the path and nothing else, which is what
// lets callers re-derive it — and is also why it collides. Two directories
// with the same base name produce the same hostname, and the second project
// to start had no name at all while the URL went on answering for the first.
//
// DisambiguateHostname is the way out: it qualifies the leftmost label, the
// one that names the project rather than where it lives.
func TestDisambiguateHostname(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		discriminator string
		want          string
	}{
		{
			name:          "a plain project takes the discriminator on its only label",
			hostname:      "analytics.localhost",
			discriminator: "a1b2c3",
			want:          "analytics-a1b2c3.localhost",
		},
		{
			name:          "a worktree keeps the repo it belongs to",
			hostname:      "feature-x.astro-cli.localhost",
			discriminator: "a1b2c3",
			want:          "feature-x-a1b2c3.astro-cli.localhost",
		},
		{
			name:          "an empty discriminator changes nothing",
			hostname:      "analytics.localhost",
			discriminator: "",
			want:          "analytics.localhost",
		},
		{
			name:          "a discriminator of nothing usable changes nothing",
			hostname:      "analytics.localhost",
			discriminator: "///",
			want:          "analytics.localhost",
		},
		{
			name:          "a hostname with no label to qualify is left alone",
			hostname:      "localhost",
			discriminator: "a1b2c3",
			want:          "localhost",
		},
		{
			name:          "the discriminator is lowercased and sanitized like any label",
			hostname:      "analytics.localhost",
			discriminator: "A1B2C3",
			want:          "analytics-a1b2c3.localhost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisambiguateHostname(tt.hostname, tt.discriminator))
		})
	}
}

// The label limit is the point of the exercise: a qualified name that DNS
// will not carry is no better than a collision. The original label gives way,
// never the discriminator, because the discriminator is the part that makes
// the name unique.
func TestDisambiguateHostnameRespectsTheLabelLimit(t *testing.T) {
	long := strings.Repeat("a", maxLabelLen)
	got := DisambiguateHostname(long+LocalhostSuffix, "a1b2c3")

	label, _, found := strings.Cut(got, ".")
	require.True(t, found)
	assert.LessOrEqual(t, len(label), maxLabelLen, "the qualified label must still be a legal DNS label")
	assert.True(t, strings.HasSuffix(label, "-a1b2c3"), "the discriminator survives, the name gives way: %q", label)
	assert.True(t, strings.HasSuffix(got, LocalhostSuffix))
}

// Two directories with the same base name are the case this exists for, and
// the qualified names have to actually differ.
func TestDisambiguateHostnameSeparatesTwoProjectsOfTheSameName(t *testing.T) {
	const base = "analytics.localhost"
	first := DisambiguateHostname(base, "a1b2c3")
	second := DisambiguateHostname(base, "d4e5f6")

	assert.NotEqual(t, first, second)
	assert.Equal(t, first, DisambiguateHostname(base, "a1b2c3"), "the same inputs give the same name on every restart")
}

// A label that is all separators sanitizes to nothing, and a name built from
// nothing but the discriminator would not say which project it was.
func TestDisambiguateHostnameLeavesAnUnusableLabelAlone(t *testing.T) {
	assert.Equal(t, "-.localhost", DisambiguateHostname("-.localhost", "a1b2c3"))
}
