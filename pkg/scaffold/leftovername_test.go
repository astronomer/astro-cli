package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// A file name in a note reads the same on every platform.
//
// The names go into Result's json, which a consumer parses — the desktop's
// conversion preview groups by them. leftovers once built the
// .astro/config.yaml entry with filepath.Join, so on Windows that note said
// `.astro\config.yaml` while envschema.LegacyRelPath, a slash-form constant,
// said `.astro/env.schema.yaml` in the same list: one contract disagreeing with
// itself about two files in the same directory, and only on one platform.
//
// That entry is now concatenated from the same kind of constant, so this pins a
// property the current code holds by construction rather than one it computes.
// Kept deliberately: it is the only assertion that would catch a filepath.Join
// coming back, and .astro/config.yaml is still the one nested name leftovers
// emits, so it is still the only note that could carry a separator at all.
func TestLeftoverNamesUseForwardSlashes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	// A saved deploy target, because that is what puts this file in the list at
	// all. Without it the file is carried silently and there is no name to
	// check the spelling of.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".astro", "config.yaml"),
		[]byte("project:\n  name: orders\n  deployment: cm1orders\n"), 0o600))
	// Beside it, the file whose name has always been slash-form, so the two
	// are compared rather than asserted in isolation.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(envschema.LegacyRelPath)),
		[]byte("env_vars:\n  - key: API_URL\n"), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	var found string
	for _, n := range res.Notes {
		if strings.Contains(n, "config.yaml") {
			found = n
		}
	}
	require.NotEmpty(t, found, "expected a note about .astro/config.yaml, got: %v", res.Notes)
	require.True(t, strings.HasPrefix(found, ".astro/config.yaml:"),
		"a name in a note must read the same on every platform, got %q", found)
	require.NotContains(t, found, `\`,
		"a backslash in a reported name makes the json contract platform-specific: %q", found)
}
