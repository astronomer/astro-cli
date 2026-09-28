//go:build !windows

package localstandalone

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

const astroEnvironments = `environments = ["sys_platform == 'linux'", "sys_platform == 'darwin' and platform_machine == 'arm64'"]`

func platformProject(t *testing.T, environments string) string {
	t.Helper()
	dir := t.TempDir()
	src := "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n\n[tool.uv]\n" + environments + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(src), 0o600))
	return dir
}

func TestCheckPlatform(t *testing.T) {
	for _, tc := range []struct {
		name, environments, goos, goarch string
		refused                          bool
	}{
		{"Apple silicon", astroEnvironments, "darwin", "arm64", false},
		{"Linux", astroEnvironments, "linux", "amd64", false},
		{"Linux on ARM", astroEnvironments, "linux", "arm64", false},
		{"Intel macOS", astroEnvironments, "darwin", "amd64", true},
		{"Intel macOS added back", `environments = ["sys_platform == 'darwin' and platform_machine == 'x86_64'"]`, "darwin", "amd64", false},
		{"a marker it cannot read", `environments = ["sys_platform == 'linux' or platform_machine == 'x86_64'"]`, "darwin", "amd64", false},
		{"no environments", "", "darwin", "amd64", false},
		{"excluded with !=", `environments = ["platform_machine != 'x86_64'"]`, "darwin", "amd64", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkPlatform(platformProject(t, tc.environments), tc.goos, tc.goarch)
			if !tc.refused {
				assert.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrPlatformExcluded)
			assert.Contains(t, err.Error(), "astro local start --docker")
			assert.Contains(t, err.Error(), "platform_machine == 'x86_64'")
		})
	}
	assert.NoError(t, checkPlatform(t.TempDir(), "darwin", "amd64"), "no manifest is plan.Build's to report")
}

// The refusal comes before uv, whose own message names neither the setting
// nor what to do instead.
func TestAnExcludedPlatformIsRefusedBeforeUV(t *testing.T) {
	e, _, _ := testEngine(t)
	e.goos, e.goarch = "darwin", "amd64"
	e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
		return syncerFunc(func(context.Context, string, string, uv.Stdio) error {
			t.Error("uv ran for a platform the project excludes")
			return nil
		}), nil
	}

	err := e.Sync(t.Context(), rt.Plan{ProjectPath: platformProject(t, astroEnvironments), Mode: rt.ModeStandalone}, rt.Callbacks{})

	require.ErrorIs(t, err, ErrPlatformExcluded)
	assert.Contains(t, err.Error(), "Intel macOS")
}
