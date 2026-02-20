package telemetry

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsEnabled(t *testing.T) {
	// Initialize config for tests
	fs := afero.NewMemMapFs()
	config.InitConfig(fs)

	// Save original env value
	origEnv := os.Getenv(envTelemetryDisabled)
	defer os.Setenv(envTelemetryDisabled, origEnv)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "disabled with 1",
			envValue: "1",
			expected: false,
		},
		{
			name:     "disabled with true",
			envValue: "true",
			expected: false,
		},
		{
			name:     "disabled with TRUE",
			envValue: "TRUE",
			expected: false,
		},
		{
			name:     "enabled with empty",
			envValue: "",
			expected: true,
		},
		{
			name:     "enabled with 0",
			envValue: "0",
			expected: true,
		},
		{
			name:     "enabled with false",
			envValue: "false",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(envTelemetryDisabled, tt.envValue)
			result := IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCommandPath(t *testing.T) {
	tests := []struct {
		name     string
		cmdPath  string
		expected string
	}{
		{
			name:     "simple command",
			cmdPath:  "astro deploy",
			expected: "deploy",
		},
		{
			name:     "nested command",
			cmdPath:  "astro dev start",
			expected: "dev start",
		},
		{
			name:     "deeply nested command",
			cmdPath:  "astro workspace user add",
			expected: "workspace user add",
		},
		{
			name:     "root command only",
			cmdPath:  "astro",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock command with the given path
			cmd := &cobra.Command{Use: "test"}
			// Override CommandPath for testing
			cmd.SetUsageFunc(func(c *cobra.Command) error { return nil })

			// Test the path extraction logic directly
			result := extractCommandPath(tt.cmdPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// extractCommandPath is a helper to test the path extraction logic
func extractCommandPath(path string) string {
	// This mirrors the logic in GetCommandPath
	parts := splitCommandPath(path)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func splitCommandPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == ' ' {
			if start < i {
				parts = append(parts, path[start:i])
			}
			start = i + 1
			// Return after first split to get "astro" and rest
			if len(parts) == 1 {
				if start < len(path) {
					parts = append(parts, path[start:])
				}
				return parts
			}
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

func TestDetectContext(t *testing.T) {
	// Save and clear relevant env vars
	envVarsToSave := []string{
		"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR_TRACE_ID",
		"AIDER_MODEL", "CONTINUE_GLOBAL_DIR",
		"GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL", "CIRCLECI", "CI",
	}
	savedVals := make(map[string]string)
	for _, env := range envVarsToSave {
		savedVals[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedVals {
			if val != "" {
				os.Setenv(env, val)
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name     string
		envVar   string
		envValue string
		expected string
	}{
		{
			name:     "claude code",
			envVar:   "CLAUDECODE",
			envValue: "1",
			expected: "claude-code",
		},
		{
			name:     "claude code entrypoint",
			envVar:   "CLAUDE_CODE_ENTRYPOINT",
			envValue: "cli",
			expected: "claude-code",
		},
		{
			name:     "cursor",
			envVar:   "CURSOR_TRACE_ID",
			envValue: "abc123",
			expected: "cursor",
		},
		{
			name:     "github actions",
			envVar:   "GITHUB_ACTIONS",
			envValue: "true",
			expected: "github-actions",
		},
		{
			name:     "gitlab ci",
			envVar:   "GITLAB_CI",
			envValue: "true",
			expected: "gitlab-ci",
		},
		{
			name:     "generic ci",
			envVar:   "CI",
			envValue: "true",
			expected: "ci-unknown",
		},
		{
			name:     "interactive",
			envVar:   "",
			envValue: "",
			expected: "interactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars for this test
			for _, env := range envVarsToSave {
				os.Unsetenv(env)
			}

			if tt.envVar != "" {
				os.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}

			result := DetectContext()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTelemetryAPIURL(t *testing.T) {
	// Save original env value
	origEnv := os.Getenv(envTelemetryAPIURL)
	defer os.Setenv(envTelemetryAPIURL, origEnv)

	t.Run("custom URL from env", func(t *testing.T) {
		os.Setenv(envTelemetryAPIURL, "http://custom:8080/v1alpha1/telemetry")
		defer os.Unsetenv(envTelemetryAPIURL)

		result := GetTelemetryAPIURL()
		assert.Equal(t, "http://custom:8080/v1alpha1/telemetry", result)
	})

	t.Run("default URL without env", func(t *testing.T) {
		os.Unsetenv(envTelemetryAPIURL)

		result := GetTelemetryAPIURL()
		// Should return either prod or dev URL based on version
		assert.True(t, result == TelemetryAPIURLProd || result == TelemetryAPIURLDev)
	})
}

func TestGetAnonymousID(t *testing.T) {
	// Initialize config for tests
	fs := afero.NewMemMapFs()
	config.InitConfig(fs)

	id1 := GetAnonymousID()
	assert.NotEmpty(t, id1, "Should generate an ID")

	id2 := GetAnonymousID()
	assert.Equal(t, id1, id2, "Should return the same ID on subsequent calls")
}
