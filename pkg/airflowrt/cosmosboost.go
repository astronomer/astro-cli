package airflowrt

import (
	"bytes"
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
// its own marker, never by its neighbor's, and one that does not parse or
// names another producer is not the CLI's: it is the user's.
//
// It reads only as far as the marker: a slim manifest can be large, and the
// CLI writes the marker first.
func CosmosBoostFileWrittenByAstro(rel string, data []byte) bool {
	var key string
	switch path.Base(rel) {
	case CosmosBoostSidecar:
		key = "generated_by"
	case CosmosBoostSlimManifest:
		key = "_generated_by"
	default:
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false
	}
	for dec.More() {
		name, err := dec.Token()
		if err != nil {
			return false
		}
		if name != key {
			if skipJSONValue(dec) != nil {
				return false
			}
			continue
		}
		var marker struct {
			Application string `json:"application"`
		}
		return dec.Decode(&marker) == nil && marker.Application == CosmosBoostProducer
	}
	return false
}

// skipJSONValue reads past the next value in dec, token by token, without
// holding it.
func skipJSONValue(dec *json.Decoder) error {
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
		if depth == 0 {
			return nil
		}
	}
}
