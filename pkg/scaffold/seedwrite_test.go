package scaffold

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

const seedWriteManifest = "[project]\nname = 'demo'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n\n[tool.uv]\nconstraint-dependencies = ['urllib3<3']\n"

// RestoreManifest puts the bytes back through a symlink to what it points at,
// so the link stays a link, as every manifest write does.
func TestRestoreManifestWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.toml"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.toml", filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Fatal(err)
	}
	if err := RestoreManifest(dir, nil, []byte(seedWriteManifest)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(dir, "pyproject.toml"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("pyproject.toml is no longer a link: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "real.toml")); string(got) != seedWriteManifest {
		t.Errorf("the link's target holds %q", got)
	}
}

// A manifest this user may not write is refused, by the restore as by an
// edit, rather than replaced by a rename.
func TestSeedWritesRefuseAReadOnlyManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not make a file read-only here")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")
	if err := os.WriteFile(path, []byte(seedWriteManifest), 0o400); err != nil {
		t.Fatal(err)
	}
	pins := &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{"pandas==2.2.3"}}
	if _, err := AddSeedConstraints(dir, nil, pins); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("AddSeedConstraints wrote a read-only manifest: %v", err)
	}
	if err := RestoreManifest(dir, nil, []byte("x")); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("RestoreManifest wrote a read-only manifest: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != seedWriteManifest {
		t.Errorf("the read-only manifest changed: %q", got)
	}
}

// The seed's constraints follow the project's own, between the comments that
// make them astro's, and a project's own constraint stays its own.
func TestAddSeedConstraintsMarksThemAsAstros(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(seedWriteManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pins := &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{"pandas==2.2.3", "sqlalchemy==2.0.50"}}
	n, err := AddSeedConstraints(dir, nil, pins)
	if err != nil || n != 2 {
		t.Fatalf("AddSeedConstraints = %d, %v", n, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	text := string(got)
	own, heading, end := strings.Index(text, "urllib3<3"), strings.Index(text, "ships these versions"), strings.Index(text, legacyPinsEnd)
	if own < 0 || heading < own || end < heading || !strings.Contains(text[heading:end], "pandas==2.2.3; python_version == '3.12'") {
		t.Errorf("the constraints are not after the project's own, between astro's comments:\n%s", text)
	}
	ed, err := tomledit.NewSurgical(got)
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := tomledit.GetLines(ed, constraintsKey)
	if pc := projectConstraints(lines); len(pc) != 1 || pc[0] != "urllib3<3" {
		t.Errorf("the project's own constraints read as %v", pc)
	}
}
