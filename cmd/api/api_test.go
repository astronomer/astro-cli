package api

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRootWithAPI builds a minimal root -> api command tree for testing.
// The root command's PersistentPreRunE is set to rootHook.
func newRootWithAPI(rootHook func(cmd *cobra.Command, args []string) error) *cobra.Command {
	root := &cobra.Command{
		Use:               "astro",
		PersistentPreRunE: rootHook,
	}
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))
	root.AddCommand(apiCmd)
	return apiCmd
}

func TestPersistentPreRunE_CallsRootHook(t *testing.T) {
	called := false
	apiCmd := newRootWithAPI(func(cmd *cobra.Command, args []string) error {
		called = true
		return nil
	})

	child := &cobra.Command{Use: "child"}
	apiCmd.AddCommand(child)

	err := apiCmd.PersistentPreRunE(child, []string{})
	require.NoError(t, err)
	assert.True(t, called, "root PersistentPreRunE should be invoked")
}

func TestPersistentPreRunE_RootHookErrorPropagates(t *testing.T) {
	rootErr := errors.New("token refresh failed")
	apiCmd := newRootWithAPI(func(cmd *cobra.Command, args []string) error {
		return rootErr
	})

	child := &cobra.Command{Use: "child"}
	apiCmd.AddCommand(child)

	err := apiCmd.PersistentPreRunE(child, []string{})
	assert.ErrorIs(t, err, rootErr)
}

func TestPersistentPreRunE_NoRootHook(t *testing.T) {
	// Root has no PersistentPreRunE -- should not panic or error.
	root := &cobra.Command{Use: "astro"}
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))
	root.AddCommand(apiCmd)

	child := &cobra.Command{Use: "child"}
	apiCmd.AddCommand(child)

	err := apiCmd.PersistentPreRunE(child, []string{})
	assert.NoError(t, err)
}

func TestNewAPICmdWithOutput_SubcommandRegistration(t *testing.T) {
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))

	names := make([]string, 0, len(apiCmd.Commands()))
	for _, sub := range apiCmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "cloud")
	assert.Contains(t, names, "airflow")
}

func TestNewAPICmdWithOutput_Flags(t *testing.T) {
	// --no-color is the root's global flag, not one of `astro api`'s own.
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))
	assert.Nil(t, apiCmd.PersistentFlags().Lookup("no-color"))
}

// `astro api` is a group with no run of its own: run bare, cobra prints its
// help and exits 0, and no pre-run (token refresh, telemetry) happens.
func TestBareAPIPrintsHelpAndRunsNothing(t *testing.T) {
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))
	assert.False(t, apiCmd.Runnable(), "astro api has a run of its own")

	preRuns := 0
	root := &cobra.Command{
		Use:               "astro",
		PersistentPreRunE: func(*cobra.Command, []string) error { preRuns++; return nil },
	}
	root.AddCommand(apiCmd)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"api"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "Make authenticated HTTP requests to Astronomer APIs")
	assert.Contains(t, out.String(), "Available Commands:")
	assert.Zero(t, preRuns)
}
