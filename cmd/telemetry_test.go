package cmd

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/telemetry"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryCmd(t *testing.T) {
	// Initialize config with a minimal home config to keep tests hermetic
	fs := afero.NewMemMapFs()
	configRaw := []byte("telemetry:\n  enabled: true\n")
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
	config.InitConfig(fs)

	t.Run("telemetry command has subcommands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newTelemetryCmd(buf)

		assert.Equal(t, "telemetry", cmd.Use)
		assert.Equal(t, 2, len(cmd.Commands()), "Should have enable and disable subcommands")
	})

	t.Run("telemetry shows status by default", func(t *testing.T) {
		_ = telemetryEnable(new(bytes.Buffer))

		buf := new(bytes.Buffer)
		err := telemetryStatus(buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Telemetry is enabled")
	})

	t.Run("telemetry enable", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := telemetryEnable(buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Telemetry enabled")
	})

	t.Run("telemetry disable", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := telemetryDisable(buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Telemetry disabled")
	})

	t.Run("telemetry status when enabled", func(t *testing.T) {
		// First enable telemetry
		_ = telemetryEnable(new(bytes.Buffer))

		buf := new(bytes.Buffer)
		err := telemetryStatus(buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Telemetry is enabled")
	})

	t.Run("telemetry status when disabled", func(t *testing.T) {
		// First disable telemetry
		_ = telemetryDisable(new(bytes.Buffer))

		buf := new(bytes.Buffer)
		err := telemetryStatus(buf)

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Telemetry is disabled")
	})
}

func TestTelemetrySendCmd(t *testing.T) {
	cmd := newTelemetrySendCmd()

	assert.Equal(t, "_telemetry-send", cmd.Use)
	assert.True(t, cmd.Hidden, "Command should be hidden")
	assert.Equal(t, "true", cmd.Annotations[telemetry.SkipPreRunAnnotation], "Should have skipPreRun annotation")
}
