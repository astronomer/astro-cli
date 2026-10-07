package ide

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

// entry is one tar entry a test archive holds.
type entry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
}

// file is a regular file entry.
func file(name, body string) entry {
	return entry{name: name, typeflag: tar.TypeReg, mode: 0o644, body: body}
}

// dirEntry is a directory entry.
func dirEntry(name string) entry { return entry{name: name, typeflag: tar.TypeDir, mode: 0o755} }

// tarBytes is the uncompressed tar of entries.
func tarBytes(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: e.mode, Linkname: e.linkname}
		if e.typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// gzipTo writes raw, gzipped, to a new archive file and returns its path.
func gzipTo(t *testing.T, raw []byte) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "project.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func archiveOf(t *testing.T, entries ...entry) string {
	t.Helper()
	return gzipTo(t, tarBytes(t, entries...))
}

// extractAt extracts the archive file at path into dst.
func extractAt(ctx context.Context, path, dst string) (archiveStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return archiveStats{}, err
	}
	defer f.Close()
	return extractTarGzArchive(ctx, f, dst)
}

// entriesIn lists what dir holds, recursively, by slash path.
func entriesIn(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if rel, _ := filepath.Rel(dir, p); rel != "." {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// symlinkOrSkip makes a symlink, skipping where this run may not (Windows
// without developer mode).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
}

// refused checks that extracting entries into dst fails with want, and that
// dst then holds only what it held before.
func refused(t *testing.T, dst, want string, entries ...entry) {
	t.Helper()
	before := entriesIn(t, dst)
	_, err := extractAt(t.Context(), archiveOf(t, entries...), dst)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want a refusal saying %q", err, want)
	}
	if got := entriesIn(t, dst); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("the import wrote: before %v, after %v", before, got)
	}
}

// An entry that would land outside the target directory fails the import,
// and the check comes before any write: the good file ahead of it is not
// written either.
func TestExtractRefusesAnEntryOutsideTheTarget(t *testing.T) {
	abs := "/tmp/astro-escape"
	if runtime.GOOS == "windows" {
		abs = `C:\astro-escape`
	}
	for _, name := range []string{"../escape", "dags/../../escape", abs} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dst := filepath.Join(parent, "project")
			if err := os.Mkdir(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			refused(t, dst, "is outside the directory being imported into", file("dags/ok.py", "ok\n"), file(name, "out\n"))
			if got := entriesIn(t, parent); len(got) != 1 || got[0] != "project" {
				t.Errorf("the import wrote %v beside the target", got)
			}
			if _, err := os.Stat(abs); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s exists", abs)
			}
		})
	}
}

// Symlinks, hard links and devices are not written, and an archive holding
// one fails the import before anything is.
func TestExtractRefusesAFileNamedLikeItsTemporaryFiles(t *testing.T) {
	name := "dags/" + tempPrefix + "0123456789abcdef" + tempSuffix
	refused(t, t.TempDir(), "has the name of an import's temporary file", file("first.py", "ok\n"), file(name, "x\n"))
}

func TestExtractRefusesEntriesItDoesNotWrite(t *testing.T) {
	for _, tc := range []struct {
		e    entry
		kind string
	}{
		{entry{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}, "symbolic link"},
		{entry{name: "hard", typeflag: tar.TypeLink, linkname: "dags/ok.py"}, "hard link"},
		{entry{name: "fifo", typeflag: tar.TypeFifo}, "named pipe"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			refused(t, t.TempDir(), "is a "+tc.kind+", which an import does not write", file("dags/ok.py", "ok\n"), tc.e)
		})
	}
}

// What the first pass refuses from the archive alone, before any write.
func TestExtractRefusesAnArchiveThatClashesWithItself(t *testing.T) {
	for _, tc := range []struct {
		name    string
		want    string
		entries []entry
	}{
		{"the same file twice", "more than once", []entry{file("a.py", "1\n"), file("a.py", "2\n")}},
		{"a file, then something under it", "which the archive also writes as a file", []entry{file("a", "1\n"), file("a/b", "2\n")}},
		{"something under a path, then a file there", "which the archive also writes as a directory", []entry{file("a/b", "1\n"), file("a", "2\n")}},
		{"a file, then a directory there", "which the archive also writes as a file", []entry{file("a", "1\n"), dirEntry("a/")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refused(t, t.TempDir(), tc.want, append([]entry{file("first.py", "ok\n")}, tc.entries...)...)
		})
	}
}

// What the first pass refuses from the target directory as it is, before
// any write.
func TestExtractRefusesWhatTheTargetCannotTake(t *testing.T) {
	t.Run("a file where the archive needs a directory", func(t *testing.T) {
		dst := t.TempDir()
		if err := os.WriteFile(filepath.Join(dst, "dags"), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		refused(t, dst, "dags is a file here, where the archive needs a directory", file("first.py", "ok\n"), file("dags/x.py", "x\n"))
	})
	t.Run("a directory where the archive has a file", func(t *testing.T) {
		dst := t.TempDir()
		if err := os.Mkdir(filepath.Join(dst, "a.py"), 0o755); err != nil {
			t.Fatal(err)
		}
		refused(t, dst, "is a directory here, where the archive has a file", file("first.py", "ok\n"), file("a.py", "x\n"))
	})
	t.Run("a symlinked directory", func(t *testing.T) {
		outside, dst := t.TempDir(), t.TempDir()
		symlinkOrSkip(t, outside, filepath.Join(dst, "dags"))
		refused(t, dst, "refusing to write through the symbolic link", file("first.py", "ok\n"), file("dags/x.py", "x\n"))
		if got := entriesIn(t, outside); len(got) != 0 {
			t.Errorf("the import wrote %v outside", got)
		}
	})
}

// Even past the first pass (a component swapped for a symlink after it), a
// write goes through the os.Root, which refuses to leave the target.
func TestExtractEntryCannotLeaveTheRoot(t *testing.T) {
	outside, dst := t.TempDir(), t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(dst, "dags"))
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	h := &tar.Header{Name: "dags/x.py", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2}
	if err := extractEntry(t.Context(), root, h, strings.NewReader("x\n")); err == nil {
		t.Fatal("a write through a symlink out of the target succeeded")
	}
	if got := entriesIn(t, outside); len(got) != 0 {
		t.Errorf("the write landed outside: %v", got)
	}
}

// A symlink already at a file's path is refused before any write: neither
// followed (the file it points at, outside, keeps its contents) nor
// replaced unasked.
func TestExtractRefusesASymlinkAtAFilesPath(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(dst, "a.txt"))

	refused(t, dst, "refusing to replace the symbolic link", file("first.py", "ok\n"), file("a.txt", "new\n"))
	if got, _ := os.ReadFile(outside); string(got) != "keep me\n" {
		t.Errorf("the file outside holds %q", got)
	}
	if info, err := os.Lstat(filepath.Join(dst, "a.txt")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("a.txt is no longer the symlink (%v)", err)
	}
}

// Names are compared as the filesystem compares them: on macOS and Windows
// two names that differ only in case (or, on Windows, in trailing dots and
// spaces) are one path, and the archive may not hold both.
func TestPlanFoldsNamesAsTheFilesystemDoes(t *testing.T) {
	sep := string(filepath.Separator)
	for _, tc := range []struct {
		goos, first, second string
		clash               bool
	}{
		{"darwin", "dags" + sep + "A.py", "dags" + sep + "a.py", true},
		{"darwin", "Dags", "dags" + sep + "x.py", true},
		{"windows", "a.py.", "A.PY", true},
		{"windows", "dags" + sep + "x.py", "DAGS " + sep + "y.py", false},
		{"windows", "Dags ", "dags" + sep + "x.py", true},
		{"linux", "dags" + sep + "A.py", "dags" + sep + "a.py", false},
		{"linux", "Dags", "dags" + sep + "x.py", false},
	} {
		t.Run(tc.goos+" "+tc.first+" "+tc.second, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			p := newPlan(root, foldFor(tc.goos))
			if err := p.checkAgainstArchive(tc.first, tc.first, false); err != nil {
				t.Fatal(err)
			}
			err = p.checkAgainstArchive(tc.second, tc.second, false)
			if (err != nil) != tc.clash {
				t.Errorf("clash = %v (%v), want %v", err != nil, err, tc.clash)
			}
		})
	}
}

// The first pass's view of the disk is taken once per path, found or not.
func TestPlanLstatsAPathOnce(t *testing.T) {
	dst := t.TempDir()
	writeFile(t, dst, "a.py", "x\n")
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p := newPlan(root, foldFor("linux"))
	if _, err := p.lstat("a.py"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.lstat("b.py"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
	if err := os.Remove(filepath.Join(dst, "a.py")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dst, "b.py", "y\n")
	if info, err := p.lstat("a.py"); err != nil || info == nil {
		t.Errorf("a.py was looked up again: %v", err)
	}
	if _, err := p.lstat("b.py"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("b.py was looked up again: %v", err)
	}
}

// swapping serves one archive to the first pass and another to the second,
// as a file rewritten between them would.
type swapping struct {
	first, second []byte
	*bytes.Reader
	reads int
}

func (s *swapping) Seek(off int64, whence int) (int64, error) {
	s.reads++
	if s.reads == 1 {
		s.Reader = bytes.NewReader(s.first)
	} else {
		s.Reader = bytes.NewReader(s.second)
	}
	return s.Reader.Seek(off, whence)
}

func gz(t *testing.T, raw []byte) []byte {
	t.Helper()
	b, err := os.ReadFile(gzipTo(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The second pass writes only what the first one checked: an archive that
// reads differently the second time fails, with nothing of the difference
// written.
func TestExtractWritesOnlyWhatItChecked(t *testing.T) {
	checked := tarBytes(t, file("a.py", "1\n"))
	for name, second := range map[string][]byte{
		"another name":     tarBytes(t, file("evil.py", "1\n")),
		"another size":     tarBytes(t, file("a.py", "12345\n")),
		"another entry":    tarBytes(t, file("a.py", "1\n"), file("evil.py", "1\n")),
		"one entry less":   tarBytes(t, dirEntry("d/")),
		"no entries":       tarBytes(t),
		"a symlink gained": tarBytes(t, file("a.py", "1\n"), entry{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}),
	} {
		t.Run(name, func(t *testing.T) {
			dst := t.TempDir()
			_, err := extractTarGzArchive(t.Context(), &swapping{first: gz(t, checked), second: gz(t, second)}, dst)
			if !errors.Is(err, errArchiveChanged) {
				t.Fatalf("got %v, want %v", err, errArchiveChanged)
			}
			for _, e := range entriesIn(t, dst) {
				if e != "a.py" {
					t.Errorf("the import wrote %s", e)
				}
			}
		})
	}
}

// A name as long as a name may be is written: the temporary name beside it
// does not grow with it.
func TestExtractWritesALongName(t *testing.T) {
	dst := t.TempDir()
	name := strings.Repeat("n", 240)
	if _, err := extractAt(t.Context(), archiveOf(t, file(name, "x\n")), dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, name)); err != nil || string(got) != "x\n" {
		t.Errorf("%s holds %q (%v)", name, got, err)
	}
}

// The limited reader passes a stream of exactly its limit, and fails one
// byte past it.
func TestLimitedReaderAllowsExactlyTheLimit(t *testing.T) {
	var out bytes.Buffer
	if n, err := io.Copy(&out, newLimitedReader(strings.NewReader("12345"), 5)); err != nil || n != 5 {
		t.Errorf("exactly the limit: %d bytes, %v", n, err)
	}
	out.Reset()
	n, err := io.Copy(&out, newLimitedReader(strings.NewReader("123456"), 5))
	if !errors.Is(err, errArchiveTooLarge) || n != 5 {
		t.Errorf("one past the limit: %d bytes, %v", n, err)
	}
}

// An archive cut short fails before anything is written, so a file it would
// have replaced is left as it was.
func TestExtractOfATruncatedArchiveLeavesFilesAlone(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "a.py"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := tarBytes(t, file("a.py", "new\n"), file("b.py", strings.Repeat("b", 4096)))
	if _, err := extractAt(t.Context(), gzipTo(t, raw[:len(raw)-3000]), dst); err == nil {
		t.Fatal("a truncated archive extracted")
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "a.py")); string(got) != "old\n" {
		t.Errorf("a.py holds %q", got)
	}
	if got := entriesIn(t, dst); len(got) != 1 {
		t.Errorf("the directory holds %v", got)
	}
}

// A write that fails part way leaves the file it would have replaced as it
// was, and no temporary file behind.
func TestWriteExtractedFailingLeavesTheOldFile(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "a.py"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeExtracted(root, "a.py", 0o644, strings.NewReader("ne"), 4); err == nil {
		t.Fatal("a short read wrote the file")
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "a.py")); string(got) != "old\n" {
		t.Errorf("a.py holds %q", got)
	}
	if got := entriesIn(t, dst); len(got) != 1 {
		t.Errorf("the directory holds %v", got)
	}
}

// An interrupted import stops, and leaves no temporary file.
func TestExtractStopsWhenInterrupted(t *testing.T) {
	dst := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := extractAt(ctx, archiveOf(t, file("a.py", "new\n")), dst); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the interrupt", err)
	}
	// Directories read nothing a reader could stop; the walk stops itself.
	if _, err := extractAt(ctx, archiveOf(t, dirEntry("dags/")), dst); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the interrupt", err)
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeExtracted(root, "b.py", 0o644, &ctxReader{ctx: ctx, r: strings.NewReader("b\n")}, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the interrupt", err)
	}
	if got := entriesIn(t, dst); len(got) != 0 {
		t.Errorf("the directory holds %v", got)
	}
}

// A temporary file an interrupted import left where this one writes is
// removed once it is old enough not to be a concurrent import's, and the
// export never carries one. Only the exact temporary shape counts: a user's
// own file with a similar name is kept and exported, and so is a directory
// of the temporary shape.
func TestStaleImportTempsAreRemovedAndNeverExported(t *testing.T) {
	const tmp = tempPrefix + "0123456789abcdef" + tempSuffix
	const young = tempPrefix + "fedcba9876543210" + tempSuffix
	dst := t.TempDir()
	writeFile(t, dst, "dags/"+tmp, "half\n")
	writeFile(t, dst, "dags/"+young, "being written\n")
	writeFile(t, dst, ".astro/"+tmp, "half\n")
	writeFile(t, dst, ".astro/config.yaml", "name: x\n")
	writeFile(t, dst, "dags/.env.astro-import-backup", "KEEP=1\n")
	writeFile(t, dst, "dags/"+tempPrefix+"backup", "KEEP=1\n")
	mustMkdir(t, dst, "dags/"+tempPrefix+"0000000000000000"+tempSuffix)
	old := time.Now().Add(-2 * staleTempAge)
	// The directory of the temporary shape is as old: only its kind keeps it.
	for _, p := range []string{"dags/" + tmp, ".astro/" + tmp, "dags/" + tempPrefix + "0000000000000000" + tempSuffix} {
		if err := os.Chtimes(filepath.Join(dst, p), old, old); err != nil {
			t.Fatal(err)
		}
	}

	archivePath := filepath.Join(t.TempDir(), "project.tar.gz")
	if _, err := createTarGzArchive(dst, archivePath, io.Discard); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := extractAt(t.Context(), archivePath, out); err != nil {
		t.Fatal(err)
	}
	want := ".astro,.astro/config.yaml,dags,dags/" + tempPrefix + "0000000000000000" + tempSuffix + ",dags/" + tempPrefix + "backup,dags/.env.astro-import-backup"
	if got := strings.Join(entriesIn(t, out), ","); got != want {
		t.Errorf("the export carried %s, want %s", got, want)
	}

	if _, err := extractAt(t.Context(), archiveOf(t, file("dags/x.py", "x\n")), dst); err != nil {
		t.Fatal(err)
	}
	gone := map[string]bool{"dags/" + tmp: true}
	for _, p := range []string{"dags/" + tmp, "dags/" + young, "dags/.env.astro-import-backup", "dags/" + tempPrefix + "backup", "dags/" + tempPrefix + "0000000000000000" + tempSuffix, ".astro/" + tmp} {
		_, err := os.Stat(filepath.Join(dst, p))
		if gone[p] != errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: removed = %v, want %v", p, errors.Is(err, os.ErrNotExist), gone[p])
		}
	}
}

// The limits: more bytes, more entries, or a bigger stream than an import
// reads fail before any write.
func TestExtractLimits(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		prev := maxImportBytes
		maxImportBytes = 10
		t.Cleanup(func() { maxImportBytes = prev })
		refused(t, t.TempDir(), "more than an import writes", file("a.py", "123456"), file("b.py", "123456"))
	})
	t.Run("one entry over the limit", func(t *testing.T) {
		prev := maxImportBytes
		maxImportBytes = 10
		t.Cleanup(func() { maxImportBytes = prev })
		refused(t, t.TempDir(), "more than an import writes", file("a.py", "12345678901"))
	})
	t.Run("entries", func(t *testing.T) {
		prev := maxImportEntries
		maxImportEntries = 2
		t.Cleanup(func() { maxImportEntries = prev })
		refused(t, t.TempDir(), "more than 2 entries", file("a.py", "1"), file("b.py", "2"), file("c.py", "3"))
	})
	t.Run("stream", func(t *testing.T) {
		prevB, prevO := maxImportBytes, archiveOverhead
		maxImportBytes, archiveOverhead = 1000, 1000
		t.Cleanup(func() { maxImportBytes, archiveOverhead = prevB, prevO })
		// Directories carry no bytes of their own, but their headers do.
		var many []entry
		for i := range 10 {
			many = append(many, dirEntry(strings.Repeat("d", 50)+string(rune('a'+i))+"/"))
		}
		refused(t, t.TempDir(), errArchiveTooLarge.Error(), many...)
	})
}

func TestDownloadRefusesAnArchiveTooLarge(t *testing.T) {
	prevB, prevO := maxImportBytes, archiveOverhead
	maxImportBytes, archiveOverhead = 5, 5
	t.Cleanup(func() { maxImportBytes, archiveOverhead = prevB, prevO })
	var got bytes.Buffer
	_, err := copyLimited(&got, strings.NewReader("0123456789abc"))
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("got %v, want %v", err, errArchiveTooLarge)
	}
}

// exporterFunc answers a session's export with a status and a body.
type exporterFunc func() (int, string)

func (f exporterFunc) ExportAstroIdeSessionTar(context.Context, string, string, string, string, *astrov1alpha1.ExportAstroIdeSessionTarParams, ...astrov1alpha1.RequestEditorFn) (*http.Response, error) {
	status, body := f()
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
}

// A download that is not a 200 fails, saying why: a 204 the API calls
// success carries no archive, and a refusal carries the API's message.
func TestDownloadFailsWithoutAnArchive(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusNoContent, "", "the server returned no archive (204)"},
		{http.StatusForbidden, `{"message":"not yours"}`, "not yours"},
	} {
		var got bytes.Buffer
		err := downloadSession(t.Context(), exporterFunc(func() (int, string) { return tc.status, tc.body }), "o", "w", "p", "s", &got)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%d: got %v, want %q", tc.status, err, tc.want)
		}
	}
}
