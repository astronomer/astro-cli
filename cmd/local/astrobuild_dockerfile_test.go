package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// The Astronomer build of Airflow a project with its own Dockerfile locks is
// the one its FROM build carries: runtime 12.6.0 runs 2.10.4, so the lookup is
// asked about 12.6.0, not about the newest build of the pin, which for a 2.10
// pin can carry a newer patch than the image. A floating FROM names no build,
// and the lookup goes by the pin as before.
func TestTheAstroBuildLookupAsksAboutTheDockerfilesBuild(t *testing.T) {
	for name, tc := range map[string]struct{ from, want string }{
		"an exact Airflow 2 build": {"quay.io/astronomer/astro-runtime:12.6.0-base", "12.6.0"},
		"an exact Airflow 3 build": {"astrocrpublic.azurecr.io/runtime:3.3-11-base", "3.3-11"},
		"a floating tag":           {"quay.io/astronomer/astro-runtime:12-base", ""},
		"a non-runtime base":       {"python:3.12-slim", ""},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := testDeps(t)
			dir := t.TempDir()
			pin := "2.10.*"
			if tc.want == "3.3-11" {
				pin = "3.3.*"
			}
			files := map[string]string{
				"pyproject.toml": "[project]\nname = 'demo'\ndependencies = ['apache-airflow==" + pin + "']\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n",
				"Dockerfile":     "FROM " + tc.from + "\nRUN echo hi\n",
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var asked []string
			d.AstroBuild = func(_ context.Context, _, runtime string) (runtimeversions.AstroBuild, error) {
				asked = append(asked, runtime)
				return runtimeversions.AstroBuild{}, runtimeversions.ErrNoAstroBuild
			}
			c := &cli{d: d}
			c.writeAstroBuild(context.Background(), dir)
			if len(asked) != 1 || asked[0] != tc.want {
				t.Errorf("lookup asked about runtime %q, want %q", asked, tc.want)
			}
		})
	}

	// [tool.astro] runtime, where there is no Dockerfile, still decides.
	t.Run("a named runtime", func(t *testing.T) {
		d, _ := testDeps(t)
		dir := t.TempDir()
		src := "[project]\nname = 'demo'\ndependencies = ['apache-airflow==2.10.*']\n\n[tool.astro]\nruntime = '12.6.0'\n"
		if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		var asked []string
		d.AstroBuild = func(_ context.Context, _, runtime string) (runtimeversions.AstroBuild, error) {
			asked = append(asked, runtime)
			return runtimeversions.AstroBuild{}, runtimeversions.ErrNoAstroBuild
		}
		(&cli{d: d}).writeAstroBuild(context.Background(), dir)
		if len(asked) != 1 || asked[0] != "12.6.0" {
			t.Errorf("lookup asked about runtime %q, want 12.6.0", asked)
		}
	})
}
