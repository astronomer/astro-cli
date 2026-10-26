package manifest

import (
	"reflect"
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
]

[dependency-groups]
dev = ['pytest']
astro-runtime = [
    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
    # The packages that image installs, so the local environment has what it has. astro writes them; to choose a version yourself, list the package in [project] dependencies.
    "apache-airflow-providers-celery",
    "psycopg2-binary",
]
`

func TestTheSeedRecordIsReadFromTheRuntimeGroup(t *testing.T) {
	m, err := Parse([]byte(seededManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" {
		t.Errorf("build, python = %q, %q, want 3.3-9, 3.12", m.UV.RuntimeBuild, m.UV.RuntimePython)
	}
	if want := []string{"apache-airflow-providers-celery", "psycopg2-binary"}; !reflect.DeepEqual(m.RuntimeGroup, want) {
		t.Errorf("RuntimeGroup = %q, want %q", m.RuntimeGroup, want)
	}
	if got := len(m.UV.ConstraintDependencies); got != 3 {
		t.Errorf("ConstraintDependencies has %d entries, want 3", got)
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
	if len(m.RuntimeGroup) != 2 {
		t.Errorf("RuntimeGroup = %q, want its two names", m.RuntimeGroup)
	}
}

// The record is read from an empty group too: a project that lists every one
// of the image's packages itself was seeded all the same.
func TestTheSeedRecordOfAnEmptyGroupIsRead(t *testing.T) {
	src := "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n\n[dependency-groups]\nastro-runtime = [\n    # " +
		SeedRecord("3.3-9", "3.12") + "\n]\n"
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.UV.RuntimeBuild != "3.3-9" || m.UV.RuntimePython != "3.12" || m.RuntimeGroup != nil {
		t.Errorf("build, python, group = %q, %q, %q", m.UV.RuntimeBuild, m.UV.RuntimePython, m.RuntimeGroup)
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
