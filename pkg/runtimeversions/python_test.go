package runtimeversions

import (
	"errors"
	"testing"
)

// projectPythonCatalog has 3.3 builds that default to 3.14, a newer one not
// released yet, a 3.2 build that defaults to 3.13, and an Airflow 2 build that
// lists no Python.
const projectPythonCatalog = `{
  "runtimeVersions": {
    "13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable"}}
  },
  "runtimeVersionsV3": {
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.3", "channel": "stable", "releaseDate": "2999-01-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.2-10": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.13"}},
    "3.1-2": {"metadata": {"airflowVersion": "3.1.0", "channel": "stable"}}
  }
}`

func TestProjectPython(t *testing.T) {
	c := parse(t, projectPythonCatalog)
	catalog := func() *Catalog { return c }
	tests := []struct {
		name, pin, runtime, requires string
		catalog                      func() *Catalog
		python, build                string
	}{
		{name: "default admitted", pin: "3.2", requires: ">=3.12", catalog: catalog, python: "3.13"},
		{name: "default admitted, newest published build", pin: "3.3", requires: ">=3.12", catalog: catalog, python: "3.14"},
		{name: "non-default needs the exact build", pin: "3.3", requires: "==3.13.*", catalog: catalog, python: "3.13", build: "3.3-8"},
		{name: "runtime named", pin: "3.3", runtime: "3.3-7", requires: "==3.13.*", catalog: catalog, python: "3.13", build: "3.3-7"},
		{name: "patch pin reads the series", pin: "3.3.1", requires: "<3.14", catalog: catalog, python: "3.13", build: "3.3-8"},
		{name: "offline", pin: "3.3", requires: "==3.13.*", catalog: func() *Catalog { return nil }},
		{name: "no catalog reader", pin: "3.3", requires: "==3.13.*"},
		{name: "no requires-python", pin: "3.3", catalog: catalog},
		{name: "unreadable specifier", pin: "3.3", requires: "==3.13rc1", catalog: catalog},
		{name: "airflow 2", pin: "2.11", requires: ">=3.10", catalog: catalog},
		{name: "airflow 2 build named", pin: "2.11", runtime: "13.11.0", requires: ">=3.10", catalog: catalog},
		{name: "major-only pin", pin: "3", requires: ">=3.12", catalog: catalog},
		{name: "build lists no Python", pin: "3.1", runtime: "3.1-2", requires: ">=3.12", catalog: catalog},
		{name: "build not listed", pin: "3.3", runtime: "3.3-12", requires: ">=3.12", catalog: catalog},
		{name: "no build of the pin", pin: "3.5", requires: ">=3.12", catalog: catalog},
	}
	for _, tt := range tests {
		python, build, err := ProjectPython(tt.pin, tt.runtime, tt.requires, tt.catalog)
		if err != nil || python != tt.python || build != tt.build {
			t.Errorf("%s: ProjectPython = %q, %q, %v; want %q, %q", tt.name, python, build, err, tt.python, tt.build)
		}
	}
}

func TestProjectPythonRefusesWhatNoBuildPythonMeets(t *testing.T) {
	c := parse(t, projectPythonCatalog)
	python, build, err := ProjectPython("3.3", "", "==3.11.*", func() *Catalog { return c })
	var notShipped *PythonNotShippedError
	if !errors.As(err, &notShipped) || python != "" || build != "" {
		t.Fatalf("ProjectPython = %q, %q, %v; want a *PythonNotShippedError", python, build, err)
	}
	if notShipped.Build != "3.3-8" || len(notShipped.Pythons) != 3 {
		t.Errorf("error names %q with %v; want 3.3-8 with its three Pythons", notShipped.Build, notShipped.Pythons)
	}
}

func TestProjectPythonReadsNoCatalogWithoutRequiresPython(t *testing.T) {
	calls := 0
	if _, _, err := ProjectPython("3.3", "", " ", func() *Catalog { calls++; return nil }); err != nil || calls != 0 {
		t.Errorf("got err %v and %d catalog reads; want neither", err, calls)
	}
}

func TestImagePythonPicksTheNewestAdmitted(t *testing.T) {
	r := Runtime{Tag: "3.3-8", PythonVersions: []string{"3.12", "3.13", "3.14"}, DefaultPythonVersion: "3.14"}
	tests := []struct {
		requires string
		want     string
		ok       bool
	}{
		{"", "", false},
		{">=3.10", "3.14", true},
		{"==3.13.*", "3.13", true},
		{"==3.13", "3.13", true},
		{"==3.13.4", "3.13", true},
		{"==3.*", "3.14", true},
		{">=3.12,<3.14", "3.13", true},
		{">=3.12, <3.13.5", "3.13", true},
		{"<=3.13", "3.13", true},
		{"<3.13", "3.12", true},
		{">3.13", "3.14", true},
		{">=3.13.2", "3.14", true},
		{"~=3.12", "3.14", true},
		{"~=3.12.1", "3.12", true},
		{"!=3.14.*", "3.13", true},
		{"!=3.14", "3.14", true},
		{"!=3.*", "", true},
		{"==3.11.*", "", true},
		{">=3.15", "", true},
		{"==3.13rc1", "", false},
		{"===3.13", "", false},
		{">=3.*", "", false},
		{"3.13", "", false},
	}
	for _, tt := range tests {
		got, ok := r.ImagePython(tt.requires)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ImagePython(%q) = %q, %v; want %q, %v", tt.requires, got, ok, tt.want, tt.ok)
		}
	}
}

func TestImagePythonKeepsAnAdmittedDefault(t *testing.T) {
	r := Runtime{Tag: "3.2-10", PythonVersions: []string{"3.12", "3.13", "3.14"}, DefaultPythonVersion: "3.13"}
	for requires, want := range map[string]string{">=3.12": "3.13", "==3.14.*": "3.14", "<3.13": "3.12"} {
		if got, ok := r.ImagePython(requires); !ok || got != want {
			t.Errorf("ImagePython(%q) = %q, %v; want %q", requires, got, ok, want)
		}
	}
}

func TestImagePythonWithoutAPythonList(t *testing.T) {
	r := Runtime{Tag: "13.11.0"}
	if got, ok := r.ImagePython(">=3.10"); ok || got != "" {
		t.Errorf("an Airflow 2 build lists no Python, so there is nothing to pick; got %q, %v", got, ok)
	}
}
