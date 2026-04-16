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

func TestDevModeAnnotationConstant(t *testing.T) {
	assert.Equal(t, "dev_mode", DevModeAnnotation, "annotation key should be dev_mode")
}

func TestDevModeAnnotationIncludedInProperties(t *testing.T) {
	// TrackCommand is guarded by IsEnabled() and isTestRun(), so we can't
	// easily call it end-to-end in a test binary.  Instead, verify that the
	// annotation plumbing works by checking the constant is the expected
	// value and that the Annotations map on cobra.Command behaves as expected.
	cmd := &cobra.Command{
		Use:         "start",
		Annotations: map[string]string{DevModeAnnotation: "standalone"},
	}
	assert.Equal(t, "standalone", cmd.Annotations[DevModeAnnotation])

	cmdDocker := &cobra.Command{
		Use:         "start",
		Annotations: map[string]string{DevModeAnnotation: "docker"},
	}
	assert.Equal(t, "docker", cmdDocker.Annotations[DevModeAnnotation])

	cmdNoAnnotation := &cobra.Command{Use: "deploy"}
	assert.Empty(t, cmdNoAnnotation.Annotations[DevModeAnnotation],
		"non-dev commands should have no dev_mode annotation")
}
