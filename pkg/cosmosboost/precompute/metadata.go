package precompute

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// application is the producer name recorded in the sidecar. (The version is owned
// by the caller and passed in to Run.) It, and where the artifacts go, are
// airflowrt's: the bundle and the generated image judge what an Astro CLI
// wrote by the same definition.
const application = airflowrt.CosmosBoostProducer

const (
	// schemaVersion lets the read-side cope with future format changes. v2:
	// filtered_manifest -> manifests, keyed by source filename.
	schemaVersion = 2

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

	sidecarDir  = airflowrt.CosmosBoostDir
	sidecarName = airflowrt.CosmosBoostSidecar
)

// Metadata is the content of the .astro/dbt_metadata.json sidecar.
type Metadata struct {
	Schema      int            `json:"schema"`
	Version     ProjectVersion `json:"version"`
	GeneratedBy GeneratedBy    `json:"generated_by"`
	// Manifests is keyed by manifest filename, so a consumer looks up its own
	// entry instead of trusting Version when a directory has several.
	Manifests map[string]ManifestVersion `json:"manifests,omitempty"`
}

// ManifestVersion is one manifest's own hash, plus its slim companion, if
// one was produced.
type ManifestVersion struct {
	Version ProjectVersion `json:"version"`
	Slim    *SlimManifest  `json:"slim_manifest,omitempty"`
}

// SlimManifest describes a slim manifest sitting next to the sidecar. Version
// hashes the slim file's own bytes, so a consumer can confirm the two are a
// matched pair without re-hashing the full manifest.
type SlimManifest struct {
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
// producer's version, recorded in generated_by. manifests describes each dbt
// manifest in dir and the slim manifest produced beside it, if any.
func sidecarArtifact(dir, algo, hash, version string, manifests map[string]ManifestVersion) Artifact {
	meta := Metadata{
		Schema:      schemaVersion,
		Version:     ProjectVersion{Algo: algo, Hash: hash},
		GeneratedBy: GeneratedBy{Application: application, Version: version},
		Manifests:   manifests,
	}

	// Metadata holds only strings and ints, so marshaling cannot fail.
	data, _ := json.MarshalIndent(meta, "", "  ") //nolint:errcheck // see above: Metadata holds only strings and ints, so this cannot fail
	data = append(data, '\n')

	return Artifact{Path: filepath.Join(dir, sidecarDir, sidecarName), Data: data}
}

// checkSidecarDir fails when something other than a directory holds dir's
// .astro (a file, or a link): a bundle cannot carry the artifacts under it.
// Whatever is at an artifact's own path - an old sidecar, a link, a
// directory - the bundle leaves out and carries the artifact in its place.
func checkSidecarDir(dir string) error {
	astroDir := filepath.Join(dir, sidecarDir)
	if info, err := os.Lstat(astroDir); err == nil && !info.IsDir() {
		return fmt.Errorf("%s is not a directory, so its Cosmos Boost files cannot sit under it", astroDir)
	}
	return nil
}
