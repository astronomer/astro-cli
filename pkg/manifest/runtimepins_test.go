package manifest

import (
	"reflect"
	"slices"
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
exclude-newer = "1 week"
constraint-dependencies = [
    'urllib3<3',
    'apache-airflow==3.3.2+astro.2',
    'apache-airflow-task-sdk==1.3.2+astro.1',
    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
    # astro writes them. To choose a version yourself, list the package in [project] dependencies.
    'a2wsgi==1.10.10',
    'urllib3==2.5.0',
    # End of what astro writes. Constraints below this line are yours.
    'numpy==2.3.0',
]
`

func TestTheRuntimePinsAreReadFromTheirComments(t *testing.T) {
	m, err := Parse([]byte(runtimePinsManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" {
		t.Errorf("build, python = %q, %q, want 3.3-9, 3.12", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
	if want := []string{"a2wsgi==1.10.10", "urllib3==2.5.0"}; !reflect.DeepEqual(m.UV.RuntimePins, want) {
		t.Errorf("RuntimePins = %q, want %q: the project's pin after the end is its own", m.UV.RuntimePins, want)
	}
	if got := len(m.UV.ConstraintDependencies); got != 6 {
		t.Errorf("ConstraintDependencies has %d entries, want all 6: uv reads every one", got)
	}
	// MWAA and Composer run neither Astronomer's Airflow nor the runtime's
	// packages: the project's own constraints are all that is left.
	if got := m.UV.ProjectConstraints(); !reflect.DeepEqual(got, []string{"urllib3<3", "numpy==2.3.0"}) {
		t.Errorf("ProjectConstraints = %q, want the project's own lines", got)
	}
	if m.UV.ExcludeNewer != "1 week" {
		t.Errorf("ExcludeNewer = %q, want the project's cutoff", m.UV.ExcludeNewer)
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
	if got := len(m.UV.ProjectConstraints()); got != 4 {
		t.Errorf("ProjectConstraints has %d entries, want the 4 that are not AstroPins", got)
	}
}

func TestRuntimePinsIn(t *testing.T) {
	head := []any{"x<2"}
	for _, c := range RuntimePinsComments("12.6.0", "3.12") {
		head = append(head, tomledit.Comment(c))
	}
	cases := []struct {
		name               string
		lines              []any
		before, pins, tail int
	}{
		{"ended", append(append(slices.Clone(head), "a==1", "b==2", tomledit.Comment(RuntimePinsEnd)), "c<3", "d==4"), 1, 5, 2},
		// No end comment, as a block written before it existed: the pins end
		// at the first line that is not one, so the project's lines after
		// them stay its own.
		{"not ended, a range after", append(slices.Clone(head), "a==1", "b==2", "c<3", "d==4"), 1, 4, 2},
		{"not ended, a comment after", append(slices.Clone(head), "a==1", tomledit.Comment("mine"), "d==4"), 1, 3, 2},
		{"not ended, nothing after", append(slices.Clone(head), "a==1"), 1, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, pins, after, build, python := RuntimePinsIn(tc.lines)
			if build != "12.6.0" || python != "3.12" {
				t.Errorf("build, python = %q, %q", build, python)
			}
			if len(before) != tc.before || len(pins) != tc.pins || len(after) != tc.tail {
				t.Errorf("before %#v\npins %#v\nafter %#v", before, pins, after)
			}
		})
	}
}
