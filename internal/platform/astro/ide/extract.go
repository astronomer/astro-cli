package ide

import (
	"archive/tar"
	"compress/gzip"
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// Limits on what one import writes. An Astro project is far smaller than any
// of them; they are there so that an archive declaring more (a decompression
// bomb, or a corrupt header) fails before it fills the disk, not after.
var (
	// maxImportBytes caps the bytes of all the files together. A var so a
	// test can lower it.
	maxImportBytes int64 = 8 * gib
	// maxImportEntries caps the entries, directories included.
	maxImportEntries = 100_000
	// archiveOverhead is what the tar stream may hold beyond the files' own
	// bytes: headers, long names and padding. Reading more than
	// maxImportBytes+archiveOverhead of it, compressed or not, fails.
	archiveOverhead int64 = 1 * gib
)

const gib = 1 << 30

// An imported file keeps its archive mode masked by importedFileMask, never
// writable by group or others, with importedFileFloor added, always readable
// and writable by its owner. The file is created with that mode, so the
// user's umask still applies: a 0600 file stays 0600.
const (
	importedFileMask  os.FileMode = 0o755
	importedFileFloor os.FileMode = 0o600
)

// The temporary file an import writes a file to before renaming it into
// place is named tempPrefix, sixteen lowercase hex digits and tempSuffix, and
// nothing else: a fixed 34 bytes, so it fits wherever the file's own name
// does, and a shape no one names a file of their own. The export leaves such
// files out, and an import removes those an interrupted one left behind (see
// removeStaleTemps).
const (
	tempPrefix = ".astro-import-"
	tempSuffix = ".tmp"
)

var tempShape = regexp.MustCompile(`^` + regexp.QuoteMeta(tempPrefix) + `[0-9a-f]{16}` + regexp.QuoteMeta(tempSuffix) + `$`)

// staleTempAge is how old a temporary file must be before an import removes
// it as left behind. A younger one may be a concurrent import's, still being
// written; an hour is longer than any import of an Astro project takes, and
// a stale file left a little longer costs nothing (the export ignores it).
// An age needs no lock file, which itself could be left behind.
const staleTempAge = time.Hour

// errArchiveTooLarge is the failure of an archive that holds more than an
// import reads.
var errArchiveTooLarge = errors.New("the archive is larger than an import reads")

// extractTarGzArchive writes a tar.gz archive's directories and regular files
// under targetDir, and counts the files and bytes it wrote.
//
// The archive comes from the Astro IDE, but it is written into a directory of
// the user's, so it is checked as untrusted, in two passes over the one open
// archive.
//
// The first pass writes nothing. It fails the import on anything that can be
// decided before writing: an entry outside targetDir; an entry of a kind an
// import does not write (a symlink, a hard link, a device); the same file
// twice, or a path that is a file in one entry and a directory in another,
// with names compared the way the filesystem compares them (without case on
// macOS and Windows); an entry whose path, as targetDir is now, runs through
// a symlink or a file, is a directory where the archive has a file (or the
// reverse), or is a symlink or anything else but a regular file where the
// archive has a file; and more entries or bytes than the limits above.
//
// The second pass writes only the entries the first one checked, in the same
// order with the same names, kinds and sizes, and fails on any other. It
// writes through an os.Root on targetDir, so no write can leave it, whatever
// changes on disk between the passes. Each file goes to a temporary file in
// its directory, created with its masked mode, and is then renamed into
// place: a file the import replaces is the old one or the new one, never
// emptied or half written. A failure in this pass (a full disk, an interrupt)
// leaves the files written before it, each one whole.
func extractTarGzArchive(ctx httpContext.Context, archive io.ReadSeeker, targetDir string) (archiveStats, error) {
	root, err := os.OpenRoot(targetDir)
	if err != nil {
		return archiveStats{}, err
	}
	defer root.Close() //nolint:errcheck // a directory handle; nothing is flushed by closing it

	p := newPlan(root, foldFor(runtime.GOOS))
	if err := walkArchive(ctx, archive, func(h *tar.Header, _ io.Reader) error { return p.check(h) }); err != nil {
		return archiveStats{}, err
	}
	p.removeStaleTemps(time.Now())
	next := 0
	if err := walkArchive(ctx, archive, func(h *tar.Header, r io.Reader) error {
		if h.Typeflag == tar.TypeXGlobalHeader {
			return nil
		}
		// Any other kind pass 1 did not check, a symlink included, is not
		// what was checked: writes(h) is false for it and it fails here.
		if !writes(h) || next >= len(p.checked) || p.checked[next] != checkedOf(h) {
			return errArchiveChanged
		}
		next++
		return extractEntry(ctx, root, h, r)
	}); err != nil {
		return archiveStats{}, err
	}
	if next != len(p.checked) {
		return archiveStats{}, errArchiveChanged
	}
	return p.stats, nil
}

// errArchiveChanged is the failure of an archive whose second reading is not
// the one the first pass checked.
var errArchiveChanged = errors.New("the archive changed between its check and its extraction")

// walkArchive calls each on every entry of the archive, read from its start,
// reading at most what the limits allow, and stops at ctx's end.
func walkArchive(ctx httpContext.Context, archive io.ReadSeeker, each func(h *tar.Header, r io.Reader) error) error {
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gz, err := gzip.NewReader(newLimitedReader(archive, maxImportBytes+archiveOverhead))
	if err != nil {
		return err
	}
	defer gz.Close() //nolint:errcheck // a reader; the tar's own errors are what matter
	tr := tar.NewReader(newLimitedReader(gz, maxImportBytes+archiveOverhead))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := each(h, tr); err != nil {
			return err
		}
	}
}

// limitedReader reads at most limit bytes of r, and fails with
// errArchiveTooLarge when r holds more, where io.LimitReader would end
// quietly. It reads one byte past the limit to tell the two apart.
type limitedReader struct {
	r    io.Reader
	left int64 // the limit + 1, less what was read
}

func newLimitedReader(r io.Reader, limit int64) *limitedReader {
	return &limitedReader{r: r, left: limit + 1}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, errArchiveTooLarge
	}
	if int64(len(p)) > l.left {
		p = p[:l.left]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left <= 0 {
		// The byte past the limit was read: r holds more than the limit.
		return n - 1, errArchiveTooLarge
	}
	return n, err
}

// checkedEntry is what the first pass checked of an entry the second writes.
type checkedEntry struct {
	rel      string
	typeflag byte
	size     int64
}

func checkedOf(h *tar.Header) checkedEntry {
	return checkedEntry{rel: filepath.Clean(filepath.FromSlash(h.Name)), typeflag: h.Typeflag, size: h.Size}
}

// writes reports whether an entry is one the second pass writes: a
// directory or a file, not the metadata the first pass skipped.
func writes(h *tar.Header) bool {
	return h.Typeflag == tar.TypeDir || h.Typeflag == tar.TypeReg
}

// plan is what the first pass learned: what the archive writes, checked, and
// what the second pass is to write.
type plan struct {
	root *os.Root
	// fold is how this filesystem compares names (see foldFor).
	fold    func(string) string
	stats   archiveStats
	checked []checkedEntry
	files   map[string]bool // file entries, by folded path
	dirs    map[string]bool // directories entries create or files sit under, folded
	onDisk  map[string]lstatResult
	// fileDirs are the directories files are written into, for
	// removeStaleTemps.
	fileDirs map[string]bool
}

type lstatResult struct {
	info fs.FileInfo
	err  error
}

func newPlan(root *os.Root, fold func(string) string) *plan {
	return &plan{
		root: root, fold: fold,
		files: map[string]bool{}, dirs: map[string]bool{}, onDisk: map[string]lstatResult{}, fileDirs: map[string]bool{},
	}
}

// foldFor is how goos's usual filesystem compares names: as they are on
// Linux; without case on macOS (APFS's default) and Windows, where trailing
// dots and spaces of a name are dropped too. Two names that fold the same are
// one path there, so the archive may not hold both.
func foldFor(goos string) func(string) string {
	switch goos {
	case "darwin":
		return strings.ToLower
	case "windows":
		return func(rel string) string {
			parts := strings.Split(strings.ToLower(rel), string(filepath.Separator))
			for i, part := range parts {
				parts[i] = strings.TrimRight(part, ". ")
			}
			return strings.Join(parts, string(filepath.Separator))
		}
	default:
		return func(rel string) string { return rel }
	}
}

// check is the first pass on one entry.
func (p *plan) check(h *tar.Header) error {
	switch h.Typeflag {
	case tar.TypeXGlobalHeader:
		// Metadata for the whole archive (git archive writes one), not a file.
		return nil
	case tar.TypeDir, tar.TypeReg:
	default:
		return fmt.Errorf("the archive entry %q is a %s, which an import does not write", h.Name, entryKind(h.Typeflag))
	}
	if len(p.checked) >= maxImportEntries {
		return fmt.Errorf("the archive holds more than %d entries, more than an import writes", maxImportEntries)
	}
	if !filepath.IsLocal(filepath.FromSlash(h.Name)) {
		return fmt.Errorf("the archive entry %q is outside the directory being imported into", h.Name)
	}
	rel := filepath.Clean(filepath.FromSlash(h.Name))
	isDir := h.Typeflag == tar.TypeDir
	if !isDir && isImportTemp(filepath.Base(rel)) {
		// Written, it would be dropped by the export and swept by a later
		// import as left behind.
		return fmt.Errorf("the archive entry %q has the name of an import's temporary file", h.Name)
	}
	if !isDir {
		if h.Size < 0 || h.Size > maxImportBytes-p.stats.bytes {
			return fmt.Errorf("the archive holds more than %d GiB, more than an import writes", maxImportBytes/gib)
		}
	}
	if err := p.checkAgainstArchive(h.Name, rel, isDir); err != nil {
		return err
	}
	if err := p.checkAgainstDisk(rel, isDir); err != nil {
		return fmt.Errorf("the archive entry %q: %w", h.Name, err)
	}
	p.checked = append(p.checked, checkedOf(h))
	if !isDir {
		p.stats.files++
		p.stats.bytes += h.Size
		p.fileDirs[filepath.Dir(rel)] = true
	}
	return nil
}

// parents lists the directories rel sits under, outermost first, "." left out.
func parents(rel string) []string {
	var out []string
	for d := filepath.Dir(rel); d != "."; d = filepath.Dir(d) {
		out = append([]string{d}, out...)
	}
	return out
}

// checkAgainstArchive refuses an entry that clashes with an earlier one: the
// same file twice, or a path that is a file in one entry and a directory in
// another, names compared as the filesystem compares them.
func (p *plan) checkAgainstArchive(name, rel string, isDir bool) error {
	for _, d := range parents(rel) {
		if p.files[p.fold(d)] {
			return fmt.Errorf("the archive entry %q is under %s, which the archive also writes as a file", name, d)
		}
		p.dirs[p.fold(d)] = true
	}
	key := p.fold(rel)
	switch {
	case p.files[key] && isDir:
		return fmt.Errorf("the archive entry %q is a directory, which the archive also writes as a file", name)
	case p.files[key]:
		return fmt.Errorf("the archive writes the file %q more than once", name)
	case !isDir && p.dirs[key]:
		return fmt.Errorf("the archive entry %q is a file, which the archive also writes as a directory", name)
	}
	if isDir {
		p.dirs[key] = true
	} else {
		p.files[key] = true
	}
	return nil
}

// checkAgainstDisk refuses an entry that targetDir, as it is now, cannot take:
// a directory on its path that is a symlink or a file; a directory where the
// entry is a file, or a file where it is a directory; or, at a file entry's
// own path, a symlink or anything else that is not a regular file (a device,
// a Windows junction), which the import would otherwise replace unasked.
func (p *plan) checkAgainstDisk(rel string, isDir bool) error {
	dirs := parents(rel)
	if isDir {
		dirs = append(dirs, rel)
	}
	for _, d := range dirs {
		info, err := p.lstat(d)
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing below exists yet; MkdirAll makes real directories.
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("refusing to write through the symbolic link %s", d)
		case !info.IsDir():
			return fmt.Errorf("%s is a file here, where the archive needs a directory", d)
		}
	}
	if isDir {
		return nil
	}
	info, err := p.lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case info.IsDir():
		return fmt.Errorf("%s is a directory here, where the archive has a file", rel)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("refusing to replace the symbolic link %s", rel)
	case !info.Mode().IsRegular():
		return fmt.Errorf("refusing to replace %s, which is not a regular file", rel)
	}
	return nil
}

// lstat is root.Lstat, once per path for the pass.
func (p *plan) lstat(rel string) (fs.FileInfo, error) {
	if r, seen := p.onDisk[rel]; seen {
		return r.info, r.err
	}
	info, err := p.root.Lstat(rel)
	p.onDisk[rel] = lstatResult{info: info, err: err}
	return info, err
}

// removeStaleTemps removes the temporary files an interrupted import left in
// the directories this one writes files into: regular files of exactly the
// temporary shape, older than staleTempAge at now. A directory that does not
// exist yet holds none.
func (p *plan) removeStaleTemps(now time.Time) {
	for d := range p.fileDirs {
		f, err := p.root.Open(d)
		if err != nil {
			continue
		}
		names, _ := f.Readdirnames(-1) //nolint:errcheck // best effort: what was read is cleaned
		f.Close()
		for _, n := range names {
			if !tempShape.MatchString(n) {
				continue
			}
			rel := filepath.Join(d, n)
			info, err := p.root.Lstat(rel)
			if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) < staleTempAge {
				continue
			}
			_ = p.root.Remove(rel) //nolint:errcheck // best effort; a temp left is ignored by export
		}
	}
}

// isImportTemp reports whether a regular file's name is of the shape an
// import's temporary files have.
func isImportTemp(name string) bool {
	return tempShape.MatchString(name)
}

// entryKind names a tar entry type an import refuses.
func entryKind(t byte) string {
	switch t {
	case tar.TypeSymlink:
		return "symbolic link"
	case tar.TypeLink:
		return "hard link"
	case tar.TypeChar, tar.TypeBlock:
		return "device"
	case tar.TypeFifo:
		return "named pipe"
	default:
		return fmt.Sprintf("tar entry of type %q", t)
	}
}

// extractEntry writes one directory or regular file, checked by the first
// pass, through root.
func extractEntry(ctx httpContext.Context, root *os.Root, h *tar.Header, r io.Reader) error {
	rel := filepath.Clean(filepath.FromSlash(h.Name))
	if h.Typeflag == tar.TypeDir {
		return root.MkdirAll(rel, DefaultDirPerm)
	}
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, DefaultDirPerm); err != nil {
			return err
		}
	}
	return writeExtracted(root, rel, extractedMode(h), &ctxReader{ctx: ctx, r: r}, h.Size)
}

// extractedMode is the mode an imported file is created with (see
// importedFileMask).
func extractedMode(h *tar.Header) os.FileMode {
	return h.FileInfo().Mode().Perm()&importedFileMask | importedFileFloor
}

// ctxReader reads r until ctx ends, so an interrupt stops a long file part
// way and its temporary file is removed.
type ctxReader struct {
	ctx httpContext.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// tempName is a fresh temporary name in rel's directory (see tempPrefix).
func tempName(rel string) string {
	return filepath.Join(filepath.Dir(rel), fmt.Sprintf("%s%016x%s", tempPrefix, rand.Uint64(), tempSuffix)) //nolint:gosec // a unique name, not a secret
}

// writeExtracted writes the size bytes r holds to rel under root, through a
// temporary file beside it renamed onto it: what was at rel is replaced only
// once the whole file is written, and is left as it was when the write fails.
func writeExtracted(root *os.Root, rel string, mode os.FileMode, r io.Reader, size int64) (err error) {
	var tmp *os.File
	var tmpRel string
	for range 10 {
		tmpRel = tempName(rel)
		tmp, err = root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = root.Remove(tmpRel) //nolint:errcheck // best-effort cleanup; the write error is what is returned
		}
	}()
	_, copyErr := io.CopyN(tmp, r, size)
	if cerr := errors.Join(copyErr, tmp.Close()); cerr != nil {
		return cerr
	}
	return fsatomic.Replace(func() error { return root.Rename(tmpRel, rel) })
}
