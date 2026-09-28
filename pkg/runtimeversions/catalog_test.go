package runtimeversions

import (
	"os"
	"slices"
	"testing"
	"time"
)

// The fixture is cut from the live catalog, plus three synthetic Airflow 3
// decoys (a future release, a yanked build, a deprecated one) that must never be
// chosen. Its _fixture key says which entries are which.
const fixturePath = "testdata/catalog.json"

// Every expected value in these tests is a literal read off the catalog by a
// person, never something computed from the fixture by the code under test.

func loadFixture(t *testing.T) *Catalog {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(%s): %v", fixturePath, err)
	}
	return c
}

// standOn fixes the release-date rule's clock at the first instant of day, UTC,
// so a build released that day counts only because the date is inclusive.
func standOn(t *testing.T, day string) {
	t.Helper()
	d, err := time.Parse(releaseDateLayout, day)
	if err != nil {
		t.Fatal(err)
	}
	prev := now
	now = func() time.Time { return d }
	t.Cleanup(func() { now = prev })
}

func parse(t *testing.T, body string) *Catalog {
	t.Helper()
	c, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return c
}

func TestLatestAirflowSeriesOnTheRecordedCatalog(t *testing.T) {
	standOn(t, "2026-09-25")
	c := loadFixture(t)

	// 3.4-1 is dated 2099, 3.5-1 is yanked and 3.6-1 deprecated: each would win
	// by version if its check were missing.
	if got, ok := c.LatestAirflowSeries("3"); !ok || got != "3.3" {
		t.Errorf(`LatestAirflowSeries("3") = %q, %v; want "3.3"`, got, ok)
	}
	// The Airflow 2 map is its own generation and never answers for 3, and the
	// Airflow 3 map never answers for 2.
	if got, ok := c.LatestAirflowSeries("2"); !ok || got != "2.11" {
		t.Errorf(`LatestAirflowSeries("2") = %q, %v; want "2.11"`, got, ok)
	}
	if got, ok := c.LatestAirflowSeries("4"); ok {
		t.Errorf(`LatestAirflowSeries("4") = %q; want none`, got)
	}
}

// 3.3-1 shipped on 2026-07-09. The day before, 3.3 is not released and 3.2 is
// the newest; on the day it is, because the date is inclusive.
func TestLatestAirflowSeriesWaitsForTheReleaseDate(t *testing.T) {
	c := loadFixture(t)

	standOn(t, "2026-07-08")
	if got, _ := c.LatestAirflowSeries("3"); got != "3.2" {
		t.Errorf("on 2026-07-08: %q, want 3.2", got)
	}
	standOn(t, "2026-07-09")
	if got, _ := c.LatestAirflowSeries("3"); got != "3.3" {
		t.Errorf("on 2026-07-09: %q, want 3.3", got)
	}
}

func TestLatestAirflowSeriesRule(t *testing.T) {
	standOn(t, "2026-09-25")
	cases := []struct {
		name, body, want string
	}{
		{
			// 3.0-18 is the most recently released build, and it is the
			// oldest series. Version decides, not date.
			name: "a later-dated backport does not win",
			body: `{"runtimeVersionsV3": {
				"3.0-18": {"metadata": {"airflowVersion": "3.0.6", "channel": "stable", "releaseDate": "2026-09-15"}},
				"3.3-1":  {"metadata": {"airflowVersion": "3.3.0", "channel": "stable", "releaseDate": "2026-07-09"}}}}`,
			want: "3.3",
		},
		{
			name: "series order is numeric, so 3.10 is above 3.9",
			body: `{"runtimeVersionsV3": {
				"3.9-4":  {"metadata": {"airflowVersion": "3.9.1", "channel": "stable", "releaseDate": "2026-01-01"}},
				"3.10-1": {"metadata": {"airflowVersion": "3.10.0", "channel": "stable", "releaseDate": "2026-01-01"}}}}`,
			want: "3.10",
		},
		{
			name: "one yanked build does not disqualify its series",
			body: `{"runtimeVersionsV3": {
				"3.1-21": {"metadata": {"airflowVersion": "3.1.8", "channel": "stable", "releaseDate": "2026-09-10"}},
				"3.2-1":  {"metadata": {"airflowVersion": "3.2.0", "channel": "stable", "releaseDate": "2026-04-14", "yanked": true}},
				"3.2-2":  {"metadata": {"airflowVersion": "3.2.0", "channel": "stable", "releaseDate": "2026-04-16"}}}}`,
			want: "3.2",
		},
		{
			name: "a series whose only build is yanked is skipped",
			body: `{"runtimeVersionsV3": {
				"3.1-21": {"metadata": {"airflowVersion": "3.1.8", "channel": "stable", "releaseDate": "2026-09-10"}},
				"3.2-1":  {"metadata": {"airflowVersion": "3.2.0", "channel": "stable", "releaseDate": "2026-04-14", "yanked": true}}}}`,
			want: "3.1",
		},
		{
			name: "a deprecated series is skipped",
			body: `{"runtimeVersionsV3": {
				"3.1-21": {"metadata": {"airflowVersion": "3.1.8", "channel": "stable", "releaseDate": "2026-09-10"}},
				"3.2-2":  {"metadata": {"airflowVersion": "3.2.0", "channel": "deprecated", "releaseDate": "2026-04-16"}}}}`,
			want: "3.1",
		},
		{
			name: "a build with no release date has not shipped",
			body: `{"runtimeVersionsV3": {
				"3.1-21": {"metadata": {"airflowVersion": "3.1.8", "channel": "stable", "releaseDate": "2026-09-10"}},
				"3.2-2":  {"metadata": {"airflowVersion": "3.2.0", "channel": "stable"}}}}`,
			want: "3.1",
		},
		{
			name: "Airflow 2 runtimes never answer for Airflow 3",
			body: `{"runtimeVersions": {
				"13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}}}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parse(t, tc.body).LatestAirflowSeries("3")
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("LatestAirflowSeries = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}
}

func TestRequiresPython(t *testing.T) {
	standOn(t, "2026-09-25")
	c := loadFixture(t)
	for series, want := range map[string]string{
		"3.3": ">=3.12",
		"3.2": ">=3.12",
		"3.1": ">=3.11",
		"3.0": ">=3.11",
	} {
		if got, ok := c.RequiresPython(series); !ok || got != want {
			t.Errorf("RequiresPython(%q) = %q, %v; want %q", series, got, ok, want)
		}
	}
	// Airflow 2 builds list no Python, and 3.9 is not in the catalog: both are
	// left to the caller's built-in rule.
	for _, series := range []string{"2.11", "3.9"} {
		if got, ok := c.RequiresPython(series); ok {
			t.Errorf("RequiresPython(%q) = %q; want none", series, got)
		}
	}
	if got, _ := c.PythonVersions("3.3"); !slices.Equal(got, []string{"3.12", "3.13", "3.14"}) {
		t.Errorf("PythonVersions(3.3) = %v", got)
	}
}

// Only the lowest listed version is read, whatever the order and whatever is
// missing between it and the highest. No upper bound is ever written.
func TestRequiresPythonReadsOnlyTheLowest(t *testing.T) {
	standOn(t, "2026-09-25")
	c := parse(t, `{"runtimeVersionsV3": {
		"3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-23",
		          "pythonVersions": ["3.14", "3.11", "3.13", "next"]}}}}`)
	if got, _ := c.RequiresPython("3.3"); got != ">=3.11" {
		t.Errorf("RequiresPython = %q, want >=3.11", got)
	}
}

// The Python list comes from the series' newest build, so a series that moved
// its floor up reports the new floor.
func TestPythonVersionsFollowTheNewestBuild(t *testing.T) {
	standOn(t, "2026-09-25")
	c := parse(t, `{"runtimeVersionsV3": {
		"3.3-9":  {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-01", "pythonVersions": ["3.11", "3.12"]}},
		"3.3-10": {"metadata": {"airflowVersion": "3.3.3", "channel": "stable", "releaseDate": "2026-09-02", "pythonVersions": ["3.12", "3.13"]}}}}`)
	if got, _ := c.RequiresPython("3.3"); got != ">=3.12" {
		t.Errorf("RequiresPython = %q, want >=3.12 from 3.3-10", got)
	}
}

func TestNewestRuntimeFor(t *testing.T) {
	c := loadFixture(t)
	for pin, want := range map[string]string{
		"2.11.2": "13.11.0",
		"2.11":   "13.11.0",
		"2":      "13.11.0",
		"2.10.5": "12.12.0", // deprecated, still an image to run
		"3.3":    "3.3-8",
		"3.2.0":  "3.2-2", // 3.2-1 is yanked
	} {
		if got, ok := c.NewestRuntimeFor(pin); !ok || got != want {
			t.Errorf("NewestRuntimeFor(%q) = %q, %v; want %q", pin, got, ok, want)
		}
	}
	// 13.5.0 is the only runtime carrying 2.11.1, and it is yanked.
	for _, pin := range []string{"2.11.1", "2.8", ""} {
		if got, ok := c.NewestRuntimeFor(pin); ok {
			t.Errorf("NewestRuntimeFor(%q) = %q; want none", pin, got)
		}
	}
}

func TestAirflowFor(t *testing.T) {
	c := loadFixture(t)
	for _, tc := range []struct{ pin, runtime, want string }{
		{"3.3", "", "3.3.2"},
		{"3.2.0", "", "3.2.0"},
		{"2.11", "", "2.11.2"},
		{"3.3", "3.2-1", "3.2.0"},
	} {
		if got, ok := c.AirflowFor(tc.pin, tc.runtime); !ok || got != tc.want {
			t.Errorf("AirflowFor(%q, %q) = %q, %v; want %q", tc.pin, tc.runtime, got, ok, tc.want)
		}
	}
	for _, tc := range []struct{ pin, runtime string }{{"2.8", ""}, {"3.3", "3.9-1"}} {
		if got, ok := c.AirflowFor(tc.pin, tc.runtime); ok {
			t.Errorf("AirflowFor(%q, %q) = %q; want none", tc.pin, tc.runtime, got)
		}
	}
}

func TestRuntimeTagOrderIsNumeric(t *testing.T) {
	c := parse(t, `{"runtimeVersionsV3": {
		"3.3-8":  {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}},
		"3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}}}}`)
	if got, _ := c.NewestRuntimeFor("3.3"); got != "3.3-10" {
		t.Errorf("NewestRuntimeFor = %q, want 3.3-10", got)
	}
}

func TestRuntimeLookup(t *testing.T) {
	c := loadFixture(t)
	r, ok := c.Runtime("3.2-1")
	if !ok || r.AirflowVersion != "3.2.0" || !r.Yanked || r.YankedReason == "" {
		t.Errorf("Runtime(3.2-1) = %+v, %v", r, ok)
	}
	if _, ok := c.Runtime("3.9-1"); ok {
		t.Error("Runtime(3.9-1) found a build the catalog does not list")
	}
}

func TestParseRefusesACatalogWithNoRuntimes(t *testing.T) {
	for _, body := range []string{`{}`, `{"runtimeVersions": {}, "runtimeVersionsV3": {}}`} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("Parse(%s) accepted a catalog with no runtimes", body)
		}
	}
	if _, err := Parse([]byte(`{"runtimeVersions": `)); err == nil {
		t.Error("Parse accepted truncated JSON")
	}
}
