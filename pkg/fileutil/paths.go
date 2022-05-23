package fileutil

import (
	"fmt"
	"io"
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

// IsEmptyDir checks if path is an empty dir
func IsEmptyDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	return err == io.EOF       // Either not empty or error, suits both cases
}
