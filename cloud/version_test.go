package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetShortVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "standard three-part version",
			version:  "1.40.1",
			expected: "1.40",
		},
		{
			name:     "version with release candidate suffix",
			version:  "2.5.3-rc1",
			expected: "2.5",
		},
		{
			name:     "version with v prefix",
			version:  "v1.40.1",
			expected: "1.40",
		},
		{
			name:     "version with v prefix and suffix",
			version:  "v3.2.1-beta",
			expected: "3.2",
		},
		{
			name:     "two-part version",
			version:  "1.40",
			expected: "1.40",
		},
		{
			name:     "single-part version",
			version:  "1",
			expected: "1",
		},
		{
			name:     "four-part version",
			version:  "1.40.1.0",
			expected: "1.40",
		},
		{
			name:     "version with build metadata",
			version:  "1.40.1+build123",
			expected: "1.40",
		},
		{
			name:     "empty string",
			version:  "",
			expected: "",
		},
		{
			name:     "complex pre-release",
			version:  "10.20.30-alpha.beta.1",
			expected: "10.20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetShortVersion(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}
