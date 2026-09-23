package precompute

import (
	"io/fs"
	"os"
	"path/filepath"
)

// dbt requires the project file to be named exactly dbt_project.yml (the .yaml
// extension is not accepted), so we match only this name.
const dbtProjectFile = "dbt_project.yml"

const manifestFile = "manifest.json"

// findProjects walks root and returns every directory that contains a
// dbt_project.yml.
//
// A dbt project is treated as a single unit: once one is found, discovery does
// not descend into it (so a nested project is not split out, and a dependency's
// own dbt_project.yml under dbt_packages/ is never mistaken for a project), and
// excluded directories are skipped entirely.
func findProjects(root string) ([]string, error) {
	var projects []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && excludedDirs[d.Name()] {
			return filepath.SkipDir
		}
		if isFile(filepath.Join(path, dbtProjectFile)) {
			projects = append(projects, path)
			return filepath.SkipDir // a project is one unit; don't recurse into it
		}
		return nil
	})
	return projects, err
}

// manifestSkipDirs are skipped when looking for manifest.json. Unlike project
// hashing we do NOT skip target/, because a compiled manifest normally lives in
// target/manifest.json; but installed packages and our own output can't hold a
// project's own manifest, so they're skipped.
var manifestSkipDirs = map[string]bool{
	"logs":         true,
	"dbt_packages": true,
	sidecarDir:     true, // .astro
	gitDir:         true, // VCS internals can't hold a project's manifest
}

// findManifests walks root and returns manifest.json file paths.
//
// A manifest whose parent directory is itself a discovered project root is
// omitted: that project's folder hash already covers a manifest sitting in its
// root. Manifests elsewhere — most importantly a standalone one shipped for a
// manifest-only (DBT_MANIFEST) deployment, or a project's target/manifest.json —
// each get their own sidecar.
func findManifests(root string, projectDirs map[string]bool) ([]string, error) {
	var manifests []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && manifestSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != manifestFile {
			return nil
		}
		if projectDirs[filepath.Dir(path)] {
			return nil // covered by the project's folder hash
		}
		manifests = append(manifests, path)
		return nil
	})
	return manifests, err
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
