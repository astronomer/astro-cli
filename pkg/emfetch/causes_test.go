package emfetch

import (
	"errors"
	"net/http"
	"testing"
)

// Every cause's exact text. docs/workspace-link.md words them and both apps
// report them, so a change here is a change to the contract: these strings are
// written out rather than built, so a reworded template fails here instead of
// agreeing with itself.
func TestCauseText(t *testing.T) {
	const domain, workspace = "astronomer-dev.io", "cmws123"
	cases := []struct {
		cause Cause
		want  string
	}{
		{CauseNotLoggedIn, "not logged in to astronomer-dev.io. Log in with `astro login astronomer-dev.io`"},
		{CauseSessionExpired, "your astronomer-dev.io session expired. Log in again with `astro login astronomer-dev.io`"},
		{CauseNoAccess, "you don't have access to this workspace on astronomer-dev.io. Check your current organization (`astro organization switch`), or ask an org admin"},
		{CauseNotFound, "workspace cmws123 was not found on astronomer-dev.io. Check `workspace` and `domain` in pyproject.toml, and your current organization"},
		{CauseSecretsWithheld, "your org disables Environment Secrets Fetching. Ask an org admin to enable it, or set the value locally"},
		{CauseNoValue, "the workspace holds no value for it"},
		{CauseOffline, "could not reach astronomer-dev.io. Check your connection, or set the value locally"},
		{CauseNoWorkspace, "the manifest sets no `workspace`. Link a workspace to the project in Astro Desktop, or set the value locally"},
	}
	for _, tc := range cases {
		if got := tc.cause.Text(domain, workspace); got != tc.want {
			t.Errorf("Cause(%d).Text:\n got  %q\n want %q", tc.cause, got, tc.want)
		}
	}
}

// A cause no constant names says so rather than returning an empty message,
// which a caller would print as a blank reason.
func TestCauseTextUnknown(t *testing.T) {
	if got, want := Cause(0).Text("astronomer.io", "w"), "unknown cause 0"; got != want {
		t.Errorf("Cause(0).Text = %q, want %q", got, want)
	}
}

func TestStatusTextFor(t *testing.T) {
	const domain, workspace = "astronomer.io", "cmws123"
	platform := errors.New("internal error")
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"401 is the session", http.StatusUnauthorized, "your astronomer.io session expired. Log in again with `astro login astronomer.io`"},
		{"403 is access", http.StatusForbidden, "you don't have access to this workspace on astronomer.io. Check your current organization (`astro organization switch`), or ask an org admin"},
		{"404 is not found", http.StatusNotFound, "workspace cmws123 was not found on astronomer.io. Check `workspace` and `domain` in pyproject.toml, and your current organization"},
		{"any other status carries the platform's error", http.StatusInternalServerError, "astronomer.io returned an error: internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusTextFor(tc.status, Read{Domain: domain, Workspace: workspace}, platform); got != tc.want {
				t.Errorf("StatusTextFor(%d):\n got  %q\n want %q", tc.status, got, tc.want)
			}
		})
	}
	if _, ok := StatusCause(http.StatusBadGateway, Read{}); ok {
		t.Error("StatusCause(502) named a cause; the contract gives it none")
	}
}

// A read under an organization names it where the cause turns on it, and the
// rest read as Text words them: the organization changes no other cause.
func TestCauseTextForNamesTheOrganization(t *testing.T) {
	r := Read{Domain: "astronomer-dev.io", Workspace: "cmws123", Organization: "clorg", OrganizationDeclared: true}
	cases := []struct {
		cause Cause
		want  string
	}{
		{CauseSecretsWithheld, "organization clorg disables Environment Secrets Fetching. Ask an org admin to enable it, or set the value locally"},
		{CauseNoOrganizationAccess, "could not read workspace cmws123 in organization clorg on astronomer-dev.io, which pyproject.toml names. " +
			"Check that you belong to it with `astro organization list`, and `organization` and `workspace` under [tool.astro]"},
		{CauseNotFound, "workspace cmws123 was not found on astronomer-dev.io. Check `workspace` and `domain` in pyproject.toml, and your current organization"},
		{CauseOffline, "could not reach astronomer-dev.io. Check your connection, or set the value locally"},
	}
	for _, tc := range cases {
		if got := tc.cause.TextFor(r); got != tc.want {
			t.Errorf("Cause(%d).TextFor:\n got  %q\n want %q", tc.cause, got, tc.want)
		}
	}
}

// A 403 or 404 under the organization the manifest names is that
// organization's cause; under the login's own it is the plain access or
// not-found cause, and other statuses are unchanged either way.
func TestStatusCauseForADeclaredOrganization(t *testing.T) {
	declared := Read{Domain: "astronomer.io", Workspace: "cmws123", Organization: "clorg", OrganizationDeclared: true}
	fallback := Read{Domain: "astronomer.io", Workspace: "cmws123", Organization: "clorg"}
	for _, tc := range []struct {
		status int
		r      Read
		want   Cause
	}{
		{http.StatusForbidden, declared, CauseNoOrganizationAccess},
		{http.StatusNotFound, declared, CauseNoOrganizationAccess},
		{http.StatusUnauthorized, declared, CauseSessionExpired},
		{http.StatusForbidden, fallback, CauseNoAccess},
		{http.StatusNotFound, fallback, CauseNotFound},
	} {
		if got, ok := StatusCause(tc.status, tc.r); !ok || got != tc.want {
			t.Errorf("StatusCause(%d, declared=%v) = (%d, %v), want %d", tc.status, tc.r.OrganizationDeclared, got, ok, tc.want)
		}
	}
	if _, ok := StatusCause(http.StatusInternalServerError, declared); ok {
		t.Error("StatusCause(500) under a declared organization named a cause")
	}
	want := CauseNoOrganizationAccess.TextFor(declared)
	if got := StatusTextFor(http.StatusForbidden, declared, errors.New("forbidden")); got != want {
		t.Errorf("StatusTextFor(403) = %q, want %q", got, want)
	}
}

func TestWithheld(t *testing.T) {
	cases := []struct {
		name            string
		obj             Object
		secretsIncluded bool
		want            bool
	}{
		// The rule the contract states: "no password" and "password withheld"
		// look the same, so a native connection read without secrets is
		// withheld even when every field it did return is filled in.
		{"native connection read without secrets", Object{Connection: true, Value: `{"conn_type":"postgres","host":"db"}`}, false, true},
		{"native connection read with secrets", Object{Connection: true, Value: `{"conn_type":"postgres"}`}, true, false},
		{"secret variable read without secrets", Object{Secret: true}, false, true},
		// Secrets were asked for and not refused, so a blank secret is one the
		// workspace holds no value for, not one the policy kept back.
		{"blank secret read with secrets", Object{Secret: true}, true, false},
		{"secret variable that came back with a value", Object{Secret: true, Value: "v"}, false, false},
		{"plain variable read without secrets", Object{Value: "v"}, false, false},
		{"blank plain variable read without secrets", Object{}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Withheld(tc.obj, tc.secretsIncluded); got != tc.want {
				t.Errorf("Withheld(%+v, secretsIncluded=%v) = %v, want %v", tc.obj, tc.secretsIncluded, got, tc.want)
			}
		})
	}
}
