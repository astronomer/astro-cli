package precompute

import (
	"encoding/json"
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

	sidecarDir  = ".astro"
	sidecarName = "dbt_metadata.json"

	sidecarDirPerm = 0o755
	// sidecarPerm is deliberately world-readable: the sidecar ships inside the
	// deploy bundle or image and the Airflow runtime user must be able to read it.
	sidecarPerm = 0o644
)

// Metadata is the content of the .astro/dbt_metadata.json sidecar. The Cosmos
// Boost plugin reads the version.hash field and uses it as the cache version key;
// it never recomputes the hash itself.
type Metadata struct {
	Schema      int            `json:"schema"`
	Version     ProjectVersion `json:"version"`
	GeneratedBy GeneratedBy    `json:"generated_by"`
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

// writeSidecar writes .astro/dbt_metadata.json inside dir. version is the
// producer's version, recorded in generated_by.
func writeSidecar(dir, algo, hash, version string) error {
	meta := Metadata{
		Schema:      schemaVersion,
		Version:     ProjectVersion{Algo: algo, Hash: hash},
		GeneratedBy: GeneratedBy{Application: application, Version: version},
	}

	// Metadata holds only strings and ints, so marshaling cannot fail.
	data, _ := json.MarshalIndent(meta, "", "  ")
	data = append(data, '\n')

	return writeArtifact(dir, sidecarName, data)
}

// writeArtifact writes data to dir/.astro/name, creating the directory if
// needed. Shared by the two artifacts a run produces - the hash sidecar and
// the slim manifest - which differ only in filename and payload.
func writeArtifact(dir, name string, data []byte) error {
	out := filepath.Join(dir, sidecarDir)
	if err := os.MkdirAll(out, sidecarDirPerm); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, name), data, sidecarPerm) //nolint:gosec // see sidecarPerm
}
