package manifest

import "testing"

func TestRuntimeRecordRoundTrips(t *testing.T) {
	build, python, ok := IsRuntimeRecord(RuntimeRecord("3.3-11", "3.14"))
	if !ok || build != "3.3-11" || python != "3.14" {
		t.Errorf("IsRuntimeRecord = %q, %q, %v", build, python, ok)
	}
	if _, _, ok := IsRuntimeRecord("a comment of the project's own"); ok {
		t.Error("an unrelated comment read as a record")
	}
}

func TestParseReadsTheRuntimeRecord(t *testing.T) {
	m, err := Parse([]byte(`[project]
name = 'demo'
dependencies = ['apache-airflow==3.3.*']

[tool.astro]

[tool.uv]
constraint-dependencies = [
    'apache-airflow==3.3.2+astro.2',
    # Astro Runtime 3.3-11 (Python 3.14) constrains these. astro writes them.
    'SQLAlchemy==2.0.50',
    # the project's own
    'urllib3<3',
]
`))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-11" || m.UV.RuntimePython != "3.14" {
		t.Errorf("record = %q, %q", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
	if len(m.UV.RuntimeConstraints) != 1 || m.UV.RuntimeConstraints[0] != "SQLAlchemy==2.0.50" {
		t.Errorf("runtime lines = %q, want SQLAlchemy's alone", m.UV.RuntimeConstraints)
	}
	got := m.UV.WithoutRuntimeConstraints(WithoutAstroPins(m.UV.ConstraintDependencies))
	if len(got) != 1 || got[0] != "urllib3<3" {
		t.Errorf("without the runtime's and the +astro lines = %q, want the project's own", got)
	}
}
