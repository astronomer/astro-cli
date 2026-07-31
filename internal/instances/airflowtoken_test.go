package instances

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mintStub stands in for an Airflow 3's own /auth/token, recording the body it
// was asked with and how often it was asked.
type mintStub struct {
	*httptest.Server
	asked map[string]string
	mints int
}

func tokenEndpoint(t *testing.T, minted string) *mintStub {
	t.Helper()
	stub := &mintStub{asked: map[string]string{}}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		stub.mints++
		if err := json.NewDecoder(r.Body).Decode(&stub.asked); err != nil {
			t.Errorf("decode mint request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"` + minted + `"}`))
	}))
	t.Cleanup(stub.Close)
	return stub
}

// TestAirflowTokenMintsWithAUsernamePair covers the FAB and simple-auth-manager
// shape: the pair the manifest named is the pair that is posted.
func TestAirflowTokenMintsWithAUsernamePair(t *testing.T) {
	stub := tokenEndpoint(t, "minted-fab")
	i := link(t, "\n[tool.astro.deployments.af3]\nurl = '"+stub.URL+"'\nauth = { method = 'airflow-token', username-env = 'AF_USER', password-env = 'AF_PASS' }\n")

	src, refresh, err := credentials(i, i.URL, Deps{
		LookupEnv:  env(map[string]string{"AF_USER": "ada", "AF_PASS": "hunter2"}),
		HTTPClient: stub.Client(),
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer minted-fab" {
		t.Fatalf("header = %q", got)
	}
	if stub.asked["username"] != "ada" || stub.asked["password"] != "hunter2" {
		t.Fatalf("minted with %v, want the username pair", stub.asked)
	}
	if refresh == nil {
		t.Fatal("no refresh hook: a short-lived minted token has to be re-mintable")
	}
}

// TestAirflowTokenMintsWithAClientPair covers Keycloak: the same endpoint, the
// OAuth field names, because that is what its auth manager reads.
func TestAirflowTokenMintsWithAClientPair(t *testing.T) {
	stub := tokenEndpoint(t, "minted-kc")
	i := link(t, "\n[tool.astro.deployments.af3]\nurl = '"+stub.URL+"'\nauth = { method = 'airflow-token', client-id-env = 'AF_ID', client-secret-env = 'AF_SECRET' }\n")

	src, _, err := credentials(i, i.URL, Deps{
		LookupEnv:  env(map[string]string{"AF_ID": "cli", "AF_SECRET": "s3cr3t"}),
		HTTPClient: stub.Client(),
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer minted-kc" {
		t.Fatalf("header = %q", got)
	}
	if stub.asked["client_id"] != "cli" || stub.asked["client_secret"] != "s3cr3t" {
		t.Fatalf("minted with %v, want the OAuth field names", stub.asked)
	}
}

func TestAirflowTokenNamesAMissingCredentialVariable(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.af3]\nurl = 'https://af3.corp.dev'\nauth = { method = 'airflow-token', client-id-env = 'AF_ID', client-secret-env = 'AF_SECRET' }\n")
	for _, missing := range []string{"AF_ID", "AF_SECRET"} {
		set := map[string]string{"AF_ID": "cli", "AF_SECRET": "s3cr3t"}
		delete(set, missing)
		_, _, err := credentials(i, i.URL, Deps{LookupEnv: env(set)})
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("err = %v, want one naming %s", err, missing)
		}
	}
}

// TestAirflowTokenIsHeldForTheRun: minting once per command is the point of
// holding it, and nothing is written down.
func TestAirflowTokenIsHeldForTheRun(t *testing.T) {
	stub := tokenEndpoint(t, "held")
	i := link(t, "\n[tool.astro.deployments.af3]\nurl = '"+stub.URL+"'\nauth = { method = 'airflow-token', username-env = 'AF_USER', password-env = 'AF_PASS' }\n")
	src, refresh, err := credentials(i, i.URL, Deps{
		LookupEnv:  env(map[string]string{"AF_USER": "ada", "AF_PASS": "hunter2"}),
		HTTPClient: stub.Client(),
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	for range 3 {
		if _, _, err := src(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if stub.mints != 1 {
		t.Fatalf("minted %d times, want the token held for the run", stub.mints)
	}
	if err := refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stub.mints != 2 {
		t.Fatalf("minted %d times, want a refresh to mint again", stub.mints)
	}
}
