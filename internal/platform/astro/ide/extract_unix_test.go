//go:build !windows

package ide

import (
	"archive/tar"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Something that is not a regular file at a file entry's path (here a named
// pipe) is refused before any write, not replaced.
func TestExtractRefusesAnIrregularFileAtAFilesPath(t *testing.T) {
	dst := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dst, "a.py"), 0o600); err != nil {
		t.Skipf("cannot make a named pipe here: %v", err)
	}
	refused(t, dst, "which is not a regular file", file("first.py", "ok\n"), file("a.py", "x\n"))
}

// File modes are masked: never writable by group or others, always readable
// and writable by the owner, never more open than the umask allows, and
// applied to a file the import replaces too. A 0600 file stays 0600.
func TestExtractMasksModes(t *testing.T) {
	umask := os.FileMode(syscall.Umask(0))
	syscall.Umask(int(umask))

	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "old.sh"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := archiveOf(t,
		entry{name: "all.sh", typeflag: tar.TypeReg, mode: 0o777, body: "a\n"},
		entry{name: "none.txt", typeflag: tar.TypeReg, mode: 0o000, body: "n\n"},
		entry{name: ".env", typeflag: tar.TypeReg, mode: 0o600, body: "SECRET=1\n"},
		entry{name: "old.sh", typeflag: tar.TypeReg, mode: 0o755, body: "new\n"},
	)
	if _, err := extractAt(t.Context(), archive, dst); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"all.sh": 0o755, "none.txt": 0o600, ".env": 0o600, "old.sh": 0o755} {
		info, err := os.Stat(filepath.Join(dst, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want&^umask {
			t.Errorf("%s is %o, want %o (umask %o)", name, got, want&^umask, umask)
		}
	}
}
