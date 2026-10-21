package runtimeversions

import "testing"

func TestImagePythonOf(t *testing.T) {
	c := parse(t, `{"runtimeVersionsV3": {
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.14"], "defaultPythonVersion": "3.14"}}
  }, "runtimeVersions": {
    "12.6.0": {"metadata": {"airflowVersion": "2.10.4", "channel": "deprecated"}}
  }}`)
	for _, tc := range []struct {
		catalog *Catalog
		build   string
		want    string
	}{
		{c, "3.3-9", "3.14"},
		{nil, "3.3-9", ""},
		{c, "12.6.0", "3.12"},
		{nil, "13.0.0", "3.12"},
		{nil, "11.4.0", "3.11"},
		{nil, "nonsense", ""},
	} {
		if got := ImagePythonOf(tc.catalog, tc.build); got != tc.want {
			t.Errorf("ImagePythonOf(%v, %q) = %q, want %q", tc.catalog != nil, tc.build, got, tc.want)
		}
	}
}

// An Astronomer build of Airflow pinned in constraint-dependencies is covered
// while the pin admits its release; other lines, and none, are no
// disagreement.
func TestAstroBuildCovered(t *testing.T) {
	for _, tt := range []struct {
		pin         string
		constraints []string
		want        bool
	}{
		{"3.3", []string{"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1"}, true},
		{"3.3.2", []string{"Apache_Airflow==3.3.2+astro.2"}, true},
		{"3.4", []string{"apache-airflow==3.3.2+astro.2"}, false},
		{"2.11", []string{"apache-airflow==2.10.4+astro.1"}, false},
		{"3.4", []string{"apache-airflow==3.3.2", "sqlalchemy<2.1"}, true},
		{"3.4", nil, true},
	} {
		if got := AstroBuildCovered(tt.pin, tt.constraints); got != tt.want {
			t.Errorf("AstroBuildCovered(%q, %q) = %v, want %v", tt.pin, tt.constraints, got, tt.want)
		}
	}
}

// A newer build is the newest published for the pin, when it is newer than
// the one given; none when that is already the newest, or newer than any.
func TestNewerBuildFor(t *testing.T) {
	standOn(t, "2026-10-04")
	c := parse(t, `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}},
    "3.3-11": {"metadata": {"airflowVersion": "3.3.3", "channel": "stable", "releaseDate": "2026-10-05"}}
  }}`)
	for _, tt := range []struct{ pin, build, want string }{
		{"3.3", "3.3-8", "3.3-10"},
		{"3.3", "3.3-10", ""},
		{"3.3", "3.3-12", ""},
	} {
		got, ok := c.NewerBuildFor(tt.pin, tt.build)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("NewerBuildFor(%q, %q) = %q, %v; want %q", tt.pin, tt.build, got, ok, tt.want)
		}
	}
}

func TestNewestPublishedRuntimeTagged(t *testing.T) {
	standOn(t, "2026-10-04")
	c := parse(t, `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}},
    "3.3-11": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-10-05"}},
    "3.30-1": {"metadata": {"airflowVersion": "3.30.0", "channel": "stable"}},
    "3.0-2": {"metadata": {"airflowVersion": "3.0.2", "channel": "stable"}}
  }, "runtimeVersions": {
    "3.0.1": {"metadata": {"airflowVersion": "2.1.0", "channel": "deprecated"}},
    "12.5.0": {"metadata": {"airflowVersion": "2.10.3", "channel": "deprecated"}},
    "12.6.0": {"metadata": {"airflowVersion": "2.10.4", "channel": "deprecated", "yanked": true}},
    "12.10.1": {"metadata": {"airflowVersion": "2.10.5", "channel": "deprecated"}},
    "13.1.0": {"metadata": {"airflowVersion": "2.11.0", "channel": "stable"}}
  }}`)
	for floating, want := range map[string]string{"3.3": "3.3-10", "12": "12.10.1", "12.6": "", "13": "13.1.0", "3.0": "3.0-2"} {
		got, ok := c.NewestPublishedRuntimeTagged(floating)
		if got != want || ok != (want != "") {
			t.Errorf("NewestPublishedRuntimeTagged(%q) = %q, %v; want %q", floating, got, ok, want)
		}
	}
}
