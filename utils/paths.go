package utils

import (
	"fmt"
	"os"
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
