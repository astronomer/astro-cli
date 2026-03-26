package airflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	PyProjectFile        = "pyproject.toml"
	ProjectFormatPy      = "pyproject"
	ProjectFormatDocker  = "dockerfile"
	ProjectFormatUnknown = "unknown"
	DefaultMode          = "docker"
	DefaultPythonVersion = "3.12"
	ModeStandalone       = "standalone"
	ModeDocker           = "docker"
)

// pyProjectTOML mirrors the subset of pyproject.toml we care about.
// Used only for reading — PinRuntimeVersion uses targeted text editing
// to avoid destroying user content outside this struct.
type pyProjectTOML struct {
	Project struct {
		Name           string   `toml:"name"`
		RequiresPython string   `toml:"requires-python"`
		Dependencies   []string `toml:"dependencies"`
	} `toml:"project"`
	Tool struct {
		Astro *astroToolConfig `toml:"astro"`
	} `toml:"tool"`
}

type astroToolConfig struct {
	AirflowVersion string          `toml:"airflow-version"`
	RuntimeVersion string          `toml:"runtime-version"`
	Mode           string          `toml:"mode"`
	Docker         *astroDockerCfg `toml:"docker"`
}

type astroDockerCfg struct {
	SystemPackages []string `toml:"system-packages"`
}

// AstroProject is the parsed, validated representation of an Astro project
// defined via pyproject.toml.
type AstroProject struct {
	Name           string
	RequiresPython string
	Dependencies   []string
	AirflowVersion string
	RuntimeVersion string
	Mode           string
	SystemPackages []string
}

// ReadProject parses pyproject.toml at the given project root and returns an
// AstroProject. Returns (nil, nil) if no pyproject.toml exists. Returns
// (nil, error) if the file exists but is broken (bad TOML, missing [tool.astro], etc.).
func ReadProject(projectPath string) (*AstroProject, error) {
	proj, _, err := TryReadProject(projectPath)
	return proj, err
}

// TryReadProject attempts to read pyproject.toml. Returns (nil, false, nil) if
// no pyproject.toml exists (not an error). Returns (nil, true, err) if the file
// exists but is broken. Returns (proj, true, nil) on success.
func TryReadProject(projectPath string) (proj *AstroProject, found bool, err error) {
	filePath := filepath.Join(projectPath, PyProjectFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("failed to read %s: %w", PyProjectFile, err)
	}

	var raw pyProjectTOML
	if unmarshalErr := toml.Unmarshal(data, &raw); unmarshalErr != nil {
		return nil, true, fmt.Errorf("failed to parse %s: %w", PyProjectFile, unmarshalErr)
	}

	if raw.Tool.Astro == nil {
		return nil, true, fmt.Errorf("%s does not contain a [tool.astro] section", PyProjectFile)
	}

	astro := raw.Tool.Astro

	if astro.AirflowVersion == "" {
		return nil, true, fmt.Errorf("[tool.astro].airflow-version is required in %s", PyProjectFile)
	}

	mode := astro.Mode
	if mode == "" {
		mode = DefaultMode
	}
	if mode != ModeStandalone && mode != ModeDocker {
		return nil, true, fmt.Errorf("[tool.astro].mode must be %q or %q, got %q", ModeStandalone, ModeDocker, mode)
	}

	var systemPackages []string
	if astro.Docker != nil {
		systemPackages = astro.Docker.SystemPackages
	}

	return &AstroProject{
		Name:           raw.Project.Name,
		RequiresPython: raw.Project.RequiresPython,
		Dependencies:   raw.Project.Dependencies,
		AirflowVersion: astro.AirflowVersion,
		RuntimeVersion: astro.RuntimeVersion,
		Mode:           mode,
		SystemPackages: systemPackages,
	}, true, nil
}

// IsPyProject returns true if the project at the given path is defined via
// pyproject.toml with a [tool.astro] section. It does not validate the file
// beyond checking for the section's existence.
func IsPyProject(projectPath string) bool {
	filePath := filepath.Join(projectPath, PyProjectFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	var raw pyProjectTOML
	if err := toml.Unmarshal(data, &raw); err != nil {
		return false
	}

	return raw.Tool.Astro != nil
}

// PinRuntimeVersion sets the runtime-version field in the [tool.astro]
// section of pyproject.toml. Uses targeted text editing (not full TOML
// roundtrip) to preserve all other content — user comments, [tool.ruff],
// [build-system], [[tool.uv.index]], etc.
func PinRuntimeVersion(projectPath, runtimeVersion string) error {
	filePath := filepath.Join(projectPath, PyProjectFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", PyProjectFile, err)
	}

	content := string(data)

	// Try to replace existing runtime-version line
	re := regexp.MustCompile(`(?m)^(\s*)runtime-version\s*=\s*["'][^"']*["']`)
	if re.MatchString(content) {
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			indent := match[:len(match)-len(strings.TrimLeft(match, " \t"))]
			return fmt.Sprintf("%sruntime-version = %q", indent, runtimeVersion)
		})
	} else {
		// Insert after airflow-version line
		avRe := regexp.MustCompile(`(?m)(^[ \t]*airflow-version\s*=\s*["'][^"']*["'][ \t]*)$`)
		if avRe.MatchString(content) {
			content = avRe.ReplaceAllStringFunc(content, func(match string) string {
				indent := match[:len(match)-len(strings.TrimLeft(match, " \t"))]
				return fmt.Sprintf("%s\n%sruntime-version = %q", match, indent, runtimeVersion)
			})
		} else {
			return fmt.Errorf("could not find airflow-version in %s to insert runtime-version", PyProjectFile)
		}
	}

	return os.WriteFile(filePath, []byte(content), 0o644) //nolint:gosec,mnd
}

// DetectProjectFormat determines the project format at the given path.
// Returns ProjectFormatPy if pyproject.toml with [tool.astro] exists,
// ProjectFormatDocker if a Dockerfile exists, or ProjectFormatUnknown.
func DetectProjectFormat(projectPath string) string {
	if IsPyProject(projectPath) {
		return ProjectFormatPy
	}

	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		return ProjectFormatDocker
	}

	return ProjectFormatUnknown
}
