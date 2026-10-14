package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// BundleType is a kind of non-DAG bundle that astro deploy bundle knows by
// its contents: how to tell a directory holds one, how to check it before
// anything is uploaded, and where it mounts when the deploy names no mount
// path. Adding a type is adding an entry to bundleTypes.
type BundleType struct {
	// Name is the type recorded on the deploy, and what --type selects.
	Name string
	// Detect reports whether dir holds a bundle of this type, for a deploy
	// that names no type. nil detects nothing.
	Detect func(dir string) bool
	// Validate checks dir is a bundle of this type. nil accepts any
	// directory.
	Validate func(dir string) error
	// DefaultMountPath is where the bundle at dir mounts when no mount path
	// is given. nil means the type has no default, so a mount path is
	// required.
	DefaultMountPath func(dir string) (string, error)
}

// BundleTypeNone is the type of a bundle that names no type and that no
// registered type detects. It is a label alone, with no defaults.
const BundleTypeNone = "none"

// DbtDefaultMountPathPrefix is where a dbt project mounts by default, below
// a directory named after the project.
const DbtDefaultMountPathPrefix = "/usr/local/airflow/dbt/"

const dbtProjectFile = "dbt_project.yml"

// bundleTypes is every bundle type with defaults, in the order detection
// tries them.
var bundleTypes = []BundleType{
	{
		Name:             "dbt",
		Detect:           isDbtProject,
		Validate:         validateDbtProject,
		DefaultMountPath: dbtDefaultMountPath,
	},
}

// ResolveBundleType is the type a bundle at dir deploys as. name is the type
// asked for: a registered type's name (in any case) selects that type, and
// any other is a free-form label with no defaults, as the deploy API takes
// any label. An empty name is the first registered type that detects dir,
// else BundleTypeNone.
func ResolveBundleType(name, dir string) BundleType {
	if name == "" {
		for _, t := range bundleTypes {
			if t.Detect != nil && t.Detect(dir) {
				return t
			}
		}
		return BundleType{Name: BundleTypeNone}
	}
	for _, t := range bundleTypes {
		if strings.EqualFold(t.Name, name) {
			return t
		}
	}
	return BundleType{Name: name}
}

// BundleTypeNames is the name of every registered bundle type, in order.
func BundleTypeNames() []string {
	names := make([]string, 0, len(bundleTypes))
	for _, t := range bundleTypes {
		names = append(names, t.Name)
	}
	return names
}

func isDbtProject(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, dbtProjectFile))
	return err == nil && info.Mode().IsRegular()
}

func validateDbtProject(dir string) error {
	path := filepath.Join(dir, dbtProjectFile)
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no %s in %s: a dbt bundle is the directory that holds the dbt project's %s", dbtProjectFile, dir, dbtProjectFile)
	}
	return err
}

// dbtDefaultMountPath is DbtDefaultMountPathPrefix and the project's name,
// from its dbt_project.yml.
func dbtDefaultMountPath(dir string) (string, error) {
	name, err := dbtProjectName(dir)
	if err != nil {
		return "", fmt.Errorf("dbt project name not found in %s: %w", dir, err)
	}
	return DbtDefaultMountPathPrefix + name, nil
}

func dbtProjectName(dir string) (string, error) {
	path := filepath.Join(dir, dbtProjectFile)
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not open %s: %w", path, err)
	}
	defer file.Close()
	var project map[string]any
	if err := yaml.NewDecoder(file).Decode(&project); err != nil {
		return "", fmt.Errorf("could not decode %s: %w", path, err)
	}
	name, ok := project["name"].(string)
	if !ok || name == "" {
		return "", errors.New("invalid dbt project name")
	}
	return name, nil
}
