package precompute

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// existingSkipDirs are not traversed below the walked root. Discovery never
// produces artifacts under them (both discovery walks skip these names at any
// depth), and generated directories like a root-owned logs/ left by a
// container bind mount may not even be readable - failing a deploy over a
// directory no artifact is ever produced for helps nobody. The root itself is
// exempt, mirroring discovery: a project that happens to live in a directory
// named logs or dbt_packages can be stamped. .git is skipped unconditionally.
// target/ is deliberately absent: a compiled manifest's sidecar belongs at
// target/.astro/.
var existingSkipDirs = map[string]bool{
	"logs":             true,
	defaultPackagesDir: true,
}

// Existing is a Cosmos Boost artifact already on disk under a walked root: a
// .astro/dbt_metadata.json or .astro/manifest.slim.json. Ours reports whether
// its own producer marker names this tool. Earlier astro-cli versions wrote
// these into the project; this one never does.
type Existing struct {
	Path string
	Ours bool
}

// FindExisting returns every .astro/dbt_metadata.json and
// .astro/manifest.slim.json under root, each judged by its own producer
// marker, never by its neighbor's, because the mixed states are real: a slim
// manifest outlives its sidecar when an older astro-cli removed only the
// sidecar, and a sidecar this tool wrote can sit beside a slim manifest it did
// not. A file that cannot be read fails the scan, since nobody could say
// whose it is.
func FindExisting(root string) ([]Existing, error) {
	var found []Existing
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == gitDir || (path != root && existingSkipDirs[d.Name()]) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != sidecarDir {
			return nil
		}
		var ours func([]byte) bool
		switch d.Name() {
		case sidecarName:
			ours = sidecarIsOurs
		case slimManifestName:
			ours = slimManifestIsOurs
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found = append(found, Existing{Path: path, Ours: ours(data)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %q for Cosmos Boost files: %w", root, err)
	}
	return found, nil
}

// sidecarIsOurs reports whether a .astro/dbt_metadata.json's generated_by
// names this tool.
func sidecarIsOurs(data []byte) bool {
	var meta Metadata
	return json.Unmarshal(data, &meta) == nil && meta.GeneratedBy.Application == application
}

// slimManifestIsOurs reports whether a .astro/manifest.slim.json's own
// _generated_by marker names this tool.
func slimManifestIsOurs(data []byte) bool {
	var marker struct {
		GeneratedBy GeneratedBy `json:"_generated_by"`
	}
	return json.Unmarshal(data, &marker) == nil && marker.GeneratedBy.Application == application
}
