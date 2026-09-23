package precompute

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
// Cosmos actually reads from a manifest node - both the DbtNode build
// (cosmos/dbt/graph.py::_build_dbt_node_from_manifest_resource) and the outlet
// URI build (cosmos/dataset.py::compute_model_outlet_uris, which needs
// database/schema/alias/name): everything else is dropped.
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
				"name": "orders",
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
		"database":           "analytics",
		"schema":             "public",
		"alias":              "orders",
		"name":               "orders",
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

// TestBuildSlimManifestHandlesAbsentInput: absent or wrong-typed input yields
// empty collections, never a null. A reader doing manifest.get("selectors", {})
// - the idiom that works against a full manifest - would get None from a null
// and fail on the whole file.
func TestBuildSlimManifestHandlesAbsentInput(t *testing.T) {
	doc := parseDoc(t, `{"metadata": {"dbt_schema_version": "v12"}, "nodes": {}}`)

	slim := buildSlimManifest(doc, "test")

	for _, section := range slimSections {
		entries, ok := slim[section].(map[string]any)
		if !ok || len(entries) != 0 {
			t.Fatalf("missing section %q must slim to an empty map, got %#v", section, slim[section])
		}
	}
	if selectors, ok := slim["selectors"].(map[string]any); !ok || len(selectors) != 0 {
		t.Fatalf("absent selectors must slim to an empty object, got %#v", slim["selectors"])
	}
	if meta := slim["metadata"].(map[string]any); len(meta) != 0 {
		t.Fatalf("absent project_name must be left out, not nulled: %#v", meta)
	}

	// Wrong-typed sections must not panic or leak through either.
	slim = buildSlimManifest(parseDoc(t, `{"metadata": 7, "selectors": 7}`), "test")
	if len(slim["metadata"].(map[string]any)) != 0 || len(slim["selectors"].(map[string]any)) != 0 {
		t.Fatalf("wrong-typed sections must slim to empty objects, got %#v", slim)
	}
}

// TestBuildSlimManifestCutsSize is the CI guard for the size (and so parse
// cost) reduction this artifact exists for. The fixture gives each resource the
// bulk a real manifest carries - code, columns, descriptions - none of which
// Cosmos reads. The floor is well under the ~3x measured on the smallest
// prototype subject: it catches a bulky field creeping back into the allowlist,
// it is not a promise about any one project.
func TestBuildSlimManifestCutsSize(t *testing.T) {
	sql := strings.Repeat("select * from {{ ref('upstream') }} -- padding\n", 4)
	nodes := map[string]any{}
	for i := range 200 {
		nodes[fmt.Sprintf("model.shop.m%d", i)] = map[string]any{
			"original_file_path": fmt.Sprintf("models/m%d.sql", i),
			"package_name":       "shop",
			"resource_type":      "model",
			"config":             map[string]any{"materialized": "table"},
			"fqn":                []any{"shop", fmt.Sprintf("m%d", i)},
			"depends_on":         map[string]any{"nodes": []any{"model.shop.upstream"}},
			// Dropped, and the bulk of a real manifest:
			"raw_code":      sql,
			"compiled_code": sql,
			"description":   strings.Repeat("a description dbt stores verbatim. ", 4),
			"columns":       map[string]any{"a": map[string]any{"description": sql, "data_type": "varchar"}},
		}
	}
	doc := map[string]any{"metadata": map[string]any{"project_name": "shop"}, "nodes": nodes}

	full, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	slim, err := json.Marshal(buildSlimManifest(doc, "test"))
	if err != nil {
		t.Fatal(err)
	}

	const maxRatio = 0.34 // ~3x smaller
	if ratio := float64(len(slim)) / float64(len(full)); ratio > maxRatio {
		t.Fatalf("slim manifest is %.0f%% of the full one (%d vs %d bytes), want at most %.0f%%",
			ratio*100, len(slim), len(full), maxRatio*100)
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
