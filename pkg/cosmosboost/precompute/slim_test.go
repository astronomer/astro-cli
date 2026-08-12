package precompute

import (
	"encoding/json"
	"reflect"
	"testing"
)

func parseDoc(t *testing.T, raw string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid fixture JSON: %v", err)
	}
	return doc
}

// TestBuildSlimManifestKeepsOnlyAllowedResourceFields pins the field allowlist
// Cosmos actually reads from a manifest node when building a DbtNode
// (cosmos/dbt/graph.py::_build_dbt_node_from_manifest_resource): everything
// else is dropped.
func TestBuildSlimManifestKeepsOnlyAllowedResourceFields(t *testing.T) {
	doc := parseDoc(t, `{
		"metadata": {"project_name": "shop"},
		"nodes": {
			"model.shop.orders": {
				"original_file_path": "models/orders.sql",
				"package_name": "shop",
				"resource_type": "model",
				"tags": ["daily"],
				"config": {"materialized": "table"},
				"fqn": ["shop", "orders"],
				"depends_on": {"nodes": ["model.shop.customers"], "macros": ["macro.dbt.foo"]},
				"database": "analytics",
				"schema": "public",
				"alias": "orders",
				"checksum": {"name": "sha256", "checksum": "abc"},
				"raw_code": "select * from customers",
				"compiled_code": "select * from customers",
				"description": "orders model"
			}
		}
	}`)

	slim := buildSlimManifest(doc, "test")

	node := slim["nodes"].(map[string]any)["model.shop.orders"].(map[string]any)
	want := map[string]any{
		"original_file_path": "models/orders.sql",
		"package_name":       "shop",
		"resource_type":      "model",
		"tags":               []any{"daily"},
		"config":             map[string]any{"materialized": "table"},
		"fqn":                []any{"shop", "orders"},
		"depends_on":         map[string]any{"nodes": []any{"model.shop.customers"}},
	}
	if !reflect.DeepEqual(node, want) {
		t.Fatalf("slim node = %+v, want %+v", node, want)
	}
}

// TestBuildSlimManifestKeepsFreshnessForSourcesOnly: freshness is only read
// for resource_type=="source" (is_freshness_effective); a model's freshness
// key (if it ever had one) must not survive slimming.
func TestBuildSlimManifestKeepsFreshnessForSourcesOnly(t *testing.T) {
	doc := parseDoc(t, `{
		"sources": {
			"source.shop.raw.orders": {
				"original_file_path": "models/sources.yml",
				"package_name": "shop",
				"resource_type": "source",
				"fqn": ["shop", "raw", "orders"],
				"freshness": {"warn_after": {"count": 1, "period": "day"}}
			}
		},
		"nodes": {
			"model.shop.orders": {
				"original_file_path": "models/orders.sql",
				"package_name": "shop",
				"resource_type": "model",
				"fqn": ["shop", "orders"],
				"freshness": {"warn_after": {"count": 1, "period": "day"}}
			}
		}
	}`)

	slim := buildSlimManifest(doc, "test")

	source := slim["sources"].(map[string]any)["source.shop.raw.orders"].(map[string]any)
	if _, ok := source["freshness"]; !ok {
		t.Fatalf("source must keep freshness: %+v", source)
	}

	node := slim["nodes"].(map[string]any)["model.shop.orders"].(map[string]any)
	if _, ok := node["freshness"]; ok {
		t.Fatalf("non-source resource must not keep freshness: %+v", node)
	}
}

// TestBuildSlimManifestDropsUnusedSections: LoadMode.DBT_MANIFEST only merges
// nodes+sources+exposures into the resource dict (cosmos/dbt/graph.py::
// _load_nodes_from_manifest_data); every other top-level section is unused.
func TestBuildSlimManifestDropsUnusedSections(t *testing.T) {
	doc := parseDoc(t, `{
		"metadata": {"project_name": "shop", "dbt_schema_version": "v12", "generated_at": "t"},
		"nodes": {},
		"sources": {},
		"exposures": {},
		"macros": {"macro.dbt.foo": {}},
		"disabled": {"model.shop.old": [{}]},
		"docs": {"doc.shop.readme": {}},
		"parent_map": {"model.shop.orders": []},
		"child_map": {"model.shop.orders": []},
		"selectors": {"my_selector": {"definition": {}}}
	}`)

	slim := buildSlimManifest(doc, "test")

	for _, dropped := range []string{"macros", "disabled", "docs", "parent_map", "child_map"} {
		if _, ok := slim[dropped]; ok {
			t.Fatalf("slim manifest must not contain %q: %+v", dropped, slim)
		}
	}
	if meta := slim["metadata"].(map[string]any); len(meta) != 1 || meta["project_name"] != "shop" {
		t.Fatalf("metadata must be reduced to project_name only, got %+v", meta)
	}
	if _, ok := slim["selectors"]; !ok {
		t.Fatalf("selectors must be kept for YAML-selector support: %+v", slim)
	}
}

// TestBuildSlimManifestHandlesMissingSections: a manifest missing an optional
// section (e.g. no exposures) must not panic and yields an empty collection.
func TestBuildSlimManifestHandlesMissingSections(t *testing.T) {
	doc := parseDoc(t, `{"metadata": {"project_name": "shop"}, "nodes": {}}`)

	slim := buildSlimManifest(doc, "test")

	for _, section := range slimSections {
		entries, ok := slim[section].(map[string]any)
		if !ok || len(entries) != 0 {
			t.Fatalf("missing section %q must slim to an empty map, got %+v", section, slim[section])
		}
	}
}

// TestBuildSlimManifestNeverEmitsNulls: an absent key must not turn into an
// explicit null. A reader doing manifest.get("selectors", {}) - the idiom that
// works against a full manifest - would get None back and fail on the whole
// file, so the slim manifest's shape stays a subset of the full one's.
func TestBuildSlimManifestNeverEmitsNulls(t *testing.T) {
	// No top-level selectors, and metadata without project_name.
	doc := parseDoc(t, `{"metadata": {"dbt_schema_version": "v12"}, "nodes": {}}`)

	slim := buildSlimManifest(doc, "test")

	if selectors, ok := slim["selectors"].(map[string]any); !ok || len(selectors) != 0 {
		t.Fatalf("absent selectors must slim to an empty object, got %#v", slim["selectors"])
	}
	meta, ok := slim["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata has the wrong type: %#v", slim["metadata"])
	}
	if _, present := meta["project_name"]; present {
		t.Fatalf("absent project_name must be left out, not set to null: %#v", meta)
	}

	// A wrong-typed metadata (not an object) must not panic or leak through.
	if slim := buildSlimManifest(parseDoc(t, `{"metadata": 7, "selectors": 7}`), "test"); len(slim["metadata"].(map[string]any)) != 0 ||
		len(slim["selectors"].(map[string]any)) != 0 {
		t.Fatalf("wrong-typed sections must slim to empty objects, got %#v", slim)
	}
}

// TestBuildSlimManifestIncludesVersionMarker: a future reader needs a way to
// tell which allowlist produced a slim manifest, so it carries its own
// schema/generated_by, independent of the sidecar sitting next to it.
func TestBuildSlimManifestIncludesVersionMarker(t *testing.T) {
	doc := parseDoc(t, `{"metadata": {"project_name": "shop"}, "nodes": {}}`)

	slim := buildSlimManifest(doc, "1.2.3")

	if slim["_schema"] != slimSchemaVersion {
		t.Fatalf("_schema = %v, want %v", slim["_schema"], slimSchemaVersion)
	}
	gb, ok := slim["_generated_by"].(GeneratedBy)
	if !ok {
		t.Fatalf("_generated_by has the wrong type: %T", slim["_generated_by"])
	}
	if gb.Application != application || gb.Version != "1.2.3" {
		t.Fatalf("_generated_by = %+v, want application=%q version=%q", gb, application, "1.2.3")
	}
}
