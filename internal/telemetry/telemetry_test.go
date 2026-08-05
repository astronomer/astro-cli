package telemetry

import (
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"

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
		assert.Contains(t, output, "usage data")
		assert.Contains(t, output, "linked to your Astro organization")
		assert.Contains(t, output, "astro telemetry disable")

		assert.Equal(t, noticeVersion, config.CFG.TelemetryNoticeShown.GetHomeString())
	})

	t.Run("does not print on subsequent calls", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configRaw := []byte("telemetry:\n  enabled: true\n  notice_shown: \"" + noticeVersion + "\"\n")
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

	// Users who accepted the v1 notice agreed to fully anonymous collection. The
	// v2 notice adds organization attribution, so they must be told once rather
	// than inheriting the change silently.
	t.Run("reprints when the accepted notice predates the current version", func(t *testing.T) {
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

		assert.Contains(t, string(out[:n]), "linked to your Astro organization")
		assert.Equal(t, noticeVersion, config.CFG.TelemetryNoticeShown.GetHomeString())
	})
}

// testJWT builds an unsigned JWT with the given expiry. currentToken only reads
// the exp claim, so the signature is irrelevant here.
func testJWT(exp time.Time) string {
	enc := func(v string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(v))
	}
	return enc(`{"alg":"RS256","typ":"JWT"}`) + "." +
		enc(fmt.Sprintf(`{"exp":%d,"sub":"test"}`, exp.Unix())) + ".sig"
}

func TestJWTExpiry(t *testing.T) {
	want := time.Now().Add(time.Hour).Truncate(time.Second)
	got, ok := jwtExpiry(testJWT(want))
	assert.True(t, ok)
	assert.Equal(t, want.Unix(), got.Unix())

	for _, bad := range []string{"", "not-a-jwt", "a.b", "a.b.c", "a.!!!.c"} {
		_, ok := jwtExpiry(bad)
		assert.False(t, ok, "expected %q to be unreadable", bad)
	}
}

func TestCurrentToken(t *testing.T) {
	origEnv := os.Getenv("ASTRO_API_TOKEN")
	defer os.Setenv("ASTRO_API_TOKEN", origEnv)

	// A context as `astro login` leaves it on the production domain: the token is
	// stored WITH its "Bearer " prefix, which currentToken must strip.
	withContext := func(t *testing.T, domain, token string) {
		t.Helper()
		fs := afero.NewMemMapFs()
		configRaw := []byte("context: prod\ncontexts:\n  prod:\n    domain: " + domain +
			"\n    token: \"Bearer " + token + "\"\n    organization: org-1\ntelemetry:\n  enabled: true\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)
	}

	t.Run("strips the stored Bearer prefix from the context token", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		live := testJWT(time.Now().Add(time.Hour))
		withContext(t, "astronomer.io", live)
		assert.Equal(t, live, currentToken())
	})

	// The gateway rejects the whole request on a bad token, so an event that
	// would have been recorded anonymously is lost instead. Never send one.
	t.Run("drops an expired token so the event still sends anonymously", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		withContext(t, "astronomer.io", testJWT(time.Now().Add(-time.Hour)))
		assert.Empty(t, currentToken())
	})

	// Expiring within the margin counts as expired: the sender is asynchronous.
	t.Run("drops a token expiring inside the freshness margin", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		withContext(t, "astronomer.io", testJWT(time.Now().Add(tokenFreshnessMargin/2)))
		assert.Empty(t, currentToken())
	})

	// Telemetry always posts to the production endpoint, which does not know
	// issuers from other environments.
	t.Run("drops a token from a non-production domain", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		withContext(t, "astronomer-dev.io", testJWT(time.Now().Add(time.Hour)))
		assert.Empty(t, currentToken())
	})

	t.Run("drops a token that is not a readable JWT", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		withContext(t, "astronomer.io", "not-a-jwt")
		assert.Empty(t, currentToken())
	})

	// A context that was never populated stores the bare prefix.
	t.Run("returns empty for a context holding only the Bearer prefix", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		fs := afero.NewMemMapFs()
		configRaw := []byte("context: prod\ncontexts:\n  prod:\n    domain: astronomer.io\n    token: \"Bearer \"\ntelemetry:\n  enabled: true\n")
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
		config.InitConfig(fs)
		assert.Empty(t, currentToken())
	})

	// ASTRO_API_TOKEN holds a raw token; the prefix is only added when persisted.
	t.Run("prefers ASTRO_API_TOKEN over the context token", func(t *testing.T) {
		withContext(t, "astronomer.io", testJWT(time.Now().Add(time.Hour)))
		envToken := testJWT(time.Now().Add(time.Hour))
		os.Setenv("ASTRO_API_TOKEN", envToken)
		assert.Equal(t, envToken, currentToken())
	})

	t.Run("drops an expired ASTRO_API_TOKEN", func(t *testing.T) {
		withContext(t, "astronomer.io", testJWT(time.Now().Add(time.Hour)))
		os.Setenv("ASTRO_API_TOKEN", testJWT(time.Now().Add(-time.Hour)))
		assert.Empty(t, currentToken())
	})

	// Logged-out users must not error or block the event — they just send
	// unattributed, which is the whole point of measuring them.
	t.Run("returns empty when there is no context and no env token", func(t *testing.T) {
		os.Setenv("ASTRO_API_TOKEN", "")
		initTestConfig(t)
		assert.Empty(t, currentToken())
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
