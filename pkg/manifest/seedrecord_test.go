package manifest

import (
	"strings"
	"testing"
)

const seededManifest = `[project]
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
    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
]
`

func TestTheSeedRecordIsReadFromItsComment(t *testing.T) {
	m, err := Parse([]byte(seededManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" {
		t.Errorf("build, python = %q, %q, want 3.3-9, 3.12", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
	if got := len(m.UV.ConstraintDependencies); got != 3 {
		t.Errorf("ConstraintDependencies has %d entries, want 3: the record is a comment", got)
	}
	if m.UV.ExcludeNewer != "1 week" {
		t.Errorf("ExcludeNewer = %q, want the project's cutoff", m.UV.ExcludeNewer)
	}
}

func TestAManifestWithoutTheRecordWasNotSeeded(t *testing.T) {
	src := strings.Replace(seededManifest, "    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.\n", "", 1)
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "" || m.UV.RuntimePython != "" {
		t.Errorf("RuntimeBuild %q, RuntimePython %q, want none", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
}

// The record alone keeps the array, with no constraint in it.
func TestTheSeedRecordAloneIsRead(t *testing.T) {
	src := "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n\n[tool.uv]\nconstraint-dependencies = [\n    # " +
		SeedRecord("3.3-9", "3.12") + "\n]\n"
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" {
		t.Errorf("build, python = %q, %q", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
}

func TestIsSeedRecord(t *testing.T) {
	if b, p, ok := IsSeedRecord(SeedRecord("12.6.0", "3.11")); !ok || b != "12.6.0" || p != "3.11" {
		t.Errorf("IsSeedRecord(SeedRecord) = %q, %q, %v", b, p, ok)
	}
	if _, _, ok := IsSeedRecord("Astro Runtime 3.3-9 (Python 3.12) ships these versions"); ok {
		t.Error("an old runtime pins heading read as a seed record")
	}
}
