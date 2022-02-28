package fileutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

var errNotHomeDir = errors.New("current working directory is a home directory")

// GetWorkingDir returns the current working directory
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// GetHomeDir returns the home directory
func GetHomeDir() (string, error) {
	return homedir.Dir()
}

// FindDirInPath walks up the current directory looking for the .astro folder
// TODO Deprecate if remains unused, removed due to
// https://github.com/astronomer/astro-cli/issues/103
func FindDirInPath(search string) (string, error) {
	// Start in our current directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return "", err
	}

	// Recursively walk up the filesystem tree
	for {
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
			return "", errNotHomeDir
		}

		// Check if our file exists
		exists, err := Exists(filepath.Join(workingDir, search), nil)
		if err != nil {
			return "", fmt.Errorf("failed to check existence of '%s': %w", filepath.Join(workingDir, search), err)
		}

		// Return where we found it
		if exists {
			return filepath.Join(workingDir, search), nil
		}

		// Set the directory, and try again
		workingDir = path.Dir(workingDir)
	}
}

// IsEmptyDir checks if path is an empty dir
func IsEmptyDir(dirPath string) bool {
	f, err := os.Open(dirPath)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	return err == io.EOF
}
