package runtimeversions

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// checkCatalog is a small catalog for the runtime checks: an Airflow 3 series
// whose builds carry two patches, a yanked build with a reason, and two
// Airflow 2 lines.
const checkCatalog = `{
  "runtimeVersions": {
    "12.9.0": {"metadata": {"airflowVersion": "2.10.5", "channel": "stable", "releaseDate": "2025-06-01"}},
    "13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}
  },
  "runtimeVersionsV3": {
    "3.3-5": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-08-01"}},
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-23"}},
    "3.2-1": {"metadata": {"airflowVersion": "3.2.0", "channel": "stable", "releaseDate": "2026-04-14",
              "yanked": true, "yankedReason": "This version has issues with environment manager connections not being found."}}
  }
}`

func kinds(fs []Finding) []FindingKind {
	var out []FindingKind
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

func TestCheckRuntime(t *testing.T) {
	c := parse(t, checkCatalog)
	cases := []struct {
		name     string
		runtime  string
		pin      string
		want     []FindingKind
		blocking bool
		mention  []string
	}{
		{name: "a build the series pin covers", runtime: "3.3-8", pin: "3.3"},
		{name: "a build carrying the exact pin", runtime: "3.3-5", pin: "3.3.1"},
		{
			name: "a build whose patch the exact pin excludes", runtime: "3.3-8", pin: "3.3.1",
			want: []FindingKind{FindingAirflowExcluded}, mention: []string{"3.3-8", "3.3.2", "3.3.1"},
		},
		{
			name: "a yanked build", runtime: "3.2-1", pin: "3.2",
			want: []FindingKind{FindingYanked}, mention: []string{"3.2-1", "environment manager connections not being found"},
		},
		{name: "an airflow 2 build of the pinned series", runtime: "13.11.0", pin: "2.11"},
		{name: "an airflow 2 build under a generation pin", runtime: "13.11.0", pin: "2"},
		{
			name: "an airflow 2 build of another series", runtime: "13.11.0", pin: "2.10",
			want: []FindingKind{FindingSeriesMismatch}, blocking: true,
			mention: []string{"13.11.0", "2.11.2", "2.10", "12.9.0"},
		},
		{
			name: "an airflow 2 build whose patch the exact pin excludes", runtime: "13.11.0", pin: "2.11.1",
			want: []FindingKind{FindingAirflowExcluded},
		},
		{
			name: "a build the catalog does not list", runtime: "3.3-99", pin: "3.3",
			want: []FindingKind{FindingUnknown}, mention: []string{"3.3-99"},
		},
		{name: "no runtime", runtime: "", pin: "3.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.CheckRuntime(tc.runtime, tc.pin)
			if strings.Join(asStrings(kinds(got)), ",") != strings.Join(asStrings(tc.want), ",") {
				t.Fatalf("findings = %+v, want kinds %v", got, tc.want)
			}
			for _, f := range got {
				if f.Blocking != tc.blocking {
					t.Errorf("%s blocking = %v, want %v", f.Kind, f.Blocking, tc.blocking)
				}
				for _, s := range tc.mention {
					if !strings.Contains(f.Message, s) {
						t.Errorf("message %q does not name %q", f.Message, s)
					}
				}
			}
		})
	}
}

func asStrings(ks []FindingKind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = string(k)
	}
	return out
}

// Offline, an Airflow 2 build proceeds and says its series went unchecked; an
// Airflow 3 build's series was already read off its tag, so it says nothing.
func TestCheckRuntimeWithNoCatalog(t *testing.T) {
	var c *Catalog
	got := c.CheckRuntime("13.11.0", "2.10")
	if len(got) != 1 || got[0].Kind != FindingSkipped || got[0].Blocking {
		t.Errorf("airflow 2 offline = %+v, want one non-blocking %s", got, FindingSkipped)
	}
	if got := c.CheckRuntime("3.3-8", "3.3.1"); len(got) != 0 {
		t.Errorf("airflow 3 offline = %+v, want nothing", got)
	}
}

func TestCheckRuntimeLoadsTheCatalog(t *testing.T) {
	serve(t, checkCatalog)

	warnings, err := CheckRuntime(t.Context(), Options{CacheDir: t.TempDir()}, "13.11.0", "2.10")
	var re *RuntimeError
	if !errors.As(err, &re) || re.Finding.Kind != FindingSeriesMismatch {
		t.Fatalf("err = %v, want a %s RuntimeError", err, FindingSeriesMismatch)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %+v, want none beside the blocking finding", warnings)
	}

	warnings, err = CheckRuntime(t.Context(), Options{CacheDir: t.TempDir()}, "3.3-8", "3.3.1")
	if err != nil || len(warnings) != 1 || warnings[0].Kind != FindingAirflowExcluded {
		t.Errorf("patch excluded = %+v, %v; want one warning", warnings, err)
	}
}

func TestCheckRuntimeOffline(t *testing.T) {
	s := serve(t, "")
	s.status = http.StatusNotFound

	warnings, err := CheckRuntime(t.Context(), Options{CacheDir: t.TempDir()}, "13.11.0", "2.10")
	if err != nil {
		t.Fatalf("offline check failed the build: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != FindingSkipped {
		t.Errorf("warnings = %+v, want the skipped note", warnings)
	}
}

// No runtime is the common case, and it costs no request.
func TestCheckRuntimeWithoutARuntimeAsksNobody(t *testing.T) {
	s := serve(t, checkCatalog)
	if warnings, err := CheckRuntime(t.Context(), Options{}, "", "3.3"); err != nil || warnings != nil {
		t.Errorf("CheckRuntime = %+v, %v", warnings, err)
	}
	if n := s.callCount(); n != 0 {
		t.Errorf("made %d requests for a manifest with no runtime", n)
	}
}
