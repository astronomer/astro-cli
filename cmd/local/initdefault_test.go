package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// stubDefault answers init's default with series 3.50, which no fallback this
// binary carries is near, and counts the lookups.
func stubDefault(d *Deps, rp string, src runtimeversions.Source) *int {
	n := 0
	d.AirflowDefault = func(context.Context) (string, string, runtimeversions.Source) {
		n++
		return "3.50", rp, src
	}
	return &n
}

// The resolver's series and requires-python land in the manifest, and the
// output says where they came from. The series is written as the requirement
// alone: no [tool.astro] airflow key, which the manifest would refuse.
func TestInitPinsTheCatalogsDefault(t *testing.T) {
	d, dir, stdout := initDeps(t)
	stubDefault(&d, ">=3.13", runtimeversions.SourceCatalog)

	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	m := readManifest(t, dir)
	for _, want := range []string{"dependencies = ['apache-airflow==3.50.*']", "requires-python = '>=3.13'"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest lacks %s:\n%s", want, m)
		}
	}
	if strings.Contains(m, "airflow = ") {
		t.Errorf("init wrote a [tool.astro] airflow key:\n%s", m)
	}
	loaded, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	if err != nil {
		t.Fatalf("the scaffolded manifest does not load: %v", err)
	}
	if got := loaded.Airflow().Pin; got != "3.50" {
		t.Errorf("Airflow().Pin = %q, want the catalog's 3.50", got)
	}
	if out := stdout.String(); !strings.Contains(out, "(Airflow 3.50, the latest from the runtime catalog)") {
		t.Errorf("output does not say the version came from the catalog:\n%s", out)
	}
}

func TestInitSaysWhereTheDefaultCameFrom(t *testing.T) {
	for src, want := range map[runtimeversions.Source]string{
		runtimeversions.SourceCatalog:      "(Airflow 3.50, the latest from the runtime catalog)",
		runtimeversions.SourceCache:        "(Airflow 3.50, the latest in the cached runtime catalog)",
		runtimeversions.SourceStaleCache:   "(Airflow 3.50, the latest in an old cached copy of the runtime catalog; could not reach the catalog)",
		runtimeversions.SourceFallback:     "(Airflow 3.50, the built-in default; could not reach the runtime catalog)",
		runtimeversions.SourceCatalogEmpty: "(Airflow 3.50, the built-in default; the runtime catalog lists no usable Airflow 3 release)",
		runtimeversions.SourceBuiltIn:      "(Airflow 3.50, the built-in default)",
	} {
		t.Run(string(src), func(t *testing.T) {
			d, _, stdout := initDeps(t)
			stubDefault(&d, "", src)
			if err := execute(t, d, "init"); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			if out := stdout.String(); !strings.Contains(out, want) {
				t.Errorf("want %q in:\n%s", want, out)
			}
		})
	}
}

// With no lookup wired at all, init says only that the version is the built-in
// default, not that a catalog it never asked could not be reached.
func TestInitWithNoLookupSaysOnlyBuiltIn(t *testing.T) {
	d, _, stdout := initDeps(t)
	d.AirflowDefault = nil
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "(Airflow "+runtimeversions.FallbackAirflowSeries+", the built-in default) in") || strings.Contains(out, "runtime catalog") {
		t.Errorf("want a bare built-in default:\n%s", out)
	}
}

// --airflow-version is the deterministic override: no lookup, and no source
// named, because nothing was defaulted.
func TestInitWithAFlagMakesNoLookup(t *testing.T) {
	d, _, stdout := initDeps(t)
	lookups := stubDefault(&d, "", runtimeversions.SourceCatalog)

	if err := execute(t, d, "init", "--airflow-version", "3.1"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if *lookups != 0 {
		t.Errorf("init with --airflow-version looked up the default %d times", *lookups)
	}
	out := stdout.String()
	if !strings.Contains(out, "(Airflow 3.1) in") {
		t.Errorf("want a bare version with no source:\n%s", out)
	}
}

// Adopting a project that pins its own Airflow asks nobody anything.
func TestInitAdoptingAPinnedProjectMakesNoLookup(t *testing.T) {
	d, dir, _ := initDeps(t)
	lookups := stubDefault(&d, "", runtimeversions.SourceCatalog)
	existing := "[project]\nname = 'orders'\ndependencies = ['apache-airflow==3.1.*']\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if *lookups != 0 {
		t.Errorf("adopting a pinned project looked up the default %d times", *lookups)
	}
	if m := readManifest(t, dir); !strings.Contains(m, "'apache-airflow==3.1.*'") || strings.Contains(m, "3.50") || strings.Contains(m, "airflow = ") {
		t.Errorf("the project's own pin did not win, alone:\n%s", m)
	}
}

func TestInitJSONNamesTheDefaultSource(t *testing.T) {
	d, _, stdout := initDeps(t)
	stubDefault(&d, "", runtimeversions.SourceStaleCache)

	if err := execute(t, d, "init", "--output", "json"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	var payload struct {
		Airflow string `json:"airflow"`
		Source  string `json:"airflowDefaultSource"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	if payload.Airflow != "3.50" || payload.Source != "stale-cache" {
		t.Errorf("payload = %+v", payload)
	}
}

// The production resolver, end to end against a fake catalog: the request
// names this CLI, and an unreachable catalog still produces a project on the
// built-in series rather than a failed init.
func TestInitReadsTheCatalogAndSurvivesItsAbsence(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	agents := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.UserAgent()
		_, _ = w.Write([]byte(`{"runtimeVersionsV3": {"3.50-1": {"metadata": {"airflowVersion": "3.50.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.13"]}}}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(runtimeversions.URLEnv, srv.URL)

	d, dir, stdout := initDeps(t)
	d.AirflowDefault = catalogDefault
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	select {
	case agent := <-agents:
		if !strings.HasPrefix(agent, "astro-cli/") {
			t.Errorf("User-Agent = %q, want astro-cli/<version>", agent)
		}
	default:
		t.Error("init never asked the catalog")
	}
	if m := readManifest(t, dir); !strings.Contains(m, "'apache-airflow==3.50.*'") || !strings.Contains(m, "requires-python = '>=3.13'") {
		t.Errorf("the catalog's answer did not reach the manifest:\n%s", m)
	}
	if !strings.Contains(stdout.String(), "the latest from the runtime catalog") {
		t.Errorf("output:\n%s", stdout.String())
	}

	// Now offline, with a cache directory holding nothing.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(runtimeversions.URLEnv, "http://127.0.0.1:1/unreachable")
	d, dir, stdout = initDeps(t)
	d.AirflowDefault = catalogDefault
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init offline: %v", err)
	}
	if m := readManifest(t, dir); !strings.Contains(m, "'apache-airflow=="+runtimeversions.FallbackAirflowSeries+".*'") {
		t.Errorf("offline init did not pin the built-in series:\n%s", m)
	}
	if !strings.Contains(stdout.String(), "the built-in default; could not reach the runtime catalog") {
		t.Errorf("offline output:\n%s", stdout.String())
	}
}
