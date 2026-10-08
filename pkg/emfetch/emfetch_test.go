package emfetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// The refusal as the API envelope carries it.
const refusalEnvelope = `{"message":"showSecrets is not allowed for this organization"}`

// A gateway or proxy rejecting the request without reaching the app, quoting
// the request URI back. It carries all three tokens: the path holds
// "organizations", the query holds showSecrets, and "not allowed" is the reason
// phrase of 405.
const gatewayEcho = "405 Not Allowed\nThe requested method is not allowed for URL " +
	"/organizations/cl123/environment-objects?limit=1000&showSecrets=true"

func TestIsOrgSecretsRefusal(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "the envelope at 403", status: http.StatusForbidden, body: refusalEnvelope, want: true},
		// A body that does not parse is the case a parsed-message check cannot
		// see, and the status it arrives under is not fixed, so both halves are
		// held here.
		{name: "no envelope at 405", status: http.StatusMethodNotAllowed, body: "showSecrets is not allowed for this organization", want: true},
		{name: "an html error page", status: http.StatusForbidden, body: "<html><body>showSecrets is not allowed for this Organization</body></html>", want: true},
		// An envelope keyed on something other than message. It parses, so a
		// check reading one field sees an empty string rather than a refusal.
		{name: "another envelope key", status: http.StatusForbidden, body: `{"error":"showSecrets is not allowed for this organization"}`, want: true},
		{name: "any other failure status", status: http.StatusTeapot, body: refusalEnvelope, want: true},
		{name: "mixed case", status: http.StatusForbidden, body: `{"message":"ShowSecrets Is Not Allowed For This Organization"}`, want: true},

		{name: "a gateway quoting the request uri", status: http.StatusMethodNotAllowed, body: gatewayEcho, want: false},
		{name: "a success carrying the words", status: http.StatusOK, body: refusalEnvelope, want: false},
		{name: "a created carrying the words", status: http.StatusCreated, body: refusalEnvelope, want: false},
		{name: "a partial carrying the words", status: http.StatusPartialContent, body: refusalEnvelope, want: false},
		{name: "a redirect carrying the words", status: http.StatusFound, body: refusalEnvelope, want: false},
		{name: "an unrelated refusal", status: http.StatusForbidden, body: `{"message":"you do not have access to this workspace"}`, want: false},
		{name: "an empty body", status: http.StatusForbidden, body: "", want: false},
		{name: "without showsecrets", status: http.StatusForbidden, body: `{"message":"this is not allowed for the organization"}`, want: false},
		{name: "without organization", status: http.StatusForbidden, body: `{"message":"showSecrets is not allowed here"}`, want: false},
		{name: "without not allowed", status: http.StatusForbidden, body: `{"message":"showSecrets is refused for this organization"}`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOrgSecretsRefusal(tc.status, []byte(tc.body)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A refusal to resolve secrets cannot be why a request that never asked for
// them failed, so RefusalFor declines to say it was.
func TestRefusalForGatesOnHavingAsked(t *testing.T) {
	if err := RefusalFor(true, http.StatusForbidden, []byte(refusalEnvelope)); !errors.Is(err, ErrOrgSecretsRefused) {
		t.Errorf("asking for secrets and being refused gave %v, want the refusal", err)
	}
	if err := RefusalFor(false, http.StatusForbidden, []byte(refusalEnvelope)); err != nil {
		t.Errorf("not asking for secrets gave %v, want nil: the caller reports the response itself", err)
	}
	if err := RefusalFor(true, http.StatusForbidden, []byte(`{"message":"no access"}`)); err != nil {
		t.Errorf("an unrelated failure gave %v, want nil", err)
	}
}

// reader records what each read asked for and answers from a script.
type reader[T any] struct {
	asked  []bool
	answer func(showSecrets bool) (T, error)
}

func (r *reader[T]) read(_ context.Context, showSecrets bool) (T, error) {
	r.asked = append(r.asked, showSecrets)
	return r.answer(showSecrets)
}

// result stands in for Astro Desktop's own result type: a struct of slices, not a
// slice. The consumers of this module return a struct pointer and a map, so the
// fallback is generic over the whole result rather than over a row type.
type result struct {
	Connections []string
	AirflowVars []string
}

func TestWithSecretsFallbackCarriesAStructResult(t *testing.T) {
	r := &reader[*result]{answer: func(bool) (*result, error) {
		return &result{Connections: []string{"warehouse"}}, nil
	}}

	got, secretsIncluded, err := WithSecretsFallback(context.Background(), true, r.read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !secretsIncluded {
		t.Error("secretsIncluded is false after a read that was not refused")
	}
	if got == nil || len(got.Connections) != 1 {
		t.Errorf("got %+v, want the struct the read returned", got)
	}
}

func TestWithSecretsFallbackCarriesAMapResult(t *testing.T) {
	r := &reader[map[string]string]{answer: func(bool) (map[string]string, error) {
		return map[string]string{"AIRFLOW_VAR_REGION": "us-east-1"}, nil
	}}

	got, _, err := WithSecretsFallback(context.Background(), true, r.read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %v, want the map the read returned", got)
	}
}

func TestWithSecretsFallbackReReadsWithoutSecretsWhenRefused(t *testing.T) {
	r := &reader[map[string]string]{answer: func(showSecrets bool) (map[string]string, error) {
		if showSecrets {
			// Wrapped, because a caller adds its own context to the refusal on
			// the way out and the fallback still has to recognize it.
			return nil, fmt.Errorf("reading the workspace: %w", ErrOrgSecretsRefused)
		}
		return map[string]string{"structural": ""}, nil
	}}

	got, secretsIncluded, err := WithSecretsFallback(context.Background(), true, r.read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretsIncluded {
		t.Error("secretsIncluded is true after the refusal, so a caller cannot tell it holds no secret values")
	}
	if len(r.asked) != 2 || !r.asked[0] || r.asked[1] {
		t.Errorf("reads asked for secrets %v, want [true false]", r.asked)
	}
	if _, ok := got["structural"]; !ok {
		t.Errorf("got %v, want the result of the second read", got)
	}
}

// A read that never asked for secrets cannot be refused for asking, so
// repeating it would only repeat the same request.
func TestWithSecretsFallbackDoesNotReReadWhenItNeverAsked(t *testing.T) {
	r := &reader[map[string]string]{answer: func(bool) (map[string]string, error) {
		return nil, ErrOrgSecretsRefused
	}}

	got, secretsIncluded, err := WithSecretsFallback(context.Background(), false, r.read)
	if !errors.Is(err, ErrOrgSecretsRefused) {
		t.Fatalf("got error %v, want the refusal returned", err)
	}
	if secretsIncluded {
		t.Error("secretsIncluded is true after a failed read")
	}
	if got != nil {
		t.Errorf("got %v alongside the error, want the zero result", got)
	}
	if len(r.asked) != 1 {
		t.Errorf("read %d times, want 1", len(r.asked))
	}
}

// A read that fails hands back the zero result, even when it also returns
// something partial. Half a workspace is not a workspace, and a caller that
// checks the error is not also expected to distrust the value beside it.
func TestWithSecretsFallbackPassesAnyOtherErrorThrough(t *testing.T) {
	wantErr := errors.New("the workspace no longer exists")
	r := &reader[map[string]string]{answer: func(bool) (map[string]string, error) {
		return map[string]string{"read_before_it_failed": ""}, wantErr
	}}

	got, secretsIncluded, err := WithSecretsFallback(context.Background(), true, r.read)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want it returned unchanged", err)
	}
	if secretsIncluded {
		t.Error("secretsIncluded is true after a failed read")
	}
	if got != nil {
		t.Errorf("got %v alongside the error, want the zero result", got)
	}
	if len(r.asked) != 1 {
		t.Errorf("read %d times, want 1 with no fallback", len(r.asked))
	}
}

// When the re-read fails too, the refusal is still the actionable half: without
// it a user is told the workspace is broken and never learns the organization
// declined to resolve secrets.
func TestWithSecretsFallbackKeepsBothCausesWhenTheReReadFails(t *testing.T) {
	secondErr := errors.New("status 500")
	r := &reader[map[string]string]{answer: func(showSecrets bool) (map[string]string, error) {
		if showSecrets {
			return nil, ErrOrgSecretsRefused
		}
		return map[string]string{"read_before_it_failed": ""}, secondErr
	}}

	got, secretsIncluded, err := WithSecretsFallback(context.Background(), true, r.read)
	if !errors.Is(err, ErrOrgSecretsRefused) {
		t.Errorf("got error %v, want the refusal still reachable", err)
	}
	if !errors.Is(err, secondErr) {
		t.Errorf("got error %v, want the second failure still reachable", err)
	}
	if got != nil {
		t.Errorf("got %v alongside the error, want the zero result", got)
	}
	if secretsIncluded {
		t.Error("secretsIncluded is true after both reads failed")
	}
}

func TestWithSecretsFallbackReportsNoSecretsWhenNoneWereAsked(t *testing.T) {
	r := &reader[map[string]string]{answer: func(bool) (map[string]string, error) {
		return map[string]string{"a": "b"}, nil
	}}

	_, secretsIncluded, err := WithSecretsFallback(context.Background(), false, r.read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretsIncluded {
		t.Error("secretsIncluded is true after a read that did not ask for secrets")
	}
}
