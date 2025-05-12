package airflow

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
)

var perm os.FileMode = 0o777

var ExtractTemplate = InitFromTemplate

// Airflow 2 files
var (
	//go:embed include/airflow2/astronomermonitoringdag.py
	Af2MonitoringDag string

	//go:embed include/airflow2/exampledag.py
	Af2ExampleDag string

	//go:embed include/airflow2/composeyml.go.tmpl
	Af2Composeyml string

	//go:embed include/airflow2/dagexampletest.py
	Af2DagExampleTest string

	//go:embed include/airflow2/dagintegritytestdefault.py
	Af2DagIntegrityTestDefault string

	//go:embed include/airflow2/dockerfile
	Af2Dockerfile string

	//go:embed include/airflow2/dockerignore
	Af2Dockerignore string

	//go:embed include/airflow2/gitignore
	Af2Gitignore string

	//go:embed include/airflow2/readme
	Af2Readme string

	//go:embed include/airflow2/settingsyml.yml
	Af2Settingsyml string

	//go:embed include/airflow2/requirements.txt
	Af2RequirementsTxt string
)

// Airflow 3 files
var (
	//go:embed include/airflow3/exampledag.py
	Af3ExampleDag string

	//go:embed include/airflow3/composeyml.go.tmpl
	Af3Composeyml string

	//go:embed include/airflow3/dagexampletest.py
	Af3DagExampleTest string

	//go:embed include/airflow3/dagintegritytestdefault.py
	Af3DagIntegrityTestDefault string

	//go:embed include/airflow3/dockerfile
	Af3Dockerfile string

	//go:embed include/airflow3/dockerignore
	Af3Dockerignore string

	//go:embed include/airflow3/gitignore
	Af3Gitignore string

	//go:embed include/airflow3/readme
	Af3Readme string

	//go:embed include/airflow3/settingsyml.yml
	Af3Settingsyml string

	//go:embed include/airflow3/requirements.txt
	Af3RequirementsTxt string
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
func Init(path, airflowImageName, airflowImageTag, template string) error {
	if template != "" {
		err := ExtractTemplate(template, path)
		if err != nil {
			return errors.Wrap(err, "failed to set up template-based astro project")
		}
		return nil
	}

	// Initialize directories
	dirs := []string{"dags", "plugins", "include"}
	if err := initDirs(path, dirs); err != nil {
		return errors.Wrap(err, "failed to create project directories")
	}

	// Initialize files
	var files map[string]string
	switch airflowversions.AirflowMajorVersionForRuntimeVersion(airflowImageTag) {
	case "3":
		// Use the floating tag for the runtime version, so that the latest patch versions are automatically used
		airflowImageFloatingTag := airflowversions.RuntimeVersionMajorMinor(airflowImageTag)
		if airflowImageFloatingTag != "" {
			airflowImageTag = airflowImageFloatingTag
		}
		files = map[string]string{
			".dockerignore":                        Af3Dockerignore,
			"Dockerfile":                           fmt.Sprintf(Af3Dockerfile, airflowImageName, airflowImageTag),
			".gitignore":                           Af3Gitignore,
			"packages.txt":                         "",
			"requirements.txt":                     Af3RequirementsTxt,
			".env":                                 "",
			"airflow_settings.yaml":                Af3Settingsyml,
			"dags/exampledag.py":                   Af3ExampleDag,
			"dags/.airflowignore":                  "",
			"README.md":                            Af3Readme,
			"tests/dags/test_dag_example.py":       Af3DagExampleTest,
			".astro/test_dag_integrity_default.py": Af3DagIntegrityTestDefault,
			".astro/dag_integrity_exceptions.txt":  "# Add dag files to exempt from parse test below. ex: dags/<test-file>",
		}
	case "2":
		files = map[string]string{
			".dockerignore":                        Af2Dockerignore,
			"Dockerfile":                           fmt.Sprintf(Af2Dockerfile, airflowImageName, airflowImageTag),
			".gitignore":                           Af2Gitignore,
			"packages.txt":                         "",
			"requirements.txt":                     Af2RequirementsTxt,
			".env":                                 "",
			"airflow_settings.yaml":                Af2Settingsyml,
			"dags/exampledag.py":                   Af2ExampleDag,
			"dags/.airflowignore":                  "",
			"README.md":                            Af2Readme,
			"tests/dags/test_dag_example.py":       Af2DagExampleTest,
			".astro/test_dag_integrity_default.py": Af2DagIntegrityTestDefault,
			".astro/dag_integrity_exceptions.txt":  "# Add dag files to exempt from parse test below. ex: dags/<test-file>",
		}
	default:
		return errors.New("unsupported Airflow major version for runtime version " + airflowImageTag)
	}
	if err := initFiles(path, files); err != nil {
		return errors.Wrap(err, "failed to create project files")
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
