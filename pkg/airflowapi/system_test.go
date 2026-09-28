package airflowapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestListProvidersReadsAirflow2InOneCallWithNoParameters(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/providers",
		`{"providers":[{"package_name":"apache-airflow-providers-http","version":"4.1.0","description":"HTTP"}],"total_entries":1}`)

	list, err := stub.client().ListProviders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Providers) != 1 || list.Providers[0].PackageName != "apache-airflow-providers-http" || list.Providers[0].Version != "4.1.0" {
		t.Fatalf("list = %+v", list)
	}
	// Airflow 2 declares no parameters here, and a strict validator refuses
	// what it did not declare.
	if q := stub.lastRequest().Query; len(q) != 0 {
		t.Errorf("query = %v, want none on airflow 2", q)
	}
}

// Airflow 3 pages the endpoint and defaults to fifty, so a Runtime image's
// providers do not fit in one answer. The client reads every page.
func TestListProvidersReadsEveryAirflow3Page(t *testing.T) {
	const total = 7
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/version":
			_, _ = fmt.Fprint(w, `{"version":"3.1.0"}`)
		case "/api/v2/providers":
			offsets = append(offsets, r.URL.Query().Get("offset"))
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			// Three a page, whatever was asked for: a server-side cap.
			var items []string
			for i := offset; i < min(offset+3, total); i++ {
				items = append(items, fmt.Sprintf(`{"package_name":"p%d","version":"1.0"}`, i))
			}
			_, _ = fmt.Fprintf(w, `{"providers":[%s],"total_entries":%d}`, strings.Join(items, ","), total)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	transport, err := NewHTTPTransport(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	list, err := New(transport).ListProviders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Providers) != total || list.Providers[total-1].PackageName != "p6" {
		t.Fatalf("got %d providers (%+v), want all %d", len(list.Providers), list.Providers, total)
	}
	if want := []string{"", "3", "6"}; !reflect.DeepEqual(offsets, want) {
		t.Errorf("offsets asked = %q, want %q", offsets, want)
	}
}

func TestListPluginsNamesWhatEachContributes(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
		body string
		want map[string][]string
	}{
		{
			name: "airflow 2", stub: newAF2Stub, path: "/api/v1/plugins",
			body: `{"plugins":[{"name":"metrics","source":"$PLUGINS_FOLDER/metrics.py","hooks":["MetricsHook"],
				"executors":[],"macros":["b_macro","a_macro"],"appbuilder_views":[{"name":"Metrics","category":"Admin"}],
				"ti_deps":[],"listeners":[]}],"total_entries":1}`,
			want: map[string][]string{"hooks": {"MetricsHook"}, "macros": {"a_macro", "b_macro"}, "appbuilder_views": {"Metrics"}},
		},
		{
			name: "airflow 3", stub: newAF3Stub, path: "/api/v2/plugins",
			body: `{"plugins":[{"name":"metrics","source":null,"macros":[],"fastapi_apps":[{"name":"metrics_api","url_prefix":"/m"}],
				"react_apps":[],"timetables":["WorkdayTimetable"]}],"total_entries":1}`,
			want: map[string][]string{"fastapi_apps": {"metrics_api"}, "timetables": {"WorkdayTimetable"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, tc.body)
			list, err := stub.client().ListPlugins(t.Context(), ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Plugins) != 1 || list.Plugins[0].Name != "metrics" {
				t.Fatalf("list = %+v", list)
			}
			if got := list.Plugins[0].Components; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("components = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConfigFlattensSectionsAndReadsBothValueShapes(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/config", `{"sections":[
		{"name":"core","options":[{"key":"executor","value":"LocalExecutor"},{"key":"fernet_key","value":["< hidden >","env var"]}]},
		{"name":"api","options":[{"key":"expose_config","value":"True"}]}]}`)

	config, err := stub.client().Config(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []ConfigOption{
		{Section: "core", Key: "executor", Value: "LocalExecutor"},
		{Section: "core", Key: "fernet_key", Value: "< hidden >", Source: "env var"},
		{Section: "api", Key: "expose_config", Value: "True"},
	}
	if !reflect.DeepEqual(config.Options, want) {
		t.Errorf("options = %+v\nwant %+v", config.Options, want)
	}
	if q := stub.lastRequest().Query; q.Has("section") {
		t.Errorf("query = %v, want no section filter", q)
	}

	if _, err := stub.client().Config(t.Context(), "core"); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Query.Get("section"); got != "core" {
		t.Errorf("section = %q, want core", got)
	}
}

// Neither generation exposes its configuration by default, and the refusal is
// something a caller explains rather than dumps.
func TestConfigRefusalReadsAsForbidden(t *testing.T) {
	stub := newAF2Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v1/config", http.StatusForbidden,
		`{"detail":"Your Airflow administrator chose not to expose the configuration, most likely for security reasons."}`)
	_, err := stub.client().Config(t.Context(), "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestUpstreamAssetEventsAsksEachGenerationItsOwnWay(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
		body string
	}{
		{
			name: "airflow 2", stub: newAF2Stub,
			path: "/api/v1/dags/report/dagRuns/dataset_triggered__1/upstreamDatasetEvents",
			body: `{"dataset_events":[{"id":4,"dataset_id":2,"dataset_uri":"s3://orders","source_dag_id":"etl"}],"total_entries":1}`,
		},
		{
			name: "airflow 3", stub: newAF3Stub,
			path: "/api/v2/dags/report/dagRuns/dataset_triggered__1/upstreamAssetEvents",
			body: `{"asset_events":[{"id":4,"asset_id":2,"uri":"s3://orders","source_dag_id":"etl"}],"total_entries":1}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, tc.body)
			list, err := stub.client().UpstreamAssetEvents(t.Context(), "report", "dataset_triggered__1")
			if err != nil {
				t.Fatal(err)
			}
			if len(list.AssetEvents) != 1 {
				t.Fatalf("list = %+v", list)
			}
			event := list.AssetEvents[0]
			if event.URI != "s3://orders" || event.AssetID != 2 || event.SourceDAGID != "etl" {
				t.Errorf("event = %+v, want both spellings folded", event)
			}
		})
	}
}

func TestListAssetsSendsTheURIPattern(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
	}{
		{"airflow 2", newAF2Stub, "/api/v1/datasets"},
		{"airflow 3", newAF3Stub, "/api/v2/assets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, `{"assets":[],"datasets":[],"total_entries":0}`)
			if _, err := stub.client().ListAssets(t.Context(), ListAssetsOptions{URIPattern: "s3://orders%"}); err != nil {
				t.Fatal(err)
			}
			if got := stub.lastRequest().Query.Get("uri_pattern"); got != "s3://orders%" {
				t.Errorf("uri_pattern = %q", got)
			}
		})
	}
}

// Airflow 2 filters on only_active and Airflow 3 on exclude_stale; the two
// mean the same thing, and sending the wrong one is silently ignored.
func TestListDAGsSpellsTheActiveFilterPerGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, want, wrong string
		stub              func(*testing.T) *airflowStub
		path              string
	}{
		{"airflow 2", "only_active", "exclude_stale", newAF2Stub, "/api/v1/dags"},
		{"airflow 3", "exclude_stale", "only_active", newAF3Stub, "/api/v2/dags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, `{"dags":[],"total_entries":0}`)
			for _, active := range []bool{true, false} {
				if _, err := stub.client().ListDAGs(t.Context(), ListDAGsOptions{OnlyActive: &active}); err != nil {
					t.Fatal(err)
				}
				q := stub.lastRequest().Query
				if got := q.Get(tc.want); got != strconv.FormatBool(active) {
					t.Errorf("%s = %q, want %v", tc.want, got, active)
				}
				if q.Has(tc.wrong) {
					t.Errorf("query = %v carries the other generation's %s", q, tc.wrong)
				}
			}
			// Unset sends neither, leaving Airflow's own default.
			if _, err := stub.client().ListDAGs(t.Context(), ListDAGsOptions{}); err != nil {
				t.Fatal(err)
			}
			if q := stub.lastRequest().Query; q.Has("only_active") || q.Has("exclude_stale") {
				t.Errorf("query = %v, want no active filter", q)
			}
		})
	}
}
