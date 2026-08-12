package precompute

import (
	"os"
	"path/filepath"
)

const slimManifestName = "manifest.slim.json"

// slimSchemaVersion lets a future reader tell which allowlist produced a slim
// manifest, independent of the sidecar's own schemaVersion (metadata.go).
const slimSchemaVersion = 1

// slimSections are the only top-level manifest collections Cosmos loads nodes
// from when parsing a dbt manifest (LoadMode.DBT_MANIFEST); everything else
// (macros, disabled, docs, parent_map, child_map, ...) is unused.
var slimSections = []string{"nodes", "sources", "exposures"}

// slimResourceFields are the per-resource fields Cosmos reads to build a
// DbtNode from a manifest resource.
var slimResourceFields = []string{"original_file_path", "package_name", "resource_type", "tags", "config", "fqn"}

// buildSlimManifest returns a manifest document containing only the fields
// Cosmos reads when building DAGs from a dbt manifest.json. doc is not
// mutated, but the result is a shallow copy: selectors and each resource's
// config/tags/fqn/depends_on.nodes are the same nested map/slice values as
// doc's, not copies. A caller that mutates doc afterward (e.g. hashDocument)
// must marshal this result to bytes first if the two need to stay
// independent - see processManifest.
//
// Only a substitute for the graph-loading read - Cosmos also reads
// manifest_path directly for dbt's own subprocess copy and per-model
// dataset/Asset outlet URIs (database/schema/alias), which this drops.
func buildSlimManifest(doc map[string]any, version string) map[string]any {
	slim := map[string]any{
		"_schema":       slimSchemaVersion,
		"_generated_by": GeneratedBy{Application: application, Version: version},
		"metadata":      map[string]any{"project_name": projectName(doc)},
		"selectors":     doc["selectors"],
	}
	for _, section := range slimSections {
		entries, _ := doc[section].(map[string]any)
		slimEntries := make(map[string]any, len(entries))
		for uniqueID, entry := range entries {
			if resource, ok := entry.(map[string]any); ok {
				slimEntries[uniqueID] = slimResource(resource)
			}
		}
		slim[section] = slimEntries
	}
	return slim
}

// projectName returns manifest.metadata.project_name, or nil if either is
// absent. is_root_project_node (Cosmos) compares each resource's
// package_name against this value, so it's the only metadata field kept.
func projectName(doc map[string]any) any {
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	return meta["project_name"]
}

// slimResource returns a copy of resource containing only the fields Cosmos
// reads: the shared fields, depends_on.nodes (never depends_on.macros, which
// Cosmos does not consume), and freshness for sources only.
func slimResource(resource map[string]any) map[string]any {
	slim := make(map[string]any, len(slimResourceFields)+2)
	for _, key := range slimResourceFields {
		if v, ok := resource[key]; ok {
			slim[key] = v
		}
	}
	if dependsOn, ok := resource["depends_on"].(map[string]any); ok {
		if nodes, ok := dependsOn["nodes"]; ok {
			slim["depends_on"] = map[string]any{"nodes": nodes}
		}
	}
	if resource["resource_type"] == "source" {
		if freshness, ok := resource["freshness"]; ok {
			slim["freshness"] = freshness
		}
	}
	return slim
}

// writeSlimManifest writes data into dir/.astro/manifest.slim.json. data must
// already be the marshaled, compact (no indentation, to keep the size
// reduction from field-filtering) JSON for a buildSlimManifest result -
// callers marshal it themselves, before hashDocument gets a chance to mutate
// any doc value the slim manifest shares (see buildSlimManifest).
func writeSlimManifest(dir string, data []byte) error {
	out := filepath.Join(dir, sidecarDir)
	if err := os.MkdirAll(out, sidecarDirPerm); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, slimManifestName), data, sidecarPerm) //nolint:gosec // see sidecarPerm
}
