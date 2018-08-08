package fileutil

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

// GetWorkingDir returns the curent working directory
func GetWorkingDir() string {
	work, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return work
}

// GetHomeDir returns the home directory
func GetHomeDir() string {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return home
}

// FindDirInPath walks up the current directory looking for the .astro folder
// TODO Deprecate if remains unused, removed due to
// https://github.com/astronomerio/astro-cli/issues/103
func FindDirInPath(search string) (string, error) {
	// Start in our current directory
	workingDir := GetWorkingDir()

	// Recursively walk up the filesystem tree
	for true {
		// Return if we're in root
		if workingDir == "/" {
			return "", nil
		}

		// If searching home path, stop at home root
		if workingDir == GetHomeDir() {
			return "", nil
		}

		// Check if our file exists
		exists := Exists(filepath.Join(workingDir, search))

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
