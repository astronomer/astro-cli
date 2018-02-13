package utils

import (
	"fmt"
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
