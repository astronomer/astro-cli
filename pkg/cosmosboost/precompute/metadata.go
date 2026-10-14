package precompute

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// application is the producer name recorded in the sidecar. (The version is owned
// by the caller and passed in to Run.)
const application = "astro"

const (
	// schemaVersion lets the read-side cope with future format changes.
	schemaVersion = 1

	// algoProjectTree hashes a whole dbt project directory (source files).
	// v2 excludes .git, which never ships in a deploy payload, so VCS activity
	// (commits, fetches, gc) cannot change the hash of unchanged content.
	algoProjectTree = "sha256-tree-v2"
	// algoManifestJSON hashes a dbt manifest.json by content, ignoring volatile
	// metadata. Used for manifest-only deployments. v2 also strips the
	// dbt-owned created_at stamped on every resource and
	// metadata.run_started_at, which dbt regenerates on every full parse, so
	// CI-built manifests hash-match locally built ones.
	algoManifestJSON = "sha256-manifest-v2"
	// algoFilteredManifest hashes the slim manifest's own bytes, not the
	// manifest it came from (that is version.hash's job).
	algoFilteredManifest = "sha256-filtered-manifest-v1"

	sidecarDir  = ".astro"
	sidecarName = "dbt_metadata.json"
)

// Metadata is the content of the .astro/dbt_metadata.json sidecar. The Cosmos
// Boost plugin reads the version.hash field and uses it as the cache version key;
// it never recomputes the hash itself.
type Metadata struct {
	Schema      int            `json:"schema"`
	Version     ProjectVersion `json:"version"`
	GeneratedBy GeneratedBy    `json:"generated_by"`
	// FilteredManifest points at the slim manifest beside this sidecar, absent
	// when none was produced. The sidecar is the consumer's entry point: it
	// already reads and schema-gates this file, so it discovers the slim
	// manifest here and falls back to the full one when the section is missing.
	FilteredManifest *FilteredManifest `json:"filtered_manifest,omitempty"`
}

// FilteredManifest describes a slim manifest sitting next to the sidecar. Path
// is relative to the sidecar's directory, and Version hashes the slim file's
// own bytes, so a consumer can confirm the two are a matched pair without
// re-hashing the full manifest.
type FilteredManifest struct {
	Schema  int            `json:"schema"`
	Path    string         `json:"path"`
	Version ProjectVersion `json:"version"`
}

// ProjectVersion identifies a dbt project's content. Hash is the version key;
// Algo names how it was computed so the read-side knows how to interpret it.
type ProjectVersion struct {
	Algo string `json:"algo"`
	Hash string `json:"hash"`
}

// GeneratedBy records which tool (and version) wrote the sidecar.
type GeneratedBy struct {
	Application string `json:"application"`
	Version     string `json:"version"`
}

// sidecarArtifact is the .astro/dbt_metadata.json for dir. version is the
// producer's version, recorded in generated_by. filtered describes a slim
// manifest produced beside it, or is nil when none was.
func sidecarArtifact(dir, algo, hash, version string, filtered *FilteredManifest) Artifact {
	meta := Metadata{
		Schema:           schemaVersion,
		Version:          ProjectVersion{Algo: algo, Hash: hash},
		GeneratedBy:      GeneratedBy{Application: application, Version: version},
		FilteredManifest: filtered,
	}

	// Metadata holds only strings and ints, so marshaling cannot fail.
	data, _ := json.MarshalIndent(meta, "", "  ") //nolint:errcheck // see above: Metadata holds only strings and ints, so this cannot fail
	data = append(data, '\n')

	return Artifact{Path: filepath.Join(dir, sidecarDir, sidecarName), Data: data}
}

// checkArtifactPaths fails when the tree holds something the artifacts could
// not replace in a bundle: a .astro under dir that is not a directory (a file,
// or a link), or anything but a regular file where an artifact goes (a
// directory, a link). Unpacked beside the artifact, either would collide with
// it. A regular file there is fine: the bundle carries the artifact instead.
func checkArtifactPaths(dir string, artifacts []Artifact) error {
	astroDir := filepath.Join(dir, sidecarDir)
	if info, err := os.Lstat(astroDir); err == nil && !info.IsDir() {
		return fmt.Errorf("%s is not a directory, so its Cosmos Boost files cannot sit under it", astroDir)
	}
	for _, a := range artifacts {
		if info, err := os.Lstat(a.Path); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file, so the Cosmos Boost file cannot take its place", a.Path)
		}
	}
	return nil
}
