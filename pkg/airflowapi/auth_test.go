package airflowapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestTokenMinterMintsABearer(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"jwt-123"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}

	scheme, value, err := minter.Credentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if scheme != bearerScheme || value != "jwt-123" {
		t.Errorf("credentials = %q %q, want the minted bearer", scheme, value)
	}
	if got := stub.lastRequest().Body; got != `{"password":"pass","username":"user"}` {
		t.Errorf("body = %q, want the credentials posted", got)
	}
}

func TestTokenMinterFallsBackToBasicWhenNothingMints(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		stub := newStub(t)
		stub.routeStatus(http.MethodPost, airflowAuthPath, status, `{"detail":"nope"}`)
		minter, err := NewTokenMinter(stub.URL, "user", "pass")
		if err != nil {
			t.Fatal(err)
		}

		scheme, value, err := minter.Credentials(t.Context())
		if err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		if scheme != basicScheme {
			t.Errorf("status %d: scheme = %q, want basic", status, scheme)
		}
		if want := base64.StdEncoding.EncodeToString([]byte("user:pass")); value != want {
			t.Errorf("status %d: value = %q, want the encoded credentials", status, value)
		}
	}
}

func TestTokenMinterMintsWithoutCredentials(t *testing.T) {
	// An Airflow 3 in all-admins mode mints for whoever asks.
	stub := newStub(t)
	stub.route(http.MethodGet, airflowAuthPath, `{"access_token":"jwt-open"}`)
	minter, err := NewTokenMinter(stub.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	scheme, value, err := minter.Credentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if scheme != bearerScheme || value != "jwt-open" {
		t.Errorf("credentials = %q %q, want the minted bearer", scheme, value)
	}
	if got := stub.lastRequest().Body; got != "" {
		t.Errorf("body = %q, want no credentials sent", got)
	}
}

func TestTokenMinterSendsNothingWhenThereIsNothingToSend(t *testing.T) {
	// No credentials and no mint endpoint is an Airflow with no auth at all.
	stub := newStub(t)
	minter, err := NewTokenMinter(stub.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	scheme, _, err := minter.Credentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if scheme != "" {
		t.Errorf("scheme = %q, want the request sent unauthenticated", scheme)
	}
	for range 2 {
		if _, _, err := minter.Credentials(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := stub.countRequests(http.MethodGet, airflowAuthPath); got != 1 {
		t.Errorf("asked %d times, want the answer held like any other", got)
	}
}

func TestTokenMinterTreatsA403AsARefusalNotAFallback(t *testing.T) {
	// Only a missing endpoint means "this Airflow does not mint". A proxy
	// refusing the mint is a refusal, and reading it as anything else would
	// swap a clear error for a confusing one.
	stub := newStub(t)
	stub.routeStatus(http.MethodPost, airflowAuthPath, http.StatusForbidden, `{"detail":"Forbidden"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = minter.Credentials(t.Context())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("err = %v, want the refusal reported", err)
	}
}

func TestTokenMinterReportsARefusal(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodPost, airflowAuthPath, http.StatusUnauthorized, `{"detail":"bad password"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "wrong")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = minter.Credentials(t.Context())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want it to read as unauthorized", err)
	}
}

func TestTokenMinterFailsWhenTheAnswerHasNoToken(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := minter.Credentials(t.Context()); err == nil {
		t.Error("accepted an answer with no access_token")
	}
}

func TestTokenMinterHoldsTheTokenUntilRefreshed(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"first"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if _, _, err := minter.Credentials(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := stub.countRequests(http.MethodPost, airflowAuthPath); got != 1 {
		t.Errorf("minted %d times, want the token held", got)
	}

	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"second"}`)
	if err := minter.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, value, err := minter.Credentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if value != "second" {
		t.Errorf("value = %q, want a freshly minted token after a refresh", value)
	}
}

func TestTokenMinterDrivesATransportThroughAnExpiredToken(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"expired"}`)
	stub.routeStatus(http.MethodGet, "/api/v2/dags", http.StatusUnauthorized, `{"detail":"expired"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewHTTPTransport(stub.URL,
		WithCredentials(minter.Credentials),
		WithRefresh(func(ctx context.Context) error {
			stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"fresh"}`)
			stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":0}`)
			return minter.Refresh(ctx)
		}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the retry with a fresh token to work", resp.StatusCode)
	}
	if got := stub.lastRequest().Header.Get("Authorization"); got != "Bearer fresh" {
		t.Errorf("Authorization = %q, want the re-minted token", got)
	}
}

func TestClientCredentialsMinterSendsTheOAuthFieldNames(t *testing.T) {
	// Keycloak's auth manager reads client_id and client_secret. Same endpoint,
	// same answer, different names — which is the whole difference.
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"jwt-kc"}`)
	minter, err := NewClientCredentialsMinter(stub.URL, "cli", "s3cr3t")
	if err != nil {
		t.Fatal(err)
	}

	scheme, value, err := minter.Credentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if scheme != bearerScheme || value != "jwt-kc" {
		t.Errorf("credentials = %q %q, want the minted bearer", scheme, value)
	}
	// The exact request the Keycloak auth manager's documentation publishes:
	// grant_type, client_id, client_secret, as JSON.
	if got := stub.lastRequest().Body; got != `{"client_id":"cli","client_secret":"s3cr3t","grant_type":"client_credentials"}` {
		t.Errorf("body = %q, want the documented client-credentials request", got)
	}
}

// TestTokenMinterSendsNoGrantTypeForAPassword: every auth manager defaults to
// the password grant, and the ones that are not Keycloak have never heard of
// the field, so sending it would be a new way to fail on the common instance.
func TestTokenMinterSendsNoGrantTypeForAPassword(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, airflowAuthPath, `{"access_token":"jwt"}`)
	minter, err := NewTokenMinter(stub.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := minter.Credentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Body; strings.Contains(got, "grant_type") {
		t.Errorf("body = %q, want no grant_type on the password grant", got)
	}
}

func TestClientCredentialsMinterRefusesToFallBackToBasic(t *testing.T) {
	// A client secret is not a password. An Airflow with no /auth/token has no
	// auth manager to exchange one, so say that rather than send the secret on
	// to a server that never asked for it.
	stub := newStub(t)
	stub.routeStatus(http.MethodPost, airflowAuthPath, http.StatusNotFound, `{"detail":"nope"}`)
	minter, err := NewClientCredentialsMinter(stub.URL, "cli", "s3cr3t")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = minter.Credentials(t.Context())
	if err == nil || !strings.Contains(err.Error(), airflowAuthPath) {
		t.Fatalf("err = %v, want one naming %s", err, airflowAuthPath)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("the secret is in the message: %v", err)
	}
}
