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
