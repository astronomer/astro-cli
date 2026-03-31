package airflow

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/util"
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

	//go:embed include/airflow3/dockerfile.client
	Af3DockerfileClient string

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

	//go:embed include/airflow3/requirements-client.txt
	Af3RequirementsTxtClient string
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
func Init(path, airflowImageName, airflowImageTag, template, clientImageTag string) error {
	if template != "" {
		err := ExtractTemplate(template, path)
		if err != nil {
			return errors.Wrap(err, "failed to set up template-based astro project")
		}
		return nil
	}

	var afVersion airflowrt.AirflowVersion
	switch airflowversions.AirflowMajorVersionForRuntimeVersion(airflowImageTag) {
	case "3":
		afVersion = airflowrt.Airflow3
	case "2":
		afVersion = airflowrt.Airflow2
	default:
		return errors.New("unsupported Airflow major version for runtime version " + airflowImageTag)
	}

	cfg := airflowrt.ScaffoldConfig{
		AirflowVersion:      afVersion,
		RuntimeImageName:    airflowImageName,
		RuntimeImageTag:     airflowImageTag,
		IncludeTests:        true,
		IncludeReadme:       true,
		IncludeSettingsYaml: true,
	}
	if clientImageTag != "" {
		baseImageRegistry := config.CFG.RemoteBaseImageRegistry.GetString()
		if util.IsAstronomerRegistry(baseImageRegistry) {
			baseImageRegistry = fmt.Sprintf("%s/baseimages", baseImageRegistry)
		}
		cfg.ClientImage = &airflowrt.ClientImageConfig{
			BaseImageRegistry: baseImageRegistry,
			ImageTag:          clientImageTag,
		}
	}

	dirs, files, err := airflowrt.Scaffold(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to scaffold project")
	}

	if err := initDirs(path, dirs); err != nil {
		return errors.Wrap(err, "failed to create project directories")
	}

	fileMap := make(map[string]string, len(files))
	for _, f := range files {
		fileMap[f.RelPath] = f.Content
	}
	if err := initFiles(path, fileMap); err != nil {
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
