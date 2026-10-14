package airflowrt

import (
	"encoding/json"
	"path"
)

// Earlier Astro CLIs wrote the Cosmos Boost sidecars into the user's dbt
// project: a .astro/dbt_metadata.json beside each dbt project or manifest,
// and a .astro/manifest.slim.json beside it. The Cosmos Boost plugin reads
// them as current, so one left behind is stale the moment the project
// changes. The CLI now generates them into what it ships and never into the
// project, and everything it ships - a bundle, a generated image - leaves out
// the ones an earlier CLI left there. These are the one definition of where
// they go and whose they are, for the bundle and the pre-deploy step (root
// module) and the generated image (pkg/imagebuild) alike.
const (
	// CosmosBoostDir is the directory, beside a dbt project or manifest, the
	// sidecars go in.
	CosmosBoostDir = ".astro"
	// CosmosBoostSidecar carries the content hash the plugin keys its cache on.
	CosmosBoostSidecar = "dbt_metadata.json"
	// CosmosBoostSlimManifest is the field-filtered manifest beside it.
	CosmosBoostSlimManifest = "manifest.slim.json"
	// CosmosBoostProducer is the producer an Astro CLI records in each.
	CosmosBoostProducer = "astro"
)

// IsCosmosBoostFile reports whether rel, slash-separated, is where a Cosmos
// Boost sidecar goes: a dbt_metadata.json or manifest.slim.json directly
// inside a .astro directory, at any depth.
func IsCosmosBoostFile(rel string) bool {
	switch path.Base(rel) {
	case CosmosBoostSidecar, CosmosBoostSlimManifest:
		return path.Base(path.Dir(rel)) == CosmosBoostDir
	}
	return false
}

// CosmosBoostFileWrittenByAstro reports whether data, the contents of the
// file at rel (one IsCosmosBoostFile accepts), carries an Astro CLI's
// producer marker: generated_by.application in a dbt_metadata.json,
// _generated_by.application in a manifest.slim.json. Each file is judged by
// its own marker, never by its neighbor's, and one that does not parse as a
// JSON object or names another producer is not the CLI's: it is the user's.
func CosmosBoostFileWrittenByAstro(rel string, data []byte) bool {
	var marker struct {
		GeneratedBy struct {
			Application string `json:"application"`
		} `json:"generated_by"`
		SlimGeneratedBy struct {
			Application string `json:"application"`
		} `json:"_generated_by"`
	}
	if json.Unmarshal(data, &marker) != nil {
		return false
	}
	switch path.Base(rel) {
	case CosmosBoostSidecar:
		return marker.GeneratedBy.Application == CosmosBoostProducer
	case CosmosBoostSlimManifest:
		return marker.SlimGeneratedBy.Application == CosmosBoostProducer
	}
	return false
}
