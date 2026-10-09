package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	homedir "github.com/mitchellh/go-homedir"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/fileutil"
)

// setHome makes home the home directory, as the root wires it: the same
// directory however it is spelled.
func setHome(d *Deps, home string) {
	d.IsHomeDir = func(dir string) bool { return fileutil.SamePath(dir, home) }
}

// astro init in the home directory would make all of ~ the project, so it
// is a usage error and writes nothing. A DIRECTORY naming somewhere under it
// is a project of its own and goes ahead.
func TestInitRefusesTheHomeDirectory(t *testing.T) {
	d, dir, stdout := initDeps(t)
	setHome(&d, dir)

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

// HOME reached through a symlink is still the home directory: the refusal
// compares directories, not their spellings.
func TestInitRefusesTheHomeDirectoryThroughASymlink(t *testing.T) {
	d, dir, _ := initDeps(t)
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	setHome(&d, link)
	err := execute(t, d, "init")
	if err == nil || !cliout.IsUsage(err) {
		t.Fatalf("want a usage error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pyproject.toml")); !os.IsNotExist(statErr) {
		t.Errorf("a refused init wrote a pyproject.toml: %v", statErr)
	}
}

// Only a directory that is a project, as astro deploy's advice counts one
// (project.IsAstroProject), draws the warning. The home directory and an
// ancestor whose pyproject.toml cannot be parsed never do.
func TestInitWarnsOnlyInsideAProject(t *testing.T) {
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const (
		malformed = "this is not : valid = toml [[[\n"
		manifest  = "[project]\nname = \"demo\"\n\n[tool.astro]\n"
	)
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, parent string)
		home  bool
		warn  bool
	}{
		{
			name: "a 1.x .astro/config.yaml without a Dockerfile",
			setup: func(t *testing.T, p string) {
				write(t, filepath.Join(p, ".astro", "config.yaml"), "project:\n  name: demo\n")
			},
			warn: true,
		},
		{
			name: "a Dockerfile and a bare .astro",
			setup: func(t *testing.T, p string) {
				write(t, filepath.Join(p, "Dockerfile"), "FROM x\n")
				if err := os.MkdirAll(filepath.Join(p, ".astro"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "a pyproject.toml that does not parse",
			setup: func(t *testing.T, p string) { write(t, filepath.Join(p, "pyproject.toml"), malformed) },
		},
		{
			name:  "a manifest in the home directory",
			setup: func(t *testing.T, p string) { write(t, filepath.Join(p, "pyproject.toml"), manifest) },
			home:  true,
		},
		{
			name:  "a malformed pyproject.toml in the home directory",
			setup: func(t *testing.T, p string) { write(t, filepath.Join(p, "pyproject.toml"), malformed) },
			home:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, parent, _ := initDeps(t)
			stderr := &bytes.Buffer{}
			d.Stderr = stderr
			tc.setup(t, parent)
			if tc.home {
				setHome(&d, parent)
			}
			if err := execute(t, d, "init", "nested"); err != nil {
				t.Fatalf("astro init nested: %v", err)
			}
			warned := strings.Contains(stderr.String(), "inside the Astro project at "+parent)
			if warned != tc.warn {
				t.Errorf("warned = %v, want %v: %q", warned, tc.warn, stderr)
			}
		})
	}
}

// Deps from NewDeps know the home directory without the root wiring
// config.IsHomeDir, and ~/.astro/config.yaml, the CLI's own settings, never
// makes ~ the project above another, even with no IsHomeDir at all: the
// predicate rules it out itself.
func TestInitUnderHomeDrawsNoFalseNestedWarning(t *testing.T) {
	d, home, _ := initDeps(t)
	if err := os.MkdirAll(filepath.Join(home, ".astro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".astro", "config.yaml"), []byte("context: cloud\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	defaults := NewDeps()
	if defaults.IsHomeDir == nil || !defaults.IsHomeDir(home) {
		t.Fatal("NewDeps does not know the home directory")
	}
	for name, isHome := range map[string]func(string) bool{"from-NewDeps": defaults.IsHomeDir, "unset": nil} {
		t.Run(name, func(t *testing.T) {
			stderr := &bytes.Buffer{}
			d.Stderr = stderr
			d.IsHomeDir = isHome
			if err := execute(t, d, "init", name); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			if strings.Contains(stderr.String(), "inside the Astro project") {
				t.Errorf("~/.astro/config.yaml drew a nested-project warning: %q", stderr)
			}
		})
	}
}
