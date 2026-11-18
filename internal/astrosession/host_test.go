package astrosession

import (
	"errors"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The host of a token run is the command's own domain, else the manifest's,
// else ASTRO_DOMAIN, else astronomer.io, and never the saved context's.
func TestTokenHost(t *testing.T) {
	tests := []struct {
		name        string
		explicit    string
		project     string
		astroDomain string
		want        string
	}{
		{"nothing names one", "", "", "", "astronomer.io"},
		{"ASTRO_DOMAIN alone", "", "", "astronomer-dev.io", "astronomer-dev.io"},
		{"the manifest alone", "", "astronomer-stage.io", "", "astronomer-stage.io"},
		{"a command's own domain alone", "astronomer-test.io", "", "", "astronomer-test.io"},
		{"the manifest over ASTRO_DOMAIN", "", "astronomer-stage.io", "astronomer-dev.io", "astronomer-stage.io"},
		{"a command's own domain over the manifest", "astronomer-test.io", "astronomer-stage.io", "", "astronomer-test.io"},
		{"a command's own domain over everything", "astronomer-test.io", "astronomer-stage.io", "astronomer-dev.io", "astronomer-test.io"},
		{"a URL is normalized as login stores a host", "https://cloud.astronomer-dev.io/", "", "", "astronomer-dev.io"},
		{"the manifest's host is normalized", "", "Cloud.Astronomer-Stage.io", "", "astronomer-stage.io"},
		{"ASTRO_DOMAIN is normalized", "", "", "https://cloud.astronomer-dev.io", "astronomer-dev.io"},
		{"blank values name nothing", "  ", "", "  ", "astronomer.io"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ASTRO_DOMAIN", tt.astroDomain)
			if got := TokenHost(tt.explicit, tt.project); got != tt.want {
				t.Fatalf("TokenHost(%q, %q) with ASTRO_DOMAIN=%q = %q, want %q", tt.explicit, tt.project, tt.astroDomain, got, tt.want)
			}
		})
	}
}

// A machine whose saved current context is dev does not move a token run's
// host: with no manifest domain and no ASTRO_DOMAIN it is astronomer.io.
func TestTokenHostIgnoresTheSavedContext(t *testing.T) {
	writeConfig(t, `context: astronomer-dev.io
contexts:
  astronomer-dev_io:
    domain: astronomer-dev.io
    token: Bearer dev
    workspace: ws-dev
`)
	t.Setenv("ASTRO_DOMAIN", "")
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-claim")
	if got := TokenHost("", ""); got != "astronomer.io" {
		t.Fatalf("host = %q, want astronomer.io whatever the saved context holds", got)
	}
}

// With no saved config at all (CI) and a manifest naming another host, a
// Workspace or Deployment token's claims are the run's selection on that host.
func TestResolveTokenClaimsOnTheManifestsHostWithNoSavedConfig(t *testing.T) {
	tests := []struct {
		name           string
		permissions    []string
		wantWorkspace  string
		wantDeployment string
	}{
		{"workspace token", []string{"organizationId:org", "workspaceId:ws-claim"}, "ws-claim", ""},
		{"deployment token", []string{"organizationId:org", "workspaceId:ws-claim", "deploymentId:dep-claim"}, "ws-claim", "dep-claim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, "")
			t.Setenv("ASTRO_DOMAIN", "")
			testUtil.UseAPIToken(t, tt.permissions...)
			got := Resolve(Request{Domain: "astronomer-dev.io", WantDeployment: true})
			if got.Workspace != tt.wantWorkspace || got.WorkspaceSource != SourceToken {
				t.Errorf("workspace = %q from %v, want %q from the token", got.Workspace, got.WorkspaceSource, tt.wantWorkspace)
			}
			if got.Deployment != tt.wantDeployment {
				t.Errorf("deployment = %q, want %q", got.Deployment, tt.wantDeployment)
			}
			if err := got.Require(); err != nil {
				t.Errorf("Require() = %v", err)
			}
		})
	}
}

// A token run on a dev saved context, naming another host, still has the
// token's claims and never the saved Workspace.
func TestResolveTokenClaimsWithADevSavedContext(t *testing.T) {
	writeConfig(t, `context: astronomer-dev.io
contexts:
  astronomer-dev_io:
    domain: astronomer-dev.io
    token: Bearer dev
    workspace: ws-dev
`)
	t.Setenv("ASTRO_DOMAIN", "")
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-claim")
	for _, domain := range []string{"", "astronomer.io", "astronomer-dev.io", "astronomer-stage.io"} {
		got := Resolve(Request{Domain: domain})
		if got.Workspace != "ws-claim" || got.WorkspaceSource != SourceToken {
			t.Errorf("domain %q: workspace = %q from %v, want the token's", domain, got.Workspace, got.WorkspaceSource)
		}
	}
}

// Missing words the cause for the token's own kind, and never calls a usable
// Workspace or Deployment token an Organization token.
func TestMissingNamesTheTokensKind(t *testing.T) {
	t.Run("organization token", func(t *testing.T) {
		writeConfig(t, "")
		testUtil.UseAPIToken(t, "organizationId:org")
		err := Resolve(Request{Advice: ByFlag}).Missing()
		if !errors.Is(err, ErrOrgTokenNoWorkspace) {
			t.Fatalf("Missing() = %v, want ErrOrgTokenNoWorkspace", err)
		}
		for _, want := range []string{"selects none (an Organization token does not)", "so pass --workspace"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%q lacks %q", err, want)
			}
		}
	})
	t.Run("deployment token with no workspace claim", func(t *testing.T) {
		writeConfig(t, "")
		testUtil.UseAPIToken(t, "organizationId:org", "deploymentId:dep")
		err := Resolve(Request{Advice: ByFlag}).Missing()
		if !errors.Is(err, ErrOrgTokenNoWorkspace) {
			t.Fatalf("Missing() = %v, want ErrOrgTokenNoWorkspace", err)
		}
		if strings.Contains(err.Error(), "Organization token") || !strings.Contains(err.Error(), "a Deployment token with no workspaceId claim") {
			t.Errorf("%q does not name a Deployment token", err)
		}
	})
	t.Run("a token that names a workspace is never missing one", func(t *testing.T) {
		for _, permissions := range [][]string{
			{"organizationId:org", "workspaceId:ws-claim"},
			{"organizationId:org", "workspaceId:ws-claim", "deploymentId:dep"},
		} {
			writeConfig(t, "")
			testUtil.UseAPIToken(t, permissions...)
			for _, domain := range []string{"", "astronomer-dev.io"} {
				if err := Resolve(Request{Domain: domain}).Require(); err != nil {
					t.Errorf("%v on %q: %v", permissions, domain, err)
				}
			}
		}
	})
	t.Run("a hand-built Target for a Workspace token blames nobody's kind", func(t *testing.T) {
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-claim")
		err := (Target{token: true}).Missing()
		if err == nil || strings.Contains(err.Error(), "Organization token") {
			t.Fatalf("Missing() = %v", err)
		}
	})
}
