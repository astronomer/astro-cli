package local

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// With no home directory for the proxy routes there is no runtime, and every
// command that needs one says why before doing anything: as an error, not a
// usage error, and never as a success.
func TestCommandsNeedingTheRuntimeFailWithoutAHome(t *testing.T) {
	noHome := errors.New("astro local needs a home directory to keep its proxy routes: set HOME or ASTRO_HOME: no home")
	for _, args := range [][]string{
		{"local", "start"},
		{"local", "status"},
		{"local", "upgrade", "airflow"},
		{"start"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, stdout := testDeps(t)
			d.Runtime = nil
			d.NoHome = noHome

			err := execute(t, d, args...)

			require.ErrorIs(t, err, noHome)
			assert.False(t, cliout.IsUsage(err), "a missing home is not a usage error")
			assert.Empty(t, stdout.String(), "nothing ran before the error")
		})
	}
}

// With a home, nothing changes: requireHome leaves every command's own RunE in
// place. (Every other test in this package runs with a home, too.)
func TestRequireHomeWithAHomeKeepsTheCommand(t *testing.T) {
	ran := false
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { ran = true; return nil }}
	group := &cobra.Command{Use: "group"}
	group.AddCommand(leaf)

	requireHome(nil, group)

	require.NoError(t, leaf.RunE(leaf, nil))
	assert.True(t, ran)
}
