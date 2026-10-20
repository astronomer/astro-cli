package manifest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

const runtimePinsManifest = `[project]
name = 'p'
requires-python = '>=3.12'
dependencies = ['apache-airflow==3.3.*', 'pandas']

[tool.astro]

[tool.uv]
constraint-dependencies = [
    'urllib3<3',
    'apache-airflow==3.3.2+astro.2',
    'apache-airflow-task-sdk==1.3.2+astro.1',
    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
    # astro writes them. To choose a version yourself, list the package in [project] dependencies.
    'a2wsgi==1.10.10',
    'urllib3==2.5.0',
]
`

func TestTheRuntimePinsAreReadFromTheirComment(t *testing.T) {
	m, err := Parse([]byte(runtimePinsManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" {
		t.Errorf("build, python = %q, %q, want 3.3-9, 3.12", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
	if want := []string{"a2wsgi==1.10.10", "urllib3==2.5.0"}; !reflect.DeepEqual(m.UV.RuntimePins, want) {
		t.Errorf("RuntimePins = %q, want %q", m.UV.RuntimePins, want)
	}
	if got := len(m.UV.ConstraintDependencies); got != 5 {
		t.Errorf("ConstraintDependencies has %d entries, want all 5: uv reads every one", got)
	}
	// MWAA and Composer run neither Astronomer's Airflow nor the runtime's
	// packages: the project's own constraint is all that is left.
	if got := m.UV.ProjectConstraints(); !reflect.DeepEqual(got, []string{"urllib3<3"}) {
		t.Errorf("ProjectConstraints = %q, want the project's own line alone", got)
	}
}

func TestAManifestWithoutTheCommentHasNoRuntimePins(t *testing.T) {
	src := strings.Replace(runtimePinsManifest, "    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.\n", "", 1)
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "" || len(m.UV.RuntimePins) != 0 {
		t.Errorf("RuntimeBuild %q, RuntimePins %q: without the comment every constraint is the project's", m.UV.RuntimeBuild, m.UV.RuntimePins)
	}
	if got := len(m.UV.ProjectConstraints()); got != 3 {
		t.Errorf("ProjectConstraints has %d entries, want the 3 that are not AstroPins", got)
	}
}

func TestRuntimePinsCommentsReadBack(t *testing.T) {
	var lines []any
	lines = append(lines, "x<2")
	for _, c := range RuntimePinsComments("12.6.0", "3.12") {
		lines = append(lines, tomledit.Comment(c))
	}
	lines = append(lines, "a==1")
	before, pins, build, python := RuntimePinsIn(lines)
	if build != "12.6.0" || python != "3.12" {
		t.Errorf("build, python = %q, %q", build, python)
	}
	if !reflect.DeepEqual(before, []any{"x<2"}) || len(pins) != 3 {
		t.Errorf("before %#v, pins %#v", before, pins)
	}
}
