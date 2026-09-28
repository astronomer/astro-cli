package airflowenv

import (
	"reflect"
	"testing"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// Each case is a URI as Airflow's own Connection(uri=...) reads it; the want
// is what Airflow stores, not what net/url happens to return. The exception is
// a URL inside an extra, marked below, where Airflow's reading is wrong.
func TestConnFromURIReadsURIsTheWayAirflowDoes(t *testing.T) {
	for _, tc := range []struct {
		name string
		uri  string
		want connmodel.Connection
	}{
		{
			name: "postgresql is postgres",
			uri:  "postgresql://u:p@db:5432/analytics",
			want: connmodel.Connection{ConnType: "postgres", ConnHost: "db", ConnPort: 5432, ConnLogin: "u", ConnPassword: "p", ConnSchema: "analytics"},
		},
		{
			name: "a hyphen in the scheme is an underscore",
			uri:  "google-cloud-platform://?project=acme",
			want: connmodel.Connection{ConnType: "google_cloud_platform", ConnExtra: map[string]any{"project": "acme"}},
		},
		{
			name: "a second scheme is the host's protocol",
			uri:  "http://https://api.example.com:8443/v1",
			want: connmodel.Connection{ConnType: "http", ConnHost: "https://api.example.com", ConnPort: 8443, ConnSchema: "v1"},
		},
		{
			name: "userinfo survives a host protocol",
			uri:  "http://https://u:p@api.example.com",
			want: connmodel.Connection{ConnType: "http", ConnHost: "https://api.example.com", ConnLogin: "u", ConnPassword: "p"},
		},
		// A URL in an extra is not a host protocol, however many "://" the
		// whole string holds. Airflow counts them across the string and would
		// mangle or refuse these; they are common enough (LocalStack's
		// endpoint_url, an OAuth callback) that reading them right matters more
		// than matching that.
		{
			name: "a URL in an extra is an extra",
			uri:  "aws://?endpoint_url=http://localhost:4566",
			want: connmodel.Connection{ConnType: "aws", ConnExtra: map[string]any{"endpoint_url": "http://localhost:4566"}},
		},
		{
			name: "a URL in the query after a path",
			uri:  "http://host/?callback=https://x.example.com/cb",
			want: connmodel.Connection{ConnType: "http", ConnHost: "host", ConnExtra: map[string]any{"callback": "https://x.example.com/cb"}},
		},
		{
			name: "two URLs in extras",
			uri:  "aws://?a=http://x&b=http://y",
			want: connmodel.Connection{ConnType: "aws", ConnExtra: map[string]any{"a": "http://x", "b": "http://y"}},
		},
		{
			name: "a host protocol and a URL in an extra",
			uri:  "http://https://api.example.com/v1?cb=https://x.example.com",
			want: connmodel.Connection{ConnType: "http", ConnHost: "https://api.example.com", ConnSchema: "v1", ConnExtra: map[string]any{"cb": "https://x.example.com"}},
		},
		{
			name: "percent-encoded userinfo and schema are decoded",
			uri:  "mysql://us%40er:p%2Fw@db/my%20db",
			want: connmodel.Connection{ConnType: "mysql", ConnHost: "db", ConnLogin: "us@er", ConnPassword: "p/w", ConnSchema: "my db"},
		},
		{
			name: "an unchanged scheme stays as written",
			uri:  "snowflake://user:pw@account/db",
			want: connmodel.Connection{ConnType: "snowflake", ConnHost: "account", ConnLogin: "user", ConnPassword: "pw", ConnSchema: "db"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConnFromURI("c", tc.uri)
			if err != nil {
				t.Fatalf("ConnFromURI(%q) = %v", tc.uri, err)
			}
			tc.want.ConnID = "c"
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ConnFromURI(%q)\n got %+v\nwant %+v", tc.uri, got, tc.want)
			}
		})
	}
}

// Airflow refuses these too: three schemes, or userinfo/port before the
// second "://", is not a host with a protocol.
func TestConnFromURIRefusesMalformedProtocolHosts(t *testing.T) {
	for _, uri := range []string{
		"http://https://ftp://host",
		"http://u:p@https://host",
		"http://host:80://x",
		"http://://host",
	} {
		if c, err := ConnFromURI("c", uri); err == nil {
			t.Errorf("ConnFromURI(%q) accepted it as %+v", uri, c)
		}
	}
}

func TestNormalizeConnType(t *testing.T) {
	for in, want := range map[string]string{
		"postgresql":            "postgres",
		"postgres":              "postgres",
		"google-cloud-platform": "google_cloud_platform",
		"aws":                   "aws",
	} {
		if got := NormalizeConnType(in); got != want {
			t.Errorf("NormalizeConnType(%q) = %q, want %q", in, got, want)
		}
	}
}
