package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "string shorter than maxLen",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "string exactly maxLen",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "string longer than maxLen",
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   5,
			expected: "",
		},
		{
			name:     "maxLen is 0",
			input:    "hello",
			maxLen:   0,
			expected: "",
		},
		{
			name:     "maxLen is 1",
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
		{
			name:     "maxLen is 2",
			input:    "hello",
			maxLen:   2,
			expected: "he",
		},
		{
			name:     "maxLen is 3",
			input:    "hello",
			maxLen:   3,
			expected: "hel",
		},
		{
			name:     "maxLen is 4 with long string",
			input:    "hello world",
			maxLen:   4,
			expected: "h...",
		},
		{
			name:     "long string truncated",
			input:    "this is a very long string that needs to be truncated",
			maxLen:   20,
			expected: "this is a very lo...",
		},
		{
			name:     "unicode characters",
			input:    "hello 世界",
			maxLen:   8,
			expected: "hello...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}
