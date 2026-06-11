package fileutil

import (
	"archive/tar"
	"bufio"
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
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
)

const openPermissions = 0o777

var (
	perm                  os.FileMode = 0o777
	openFile                          = os.OpenFile
	readFile                          = os.ReadFile
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

// Tar archives the source directory into the target file. Entries matching
// excludePathPrefixes (compared against the final tar path) or the optional
// skip predicate (given the source-relative, slash-separated path) are omitted.
func Tar(source, target string, prependBaseDir bool, excludePathPrefixes []string, skip func(relPath string) bool) error {
	tarfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	tarball := tar.NewWriter(tarfile)
	defer tarball.Close()

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return nil
	}

	if !sourceInfo.IsDir() {
		return errors.New("source is not a directory")
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
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

			// set the tar file path to be relative to the source directory
			relName := strings.TrimPrefix(path, filepath.Clean(source))
			relName = strings.TrimPrefix(relName, string(filepath.Separator))

			if skip != nil && skip(filepath.ToSlash(relName)) {
				logger.Debugf("Excluding tarball path: %s", relName)
				return nil
			}

			headerName := relName
			if prependBaseDir {
				// prepend the base of the source directory to the tar file path, e.g. prepend "dags/" to "my_dag.py"
				baseDir := filepath.Base(source)
				headerName = filepath.Join(baseDir, headerName)
			}
			// force use forward slashes in tar files
			headerName = filepath.ToSlash(headerName)

			// ignore excluded paths
			if hasAnyPrefix(headerName, excludePathPrefixes) {
				logger.Debugf("Excluding tarball path: %s", headerName)
				return nil
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
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// this functions reads a whole file into memory and returns a slice of its lines.
func Read(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func ReadFileToString(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func Contains(elems []string, param string) (exist bool, position int) {
	for index, elem := range elems {
		if param == elem {
			return true, index
		}
	}
	return false, 0
}

// This function finds all files of a specific extension
func GetFilesWithSpecificExtension(folderPath, ext string) []string {
	var files []string
	filepath.Walk(folderPath, func(path string, f os.FileInfo, _ error) error { //nolint:errcheck
		if f != nil && !f.IsDir() {
			r, err := regexp.MatchString(ext, f.Name())
			if err == nil && r {
				files = append(files, f.Name())
			}
		}
		return nil
	})

	return files
}

func AddLineToFile(filePath, lineText, commentText string) error {
	f, err := openFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm) //nolint:mnd
	if err != nil {
		return err
	}
	content, err := readFile(filePath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(content), lineText) {
		_, err = f.WriteString("\n" + lineText + " " + commentText)
		if err != nil {
			return err
		}
	}
	f.Close()
	return err
}

// This function removes airflow db init from the Dockerfile. Needed by astro run command
func RemoveLineFromFile(filePath, lineText, commentText string) error {
	f, err := openFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm) //nolint:mnd
	if err != nil {
		return err
	}
	content, err := readFile(filePath)
	if err != nil {
		return err
	}
	if strings.Contains(string(content), lineText) {
		lastInd := strings.LastIndex(string(content), "\n"+lineText+commentText)
		if lastInd != -1 {
			err = WriteStringToFile(filePath, string(content)[:lastInd])
			if err != nil {
				return err
			}
		}
	}
	f.Close()
	return err
}

func CreateFile(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), openPermissions); err != nil {
		return nil, err
	}
	return os.Create(p)
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
		writer.Close()

		headers := args.Headers
		headers["Content-Type"] = writer.FormDataContentType()
		fmt.Println(args.RetryDisplayMessage)
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
		defer response.Body.Close() //nolint:gocritic
		data, _ := io.ReadAll(response.Body)
		responseStatusCode := response.StatusCode

		// Return success for 2xx status code
		if response.StatusCode == http.StatusOK {
			currentUploadError = nil
			fmt.Println("upload successful")
			break
		}

		strippedOutData, _ := util.StripOutKeysFromJSONByteArray(data, []string{"exceptions", "args", "path", "status"})
		currentUploadError = fmt.Errorf("file upload failed. Status code: %d and Message: %s", responseStatusCode, string(strippedOutData)) //nolint

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
	defer gzipWriter.Close()

	// Copy contents of the file to the gzip writer
	_, err = io.Copy(gzipWriter, srcFile)
	return err
}

func IsHidden(filePath string) bool {
	components := strings.Split(filePath, string(filepath.Separator))
	for _, component := range components {
		if strings.HasPrefix(component, ".") {
			return true
		}
	}
	return false
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

// CopyDirectory recursively copies a directory from src to dst.
func CopyDirectory(src, dst string) error {
	return CopyDirectoryFiltered(src, dst, nil)
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
