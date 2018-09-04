package airflow

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/iancoleman/strcase"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
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
		if err := os.MkdirAll(dir, 0777); err != nil {
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
		_, err := fileutil.Exists(fullpath)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			return errors.Wrapf(err, "failed to create file '%s'", fullpath)
		}
	}

	return nil
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
	if err := initDirs(path, dirs); err != nil {
		return errors.Wrap(err, "failed to create project directories")
	}

	// Initialize files
	if err := initFiles(path, files); err != nil {
		return errors.Wrap(err, "failed to create project files")
	}

	return nil
}

func validateOrCreateProjectName(path, projectName string) (string, error) {
	if len(projectName) != 0 {
		projectNameValid := regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).MatchString
		if !projectNameValid(projectName) {
			return "", errors.New(messages.CONFIG_PROJECT_NAME_ERROR)
		}
	} else {
		projectDirectory := filepath.Base(path)

		return strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1), nil
	}

	return projectName, nil
}
