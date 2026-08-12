package precompute

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const slimManifestName = "manifest.slim.json"

// slimSections are the only top-level manifest collections Cosmos loads nodes
// from when parsing a dbt manifest (LoadMode.DBT_MANIFEST); everything else
// (macros, disabled, docs, parent_map, child_map, ...) is unused.
var slimSections = []string{"nodes", "sources", "exposures"}

// slimResourceFields are the per-resource fields Cosmos reads to build a
// DbtNode from a manifest resource.
var slimResourceFields = []string{"original_file_path", "package_name", "resource_type", "tags", "config", "fqn"}

// buildSlimManifest returns a manifest document containing only the fields
// Cosmos reads when building DAGs from a dbt manifest.json, so the file the
// plugin loads at parse time is a fraction of the size of the original. doc
// is not mutated.
func buildSlimManifest(doc map[string]any) map[string]any {
	slim := map[string]any{
		"metadata":  map[string]any{"project_name": projectName(doc)},
		"selectors": doc["selectors"],
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

// writeSlimManifest writes the slim manifest JSON into dir/.astro/manifest.slim.json,
// compactly (no indentation) to keep the size reduction from field-filtering.
func writeSlimManifest(dir string, slim map[string]any) error {
	// slim holds only JSON-native types built by buildSlimManifest, so
	// marshaling cannot fail.
	data, _ := json.Marshal(slim)

	out := filepath.Join(dir, sidecarDir)
	if err := os.MkdirAll(out, sidecarDirPerm); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, slimManifestName), data, sidecarPerm) //nolint:gosec // see sidecarPerm
}
