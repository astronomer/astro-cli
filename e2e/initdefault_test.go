//go:build e2e

package e2e

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fallbackSeries reads runtimeversions.FallbackAirflowSeries out of its source
// file, as data. The suite imports no astro-cli package (see go.mod), and the
// constant moves by a weekly PR, so a literal here would go stale on the first
// bump. A Windows checkout carries CRLF line endings, hence the optional \r.
func fallbackSeries(t *testing.T) string {
	t.Helper()
	src := read(t, filepath.Join("..", "pkg", "runtimeversions", "default.go"))
	m := regexp.MustCompile(`(?m)^const FallbackAirflowSeries = "([^"]+)"\r?$`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("no FallbackAirflowSeries constant in pkg/runtimeversions/default.go")
	}
	return m[1]
}

// Offline, which is every case in this suite unless it says otherwise, init
// still makes a project: on the built-in series, and saying so.
func TestInitOfflinePinsTheBuiltInSeries(t *testing.T) {
	tier(t, 0)
	series := fallbackSeries(t)

	p := newProject(t)
	r := p.run("init", "--name", "demo").requireSuccess()

	if want := "(Airflow " + series + ", the built-in default; could not reach the runtime catalog)"; !strings.Contains(r.Stdout, want) {
		t.Errorf("want %q in:\n%s", want, r.output())
	}
	if m := read(t, filepath.Join(p.Dir, "pyproject.toml")); !strings.Contains(m, "apache-airflow=="+series+".*") {
		t.Errorf("the manifest does not pin the built-in series %s:\n%s", series, m)
	}
}

// With a catalog to read, init pins its newest supported series and the
// requires-python its runtime ships, and names the catalog as the source. The
// catalog serves 3.50, far from any real series, so the answer cannot be the
// built-in one by coincidence.
func TestInitPinsTheCatalogsSeries(t *testing.T) {
	tier(t, 0)
	agents := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.UserAgent()
		_, _ = w.Write([]byte(`{"runtimeVersionsV3": {
			"3.50-1": {"metadata": {"airflowVersion": "3.50.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.13", "3.14"]}},
			"3.51-1": {"metadata": {"airflowVersion": "3.51.0", "channel": "stable", "releaseDate": "2026-01-01", "yanked": true, "pythonVersions": ["3.15"]}}}}`))
	}))
	t.Cleanup(srv.Close)

	p := newProject(t)
	p.catalogURL = srv.URL
	r := p.run("init", "--name", "demo").requireSuccess()

	if !strings.Contains(r.Stdout, "(Airflow 3.50, the latest from the runtime catalog)") {
		t.Errorf("init did not report the catalog's series:\n%s", r.output())
	}
	m := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	for _, want := range []string{"apache-airflow==3.50.*", "requires-python = '>=3.13'"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest lacks %s:\n%s", want, m)
		}
	}
	// The series is the requirement alone: a [tool.astro] airflow key beside it
	// would stop the manifest loading.
	if regexp.MustCompile(`(?m)^airflow\s*=`).MatchString(m) {
		t.Errorf("init wrote a [tool.astro] airflow key:\n%s", m)
	}
	select {
	case ua := <-agents:
		if !strings.HasPrefix(ua, "astro-cli/") {
			t.Errorf("User-Agent = %q, want astro-cli/<version>", ua)
		}
	default:
		t.Error("init never asked the catalog")
	}

	// A second init under the same cache reads the copy the first one wrote,
	// and asks nobody.
	q := p.sibling("second")
	q.catalogURL = srv.URL
	r = q.run("init", "--name", "second").requireSuccess()
	if !strings.Contains(r.Stdout, "(Airflow 3.50, the latest in the cached runtime catalog)") {
		t.Errorf("the second init did not use the cache:\n%s", r.output())
	}
	if len(agents) != 0 {
		t.Errorf("the second init asked the catalog again")
	}
}

// --airflow-version is the deterministic override and needs no catalog.
func TestInitWithAnAirflowVersionNamesNoSource(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	r := p.run("init", "--name", "demo", "--airflow-version", "3.1").requireSuccess()
	if !strings.Contains(r.Stdout, "(Airflow 3.1) in") || strings.Contains(r.Stdout, "runtime catalog") {
		t.Errorf("want the flag's version and no source:\n%s", r.output())
	}
}
