package airflowrt

import (
	"encoding/json"
	"path"
	"strings"
)

// Earlier Astro CLIs wrote the Cosmos Boost sidecars into the user's dbt
// project: a .astro/dbt_metadata.json beside each dbt project or manifest,
// and a slim manifest beside it for each dbt manifest there
// (.astro/manifest.slim.json for a manifest.json). The Cosmos Boost plugin reads
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
	// CosmosBoostSlimManifestSuffix ends a field-filtered manifest's name.
	// There is one beside the sidecar for each dbt manifest, named after it:
	// manifest_full.json's is manifest_full.slim.json.
	CosmosBoostSlimManifestSuffix = ".slim.json"
	// CosmosBoostSlimManifest is manifest.json's field-filtered manifest.
	CosmosBoostSlimManifest = "manifest" + CosmosBoostSlimManifestSuffix
	// CosmosBoostProducer is the producer an Astro CLI records in each.
	CosmosBoostProducer = "astro"
)

// IsCosmosBoostFile reports whether rel, slash-separated, is where a Cosmos
// Boost sidecar goes: a dbt_metadata.json or a *.slim.json directly inside a
// .astro directory, at any depth.
func IsCosmosBoostFile(rel string) bool {
	base := path.Base(rel)
	return (base == CosmosBoostSidecar || isSlimManifest(base)) && path.Base(path.Dir(rel)) == CosmosBoostDir
}

// isSlimManifest reports whether name is a slim manifest's: something
// before CosmosBoostSlimManifestSuffix.
func isSlimManifest(name string) bool {
	return len(name) > len(CosmosBoostSlimManifestSuffix) && strings.HasSuffix(name, CosmosBoostSlimManifestSuffix)
}

// CosmosBoostFileWrittenByAstro reports whether data, the contents of the
// file at rel (one IsCosmosBoostFile accepts), carries an Astro CLI's
// producer marker: generated_by.application in a dbt_metadata.json,
// _generated_by.application in a slim manifest. Each file is judged by
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
	switch base := path.Base(rel); {
	case base == CosmosBoostSidecar:
		return marker.GeneratedBy.Application == CosmosBoostProducer
	case isSlimManifest(base):
		return marker.SlimGeneratedBy.Application == CosmosBoostProducer
	}
	return false
}
