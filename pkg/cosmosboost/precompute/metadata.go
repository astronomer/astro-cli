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
// needed. The write is atomic — a temp file in the same directory, renamed
// over the destination — so a failed or interrupted write leaves the previous
// contents (or nothing) rather than a truncated file. That matters most for
// the slim manifest, which is large enough for a partial write to be real:
// truncated JSON is unusable to the plugin AND indistinguishable from another
// producer's file to Cleanup, which would then report it as an unrecognized
// artifact and have EnsureClean block every later deploy over a file we wrote
// ourselves. A hard kill between create and rename can still strand a temp
// file in .astro, which only keeps the directory from being pruned.
func writeArtifact(dir, name string, data []byte) error {
	out := filepath.Join(dir, sidecarDir)
	if err := os.MkdirAll(out, sidecarDirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(out, name+".tmp-*")
	if err != nil {
		return err
	}
	// Both are no-ops on the success path: Close is already done and the
	// rename has taken the temp name out of the directory.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp opens at 0o600; artifacts need sidecarPerm to be readable by
	// the Airflow runtime user once shipped.
	if err := os.Chmod(tmp.Name(), sidecarPerm); err != nil { //nolint:gosec // see sidecarPerm
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(out, name))
}
