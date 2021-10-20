package airflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/pkg/fileutil"
)

const (
	defaultDirPerm os.FileMode = 0777
)

func initDirs(root string, dirs []string) error {
	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		_, err := fileutil.Exists(fullpath)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		// Create directory
		if err := os.MkdirAll(fullpath, defaultDirPerm); err != nil {
			return errors.Wrapf(err, "failed to create dir '%s'", dir)
		}
	}

	return nil
}

func initFiles(root string, files map[string]string) error {
	// Create the files
	for file, content := range files {
		// Create full path to file
		fullpath := filepath.Join(root, file)

		// Move on if already exists
		fileExist, err := fileutil.Exists(fullpath)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		if fileExist {
			continue
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			return errors.Wrapf(err, "failed to create file '%s'", fullpath)
		}
	}

	return nil
}

// Init will scaffold out a new airflow project
func Init(path, airflowImageTag string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore":         include.Dockerignore,
		"Dockerfile":            fmt.Sprintf(include.Dockerfile, airflowImageTag),
		".gitignore":            include.Gitignore,
		"packages.txt":          "",
		"requirements.txt":      "",
		".env":                  "",
		"airflow_settings.yaml": include.Settingsyml,
		"dags/example-dag.py":   include.Exampledag,
	}

	// Initailize directories
	if err := initDirs(path, dirs); err != nil {
		return errors.Wrap(err, "failed to create project directories")
	}

	// Initialize files
	if err := initFiles(path, files); err != nil {
		return errors.Wrap(err, "failed to create project files")
	}

	return nil
}
