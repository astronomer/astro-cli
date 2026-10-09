package httputil

import (
	"context"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/version"
)

// headersFor runs the request editor over a fresh request and hands back what
// it set.
func headersFor(t *testing.T) http.Header {
	t.Helper()
	edit := NewRequestEditorFn(func() (string, string, error) {
		return "token", "https://api.astronomer.test", nil
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/organizations", http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	if err := edit(context.Background(), req); err != nil {
		t.Fatalf("editing the request: %v", err)
	}
	return req.Header
}

// Every request has to name the client that sent it. The version header read a
// link-time variable directly, so a build without the -ldflags sent an empty
// one — and the API cannot tell an old CLI from a new one on an empty header.
func TestClientVersionHeaderIsAlwaysSet(t *testing.T) {
	// A build that skipped the -ldflags, which is what made this reachable.
	prev := version.CurrVersion
	version.CurrVersion = ""
	t.Cleanup(func() { version.CurrVersion = prev })

	// Which client the CLI claims to be is read off the environment, and these
	// tests run under GitHub Actions, which sets GITHUB_ACTIONS itself. So each
	// case states the whole environment and every key is set, empty included —
	// adding to the ambient one passes locally and picks the wrong switch arm
	// in CI.
	keys := []string{"GITHUB_ACTIONS", "DEPLOY_ACTION", "DEPLOY_ACTION_VERSION"}

	for _, tc := range []struct {
		name       string
		env        map[string]string
		identifier string
		wantVer    string // empty means "whatever the CLI reports, but not empty"
	}{
		{
			name:       "the CLI itself",
			identifier: "cli",
		},
		{
			name:       "a github action",
			env:        map[string]string{"GITHUB_ACTIONS": "true"},
			identifier: "github-action",
		},
		{
			name: "the deploy action, which reports its own version",
			env: map[string]string{
				"GITHUB_ACTIONS":        "true",
				"DEPLOY_ACTION":         "true",
				"DEPLOY_ACTION_VERSION": "v1.2.3",
			},
			identifier: "deploy-action",
			wantVer:    "v1.2.3",
		},
		{
			// The gap: the action is in play but exported no version of its
			// own, so this arm sent "" no matter what the CLI knew about itself.
			name: "the deploy action, having exported no version",
			env: map[string]string{
				"GITHUB_ACTIONS": "true",
				"DEPLOY_ACTION":  "true",
			},
			identifier: "deploy-action",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range keys {
				t.Setenv(k, tc.env[k])
			}
			h := headersFor(t)

			if got := h.Get("x-astro-client-identifier"); got != tc.identifier {
				t.Errorf("x-astro-client-identifier = %q, want %q", got, tc.identifier)
			}
			got := h.Get("x-astro-client-version")
			switch {
			case tc.wantVer != "" && got != tc.wantVer:
				t.Errorf("x-astro-client-version = %q, want %q", got, tc.wantVer)
			case tc.wantVer == "" && got == "":
				t.Error("x-astro-client-version is empty, so the API cannot tell what called it")
			}
			if ua := h.Get("User-Agent"); ua == "astro-cli/" {
				t.Errorf("User-Agent = %q, which names no version", ua)
			}
		})
	}
}

func TestNormalizeAPIErrorUnlinkedPRPreview(t *testing.T) {
	forbidden := []byte(`{"message":"oauth_user with id auth0|123 is forbidden"}`)
	for _, tc := range []struct {
		name string
		url  string
		body []byte
		want string
	}{
		{
			name: "a PR preview with no user for the login yet",
			url:  "https://pr41517.api.astronomer-dev.io/v1/organizations",
			body: forbidden,
			want: "you're logged in to PR previews, but pr41517 has no user for you yet. Run astro login pr41517 to create it, then run the command again",
		},
		{
			name: "the same refusal from dev",
			url:  "https://api.astronomer-dev.io/v1/organizations",
			body: forbidden,
			want: "oauth_user with id auth0|123 is forbidden",
		},
		{
			name: "another 403 from a PR preview",
			url:  "https://pr41517.api.astronomer-dev.io/v1/organizations",
			body: []byte(`{"message":"missing permission"}`),
			want: "missing permission",
		},
		{
			name: "another refusal worded is forbidden from a PR preview",
			url:  "https://pr41517.api.astronomer-dev.io/v1/organizations",
			body: []byte(`{"message":"deployment dep-1 is forbidden"}`),
			want: "deployment dep-1 is forbidden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tc.url, http.NoBody)
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			err = NormalizeAPIError(&http.Response{StatusCode: http.StatusForbidden, Request: req}, tc.body)
			if err == nil || err.Error() != tc.want {
				t.Errorf("NormalizeAPIError = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("a response with no request", func(t *testing.T) {
		err := NormalizeAPIError(&http.Response{StatusCode: http.StatusForbidden}, forbidden)
		if err == nil || err.Error() != "oauth_user with id auth0|123 is forbidden" {
			t.Errorf("NormalizeAPIError = %v", err)
		}
	})
}
