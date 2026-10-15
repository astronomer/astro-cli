package deploy

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// BundleType is a kind of non-DAG bundle that astro deploy bundle knows by
// its contents: how to tell a directory holds one, how to check it, what the
// bundle is called, and where it mounts when the deploy names no mount path.
// Adding a type is adding an entry to bundleTypes.
type BundleType struct {
	// Name is the type recorded on the deploy, and what --type selects.
	Name string
	// Detect reports whether dir holds a bundle of this type, for a deploy
	// that names no type. nil detects nothing.
	Detect func(dir string) bool
	// Validate checks dir is a bundle of this type, before a mount path is
	// derived from it. nil accepts any directory.
	Validate func(dir string) error
	// BundleName is the name of the bundle at dir (a dbt project's name).
	// nil means bundles of this type have none.
	BundleName func(dir string) (string, error)
	// MountPathPrefix is the directory in Airflow a bundle of this type
	// mounts below by default, at MountPathPrefix + its BundleName. A mount
	// path below it says a bundle is of this type, for a delete that names
	// no type and runs where no bundle is. Empty means no default.
	MountPathPrefix string
}

// HasDefaultMountPath reports whether a bundle of this type has a mount path
// without one being given.
func (t *BundleType) HasDefaultMountPath() bool {
	return t.MountPathPrefix != "" && t.BundleName != nil
}

// DefaultMountPath is where the bundle at dir mounts when no mount path is
// given: MountPathPrefix and the bundle's name. It is an error for a type
// with no default (HasDefaultMountPath).
func (t *BundleType) DefaultMountPath(dir string) (string, error) {
	if !t.HasDefaultMountPath() {
		return "", fmt.Errorf("a bundle of type %s has no default mount path", t.Name)
	}
	name, err := t.BundleName(dir)
	if err != nil {
		return "", err
	}
	return t.MountPathPrefix + name, nil
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
		Name:            "dbt",
		Detect:          isDbtProject,
		Validate:        validateDbtProject,
		BundleName:      dbtProjectName,
		MountPathPrefix: DbtDefaultMountPathPrefix,
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

// BundleTypeForMountPath is the registered type whose default mount path
// mountPath has the shape of, MountPathPrefix and one path component, and
// whether there is one. The path is cleaned first, so .. cannot climb out of
// the prefix and a nested path is no default's.
func BundleTypeForMountPath(mountPath string) (BundleType, bool) {
	clean := pathpkg.Clean(mountPath)
	for _, t := range bundleTypes {
		if t.MountPathPrefix == "" {
			continue
		}
		rest, ok := strings.CutPrefix(clean, t.MountPathPrefix)
		if ok && rest != "" && !strings.Contains(rest, "/") {
			return t, true
		}
	}
	return BundleType{}, false
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

// dbtProjectName is the project's name, from its dbt_project.yml.
func dbtProjectName(dir string) (string, error) {
	path := filepath.Join(dir, dbtProjectFile)
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("dbt project name not found in %s: could not open %s: %w", dir, path, err)
	}
	defer file.Close()
	var project map[string]any
	if err := yaml.NewDecoder(file).Decode(&project); err != nil {
		return "", fmt.Errorf("dbt project name not found in %s: could not decode %s: %w", dir, path, err)
	}
	name, ok := project["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("dbt project name not found in %s: invalid dbt project name", dir)
	}
	return name, nil
}
