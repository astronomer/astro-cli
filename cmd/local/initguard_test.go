package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// setHome points os.UserHomeDir at dir, on every platform.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// astro init in the home directory would make all of ~ the project, so it
// is a usage error and writes nothing. A DIRECTORY naming somewhere under it
// is a project of its own and goes ahead.
func TestInitRefusesTheHomeDirectory(t *testing.T) {
	d, dir, stdout := initDeps(t)
	setHome(t, dir)

	err := execute(t, d, "init")
	if err == nil || !cliout.IsUsage(err) || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("want a usage error naming the home directory, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pyproject.toml")); !os.IsNotExist(statErr) {
		t.Errorf("a refused init wrote a pyproject.toml: %v", statErr)
	}

	// Under json, the one error object, of kind usage.
	stdout.Reset()
	if err := execute(t, d, "init", "-o", "json"); err == nil {
		t.Fatal("astro init -o json in the home directory succeeded")
	}
	var obj cliout.ErrorObject
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout.String())
	}
	if obj.Kind != "usage" || !strings.Contains(obj.Error, "home directory") {
		t.Errorf("error object = %+v, want kind usage naming the home directory", obj)
	}

	if err := execute(t, d, "init", "my-project"); err != nil {
		t.Fatalf("astro init my-project from the home directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "my-project", "pyproject.toml")); err != nil {
		t.Errorf("missing my-project/pyproject.toml: %v", err)
	}
}

// astro init inside a project warns, on stderr, that commands below it now
// find the new project, and still makes it: a repository can hold several.
func TestInitWarnsInsideAProject(t *testing.T) {
	d, dir, stdout := initDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("a project of its own drew a warning: %s", stderr)
	}

	stdout.Reset()
	if err := execute(t, d, "init", "nested", "-o", "json"); err != nil {
		t.Fatalf("astro init nested: %v", err)
	}
	if !strings.Contains(stderr.String(), "inside the Astro project at "+dir) {
		t.Errorf("no warning naming the enclosing project %s: %q", dir, stderr)
	}
	if !json.Valid([]byte(stdout.String())) {
		t.Errorf("the warning reached stdout under json:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "pyproject.toml")); err != nil {
		t.Errorf("the nested project was not made: %v", err)
	}
}
