package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A secrets directory left group- or world-accessible is narrowed to the
// owner on first use.
func TestLooseDirIsTightened(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not govern access on Windows")
	}
	for name, op := range map[string]func(*keyringStore) error{
		"ListMeta": func(s *keyringStore) error { _, err := s.ListMeta(); return err },
		"Set":      func(s *keyringStore) error { return s.Set("env:global:A", "v") },
		"Get": func(s *keyringStore) error {
			_, err := s.Get("env:global:A")
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "secrets")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o755); err != nil { // past the umask
				t.Fatal(err)
			}
			if err := op(testStore(t, newFakeKeyring(), "astro-test", dir)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			fi, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			if perm := fi.Mode().Perm(); perm != dirPerm {
				t.Fatalf("dir mode = %o, want %o", perm, dirPerm)
			}
		})
	}
}

// A symlinked secrets directory is refused by every operation, nothing is
// written through it, and no master key is minted for it.
func TestSymlinkedDirIsRefused(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "secrets")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro-test", link)
	ops := map[string]func() error{
		"Set":      func() error { return s.Set("env:global:A", "v") },
		"SetPlain": func() error { return s.SetPlain("env:global:A", "v") },
		"Get":      func() error { _, err := s.Get("env:global:A"); return err },
		"Delete":   func() error { return s.Delete("env:global:A") },
		"ListMeta": func() error { _, err := s.ListMeta(); return err },
		"Upgrade":  func() error { _, err := s.Upgrade(); return err },
		"OpenLinks": func() error {
			_, err := OpenLinks(link)
			return err
		},
		"UpdateLinks": func() error {
			return UpdateLinks(link, func(map[string]Reach) error { return nil })
		},
	}
	for name, op := range ops {
		if err := op(); !errors.Is(err, ErrVaultDirUnsafe) {
			t.Errorf("%s through a symlinked dir: err = %v, want ErrVaultDirUnsafe", name, err)
		}
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("%d files written through the symlink", len(entries))
	}
	if kr.sets != 0 {
		t.Fatalf("a master key was minted for a vault behind a symlink (%d keyring writes)", kr.sets)
	}
}

func TestDirThatIsAFileIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore(t, newFakeKeyring(), "astro-test", path).ListMeta(); !errors.Is(err, ErrVaultDirUnsafe) {
		t.Fatalf("err = %v, want ErrVaultDirUnsafe", err)
	}
}
