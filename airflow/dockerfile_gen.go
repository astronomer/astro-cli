package airflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
)

const (
	generatedDockerfileComment = "# Auto-generated from pyproject.toml — do not edit.\n# Changes should be made in pyproject.toml.\n"
	genDirPerm                 = os.FileMode(0o755) //nolint:mnd
	genFilePerm                = os.FileMode(0o644) //nolint:mnd
	generatedDockerfileName    = "Dockerfile.pyproject"
)

// validDebPkgRe matches valid Debian package names: starts with alnum, then alnum/./+/-
var validDebPkgRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+\-]*$`)

// GenerateDockerfile produces Dockerfile content from an AstroProject.
// Returns an error if the project has invalid fields.
func GenerateDockerfile(project *AstroProject) (string, error) {
	if project.RuntimeVersion == "" {
		return "", fmt.Errorf("runtime-version is required to generate a Dockerfile")
	}

	for _, pkg := range project.SystemPackages {
		if !validDebPkgRe.MatchString(pkg) {
			return "", fmt.Errorf("invalid system package name %q: must match Debian package naming rules", pkg)
		}
	}

	var b strings.Builder

	b.WriteString(generatedDockerfileComment)

	imageName := AstroRuntimeAirflow3ImageName
	registry := AstroImageRegistryBaseImageName
	b.WriteString(fmt.Sprintf("FROM %s/%s:%s\n", registry, imageName, project.RuntimeVersion))

	if len(project.SystemPackages) > 0 {
		b.WriteString("USER root\n")
		b.WriteString(fmt.Sprintf(
			"RUN apt-get update && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*\n",
			strings.Join(project.SystemPackages, " "),
		))
		b.WriteString("USER astro\n")
	}

	return b.String(), nil
}

// EnsureDockerfile checks if the project uses pyproject.toml and generates
// a Dockerfile in .astro/ if needed. Returns the path to the Dockerfile to use
// (either the generated one or the original).
func EnsureDockerfile(airflowHome, originalDockerfile string) (string, error) {
	proj, found, err := TryReadProject(airflowHome)
	if !found {
		return originalDockerfile, nil
	}
	if err != nil {
		return "", fmt.Errorf("error reading pyproject.toml: %w", err)
	}

	// If a hand-written Dockerfile exists in the project root, use it instead
	// of generating one. This supports users who need custom Docker steps
	// beyond what pyproject.toml can express (the "eject" pattern).
	dockerfilePath := filepath.Join(airflowHome, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		return "Dockerfile", nil
	}

	// Resolve runtime-version from airflow-version if missing, and pin it
	if proj.RuntimeVersion == "" {
		if proj.AirflowVersion == "" {
			return "", fmt.Errorf("[tool.astro] requires airflow-version in pyproject.toml")
		}
		resolved := airflowversions.GetLatestRuntimeForAirflow(proj.AirflowVersion)
		if resolved == "" {
			return "", fmt.Errorf("could not resolve a runtime version for airflow-version %q", proj.AirflowVersion)
		}
		fmt.Printf("Resolved runtime-version %q for airflow-version %q (pinning to pyproject.toml)\n", resolved, proj.AirflowVersion)
		fmt.Println("Note: one airflow-version may have multiple runtime versions. Pin runtime-version in pyproject.toml to avoid accidental upgrades on deploy.")
		if pinErr := PinRuntimeVersion(airflowHome, resolved); pinErr != nil {
			fmt.Printf("Warning: could not pin runtime-version to pyproject.toml: %s\n", pinErr)
		}
		proj.RuntimeVersion = resolved
	}

	// Validate airflow-version matches runtime-version
	if proj.AirflowVersion != "" {
		actual := airflowversions.GetAirflowVersionForRuntime(proj.RuntimeVersion)
		if actual != "" && actual != proj.AirflowVersion {
			fmt.Printf("Warning: airflow-version %q in pyproject.toml does not match runtime-version %q (which bundles Airflow %s). Consider updating airflow-version or runtime-version.\n",
				proj.AirflowVersion, proj.RuntimeVersion, actual)
		}
	}

	content, err := GenerateDockerfile(proj)
	if err != nil {
		return "", err
	}

	genDir := filepath.Join(airflowHome, ".astro")
	if err := os.MkdirAll(genDir, genDirPerm); err != nil {
		return "", fmt.Errorf("error creating .astro directory: %w", err)
	}

	genPath := filepath.Join(genDir, generatedDockerfileName)
	if err := os.WriteFile(genPath, []byte(content), genFilePerm); err != nil {
		return "", fmt.Errorf("error writing generated Dockerfile: %w", err)
	}

	return genPath, nil
}
