package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// setHome makes dir the home directory os.UserHomeDir reports.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// astro init in the home directory would make every file under ~ the
// project, so it is a usage error that writes nothing, however the directory
// is spelled. A DIRECTORY naming somewhere under it goes ahead.
func TestInitRefusesTheHomeDirectory(t *testing.T) {
	d, dir, _ := initDeps(t)
	setHome(t, dir)

	for _, arg := range []string{".", dir, dir + string(filepath.Separator)} {
		err := execute(t, d, "init", arg)
		if err == nil || !cliout.IsUsage(err) || !strings.Contains(err.Error(), "is your home directory") {
			t.Fatalf("astro init %s in the home directory = %v, want a usage error naming it", arg, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); !os.IsNotExist(err) {
		t.Errorf("a refused init wrote a pyproject.toml: %v", err)
	}

	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(dir, link); err == nil {
		if err := execute(t, d, "init", link); err == nil || !cliout.IsUsage(err) {
			t.Errorf("astro init through a symlink to the home directory = %v, want a usage error", err)
		}
	}

	if err := execute(t, d, "init", "my-project"); err != nil {
		t.Fatalf("astro init my-project from the home directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "my-project", "pyproject.toml")); err != nil {
		t.Errorf("missing my-project/pyproject.toml: %v", err)
	}
}

// A home directory that already is a project is init's to re-run, as any
// project is: it answers what init says of a project anywhere.
func TestInitReRunsInAHomeDirectoryProject(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := execute(t, d, "init"); err != nil { // made before it was ~
		t.Fatalf("astro init: %v", err)
	}
	setHome(t, dir)
	err := execute(t, d, "init")
	if !errors.Is(err, scaffold.ErrAlreadyAstroProject) || cliout.IsUsage(err) {
		t.Fatalf("astro init again in a home directory that is a project = %v, want %v", err, scaffold.ErrAlreadyAstroProject)
	}
}
