package astrosession

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// runKind is what the run authenticates with.
type runKind string

const (
	interactive runKind = "interactive login"
	orgToken    runKind = "organization token"
	wsToken     runKind = "workspace token"
	depToken    runKind = "deployment token"
	legacyKey   runKind = "legacy key"
)

// useRun sets up a run of kind on astronomer.io: the saved context, as the
// run sees it. A token run sees the claim its setup overlaid on the context
// (none for an Organization token), never the saved login's Workspace; a
// legacy key run sees the first Deployment's Workspace.
func useRun(t *testing.T, kind runKind, saved string) {
	t.Helper()
	t.Setenv(EnvAPIToken, "")
	t.Setenv("ASTRONOMER_KEY_ID", "")
	ctxWorkspace := saved
	switch kind {
	case interactive:
		// the saved login's own Workspace
	case orgToken:
		testUtil.UseAPIToken(t, "organizationId:org")
		ctxWorkspace = ""
	case wsToken:
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-claim")
		ctxWorkspace = "ws-claim"
	case depToken:
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-claim", "deploymentId:dep-claim")
		ctxWorkspace = "ws-claim"
	case legacyKey:
		t.Setenv("ASTRONOMER_KEY_ID", "key")
		ctxWorkspace = "ws-first"
	}
	writeConfig(t, fmt.Sprintf(`context: astronomer.io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: Bearer x
    workspace: %q
  astronomer-stage_io:
    domain: astronomer-stage.io
    token: Bearer y
    workspace: ws-stage
`, ctxWorkspace))
}

func TestResolveWorkspacePrecedence(t *testing.T) {
	const saved = "ws-saved"
	tests := []struct {
		name    string
		kind    runKind
		flag    string
		project string
		want    string
		source  Source
	}{
		// each source alone
		{"flag alone", interactive, "ws-flag", "", "ws-flag", SourceFlag},
		{"project alone", interactive, "", "ws-project", "ws-project", SourceProject},
		{"saved login alone", interactive, "", "", saved, SourceContext},
		{"workspace token alone", wsToken, "", "", "ws-claim", SourceToken},
		{"deployment token alone", depToken, "", "", "ws-claim", SourceToken},
		{"legacy key alone", legacyKey, "", "", "ws-first", SourceLegacyKey},
		{"organization token alone selects none", orgToken, "", "", "", SourceNone},

		// each pair, the higher of the two winning
		{"flag over project", interactive, "ws-flag", "ws-project", "ws-flag", SourceFlag},
		{"flag over saved login", interactive, "ws-flag", "", "ws-flag", SourceFlag},
		{"project over saved login", interactive, "", "ws-project", "ws-project", SourceProject},
		{"flag over workspace token", wsToken, "ws-flag", "", "ws-flag", SourceFlag},
		{"project over workspace token", wsToken, "", "ws-project", "ws-project", SourceProject},
		{"flag over deployment token", depToken, "ws-flag", "", "ws-flag", SourceFlag},
		{"project over deployment token", depToken, "", "ws-project", "ws-project", SourceProject},
		{"flag over legacy key", legacyKey, "ws-flag", "", "ws-flag", SourceFlag},
		{"project over legacy key", legacyKey, "", "ws-project", "ws-project", SourceProject},
		{"flag supplies an organization token's", orgToken, "ws-flag", "", "ws-flag", SourceFlag},
		{"project supplies an organization token's", orgToken, "", "ws-project", "ws-project", SourceProject},

		// all of them
		{"flag over project over saved login", interactive, "ws-flag", "ws-project", "ws-flag", SourceFlag},
		{"flag over project over token claim", wsToken, "ws-flag", "ws-project", "ws-flag", SourceFlag},
		{"flag over project over nothing", orgToken, "ws-flag", "ws-project", "ws-flag", SourceFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, saved)
			got := Resolve(Request{Workspace: tt.flag, Project: tt.project})
			if got.Workspace != tt.want || got.WorkspaceSource != tt.source {
				t.Fatalf("got %q from %v, want %q from %v", got.Workspace, got.WorkspaceSource, tt.want, tt.source)
			}
			if got.Found() != (tt.want != "") {
				t.Fatalf("Found() = %v for %q", got.Found(), tt.want)
			}
			if err := got.Require(); (err == nil) != (tt.want != "") {
				t.Fatalf("Require() = %v for %q", err, tt.want)
			}
		})
	}
}

// A saved login with no current Workspace is no source either.
func TestResolveSavedLoginWithNoWorkspace(t *testing.T) {
	useRun(t, interactive, "")
	got := Resolve(Request{})
	if got.Found() || got.WorkspaceSource != SourceNone {
		t.Fatalf("got %q from %v", got.Workspace, got.WorkspaceSource)
	}
}

// A token's claim is the run's selection on the current host alone, and a
// saved login's Workspace on another host is that login's, not this run's.
func TestResolveOnAnotherHost(t *testing.T) {
	tests := []struct {
		name   string
		kind   runKind
		flag   string
		domain string
		want   string
		source Source
	}{
		{"interactive login takes the other host's saved Workspace", interactive, "", "astronomer-stage.io", "ws-stage", SourceContext},
		{"interactive login on its own host", interactive, "", "astronomer.io", "ws-saved", SourceContext},
		{"organization token takes none from another host", orgToken, "", "astronomer-stage.io", "", SourceNone},
		{"workspace token's claim is not another host's", wsToken, "", "astronomer-stage.io", "", SourceNone},
		{"deployment token's claim is not another host's", depToken, "", "astronomer-stage.io", "", SourceNone},
		{"workspace token on its own host", wsToken, "", "astronomer.io", "ws-claim", SourceToken},
		{"the domain is read as the config spells it", wsToken, "", "ASTRONOMER_IO", "ws-claim", SourceToken},
		{"a flag wins on another host", orgToken, "ws-flag", "astronomer-stage.io", "ws-flag", SourceFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, "ws-saved")
			got := Resolve(Request{Workspace: tt.flag, Domain: tt.domain})
			if got.Workspace != tt.want || got.WorkspaceSource != tt.source {
				t.Fatalf("got %q from %v, want %q from %v", got.Workspace, got.WorkspaceSource, tt.want, tt.source)
			}
		})
	}
}

func TestResolveDeployment(t *testing.T) {
	tests := []struct {
		name   string
		kind   runKind
		flag   string
		domain string
		want   string
		source Source
	}{
		{"no token, no flag", interactive, "", "", "", SourceNone},
		{"flag alone", interactive, "dep-flag", "", "dep-flag", SourceFlag},
		{"deployment token alone", depToken, "", "", "dep-claim", SourceToken},
		{"flag over deployment token", depToken, "dep-flag", "", "dep-flag", SourceFlag},
		{"workspace token names none", wsToken, "", "", "", SourceNone},
		{"organization token names none", orgToken, "", "", "", SourceNone},
		{"legacy key names none", legacyKey, "", "", "", SourceNone},
		{"deployment token on its own host", depToken, "", "astronomer.io", "dep-claim", SourceToken},
		{"deployment token is no Deployment of another host", depToken, "", "astronomer-stage.io", "", SourceNone},
		{"flag on another host", depToken, "dep-flag", "astronomer-stage.io", "dep-flag", SourceFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, "ws-saved")
			got := Resolve(Request{Deployment: tt.flag, WantDeployment: true, Domain: tt.domain})
			if got.Deployment != tt.want || got.DeploymentSource != tt.source {
				t.Fatalf("got %q from %v, want %q from %v", got.Deployment, got.DeploymentSource, tt.want, tt.source)
			}
		})
	}
}

func TestTokenDeploymentNeedsExactlyOneDeploymentPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		want        string
	}{
		{"one", []string{"workspaceId:ws", "deploymentId:dep"}, "dep"},
		{"none", []string{"workspaceId:ws"}, ""},
		{"several", []string{"deploymentId:a", "deploymentId:b"}, ""},
		{"the same twice", []string{"deploymentId:a", "deploymentId:a"}, ""},
		{"empty id", []string{"deploymentId:"}, ""},
		{"a permission with no colon is skipped", []string{"admin", "deploymentId:dep"}, "dep"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUtil.UseAPIToken(t, tt.permissions...)
			if got := TokenDeployment(); got != tt.want {
				t.Fatalf("TokenDeployment() = %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("no token", func(t *testing.T) {
		t.Setenv(EnvAPIToken, "")
		if got := TokenDeployment(); got != "" {
			t.Fatalf("TokenDeployment() = %q with no token", got)
		}
	})
	t.Run("a token that is not a token", func(t *testing.T) {
		t.Setenv(EnvAPIToken, "token")
		if got := TokenDeployment(); got != "" {
			t.Fatalf("TokenDeployment() = %q for a malformed token", got)
		}
	})
}

// The error and its advice come from the run and from what the command takes.
func TestMissingWorkspaceIsWordedByWhatTheCommandAccepts(t *testing.T) {
	custom := errors.New("current workspace context not set")
	tests := []struct {
		name    string
		kind    runKind
		accepts Accepts
		noSaved error
		is      error
		suffix  string
	}{
		{"organization token, flag", orgToken, ByFlag, nil, ErrOrgTokenNoWorkspace, ", so pass --workspace"},
		{"organization token, flag and project", orgToken, ByFlag | ByProject, nil, ErrOrgTokenNoWorkspace, ", so pass --workspace, or run in a project linked to one"},
		{"organization token, flag, project and id", orgToken, ByFlag | ByProject | ByID, nil, ErrOrgTokenNoWorkspace, ", so pass --workspace or the token's ID, or run in a project linked to one"},
		{"organization token, current only", orgToken, 0, nil, ErrOrgTokenNoWorkspace, ", so use a Workspace API token"},
		{"organization token ignores the saved login's words", orgToken, ByFlag, custom, ErrOrgTokenNoWorkspace, ", so pass --workspace"},
		{"saved login", interactive, ByFlag, nil, ErrNoWorkspace, ""},
		{"saved login with its own words", interactive, ByFlag | ByProject, custom, custom, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, "")
			got := Resolve(Request{Accepts: tt.accepts, NoSaved: tt.noSaved})
			err := got.Require()
			if !errors.Is(err, tt.is) {
				t.Fatalf("err = %v, want %v", err, tt.is)
			}
			if tt.suffix != "" && !strings.HasSuffix(err.Error(), tt.suffix) {
				t.Fatalf("err = %q, want it to end %q", err, tt.suffix)
			}
			if tt.suffix == "" && err.Error() != tt.is.Error() {
				t.Fatalf("err = %q, want exactly %q", err, tt.is)
			}
			if err2 := got.Missing(); err2.Error() != err.Error() {
				t.Fatalf("Missing() = %q, Require() = %q", err2, err)
			}
		})
	}
}

// A caller handed a login reads its selection, as the run's own.
func TestResolveReadsTheLoginItIsHanded(t *testing.T) {
	useRun(t, interactive, "ws-saved")
	handed := &config.Context{Workspace: "ws-handed"}
	got := Resolve(Request{Login: handed})
	if got.Workspace != "ws-handed" || got.WorkspaceSource != SourceContext {
		t.Fatalf("got %q from %v", got.Workspace, got.WorkspaceSource)
	}
	if got := Resolve(Request{Login: handed, Workspace: "ws-flag"}); got.Workspace != "ws-flag" {
		t.Fatalf("the flag lost to the login: %q", got.Workspace)
	}
	useRun(t, orgToken, "ws-saved")
	if got := Resolve(Request{Login: &config.Context{}}); got.Found() || !errors.Is(got.Require(), ErrOrgTokenNoWorkspace) {
		t.Fatalf("an Organization token's empty login found %q", got.Workspace)
	}
}

// With no login at all there is no Workspace, and a flag needs none.
func TestResolveWithNoLoginAtAll(t *testing.T) {
	t.Setenv(EnvAPIToken, "")
	testUtil.InitTestConfig(testUtil.Initial)
	got := Resolve(Request{})
	if got.Found() || !errors.Is(got.Require(), ErrNoWorkspace) {
		t.Fatalf("found %q, err %v", got.Workspace, got.Require())
	}
	if got := Resolve(Request{Workspace: "ws-flag"}); !got.Found() {
		t.Fatal("a flag needs no login")
	}
	// A command that has always said the login is unreadable still does.
	err := Resolve(Request{LoginError: true}).Require()
	if err == nil || errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("err = %v, want the login's own", err)
	}
}

func TestRequireWorkspaceTakesTheFlagOrTheRunsOwn(t *testing.T) {
	useRun(t, orgToken, "")
	if ws, err := RequireWorkspace("ws-flag"); err != nil || ws != "ws-flag" {
		t.Fatalf("got %q, %v", ws, err)
	}
	if _, err := RequireWorkspace(""); !errors.Is(err, ErrOrgTokenNoWorkspace) || !strings.HasSuffix(err.Error(), ", so pass --workspace") {
		t.Fatalf("err = %v", err)
	}
	useRun(t, wsToken, "")
	if ws, err := RequireWorkspace(""); err != nil || ws != "ws-claim" {
		t.Fatalf("got %q, %v", ws, err)
	}
}

func TestMissingWorkspaceAdviceForAPathNamesThePath(t *testing.T) {
	useRun(t, orgToken, "")
	err := Resolve(Request{Accepts: ByPath}).Require()
	if !errors.Is(err, ErrOrgTokenNoWorkspace) || !strings.HasSuffix(err.Error(), ", so write the id in the path") {
		t.Fatalf("err = %v", err)
	}
}

// A caller that asks for no Deployment gets none from a token, whatever it names.
func TestResolveReadsTheTokenForADeploymentOnlyWhenAsked(t *testing.T) {
	useRun(t, depToken, "ws-saved")
	if got := Resolve(Request{}); got.Deployment != "" || got.DeploymentSource != SourceNone {
		t.Fatalf("got %q from %v without asking", got.Deployment, got.DeploymentSource)
	}
	if got := Resolve(Request{WantDeployment: true}); got.Deployment != "dep-claim" {
		t.Fatalf("got %q when asked", got.Deployment)
	}
}

func TestMissingWorkspaceAdviceForADeploymentNameNamesTheID(t *testing.T) {
	useRun(t, orgToken, "")
	err := Resolve(Request{Accepts: ByDeployment}).Require()
	if !errors.Is(err, ErrOrgTokenNoWorkspace) || !strings.HasSuffix(err.Error(), ", so pass -d with the Deployment id, or run in a project that links its workspace") {
		t.Fatalf("err = %v", err)
	}
}
