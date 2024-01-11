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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

const openPermissions = 0o777

var (
	perm     os.FileMode = 0o777
	openFile             = os.OpenFile
	readFile             = os.ReadFile
	ioCopy               = io.Copy
)

// Exists returns a boolean indicating if the given path already exists
func Exists(path string, fs afero.Fs) (bool, error) {
	if path == "" {
		return false, nil
	}

	if fs == nil {
		_, err := os.Stat(path)
		if err == nil {
			return true, nil
		}

		if !os.IsNotExist(err) {
			return false, errors.Wrap(err, "cannot determine if path exists, error ambiguous")
		}
	} else {
		res, err := afero.Exists(fs, path)
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
func WriteStringToFile(path, s string) error {
	return WriteToFile(path, strings.NewReader(s))
}

// WriteToFile writes an io.Reader to a file if it does not exst
func WriteToFile(path string, r io.Reader) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, perm); err != nil {
			return err
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(file, r)
	return err
}

func Tar(source, target string) error {
	filename := filepath.Base(source)
	target = filepath.Join(target, fmt.Sprintf("%s.tar", filename))
	tarfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	tarball := tar.NewWriter(tarfile)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			if err != nil {
				return err
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

			if baseDir != "" {
				header.Name = filepath.ToSlash(filepath.Join(baseDir, strings.TrimPrefix(path, source)))
			}

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

// this functions reads a whole file into memory and returns a slice of its lines.
func Read(path string) ([]string, error) {
	file, err := os.Open(path)
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
	f, err := openFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm) //nolint:gomnd
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
	f, err := openFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm) //nolint:gomnd
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

func UploadFile(filePath, targetURL, formFileFieldName string, headers map[string]string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	// Create a new buffer to store the multipart/form-data request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create a form field for the file
	fileWriter, err := writer.CreateFormFile(formFileFieldName, filePath)
	if err != nil {
		return fmt.Errorf("error creating form file: %w", err)
	}

	// Copy the file content into the form field
	_, err = ioCopy(fileWriter, file)
	if err != nil {
		return fmt.Errorf("error copying file content: %w", err)
	}

	// Close the multipart writer to finalize the request
	writer.Close()

	req, err := http.NewRequestWithContext(http_context.Background(), "POST", targetURL, body)
	if err != nil {
		return err
	}

	headers["Content-Type"] = writer.FormDataContentType()

	// set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Perform the request
	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return fmt.Errorf("error making POST request: %w", err)
	}
	defer response.Body.Close()

	// Check the response status
	if response.StatusCode == http.StatusOK {
		return nil
	}
	data, _ := io.ReadAll(response.Body)
	return fmt.Errorf("file upload failed. Status code: %d and Message: %s", response.StatusCode, string(data)) //nolint
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
