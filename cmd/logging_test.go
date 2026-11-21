package cmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// keepLogger restores the logger's level and output after a test that sets
// them up.
func keepLogger(t *testing.T) {
	t.Helper()
	level := logger.GetLevel()
	t.Cleanup(func() { logger.SetLevel(level); logger.SetOutput(os.Stderr) })
}

// The level comes from the first of --verbosity, ASTRO_VERBOSITY and the
// verbosity setting that is set. A flag counts when it was given, whatever its
// value: --verbosity warning used to read as "not given", because it was the
// default's spelling, and the setting won over it.
func TestSetupLoggingTakesTheFirstSourceThatIsSet(t *testing.T) {
	keepLogger(t)
	for _, tc := range []struct {
		name    string
		flag    string
		env     string
		setting string
		want    slog.Level
	}{
		{name: "nothing set", want: slog.LevelWarn},
		{name: "the setting", setting: "info", want: slog.LevelInfo},
		{name: "the environment over the setting", env: "debug", setting: "error", want: slog.LevelDebug},
		{name: "the flag over both", flag: "error", env: "debug", setting: "debug", want: slog.LevelError},
		{name: "the default's spelling, given", flag: "warning", setting: "debug", want: slog.LevelWarn},
		{name: "a 1.x name", flag: "panic", want: logger.LevelOff},
		{name: "a bad environment under a good flag", flag: "error", env: "loud", want: slog.LevelError},
		{name: "a bad setting falls to the default", setting: "loud", want: slog.LevelWarn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			t.Setenv(verbosityEnv, tc.env)
			if tc.setting != "" {
				require.NoError(t, config.CFG.Verbosity.SetHomeString(tc.setting))
			}
			root := &cobra.Command{Use: "astro"}
			addVerbosityFlag(root.PersistentFlags())
			sub := &cobra.Command{Use: "sub"}
			root.AddCommand(sub)
			if tc.flag != "" {
				require.NoError(t, root.PersistentFlags().Set(verbosityFlag, tc.flag))
			}
			require.NoError(t, SetupLogging(sub, nil))
			assert.Equal(t, tc.want, logger.GetLevel())
		})
	}
}

// A bad level in the verbosity setting is reported, not obeyed and not fatal.
func TestSetupLoggingReportsABadSetting(t *testing.T) {
	keepLogger(t)
	var buf bytes.Buffer
	orig := logOutput
	logOutput = func() io.Writer { return &buf }
	t.Cleanup(func() { logOutput = orig })
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Setenv(verbosityEnv, "")
	require.NoError(t, config.CFG.Verbosity.SetHomeString("loud"))
	root := &cobra.Command{Use: "astro"}
	addVerbosityFlag(root.PersistentFlags())
	require.NoError(t, SetupLogging(root, nil))
	assert.Equal(t, logger.DefaultLevel, logger.GetLevel())
	assert.Equal(t, "Warning: ignoring the verbosity setting: \"loud\" is not a log level: use debug, info, warn or error\n", buf.String())
}

// A bad ASTRO_VERBOSITY fails the run as a bad --verbosity does: a usage error
// naming the variable, whatever the setting says.
func TestSetupLoggingRefusesABadEnvironmentLevel(t *testing.T) {
	keepLogger(t)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Setenv(verbosityEnv, "loud")
	require.NoError(t, config.CFG.Verbosity.SetHomeString("info"))
	root := &cobra.Command{Use: "astro"}
	addVerbosityFlag(root.PersistentFlags())
	err := SetupLogging(root, nil)
	require.ErrorContains(t, err, `ASTRO_VERBOSITY: "loud" is not a log level`)
	assert.True(t, cliout.IsUsage(err))
}

// A bad --verbosity fails the command line, as any bad flag value does.
func TestVerbosityFlagRefusesABadLevel(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	addVerbosityFlag(root.PersistentFlags())
	err := root.PersistentFlags().Set(verbosityFlag, "loud")
	require.ErrorContains(t, err, `"loud" is not a log level`)
}

// The commands that skip the rest of the pre-run (all of astro local, the
// proxy daemon) still set up logging: --verbosity used to do nothing on them.
func TestSkipPreRunCommandsSetUpLogging(t *testing.T) {
	keepLogger(t)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Setenv(verbosityEnv, "")
	root := buildTree(t, astroTree).root
	for _, path := range [][]string{{"local", "start"}, {"local", "status"}} {
		cmd, _, err := root.Find(path)
		require.NoError(t, err, path)
		require.Equal(t, "true", cmd.Annotations[telemetry.SkipPreRunAnnotation], cmd.CommandPath())

		logger.SetLevel(logger.LevelOff)
		require.NoError(t, root.PersistentFlags().Set(verbosityFlag, "debug"))
		require.NoError(t, root.PersistentPreRunE(cmd, nil), cmd.CommandPath())
		assert.Equal(t, slog.LevelDebug, logger.GetLevel(), cmd.CommandPath())
	}
}
