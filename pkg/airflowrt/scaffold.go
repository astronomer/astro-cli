package airflowrt

import (
	_ "embed"
	"fmt"
)

// Shared template files (identical across Airflow versions).
var (
	//go:embed include/gitignore
	Gitignore string

	//go:embed include/dockerignore
	Dockerignore string
)

// Airflow 3 template files.
var (
	//go:embed include/airflow3/exampledag.py
	Af3ExampleDag string

	//go:embed include/airflow3/dagexampletest.py
	Af3DagExampleTest string

	//go:embed include/airflow3/dagintegritytestdefault.py
	Af3DagIntegrityTestDefault string

	//go:embed include/airflow3/dockerfile
	Af3Dockerfile string

	//go:embed include/airflow3/dockerfile.client
	Af3DockerfileClient string

	//go:embed include/airflow3/readme
	Af3Readme string

	//go:embed include/airflow3/settingsyml.yml
	Af3Settingsyml string

	//go:embed include/airflow3/requirements.txt
	Af3RequirementsTxt string

	//go:embed include/airflow3/requirements-client.txt
	Af3RequirementsTxtClient string
)

// Airflow 2 template files.
var (
	//go:embed include/airflow2/exampledag.py
	Af2ExampleDag string

	//go:embed include/airflow2/dagexampletest.py
	Af2DagExampleTest string

	//go:embed include/airflow2/dagintegritytestdefault.py
	Af2DagIntegrityTestDefault string

	//go:embed include/airflow2/dockerfile
	Af2Dockerfile string

	//go:embed include/airflow2/readme
	Af2Readme string

	//go:embed include/airflow2/settingsyml.yml
	Af2Settingsyml string

	//go:embed include/airflow2/requirements.txt
	Af2RequirementsTxt string
)

// AirflowVersion selects the template set for scaffolding.
type AirflowVersion int

const (
	Airflow2 AirflowVersion = 2
	Airflow3 AirflowVersion = 3
)

// ScaffoldConfig holds all parameters needed to scaffold a project.
type ScaffoldConfig struct {
	// AirflowVersion selects the AF2 or AF3 template set. Required.
	AirflowVersion AirflowVersion

	// RuntimeImageName is the full image reference (registry + name), e.g. "astrocrpublic.azurecr.io/runtime".
	RuntimeImageName string

	// RuntimeImageTag is the tag, e.g. "3.1-12".
	RuntimeImageTag string

	// ProjectName is written into .astro/config.yaml.
	// If empty, no config.yaml is generated.
	ProjectName string

	// IncludeTests adds tests/dags/test_dag_example.py and .astro/test_dag_integrity_default.py.
	IncludeTests bool

	// IncludeReadme adds README.md.
	IncludeReadme bool

	// IncludeSettingsYaml adds airflow_settings.yaml.
	IncludeSettingsYaml bool

	// ClientImage, if non-nil, adds Dockerfile.client, requirements-client.txt, and packages-client.txt.
	// Only valid with Airflow3.
	ClientImage *ClientImageConfig
}

// ClientImageConfig holds parameters for remote-execution client files.
type ClientImageConfig struct {
	BaseImageRegistry string
	ImageTag          string
}

// ScaffoldFile represents a single file to be written during scaffolding.
type ScaffoldFile struct {
	// RelPath is the slash-separated path relative to the project root.
	RelPath string
	// Content is the file content. Empty string means create an empty file.
	Content string
}

// Scaffold returns the directories and files needed for a new Astro project.
// It does NOT touch the filesystem — callers handle writing with their own
// semantics (skip-existing, overwrite, permissions, etc.).
func Scaffold(cfg ScaffoldConfig) (dirs []string, files []ScaffoldFile, err error) {
	if cfg.RuntimeImageName == "" || cfg.RuntimeImageTag == "" {
		return nil, nil, fmt.Errorf("RuntimeImageName and RuntimeImageTag are required")
	}

	// Directories
	dirs = []string{"dags", "plugins", "include", ".astro"}
	if cfg.IncludeTests {
		dirs = append(dirs, "tests/dags")
	}

	// Common files (shared across AF versions)
	files = []ScaffoldFile{
		{".gitignore", Gitignore},
		{".dockerignore", Dockerignore},
		{"packages.txt", ""},
		{".env", ""},
		{"dags/.airflowignore", ""},
		{".astro/dag_integrity_exceptions.txt", "# Add dag files to exempt from parse test below. ex: dags/<test-file>"},
	}

	// Version-specific files
	switch cfg.AirflowVersion {
	case Airflow3:
		files = append(files,
			ScaffoldFile{"Dockerfile", fmt.Sprintf(Af3Dockerfile, cfg.RuntimeImageName, cfg.RuntimeImageTag)},
			ScaffoldFile{"requirements.txt", Af3RequirementsTxt},
			ScaffoldFile{"dags/exampledag.py", Af3ExampleDag},
		)
		if cfg.IncludeReadme {
			files = append(files, ScaffoldFile{"README.md", Af3Readme})
		}
		if cfg.IncludeSettingsYaml {
			files = append(files, ScaffoldFile{"airflow_settings.yaml", Af3Settingsyml})
		}
		if cfg.IncludeTests {
			files = append(files,
				ScaffoldFile{"tests/dags/test_dag_example.py", Af3DagExampleTest},
				ScaffoldFile{".astro/test_dag_integrity_default.py", Af3DagIntegrityTestDefault},
			)
		}
		if cfg.ClientImage != nil {
			files = append(files,
				ScaffoldFile{"Dockerfile.client", fmt.Sprintf(Af3DockerfileClient, cfg.ClientImage.BaseImageRegistry, cfg.ClientImage.ImageTag)},
				ScaffoldFile{"requirements-client.txt", Af3RequirementsTxtClient},
				ScaffoldFile{"packages-client.txt", ""},
			)
		}
	case Airflow2:
		files = append(files,
			ScaffoldFile{"Dockerfile", fmt.Sprintf(Af2Dockerfile, cfg.RuntimeImageName, cfg.RuntimeImageTag)},
			ScaffoldFile{"requirements.txt", Af2RequirementsTxt},
			ScaffoldFile{"dags/exampledag.py", Af2ExampleDag},
		)
		if cfg.IncludeReadme {
			files = append(files, ScaffoldFile{"README.md", Af2Readme})
		}
		if cfg.IncludeSettingsYaml {
			files = append(files, ScaffoldFile{"airflow_settings.yaml", Af2Settingsyml})
		}
		if cfg.IncludeTests {
			files = append(files,
				ScaffoldFile{"tests/dags/test_dag_example.py", Af2DagExampleTest},
				ScaffoldFile{".astro/test_dag_integrity_default.py", Af2DagIntegrityTestDefault},
			)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported Airflow version: %d", cfg.AirflowVersion)
	}

	// Project config
	if cfg.ProjectName != "" {
		files = append(files, ScaffoldFile{".astro/config.yaml", fmt.Sprintf("project:\n  name: %s\n", cfg.ProjectName)})
	}

	return dirs, files, nil
}
