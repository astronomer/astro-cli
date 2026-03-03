package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveHostname(t *testing.T) {
	tests := []struct {
		name       string
		projectDir string
		expected   string
	}{
		{
			name:       "simple name",
			projectDir: "/home/user/my-project",
			expected:   "my-project.localhost",
		},
		{
			name:       "uppercase",
			projectDir: "/home/user/My-Project",
			expected:   "my-project.localhost",
		},
		{
			name:       "spaces and special chars",
			projectDir: "/home/user/my project @v2",
			expected:   "my-project-v2.localhost",
		},
		{
			name:       "underscores",
			projectDir: "/home/user/my_project_v2",
			expected:   "my-project-v2.localhost",
		},
		{
			name:       "leading special chars",
			projectDir: "/home/user/---my-project",
			expected:   "my-project.localhost",
		},
		{
			name:       "trailing special chars",
			projectDir: "/home/user/my-project---",
			expected:   "my-project.localhost",
		},
		{
			name:       "dots in name",
			projectDir: "/home/user/my.project.v2",
			expected:   "my-project-v2.localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname, err := DeriveHostname(tt.projectDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, hostname)
		})
	}
}

func TestDeriveHostname_Empty(t *testing.T) {
	// A directory that results in an empty label after cleanup
	_, err := DeriveHostname("/home/user/---")
	assert.Error(t, err)
}

func TestDeriveHostname_LongName(t *testing.T) {
	longName := strings.Repeat("a", 100)
	hostname, err := DeriveHostname("/home/user/" + longName)
	require.NoError(t, err)
	// Should be truncated to 63 chars + ".localhost"
	label := strings.TrimSuffix(hostname, ".localhost")
	assert.LessOrEqual(t, len(label), maxLabelLen)
	assert.True(t, strings.HasSuffix(hostname, ".localhost"))
}
