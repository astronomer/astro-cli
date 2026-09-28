//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `astro init` holds a project to the Airflow a deployment runs: Astronomer's
// build, from Astronomer's index, which differs from PyPI's release of the
// same number. uv reads the settings from the project's own pyproject.toml, so
// a plain `uv sync` installs the same build `astro local start` does.
//
// The build is picked as of the suite's exclude-newer date, like everything
// else uv resolves here, so a build published later does not move this case.
func TestInitHoldsAirflowToAstronomersBuild(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)

	data, err := os.ReadFile(filepath.Join(p.Dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{"'apache-airflow-core'", "'apache-airflow-task-sdk'", "apache-airflow==3.3.", "+astro.", "https://pip.astronomer.io/v2/", "explicit = true"} {
		if !strings.Contains(manifest, want) {
			t.Errorf("pyproject.toml lacks %q:\n%s", want, manifest)
		}
	}
	builds, err := filepath.Glob(filepath.Join(p.Dir, ".venv", "lib", "python*", "site-packages", "apache_airflow-*+astro.*.dist-info"))
	if err != nil || len(builds) != 1 {
		t.Errorf("uv sync installed no Astronomer build of Airflow: %v (%v)", builds, err)
	}
	if r := p.run("local", "check").requireSuccess(); !strings.Contains(r.Stdout, "checks passed") {
		t.Errorf("the check does not pass against the build\n%s", r.output())
	}
}
