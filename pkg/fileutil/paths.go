package fileutil

import (
	"os"

	homedir "github.com/mitchellh/go-homedir"
)

// GetWorkingDir returns the current working directory
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// GetHomeDir returns the home directory
func GetHomeDir() (string, error) {
	return homedir.Dir()
}
