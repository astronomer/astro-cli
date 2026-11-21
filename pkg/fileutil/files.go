package fileutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	http_context "context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
)

const openPermissions = 0o755

var (
	perm                  os.FileMode = 0o755
	ioCopy                            = io.Copy
	newRequestWithContext             = http.NewRequestWithContext
)

type UploadFileArguments struct {
	// Context ends the upload, and the waits between its retries, when the run
	// is interrupted; nil is context.Background().
	Context             http_context.Context
	FilePath            string
	TargetURL           string
	FormFileFieldName   string
	Headers             map[string]string
	Description         string
	MaxTries            int
	InitialDelayInMS    int
	BackoffFactor       int
	RetryDisplayMessage string
	// Out is where the upload's progress lines go; nil is os.Stdout.
	Out io.Writer
}

func (a *UploadFileArguments) ctx() http_context.Context {
	if a.Context != nil {
		return a.Context
	}
	return http_context.Background()
}

func (a *UploadFileArguments) out() io.Writer {
	if a.Out != nil {
		return a.Out
	}
	return os.Stdout
}

// Exists returns a boolean indicating if the given path already exists
func Exists(filePath string, fs afero.Fs) (bool, error) {
	if filePath == "" {
		return false, nil
	}

	if fs == nil {
		_, err := os.Stat(filePath)
		if err == nil {
			return true, nil
		}

		if !os.IsNotExist(err) {
			return false, errors.Wrap(err, "cannot determine if path exists, error ambiguous")
		}
	} else {
		res, err := afero.Exists(fs, filePath)
		if res {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("cannot determine if path exists, error ambiguous: %w", err)
		}
	}

	return false, nil
}

// TarOverlay changes what Tar writes without touching source. Paths are
// slash-separated and relative to source, before any base directory is
// prepended.
type TarOverlay struct {
	// Skip, when set, is asked about each entry the walk reaches below source -
	// a file, a link, a directory - by rel, its path on disk and what Lstat
	// says of it, and leaves it out on true: a directory with everything
	// under it. An error fails the tarball.
	Skip func(rel, file string, info os.FileInfo) (bool, error)
	// Extra adds a regular file with these bytes at each path, in place of a
	// file or link the walk finds there. A directory there is still walked
	// unless Skip leaves it out, and its entries would collide with the file:
	// a caller adding an extra where one may exist skips it too.
	Extra map[string][]byte
}

// extraFileMode is an Extra file's mode in the tarball: world-readable, since
// whatever unpacks it (the Airflow runtime user) must be able to read it.
const extraFileMode = 0o644

// TarGz writes the directory source to the gzip-compressed tarball target,
// in one pass, its paths relative to source, or to source's parent with
// prependBaseDir, changed by overlay. A source that is not there makes an
// empty tarball, not an error.
func TarGz(source, target string, prependBaseDir bool, excludePathPrefixes []string, overlay TarOverlay) error {
	src := GzSource(source, prependBaseDir, excludePathPrefixes)
	return writeTar(src.Dir, target, src.BaseDir, src.Excludes, overlay, true, false)
}

// GzSource is the TarSource TarGz writes with the same arguments: with
// prependBaseDir, each entry under source's own name, the name it is given by
// (a dags/ that is a link is still "dags").
func GzSource(source string, prependBaseDir bool, excludePathPrefixes []string) TarSource {
	src := TarSource{Dir: source, Excludes: excludePathPrefixes}
	if prependBaseDir {
		src.BaseDir = filepath.Base(source)
	}
	return src
}

// TarDir writes the directory source to the tarball target, each path in it
// under baseDir: "dags" makes dags/my_dag.py of source/my_dag.py, whatever
// source is called. Unlike TarGz it fails on a source that is not there, or
// that goes while it is read, rather than writing a tarball short of it.
func TarDir(source, target, baseDir string) error {
	return writeTar(source, target, baseDir, nil, TarOverlay{}, false, true)
}

// createTarFile creates the tarball writeTar writes; a test fails its Close.
var createTarFile = func(name string) (io.WriteCloser, error) { return createFresh(name) }

// createFresh creates name as a new file for an archive the CLI writes,
// replacing whatever is there. os.Create would write through a symlink at
// name - a dags.tar a project holds as a link, say - and truncate the file it
// points at, outside the project. Removing the entry first unlinks a symlink
// rather than its target, and O_EXCL fails on one put back in between instead
// of following it. A directory at name is left alone, and fails the create as
// os.Create fails on one.
func createFresh(name string) (*os.File, error) {
	if info, err := os.Lstat(name); err == nil && !info.IsDir() {
		if err := os.Remove(name); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, archiveFileMode)
}

// archiveFileMode is the mode createFresh creates with, os.Create's: the
// user's umask decides the rest.
const archiveFileMode = 0o666

// writeTar writes the tarball, gzip-compressed with gzipped. A tarball whose
// footer, compressed stream or file did not close is not one - a full disk
// shows up there - so failing to close any of them fails it.
func writeTar(source, target, baseDir string, excludePathPrefixes []string, overlay TarOverlay, gzipped, mustExist bool) (err error) {
	tarfile, err := createTarFile(target)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := tarfile.Close(); err == nil {
			err = cerr
		}
	}()

	var out io.Writer = tarfile
	if gzipped {
		gz := gzip.NewWriter(tarfile)
		// Closing flushes what is still compressed in memory; deferred after
		// the file's, it runs before it.
		defer func() {
			if cerr := gz.Close(); err == nil {
				err = cerr
			}
		}()
		out = gz
	}

	tarball := tar.NewWriter(out)
	// Closing writes the footer; deferred last, it runs first.
	defer func() {
		if cerr := tarball.Close(); err == nil {
			err = cerr
		}
	}()
	return addToTar(tarball, source, target, baseDir, excludePathPrefixes, overlay, mustExist)
}

// TarSource is a directory as a tarball of it is written (TarDir, TarGz): Dir,
// each entry's name under BaseDir ("" for none), and the entries whose name
// starts with one of Excludes left out. A caller that both writes the tarball
// and looks at what it carries (WalkTar) builds one TarSource for both, so the
// two cannot disagree on a file.
type TarSource struct {
	Dir      string
	BaseDir  string
	Excludes []string
}

// WalkTar calls visit with each entry a tarball of src carries from disk, as
// TarDir and TarGz write it: its path on disk, its name in the tarball, and
// what Lstat says of it. A link in src is an entry of its own, as the tarball
// carries it, not the file it points at; a Dir that is itself a link is
// walked as the directory it points at. A Dir that is not there carries
// nothing. It is the same walk the tarball is written from (walkTar).
func WalkTar(src TarSource, visit func(path, name string, info os.FileInfo) error) error {
	return walkTar(src.Dir, "", src.BaseDir, src.Excludes, TarOverlay{}, false, visit)
}

// addToTar writes the directory source into tarball, as writeTar says.
func addToTar(tarball *tar.Writer, source, target, baseDir string, excludePathPrefixes []string, overlay TarOverlay, mustExist bool) error {
	err := walkTar(source, target, baseDir, excludePathPrefixes, overlay, mustExist, func(path, name string, info os.FileInfo) error {
		var link string
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			var err error
			if link, err = os.Readlink(path); err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = name
		logger.Debugf("Adding to tarball: %s", header.Name)

		if err := tarball.WriteHeader(header); err != nil {
			return err
		}

		if !info.Mode().IsRegular() { // nothing more to do for non-regular
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarball, file)
		return err
	})
	if err != nil {
		return err
	}
	return addExtrasToTar(tarball, baseDir, overlay.Extra)
}

// walkTar calls visit with each entry of source a tarball carries: every
// file and link the walk reaches, under its name in the tarball (tarEntryName),
// but not the tarball itself (target, when it sits in source), and not a
// directory, which the tarball does not hold as an entry of its own. A source
// that is a link is walked as the directory it points at: filepath.Walk does
// not descend through a link at its root, so a dags/ that was a link went
// into the tarball as that one link, to a path on the machine that wrote it,
// and none of its files did. A source that is not there is an error with
// mustExist, and carries nothing without it.
func walkTar(source, target, baseDir string, excludePathPrefixes []string, overlay TarOverlay, mustExist bool, visit func(path, name string, info os.FileInfo) error) error {
	root := source
	if resolved, err := filepath.EvalSymlinks(source); err == nil {
		root = resolved
	}
	sourceInfo, err := os.Stat(root)
	if err != nil {
		if mustExist {
			return err
		}
		return nil
	}

	if !sourceInfo.IsDir() {
		return errors.New("source is not a directory")
	}

	// The tarball is compared by its path as the walk spells it, which is
	// root's: a target written into a source given through a link is still
	// recognized.
	if target != "" {
		if dir, err := filepath.EvalSymlinks(filepath.Dir(target)); err == nil {
			target = filepath.Join(dir, filepath.Base(target))
		}
	}

	return filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return tarDir(path, root, info, overlay)
			}

			if path == target {
				return nil
			}

			name, ok, err := tarEntryName(path, root, baseDir, excludePathPrefixes, overlay, info)
			if err != nil || !ok {
				return err
			}
			return visit(path, name, info)
		})
}

// tarDir decides whether the walk descends into the directory at dir:
// filepath.SkipDir for one the overlay skips, or a bytecode cache.
func tarDir(dir, source string, info os.FileInfo, overlay TarOverlay) error {
	if dir != source && overlay.Skip != nil {
		rel, err := filepath.Rel(source, dir)
		if err != nil {
			return err
		}
		skip, err := overlay.Skip(filepath.ToSlash(rel), dir, info)
		if err != nil {
			return err
		}
		if skip {
			logger.Debugf("Leaving out of the tarball: %s", rel)
			return filepath.SkipDir
		}
	}
	return skipPycache(info, dir, source)
}

// tarEntryName is the name file goes into the tarball under, or false when
// it stays out: one under an excluded prefix, or one the overlay skips or
// replaces. info is what Lstat says of file.
func tarEntryName(file, source, baseDir string, excludePathPrefixes []string, overlay TarOverlay, info os.FileInfo) (name string, ok bool, err error) {
	// the tar file path is relative to the source directory, which may be "."
	relName, err := filepath.Rel(source, file)
	if err != nil {
		return "", false, err
	}
	// prepend the base directory to the tar file path, e.g. "dags/" to "my_dag.py",
	// with forward slashes whatever the platform
	headerName := filepath.ToSlash(filepath.Join(baseDir, relName))

	// ignore excluded paths
	for _, excludePathPrefix := range excludePathPrefixes {
		if strings.HasPrefix(headerName, excludePathPrefix) {
			logger.Debugf("Excluding tarball path: %s", headerName)
			return "", false, nil
		}
	}

	rel := filepath.ToSlash(relName)
	if overlay.Skip != nil {
		skip, err := overlay.Skip(rel, file, info)
		if err != nil {
			return "", false, err
		}
		if skip {
			logger.Debugf("Leaving out of the tarball: %s", rel)
			return "", false, nil
		}
	}
	if overlay.Extra[rel] != nil {
		logger.Debugf("Replacing in the tarball: %s", rel)
		return "", false, nil
	}
	return headerName, true, nil
}

// addExtrasToTar writes each of extra into tarball under baseDir, in path
// order so the same overlay makes the same tarball.
func addExtrasToTar(tarball *tar.Writer, baseDir string, extra map[string][]byte) error {
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	now := time.Now()
	for _, name := range names {
		data := extra[name]
		header := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     path.Join(filepath.ToSlash(baseDir), name),
			Mode:     extraFileMode,
			Size:     int64(len(data)),
			ModTime:  now,
		}
		logger.Debugf("Adding to tarball: %s", header.Name)
		if err := tarball.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarball.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// skipPycache keeps bytecode caches out of a tarball. Only the directory is
// skipped: a sourceless .pyc outside __pycache__ is importable, so it ships.
func skipPycache(info os.FileInfo, dirPath, source string) error {
	if info.Name() == "__pycache__" && dirPath != source {
		logger.Debugf("Excluding tarball path: %s", dirPath)
		return filepath.SkipDir
	}
	return nil
}

func ReadFileToString(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FilesWithExtension finds the names of the files under folderPath whose
// names match ext. It fails on a path under folderPath it cannot read, rather
// than counting it as having none.
func FilesWithExtension(folderPath, ext string) ([]string, error) {
	var files []string
	err := filepath.Walk(folderPath, func(path string, f os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if f != nil && !f.IsDir() {
			r, err := regexp.MatchString(ext, f.Name())
			if err == nil && r {
				files = append(files, f.Name())
			}
		}
		return nil
	})
	return files, err
}

// backOff waits retryDelayInMS, or until ctx ends, and returns the next
// delay (exponential backoff).
func backOff(ctx http_context.Context, retryDelayInMS, backoffFactor int) int {
	t := time.NewTimer(time.Duration(retryDelayInMS) * time.Millisecond)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
	retryDelayInMS *= backoffFactor
	return retryDelayInMS
}

// multipartBody is the multipart/form-data request body of one upload try:
// the file's content, and the description when there is one. It returns the
// body's Content-Type.
func multipartBody(args *UploadFileArguments, file io.Reader) (body *bytes.Buffer, contentType string, err error) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	// Create a form field for the file
	fileWriter, err := writer.CreateFormFile(args.FormFileFieldName, args.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("error creating form file: %w", err)
	}
	// Copy the file content into the form field
	if _, err = ioCopy(fileWriter, file); err != nil {
		return nil, "", fmt.Errorf("error copying file content: %w", err)
	}
	// Add the description field to the multipart request
	if args.Description != "" {
		if err = writer.WriteField("description", args.Description); err != nil {
			return nil, "", fmt.Errorf("error adding description field: %w", err)
		}
	}
	// Close the multipart writer to finalize the request
	writer.Close() //nolint:errcheck // best-effort close
	return body, writer.FormDataContentType(), nil
}

func UploadFile(args *UploadFileArguments) error {
	ctx := args.ctx()
	file, err := os.Open(args.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	retryDelayInMS := args.InitialDelayInMS
	currentUploadError := err

	for i := 1; i <= args.MaxTries; i++ {
		body, contentType, err := multipartBody(args, file)
		if err != nil {
			return err
		}

		headers := args.Headers
		headers["Content-Type"] = contentType
		fmt.Fprintln(args.out(), args.RetryDisplayMessage)
		req, err := newRequestWithContext(ctx, "POST", args.TargetURL, body)
		if err != nil {
			currentUploadError = err
			if i < args.MaxTries {
				retryDelayInMS = backOff(ctx, retryDelayInMS, args.BackoffFactor)
			}
			continue
		}

		// set headers
		setHeaders(req, headers)

		client := &http.Client{}
		response, err := client.Do(req)
		if err != nil {
			currentUploadError = err
			// An interrupt ends the upload; it is no failure to retry.
			if ctx.Err() != nil {
				return err
			}
			// If we have a dial tcp error, we should retry with exponential backoff
			if i < args.MaxTries {
				retryDelayInMS = backOff(ctx, retryDelayInMS, args.BackoffFactor)
			}
			continue
		}
		data, _ := io.ReadAll(response.Body) //nolint:errcheck // error deliberately ignored in this shell code
		response.Body.Close()
		responseStatusCode := response.StatusCode

		// Return success for 2xx status code
		if response.StatusCode == http.StatusOK {
			currentUploadError = nil
			fmt.Fprintln(args.out(), "upload successful")
			break
		}

		strippedOutData, _ := util.StripOutKeysFromJSONByteArray(data, []string{"exceptions", "args", "path", "status"}) //nolint:errcheck // error deliberately ignored in this shell code
		currentUploadError = fmt.Errorf("file upload failed. Status code: %d and Message: %s", responseStatusCode, string(strippedOutData))

		// don't retry for 4xx since it is a client side error
		if responseStatusCode >= http.StatusBadRequest && responseStatusCode < http.StatusInternalServerError {
			break
		}
		if i < args.MaxTries {
			retryDelayInMS = backOff(ctx, retryDelayInMS, args.BackoffFactor)
		}
		// Reset the file pointer to the beginning of the file
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("error seeking file: %w", err)
		}
	}
	return currentUploadError
}

func setHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func GzipFile(srcFilePath, destFilePath string) error {
	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := createFresh(destFilePath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Create a gzip writer on top of file writer
	gzipWriter := gzip.NewWriter(destFile)
	defer gzipWriter.Close() //nolint:errcheck // best-effort close

	// Copy contents of the file to the gzip writer
	_, err = io.Copy(gzipWriter, srcFile)
	return err
}

// CopyFile copies a file from src to dst, preserving file permissions
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

// CopyDirectoryFiltered recursively copies a directory from src to dst.
//
// If skip is non-nil it is invoked for every entry with the entry's path
// relative to the original src root (slash-delimited) and whether the entry is
// a directory. Returning true skips that entry; skipping a directory also skips
// its contents.
//
// Symlinks are recreated as symlinks rather than dereferenced. Following a
// symlink that resolves to a directory previously caused a cryptic
// "is a directory" read error (the link was treated as a regular file and
// os.Open followed it to its target directory).
func CopyDirectoryFiltered(src, dst string, skip func(relPath string, isDir bool) bool) error {
	return copyDirectory(src, dst, "", skip)
}

func copyDirectory(src, dst, relBase string, skip func(relPath string, isDir bool) bool) error {
	// Use Lstat so we do not dereference src itself if it is a symlink.
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		relPath := path.Join(relBase, entry.Name())
		isDir := entry.IsDir()

		if skip != nil && skip(relPath, isDir) {
			continue
		}

		switch {
		case entry.Type()&os.ModeSymlink != 0:
			// Recreate the symlink instead of following it. Dereferencing a
			// symlink that points at a directory caused "is a directory" errors.
			if err := copySymlink(srcPath, dstPath); err != nil {
				return err
			}
		case isDir:
			// Recursively copy subdirectory
			if err := copyDirectory(srcPath, dstPath, relPath, skip); err != nil {
				return err
			}
		default:
			// Copy file
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copySymlink recreates the symlink at src as a symlink at dst, preserving its
// target verbatim (without following it).
func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	return os.Symlink(target, dst)
}
