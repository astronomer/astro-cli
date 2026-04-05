package telemetry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// envVarNames extracts the environment variable names from an envMapping slice.
func envVarNames(mappings []envMapping) []string {
	names := make([]string, len(mappings))
	for i, m := range mappings {
		names[i] = m.envVar
	}
	return names
}

func TestIsDisabledByEnv(t *testing.T) {
	origEnv := os.Getenv("ASTRO_TELEMETRY_DISABLED")
	defer os.Setenv("ASTRO_TELEMETRY_DISABLED", origEnv)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"disabled with 1", "1", true},
		{"disabled with true", "true", true},
		{"disabled with TRUE", "TRUE", true},
		{"not disabled with empty", "", false},
		{"not disabled with 0", "0", false},
		{"not disabled with false", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("ASTRO_TELEMETRY_DISABLED", tt.envValue)
			result := IsDisabledByEnv()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectAgent(t *testing.T) {
	agentVars := envVarNames(agentEnvVars)
	savedVals := make(map[string]string)
	for _, env := range agentVars {
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
		{"claude code", "CLAUDECODE", "1", "claude-code"},
		{"claude code entrypoint", "CLAUDE_CODE_ENTRYPOINT", "cli", "claude-code"},
		{"cursor", "CURSOR_TRACE_ID", "abc123", "cursor"},
		{"snowflake cortex", "CORTEX_SESSION_ID", "session-123", "snowflake-cortex"},
		{"gemini cli", "GEMINI_CLI", "1", "gemini-cli"},
		{"opencode", "OPENCODE", "1", "opencode"},
		{"codex", "CODEX_API_KEY", "sk-test", "codex"},
		{"no agent", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, env := range agentVars {
				os.Unsetenv(env)
			}
			if tt.envVar != "" {
				os.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}
			result := DetectAgent()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectCISystem(t *testing.T) {
	ciVars := envVarNames(ciEnvVars)
	savedVals := make(map[string]string)
	for _, env := range ciVars {
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
		{"github actions", "GITHUB_ACTIONS", "true", "github-actions"},
		{"gitlab ci", "GITLAB_CI", "true", "gitlab-ci"},
		{"jenkins via hudson", "HUDSON_URL", "http://jenkins:8080", "jenkins"},
		{"azure devops", "TF_BUILD", "True", "azure-devops"},
		{"bitbucket pipelines", "BITBUCKET_BUILD_NUMBER", "42", "bitbucket-pipelines"},
		{"aws codebuild", "CODEBUILD_BUILD_ID", "build-123", "aws-codebuild"},
		{"teamcity", "TEAMCITY_VERSION", "2023.05", "teamcity"},
		{"buildkite", "BUILDKITE", "true", "buildkite"},
		{"codefresh", "CF_BUILD_ID", "build-456", "codefresh"},
		{"travis ci", "TRAVIS", "true", "travis-ci"},
		{"generic ci", "CI", "true", "ci-unknown"},
		{"no ci", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, env := range ciVars {
				os.Unsetenv(env)
			}
			if tt.envVar != "" {
				os.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}
			result := DetectCISystem()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTestRun(t *testing.T) {
	result := isTestRun()
	assert.True(t, result, "should return true when running inside a Go test binary")
}

func TestGetTelemetryAPIURL(t *testing.T) {
	origEnv := os.Getenv("ASTRO_TELEMETRY_API_URL")
	defer os.Setenv("ASTRO_TELEMETRY_API_URL", origEnv)

	t.Run("custom URL from env", func(t *testing.T) {
		os.Setenv("ASTRO_TELEMETRY_API_URL", "http://custom:8080/v1alpha1/telemetry")
		defer os.Unsetenv("ASTRO_TELEMETRY_API_URL")

		result := GetTelemetryAPIURL()
		assert.Equal(t, "http://custom:8080/v1alpha1/telemetry", result)
	})

	t.Run("default URL without env", func(t *testing.T) {
		os.Unsetenv("ASTRO_TELEMETRY_API_URL")

		result := GetTelemetryAPIURL()
		assert.Equal(t, TelemetryAPIURL, result)
	})
}
