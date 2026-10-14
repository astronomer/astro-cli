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

// WriteStringToFile write a string to a file
func WriteStringToFile(filePath, s string) error {
	return WriteToFile(filePath, strings.NewReader(s))
}

// WriteToFile writes an io.Reader to a file if it does not exst
func WriteToFile(filePath string, r io.Reader) error {
	dir := filepath.Dir(filePath)
	if dir != "" {
		if err := os.MkdirAll(dir, perm); err != nil {
			return err
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(file, r)
	return err
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
	// file or link the walk finds there. A directory there is not replaced:
	// callers must not add an extra where one exists.
	Extra map[string][]byte
}

// extraFileMode is an Extra file's mode in the tarball: world-readable, since
// whatever unpacks it (the Airflow runtime user) must be able to read it.
const extraFileMode = 0o644

// Tar writes the directory source to the tarball target, its paths
// relative to source, or to source's parent with prependBaseDir, changed by
// overlay. A source that is not there makes an empty tarball, not an error.
func Tar(source, target string, prependBaseDir bool, excludePathPrefixes []string, overlay TarOverlay) error {
	baseDir := ""
	if prependBaseDir {
		baseDir = filepath.Base(source)
	}
	return writeTar(source, target, baseDir, excludePathPrefixes, overlay, false)
}

// TarDir writes the directory source to the tarball target, each path in it
// under baseDir: "dags" makes dags/my_dag.py of source/my_dag.py, whatever
// source is called. Unlike Tar it fails on a source that is not there, or
// that goes while it is read, rather than writing a tarball short of it.
func TarDir(source, target, baseDir string) error {
	return writeTar(source, target, baseDir, nil, TarOverlay{}, true)
}

// createTarFile creates the tarball writeTar writes; a test fails its Close.
var createTarFile = func(name string) (io.WriteCloser, error) { return os.Create(name) }

// writeTar writes the tarball. A tarball whose footer or file did not close
// is not one, so failing to close either fails it.
func writeTar(source, target, baseDir string, excludePathPrefixes []string, overlay TarOverlay, mustExist bool) (err error) {
	tarfile, err := createTarFile(target)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := tarfile.Close(); err == nil {
			err = cerr
		}
	}()

	tarball := tar.NewWriter(tarfile)
	// Closing writes the footer; deferred after the file's, it runs first.
	defer func() {
		if cerr := tarball.Close(); err == nil {
			err = cerr
		}
	}()
	return addToTar(tarball, source, target, baseDir, excludePathPrefixes, overlay, mustExist)
}

// addToTar writes the directory source into tarball, as writeTar says.
func addToTar(tarball *tar.Writer, source, target, baseDir string, excludePathPrefixes []string, overlay TarOverlay, mustExist bool) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		if mustExist {
			return err
		}
		return nil
	}

	if !sourceInfo.IsDir() {
		return errors.New("source is not a directory")
	}

	err = filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return tarDir(path, source, info, overlay)
			}

			if path == target {
				return nil
			}

			var link string
			if info.Mode()&os.ModeSymlink == os.ModeSymlink {
				if link, err = os.Readlink(path); err != nil {
					return err
				}
			}

			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}

			headerName, ok, err := tarEntryName(path, source, baseDir, excludePathPrefixes, overlay, info)
			if err != nil || !ok {
				return err
			}
			header.Name = headerName
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

func backOff(retryDelayInMS, backoffFactor int) int {
	// exponential backoff
	time.Sleep(time.Duration(retryDelayInMS) * time.Millisecond)
	retryDelayInMS *= backoffFactor
	return retryDelayInMS
}

func UploadFile(args *UploadFileArguments) error {
	file, err := os.Open(args.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	retryDelayInMS := args.InitialDelayInMS
	currentUploadError := err

	for i := 1; i <= args.MaxTries; i++ {
		// Create a new buffer to store the multipart/form-data request
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		// Create a form field for the file
		fileWriter, err := writer.CreateFormFile(args.FormFileFieldName, args.FilePath)
		if err != nil {
			return fmt.Errorf("error creating form file: %w", err)
		}
		// Copy the file content into the form field
		_, err = ioCopy(fileWriter, file)
		if err != nil {
			return fmt.Errorf("error copying file content: %w", err)
		}
		// Add the description field to the multipart request
		if args.Description != "" {
			err = writer.WriteField("description", args.Description)
			if err != nil {
				return fmt.Errorf("error adding description field: %w", err)
			}
		}
		// Close the multipart writer to finalize the request
		writer.Close() //nolint:errcheck // best-effort close

		headers := args.Headers
		headers["Content-Type"] = writer.FormDataContentType()
		fmt.Fprintln(args.out(), args.RetryDisplayMessage)
		req, err := newRequestWithContext(http_context.Background(), "POST", args.TargetURL, body)
		if err != nil {
			currentUploadError = err
			if i < args.MaxTries {
				retryDelayInMS = backOff(retryDelayInMS, args.BackoffFactor)
			}
			continue
		}

		// set headers
		setHeaders(req, headers)

		client := &http.Client{}
		response, err := client.Do(req)
		if err != nil {
			currentUploadError = err
			// If we have a dial tcp error, we should retry with exponential backoff
			if i < args.MaxTries {
				retryDelayInMS = backOff(retryDelayInMS, args.BackoffFactor)
			}
			continue
		}
		defer response.Body.Close()          //nolint:gocritic // intentional in this shell code
		data, _ := io.ReadAll(response.Body) //nolint:errcheck // error deliberately ignored in this shell code
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
			retryDelayInMS = backOff(retryDelayInMS, args.BackoffFactor)
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

	destFile, err := os.Create(destFilePath)
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
