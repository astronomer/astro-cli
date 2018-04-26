package airflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
)

func initDirs(root string, dirs []string) bool {
	// Any inputs exist
	exists := false

	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		if fileutil.Exists(fullpath) {
			exists = true
			continue
		}

		// Create directory
		if err := os.MkdirAll(dir, 0777); err != nil {
			fmt.Println(err)
		}
	}

	return exists
}

func initFiles(root string, files map[string]string) bool {
	// Any inputs exist
	exists := false

	// Create the files
	for file, content := range files {
		// Create full path to file
		fullpath := filepath.Join(root, file)

		// Move on if already exiss
		if fileutil.Exists(fullpath) {
			exists = true
			continue
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			fmt.Println(err)
		}
	}

	return exists
}

// Init will scaffold out a new airflow project
func Init(path string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore":       include.Dockerignore,
		"Dockerfile":          include.Dockerfile,
		"packages.txt":        "",
		"requirements.txt":    "",
		"dags/example-dag.py": include.Exampledag,
	}

	// Initailize directories
	initDirs(path, dirs)

	// Initialize files
	initFiles(path, files)

	return nil
}

// Create new airflow deployment
func Create(title string) error {
	HTTP := houston.NewHTTPClient()
	API := houston.NewHoustonClient(HTTP)

	body, houstonErr := API.CreateDeployment(title)
	if houstonErr != nil {
		return houstonErr
	}

	fmt.Println(body.Data.CreateDeployment.Message)
	return nil
}

// List all airflow deployments
func List() error {
	HTTP := houston.NewHTTPClient()
	API := houston.NewHoustonClient(HTTP)

	body, houstonErr := API.FetchDeployments()
	if houstonErr != nil {
		return houstonErr
	}

	for _, d := range body.Data.FetchDeployments {
		rowTmp := "Title: %s\nId: %s\nRelease: %s\nVersion: %s\n\n"
		fmt.Printf(rowTmp, d.Title, d.Id, d.ReleaseName, d.Version)
	}
	return nil
}
