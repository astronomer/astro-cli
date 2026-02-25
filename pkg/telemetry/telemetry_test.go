package telemetry

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestConfig(t *testing.T) {
	t.Helper()
	fs := afero.NewMemMapFs()
	configRaw := []byte("telemetry:\n  enabled: true\n")
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
	config.InitConfig(fs)
}

func TestIsEnabled(t *testing.T) {
	initTestConfig(t)

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
	rootCmd := &cobra.Command{Use: "astro"}
	deployCmd := &cobra.Command{Use: "deploy"}
	devCmd := &cobra.Command{Use: "dev"}
	startCmd := &cobra.Command{Use: "start"}
	workspaceCmd := &cobra.Command{Use: "workspace"}
	userCmd := &cobra.Command{Use: "user"}
	addCmd := &cobra.Command{Use: "add"}

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(startCmd)
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(userCmd)
	userCmd.AddCommand(addCmd)

	tests := []struct {
		name     string
		cmd      *cobra.Command
		expected string
	}{
		{
			name:     "simple command",
			cmd:      deployCmd,
			expected: "deploy",
		},
		{
			name:     "nested command",
			cmd:      startCmd,
			expected: "dev start",
		},
		{
			name:     "deeply nested command",
			cmd:      addCmd,
			expected: "workspace user add",
		},
		{
			name:     "root command only",
			cmd:      rootCmd,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCommandPath(tt.cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
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
		assert.Equal(t, TelemetryAPIURL, result)
	})
}

func TestShowFirstRunNotice(t *testing.T) {
	t.Run("prints notice on first call", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configRaw := []byte("telemetry:\n  enabled: true\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)

		// Capture stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		showFirstRunNotice()

		w.Close()
		out := make([]byte, 1024)
		n, _ := r.Read(out)
		os.Stderr = oldStderr

		output := string(out[:n])
		assert.Contains(t, output, "anonymous usage data")
		assert.Contains(t, output, "astro telemetry disable")

		// Config should now be marked
		assert.Equal(t, "true", config.CFG.TelemetryNoticeShown.GetHomeString())
	})

	t.Run("does not print on subsequent calls", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configRaw := []byte("telemetry:\n  enabled: true\n  notice_shown: \"true\"\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)

		// Capture stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		showFirstRunNotice()

		w.Close()
		out := make([]byte, 1024)
		n, _ := r.Read(out)
		os.Stderr = oldStderr

		assert.Equal(t, 0, n, "Should not print anything on subsequent calls")
	})
}

func TestGetAnonymousID(t *testing.T) {
	initTestConfig(t)

	id1 := GetAnonymousID()
	assert.NotEmpty(t, id1, "Should generate an ID")

	id2 := GetAnonymousID()
	assert.Equal(t, id1, id2, "Should return the same ID on subsequent calls")
}
