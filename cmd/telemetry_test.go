package cmd

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// runTelemetryCmd runs `astro telemetry` with args, against a home config
// that has telemetry on.
func runTelemetryCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCommands(func(out io.Writer) []*cobra.Command {
		return []*cobra.Command{newTelemetryCmd(out)}
	}, append([]string{"telemetry"}, args...)...)
}

func TestTelemetryCmd(t *testing.T) {
	// Initialize config with a minimal home config to keep tests hermetic
	fs := afero.NewMemMapFs()
	configRaw := []byte("telemetry:\n  enabled: true\n")
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
	config.InitConfig(fs)
	t.Setenv("ASTRO_TELEMETRY_DISABLED", "")

	t.Run("telemetry command has subcommands", func(t *testing.T) {
		cmd := newTelemetryCmd(io.Discard)
		assert.Equal(t, "telemetry", cmd.Use)
		assert.Equal(t, 2, len(cmd.Commands()), "Should have enable and disable subcommands")
	})

	// Text is the line each command always printed, and nothing else.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"enable"}, "Telemetry enabled\n"},
		{nil, "Telemetry is enabled\n"},
		{[]string{"disable"}, "Telemetry disabled\n"},
		{nil, "Telemetry is disabled\n"},
		{[]string{"enable"}, "Telemetry enabled\n"},
	} {
		stdout, _, err := runTelemetryCmd(t, c.args...)
		require.NoError(t, err, c.args)
		assert.Equal(t, c.want, stdout, c.args)
	}

	// Under json each publishes the state it left, which a status reads back.
	state := func(t *testing.T, args ...string) telemetryState {
		t.Helper()
		stdout, stderr, err := runTelemetryCmd(t, append(args, "-o", "json")...)
		require.NoError(t, err, args)
		assert.Empty(t, stderr, args)
		var got telemetryState
		require.NoError(t, json.Unmarshal([]byte(stdout), &got), stdout)
		return got
	}
	t.Run("json reports the state each command leaves", func(t *testing.T) {
		assert.Equal(t, telemetryState{Enabled: false}, state(t, "disable"))
		assert.False(t, config.CFG.TelemetryEnabled.GetBool(), "the disable was written")
		assert.Equal(t, telemetryState{Enabled: false}, state(t))
		assert.Equal(t, telemetryState{Enabled: true}, state(t, "enable"))
		assert.True(t, config.CFG.TelemetryEnabled.GetBool(), "the enable was written")
		assert.Equal(t, telemetryState{Enabled: true}, state(t))
	})

	t.Run("json says when ASTRO_TELEMETRY_DISABLED outranks an enable", func(t *testing.T) {
		t.Setenv("ASTRO_TELEMETRY_DISABLED", "1")
		assert.Equal(t, telemetryState{Enabled: false, DisabledByEnv: true}, state(t, "enable"))
		assert.True(t, config.CFG.TelemetryEnabled.GetBool(), "the setting is still written")
		stdout, _, err := runTelemetryCmd(t)
		require.NoError(t, err)
		assert.Equal(t, "Telemetry is disabled\n", stdout)
	})
}

func TestTelemetrySendCmd(t *testing.T) {
	cmd := newTelemetrySendCmd()

	assert.Equal(t, "_telemetry-send", cmd.Use)
	assert.True(t, cmd.Hidden, "Command should be hidden")
	assert.Equal(t, "true", cmd.Annotations[telemetry.SkipPreRunAnnotation], "Should have skipPreRun annotation")
}
