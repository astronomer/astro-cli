package telemetry

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestUnknownCommandPath(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	dev := &cobra.Command{Use: "dev"}
	root.AddCommand(dev)

	tests := []struct {
		name   string
		parent *cobra.Command
		word   string
		want   string
	}{
		{"at root", root, "fly", "fly"},
		{"under a parent", dev, "restrt", "dev restrt"},
		{"dashes and digits are command-like", root, "run-dag2", "run-dag2"},
		{"punctuation is redacted", dev, "tok_ab123!!", "dev <redacted>"},
		{"a token-shaped word is redacted", root, "ey_JhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", RedactedCommand},
		{"capitals are redacted", root, "Deploy", RedactedCommand},
		{"a leading digit is redacted", root, "2fly", RedactedCommand},
		{"a path is redacted", root, "/Users/me/project", RedactedCommand},
		{"an over-long word is redacted", root, strings.Repeat("a", maxCommandLength+1), RedactedCommand},
		{"a word at the limit is kept", root, strings.Repeat("a", maxCommandLength), strings.Repeat("a", maxCommandLength)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, unknownCommandPath(tt.parent, tt.word))
		})
	}
}

func TestTrackUnknownCommandSendsNothingInTests(t *testing.T) {
	initTestConfig(t)

	assert.NotPanics(t, func() {
		TrackUnknownCommand(&cobra.Command{Use: "astro"}, "fly", "")
	})
}
