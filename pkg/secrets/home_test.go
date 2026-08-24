package secrets

import (
	"path/filepath"
	"testing"
)

// setIsolatedHome points os.UserHomeDir at dir on every platform. HOME alone is
// not enough on Windows, where it reads USERPROFILE.
func setIsolatedHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestDefaultDirUsesTheAstroHome(t *testing.T) {
	home := t.TempDir()
	setIsolatedHome(t, home)

	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if want := filepath.Join(home, ".astro", "secrets"); got != want {
		t.Errorf("DefaultDir = %q, want %q", got, want)
	}
}

// The vault's location must not depend on the process environment, and this is
// the assertion that says so on purpose rather than by omission. DefaultDir's doc
// carries the reasoning.
func TestVaultLocationIgnoresAstroHome(t *testing.T) {
	home := t.TempDir()
	setIsolatedHome(t, home)
	t.Setenv("ASTRO_HOME", t.TempDir())

	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if want := filepath.Join(home, ".astro", "secrets"); got != want {
		t.Errorf("DefaultDir = %q, want %q — ASTRO_HOME must not move the vault", got, want)
	}
}

// The same, one level down, so a caller placing a sibling of the vault lands in
// the same tree.
func TestAstroHomeIgnoresAstroHomeEnv(t *testing.T) {
	home := t.TempDir()
	setIsolatedHome(t, home)
	t.Setenv("ASTRO_HOME", t.TempDir())

	got, err := astroHome()
	if err != nil {
		t.Fatalf("AstroHome: %v", err)
	}
	if want := filepath.Join(home, ".astro"); got != want {
		t.Errorf("AstroHome = %q, want %q", got, want)
	}
}

// The vault must be a child of the astro home, so a caller placing a sibling
// file (the CLI's config.yaml, its env file) lands in the same tree. Asserting
// the relationship rather than only the two strings is what catches a later
// "tidy" that moves one and not the other.
func TestDefaultDirIsUnderAstroHome(t *testing.T) {
	setIsolatedHome(t, t.TempDir())

	home, err := astroHome()
	if err != nil {
		t.Fatalf("AstroHome: %v", err)
	}
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if filepath.Dir(dir) != home {
		t.Errorf("DefaultDir %q is not directly under AstroHome %q", dir, home)
	}
}

// The hazard behind the home lookup, which nothing pinned once the ASTRO_HOME
// reading (and its blank-value test) went away: a resolver that swallows the
// error and joins onto "" puts the vault at ./.astro/secrets — a different vault
// per working directory, with every secret appearing to vanish when the user
// cd's. That shape exists elsewhere in this repo, so it is one refactor away.
func TestDefaultDirFailsWhenTheHomeIsUnknown(t *testing.T) {
	setIsolatedHome(t, "")

	dir, err := DefaultDir()
	if err == nil {
		t.Fatalf("DefaultDir = %q, want an error when the home directory cannot be resolved", dir)
	}
	if dir != "" {
		t.Errorf("DefaultDir = %q, want empty alongside the error", dir)
	}
	if filepath.IsLocal(dir) && dir != "" {
		t.Errorf("DefaultDir = %q is a relative path; that is a vault per working directory", dir)
	}
}
