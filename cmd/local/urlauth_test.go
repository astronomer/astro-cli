package local

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/instances"
)

// `astro af … --url` with ASTRO_AIRFLOW_USERNAME and ASTRO_AIRFLOW_PASSWORD,
// through the real root: the resolution, the transport, and the command.

// runURLWithPair runs `astro af dags list --url` against the stub with a
// username and password in the environment and nothing else.
func runURLWithPair(t *testing.T, stub *airflowStub) error {
	t.Helper()
	d, _, _ := queryDeps(t)
	t.Setenv(instances.EnvUsername, "ada")
	t.Setenv(instances.EnvPassword, "hunter2")
	return execute(t, d, afName, "dags", "list", "--url", stub.URL)
}

// An Airflow 3 answers API calls only with the JWT its /auth/token mints, so
// the pair is exchanged there first and the token is what the request carries.
func TestURLTargetMintsOnAirflow3(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/auth/token", `{"access_token":"minted-jwt"}`)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)

	if err := runURLWithPair(t, stub); err != nil {
		t.Fatalf("af dags list --url: %v", err)
	}
	var mint struct{ Username, Password string }
	if err := json.Unmarshal([]byte(stub.request(http.MethodPost, "/auth/token").Body), &mint); err != nil {
		t.Fatalf("mint body: %v", err)
	}
	if mint.Username != "ada" || mint.Password != "hunter2" {
		t.Errorf("minted as %q, want the pair from the environment", mint.Username)
	}
	if got := stub.request(http.MethodGet, "/api/v2/dags").Header.Get("Authorization"); got != "Bearer minted-jwt" {
		t.Errorf("Authorization = %q, want the minted token", got)
	}
}

// An Airflow 2 serves no /auth/token, and there the pair goes on the request
// as basic auth, which is all that ever worked against it.
func TestURLTargetKeepsBasicAuthOnAirflow2(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags", `{"dags":[],"total_entries":0}`)

	if err := runURLWithPair(t, stub); err != nil {
		t.Fatalf("af dags list --url: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:hunter2"))
	if got := stub.request(http.MethodGet, "/api/v1/dags").Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want basic auth", got)
	}
}
