package precompute

import (
	"encoding/json"
	"fmt"
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

// TestBuildSlimManifestKeepsOutletURIFields is the regression for the outlet
// URI read specifically. compute_model_outlet_uris builds
// "database.schema.alias" from this file under ExecutionMode.WATCHER on
// Kubernetes/GKE, and skips the model silently when any part is missing - so
// dropping these would cost dataset/Asset outlets with nothing in the logs to
// explain it. `name` is the fallback when `alias` is absent.
func TestBuildSlimManifestKeepsOutletURIFields(t *testing.T) {
	doc := parseDoc(t, `{
		"nodes": {
			"seed.shop.countries": {
				"original_file_path": "seeds/countries.csv", "package_name": "shop",
				"resource_type": "seed", "database": "analytics", "schema": "raw", "name": "countries"
			}
		}
	}`)

	node := buildSlimManifest(doc, "test")["nodes"].(map[string]any)["seed.shop.countries"].(map[string]any)

	for _, key := range []string{"database", "schema", "name"} {
		if _, ok := node[key]; !ok {
			t.Fatalf("outlet URI field %q dropped: %+v", key, node)
		}
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

// bulkyManifest builds a manifest whose resources carry the fields a real dbt
// manifest is mostly made of - raw_code/compiled_code, columns, docs blocks,
// checksums, patch paths, depends_on.macros - none of which Cosmos reads. The
// proportions matter more than the absolute size: this is what makes the
// reduction ratio below meaningful rather than arbitrary.
func bulkyManifest(nodeCount int) map[string]any {
	sql := "select * from {{ ref('upstream') }} where 1=1 -- " +
		"padding to approximate a real model body, which dominates manifest size\n"
	nodes := map[string]any{}
	for i := range nodeCount {
		id := fmt.Sprintf("model.shop.m%d", i)
		columns := map[string]any{}
		for c := range 12 {
			columns[fmt.Sprintf("col_%d", c)] = map[string]any{
				"name": fmt.Sprintf("col_%d", c), "description": "a column description that dbt stores verbatim",
				"meta": map[string]any{}, "data_type": "varchar", "constraints": []any{}, "tags": []any{},
			}
		}
		nodes[id] = map[string]any{
			// Read by Cosmos:
			"original_file_path": fmt.Sprintf("models/m%d.sql", i),
			"package_name":       "shop",
			"resource_type":      "model",
			"tags":               []any{"nightly"},
			"config":             map[string]any{"materialized": "table", "tags": []any{"nightly"}},
			"fqn":                []any{"shop", fmt.Sprintf("m%d", i)},
			"depends_on":         map[string]any{"nodes": []any{"model.shop.upstream"}, "macros": []any{"macro.dbt.ref", "macro.dbt.config"}},
			// Not read by Cosmos, and the bulk of a real manifest:
			"raw_code":      sql + sql + sql,
			"compiled_code": sql + sql + sql + sql,
			"columns":       columns,
			"description":   "a model description that dbt stores verbatim in the manifest",
			"checksum":      map[string]any{"name": "sha256", "checksum": "b1946ac92492d2347c6235b4d2611184b1946ac92492d2347c6235b4d2611184"},
			"patch_path":    fmt.Sprintf("shop://models/schema/m%d.yml", i),
			"docs":          map[string]any{"show": true, "node_color": nil},
			"unrendered_config": map[string]any{
				"materialized": "table", "tags": []any{"nightly"},
			},
			"created_at": 1754300000.1,
			"meta":       map[string]any{"owner": "analytics"},
		}
	}
	return map[string]any{
		"metadata":  map[string]any{"project_name": "shop", "dbt_schema_version": "https://schemas.getdbt.com/dbt/manifest/v12.json"},
		"nodes":     nodes,
		"sources":   map[string]any{},
		"exposures": map[string]any{},
		"selectors": map[string]any{},
		// Whole sections Cosmos never touches.
		"macros":     map[string]any{"macro.dbt.ref": map[string]any{"macro_sql": sql + sql, "depends_on": map[string]any{"macros": []any{}}}},
		"parent_map": map[string]any{"model.shop.m0": []any{"model.shop.upstream"}},
		"child_map":  map[string]any{"model.shop.upstream": []any{"model.shop.m0"}},
		"docs":       map[string]any{"doc.shop.readme": map[string]any{"block_contents": sql}},
		"disabled":   map[string]any{},
	}
}

// TestBuildSlimManifestCutsSize is the CI guard for the acceptance criterion
// this artifact exists to satisfy: a measurable size (and so parse-cost)
// reduction on a large project. The prototype this was modeled on measured
// roughly 3x on its smaller subject and 7x on its larger one, so the floor
// asserted here is deliberately below the low end - it catches allowlist creep
// (a bulky field quietly added back), not a promise about any one customer's
// manifest.
func TestBuildSlimManifestCutsSize(t *testing.T) {
	doc := bulkyManifest(200)
	full, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	slim, err := json.Marshal(buildSlimManifest(doc, "test"))
	if err != nil {
		t.Fatal(err)
	}

	const maxRatio = 0.34 // ~3x smaller
	ratio := float64(len(slim)) / float64(len(full))
	if ratio > maxRatio {
		t.Fatalf("slim manifest is %.0f%% of the full one (%d vs %d bytes), want at most %.0f%%",
			ratio*100, len(slim), len(full), maxRatio*100)
	}
	t.Logf("slim manifest is %.1fx smaller (%d -> %d bytes)", 1/ratio, len(full), len(slim))
}

// TestBuildSlimManifestNoRegressionOnSmallManifest: the other half of that
// criterion. A manifest holding nothing but fields Cosmos reads cannot be
// slimmed, so the slim copy is necessarily a little larger - its own
// _schema/_generated_by markers. That overhead must stay a small constant, not
// grow with the manifest.
func TestBuildSlimManifestNoRegressionOnSmallManifest(t *testing.T) {
	doc := parseDoc(t, `{"metadata":{"project_name":"shop"},"selectors":{},
	  "nodes":{"model.shop.a":{"original_file_path":"models/a.sql","package_name":"shop",
	  "resource_type":"model","tags":[],"config":{},"fqn":["shop","a"]}}}`)
	full, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	slim, err := json.Marshal(buildSlimManifest(doc, "1.44.0"))
	if err != nil {
		t.Fatal(err)
	}

	// The markers, plus the empty sources/exposures sections always emitted.
	const maxOverhead = 160
	if grew := len(slim) - len(full); grew > maxOverhead {
		t.Fatalf("slim copy of an already-minimal manifest grew by %d bytes (%d -> %d), want at most %d",
			grew, len(full), len(slim), maxOverhead)
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
