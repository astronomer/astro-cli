package fileutil

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
)

// GetWorkingDir returns the curent working directory
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// GetHomeDir returns the home directory
func GetHomeDir() (string, error) {
	return homedir.Dir()
}

// FindDirInPath walks up the current directory looking for the .astro folder
// TODO Deprecate if remains unused, removed due to
// https://github.com/sjmiller609/astro-cli/issues/103
func FindDirInPath(search string) (string, error) {
	// Start in our current directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return "", err
	}

	// Recursively walk up the filesystem tree
	for true {
		// Return if we're in root
		if workingDir == "/" {
			return "", nil
		}

		// If searching home path, stop at home root
		homeDir, err := GetHomeDir()
		if err != nil {
			return "", err
		}

		if workingDir == homeDir {
			return "", errors.New("current working directory is a home directory")
		}

		// Check if our file exists
		exists, err := Exists(filepath.Join(workingDir, search))
		if err != nil {
			return "", errors.Wrapf(err, "failed to check existence of '%s'", filepath.Join(workingDir, search))
		}

		// Return where we found it
		if exists {
			return filepath.Join(workingDir, search), nil
		}

		// Set the directory, and try again
		workingDir = path.Dir(workingDir)
	}

	return "", nil
}

// IsEmptyDir checks if path is an empty dir
func IsEmptyDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true
	}
	return false // Either not empty or error, suits both cases
}
