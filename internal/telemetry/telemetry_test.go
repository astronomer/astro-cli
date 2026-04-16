package telemetry

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
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

	origEnv := os.Getenv("ASTRO_TELEMETRY_DISABLED")
	defer os.Setenv("ASTRO_TELEMETRY_DISABLED", origEnv)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"disabled with 1", "1", false},
		{"disabled with true", "true", false},
		{"disabled with TRUE", "TRUE", false},
		{"enabled with empty", "", true},
		{"enabled with 0", "0", true},
		{"enabled with false", "false", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("ASTRO_TELEMETRY_DISABLED", tt.envValue)
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
		{"simple command", deployCmd, "deploy"},
		{"nested command", startCmd, "dev start"},
		{"deeply nested command", addCmd, "workspace user add"},
		{"root command only", rootCmd, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCommandPath(tt.cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInteractive(t *testing.T) {
	result := IsInteractive()
	assert.False(t, result, "should return false in test runner since stdin is not a terminal")
}

func TestShowFirstRunNotice(t *testing.T) {
	t.Run("prints notice on first call", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configRaw := []byte("telemetry:\n  enabled: true\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)

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

		assert.Equal(t, "true", config.CFG.TelemetryNoticeShown.GetHomeString())
	})

	t.Run("does not print on subsequent calls", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configRaw := []byte("telemetry:\n  enabled: true\n  notice_shown: \"true\"\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)

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

func TestBuildCommandProperties_DevMode(t *testing.T) {
	rootCmd := &cobra.Command{Use: "astro"}
	devCmd := &cobra.Command{Use: "dev"}
	startCmd := &cobra.Command{Use: "start"}
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(startCmd)

	t.Run("includes dev_mode when annotation is set", func(t *testing.T) {
		startCmd.Annotations = map[string]string{DevModeAnnotation: "standalone"}
		props := buildCommandProperties(startCmd)
		assert.Equal(t, "standalone", props[DevModeAnnotation])
		assert.Equal(t, "dev start", props["command"])
	})

	t.Run("includes docker dev_mode annotation", func(t *testing.T) {
		startCmd.Annotations = map[string]string{DevModeAnnotation: "docker"}
		props := buildCommandProperties(startCmd)
		assert.Equal(t, "docker", props[DevModeAnnotation])
	})

	t.Run("omits dev_mode for non-dev commands", func(t *testing.T) {
		deployCmd := &cobra.Command{Use: "deploy"}
		rootCmd.AddCommand(deployCmd)
		props := buildCommandProperties(deployCmd)
		_, hasDevMode := props[DevModeAnnotation]
		assert.False(t, hasDevMode, "non-dev commands should not have dev_mode property")
	})
}
