package precompute

const slimManifestName = "manifest.slim.json"

// slimSchemaVersion identifies the allowlist that produced a slim manifest, so
// a reader that doesn't recognize it can fall back to the full manifest.
const slimSchemaVersion = 1

// slimSections are the only top-level collections Cosmos loads nodes from
// (cosmos/dbt/graph.py::_load_nodes_from_manifest_data).
var slimSections = []string{"nodes", "sources", "exposures"}

// slimResourceFields are the per-resource fields Cosmos reads. The first six
// build the DbtNode (graph.py::_build_dbt_node_from_manifest_resource); config
// is kept whole because selectors reach arbitrary depth into config.meta.*. The
// last four build per-model dataset outlet URIs as "database.schema.alias",
// name being the alias fallback (dataset.py::compute_model_outlet_uris), which
// under ExecutionMode.WATCHER on Kubernetes reads this file directly.
var slimResourceFields = []string{
	"original_file_path", "package_name", "resource_type", "tags", "config", "fqn",
	"database", "schema", "alias", "name",
}

// buildSlimManifest returns a manifest document holding only the fields Cosmos
// reads. doc is not mutated, but the result shares doc's nested values, so a
// caller that later mutates doc (hashDocument does) must marshal this to bytes
// first - see processManifest.
func buildSlimManifest(doc map[string]any, version string) map[string]any {
	slim := map[string]any{
		"_schema":       slimSchemaVersion,
		"_generated_by": GeneratedBy{Application: application, Version: version},
		"metadata":      slimMetadata(doc),
		"selectors":     objectOrEmpty(doc["selectors"]),
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

// slimMetadata keeps project_name and nothing else - is_root_project_node
// compares each resource's package_name against it. An absent project_name is
// left out rather than nulled, matching the full manifest's shape.
func slimMetadata(doc map[string]any) map[string]any {
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	slim := map[string]any{}
	if name, ok := meta["project_name"]; ok {
		slim["project_name"] = name
	}
	return slim
}

// objectOrEmpty returns v when it is a JSON object, else an empty one: never
// null, which would break a reader doing manifest.get("selectors", {}).
func objectOrEmpty(v any) map[string]any {
	object, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return object
}

// slimResource keeps the allowlisted fields plus depends_on.nodes (never
// .macros, which Cosmos ignores) and freshness for sources only.
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
