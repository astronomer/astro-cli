package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	pkgmanifest "github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// pythonCatalogJSON has 3.3 builds that run Python 3.14 by default and a 3.2
// build that runs 3.13, all of which also ship 3.12.
const pythonCatalogJSON = `{
  "runtimeVersionsV3": {
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.2-10": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.13"}}
  }
}`

func pythonCatalog(t *testing.T) func() *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(pythonCatalogJSON))
	if err != nil {
		t.Fatal(err)
	}
	return func() *runtimeversions.Catalog { return c }
}

// projectWith writes a manifest pinning airflow and stating requiresPython
// (none when empty), with extra appended under [tool.astro].
func projectWith(t *testing.T, airflow, requiresPython, extra string) string {
	t.Helper()
	body := "[project]\nname = 'demo'\n"
	if requiresPython != "" {
		body += "requires-python = '" + requiresPython + "'\n"
	}
	body += "dependencies = ['apache-airflow==" + airflow + "']\n\n[tool.astro]\n" + extra
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return dir
}

func TestBuildPicksTheImagesPythonForAStandaloneVenv(t *testing.T) {
	for _, tc := range []struct {
		name, airflow, requires string
		catalog                 bool
		want                    string
	}{
		{name: "what astro init writes runs the build's default", airflow: "3.2.*", requires: ">=3.12", catalog: true, want: "3.13"},
		{name: "a pinned minor", airflow: "3.3.*", requires: "==3.13.*", catalog: true, want: "3.13"},
		{name: "offline, requires-python stated", airflow: "3.3.*", requires: "==3.13.*", want: ""},
		{name: "offline, none stated", airflow: "3.3.*", want: "3.12"},
		{name: "airflow 2", airflow: "2.10.*", requires: ">=3.9", catalog: true, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := projectWith(t, tc.airflow, tc.requires, "")
			opts := Options{}
			if tc.catalog {
				opts.PythonCatalog = pythonCatalog(t)
			}
			built, err := Build(dir, opts)
			if err != nil {
				t.Fatal(err)
			}
			if built.Plan.PythonVersion != tc.want {
				t.Errorf("Plan.PythonVersion = %q, want %q", built.Plan.PythonVersion, tc.want)
			}
			if built.PythonNote != "" {
				t.Errorf("PythonNote = %q, want none", built.PythonNote)
			}
		})
	}
}

// Docker mode picks the image's Python itself, so a docker-mode plan reads no
// catalog for a field it never uses.
func TestBuildReadsNoCatalogForADockerPlan(t *testing.T) {
	dir := projectWith(t, "3.3.*", "==3.13.*", "")
	calls := 0
	built, err := Build(dir, Options{Mode: localrt.ModeDocker, PythonCatalog: func() *runtimeversions.Catalog {
		calls++
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || built.Plan.PythonVersion != "" {
		t.Errorf("read the catalog %d times, PythonVersion %q; want neither", calls, built.Plan.PythonVersion)
	}
}

// A requires-python the build ships no Python for refuses an image, but not a
// venv: uv may still satisfy it, so the start goes on with a warning.
func TestBuildWarnsRatherThanRefusesAPythonNoBuildShips(t *testing.T) {
	dir := projectWith(t, "3.3.*", "==3.11.*", "")
	built, err := Build(dir, Options{PythonCatalog: pythonCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.PythonVersion != "" {
		t.Errorf("PythonVersion = %q, want empty: uv picks within requires-python", built.Plan.PythonVersion)
	}
	if !strings.Contains(built.PythonNote, "admits none of the Pythons runtime 3.3-8 ships (3.12, 3.13, 3.14)") {
		t.Errorf("PythonNote = %q, want it to name the build and its Pythons", built.PythonNote)
	}
}

// The image a Docker-mode start or a deploy builds and the venv a standalone
// start builds run the same Python for the same manifest, whenever the catalog
// lets either decide.
func TestStandaloneAndImageAgreeOnPython(t *testing.T) {
	catalog := pythonCatalog(t)
	for _, tc := range []struct{ airflow, runtime, requires string }{
		{"3.2.*", "", ">=3.12"},
		{"3.2.*", "", "==3.14.*"},
		{"3.3.*", "", ">=3.12"},
		{"3.3.*", "", "==3.13.*"},
		{"3.3.*", "", ">=3.12,<3.14"},
		{"3.3.*", "3.3-7", "==3.12.*"},
		{"3.3.*", "3.3-7", ">=3.10"},
	} {
		extra := ""
		if tc.runtime != "" {
			extra = "runtime = '" + tc.runtime + "'\n"
		}
		dir := projectWith(t, tc.airflow, tc.requires, extra)
		built, err := Build(dir, Options{PythonCatalog: catalog})
		if err != nil {
			t.Fatal(err)
		}
		m, err := pkgmanifest.Load(filepath.Join(dir, project.Marker))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := imagebuild.RuntimeImageForPython(m.Airflow().Pin, m.Airflow().Runtime, m.Project.RequiresPython, catalog)
		if err != nil {
			t.Fatal(err)
		}
		if got := imagePython(t, ref, m.Airflow().Pin, catalog()); got != built.Plan.PythonVersion {
			t.Errorf("%+v: the image %s runs %s, the venv %q", tc, ref, got, built.Plan.PythonVersion)
		}
	}
}

// imagePython is the Python an image reference runs: the one its -python-X.Y
// suffix names, or else the default of the build its tag serves.
func imagePython(t *testing.T, ref, pin string, c *runtimeversions.Catalog) string {
	t.Helper()
	tag := ref[strings.LastIndex(ref, ":")+1:]
	if _, python, ok := strings.Cut(tag, "-python-"); ok {
		return python
	}
	build := tag
	if series, _ := imagebuild.AirflowSeries(pin); tag == series {
		build, _ = c.NewestPublishedRuntimeFor(series)
	}
	r, ok := c.Runtime(build)
	if !ok {
		t.Fatalf("no build %s in the catalog for %s", build, ref)
	}
	return r.DefaultPythonVersion
}
