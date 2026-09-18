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
// conversion preview groups by them. leftovers built the .astro/config.yaml
// entry with filepath.Join, so on Windows that note said `.astro\config.yaml`
// while envschema.LegacyRelPath, a slash-form constant, said
// `.astro/env.schema.yaml` in the same list: one contract disagreeing with
// itself about two files in the same directory, and only on one platform.
//
// Asserted here rather than only end-to-end because this is a detail of the
// contract, and a unit test says so where the table is.
func TestLeftoverNamesUseForwardSlashes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".astro", "config.yaml"),
		[]byte("project:\n  name: orders\n"), 0o600))
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
