package airflow

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
)

var perm os.FileMode = 0o777

var (
	//go:embed include/astronomermonitoringdag.py
	MonitoringDag string

	//go:embed include/exampledag.py
	ExampleDag string

	//go:embed include/composeyml.yml
	Composeyml string

	//go:embed include/dagexampletest.py
	DagExampleTest string

	//go:embed include/dagintegritytestdefault.py
	DagIntegrityTestDefault string

	//go:embed include/dockerfile
	Dockerfile string

	//go:embed include/test-conflicts.dockerfile
	testConflictsDockerfile string

	//go:embed include/dockerignore
	Dockerignore string

	//go:embed include/gitignore
	Gitignore string

	//go:embed include/readme
	Readme string

	//go:embed include/settingsyml.yml
	Settingsyml string

	//go:embed include/requirements.txt
	RequirementsTxt string
)

func initDirs(root string, dirs []string) error {
	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		_, err := fileutil.Exists(fullpath, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		// Create directory
		if err := os.MkdirAll(fullpath, perm); err != nil {
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
		fileExist, err := fileutil.Exists(fullpath, nil)
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
func Init(path, airflowImageName, airflowImageTag string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore":                        Dockerignore,
		"Dockerfile":                           fmt.Sprintf(Dockerfile, airflowImageName, airflowImageTag),
		".gitignore":                           Gitignore,
		"packages.txt":                         "",
		"requirements.txt":                     RequirementsTxt,
		".env":                                 "",
		"airflow_settings.yaml":                Settingsyml,
		"dags/exampledag.py":                   ExampleDag,
		"dags/.airflowignore":                  "",
		"README.md":                            Readme,
		"tests/dags/test_dag_example.py":       DagExampleTest,
		".astro/test_dag_integrity_default.py": DagIntegrityTestDefault,
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

func initConflictTest(path, airflowImageName, airflowImageTag string) error {
	// Map of files to create
	files := map[string]string{
		"conflict-check.Dockerfile": fmt.Sprintf(testConflictsDockerfile, airflowImageName, airflowImageTag),
	}
	// Initialize files
	if err := initFiles(path, files); err != nil {
		return errors.Wrap(err, "failed to create upgrade check files")
	}
	return nil
}

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", name, componentName)
}

// imageName creates an airflow image name
func ImageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}
