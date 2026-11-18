package astrosession

import (
	"errors"
	"testing"
)

// A Workspace named by the flag or the project that is not the one the run's
// Workspace or Deployment token is for is refused, before any request, with
// where each came from. Resolve still reports what won, so a caller that
// reads Workspace without Require checks Disagrees.
func TestResolveRefusesAWorkspaceTheTokenIsNotFor(t *testing.T) {
	const (
		wsFlag    = "ASTRO_API_TOKEN is a Workspace token for ws-claim, but --workspace names ws-flag: drop --workspace, or use a token for ws-flag"
		wsProject = "ASTRO_API_TOKEN is a Workspace token for ws-claim, but the project's link names ws-project: run outside that project, or use a token for ws-project"
		dpFlag    = "ASTRO_API_TOKEN is a Deployment token for Workspace ws-claim, but --workspace names ws-flag: drop --workspace, or use a token for ws-flag"
		dpProject = "ASTRO_API_TOKEN is a Deployment token for Workspace ws-claim, but the project's link names ws-project: run outside that project, or use a token for ws-project"
	)
	tests := []struct {
		name    string
		kind    runKind
		flag    string
		project string
		want    string // "" for no refusal
	}{
		{"flag against a workspace token", wsToken, "ws-flag", "", wsFlag},
		{"project against a workspace token", wsToken, "", "ws-project", wsProject},
		{"flag against a deployment token", depToken, "ws-flag", "", dpFlag},
		{"project against a deployment token", depToken, "", "ws-project", dpProject},
		{"the flag decides, not the project behind it", wsToken, "ws-flag", "ws-claim", wsFlag},
		{"a flag that matches wins over a project that does not", wsToken, "ws-claim", "ws-project", ""},
		{"flag that matches", wsToken, "ws-claim", "", ""},
		{"project that matches", depToken, "", "ws-claim", ""},
		{"no Workspace named", wsToken, "", "", ""},
		{"an organization token has no Workspace to disagree with: flag", orgToken, "ws-flag", "", ""},
		{"an organization token has no Workspace to disagree with: project", orgToken, "", "ws-project", ""},
		{"a saved login has none either", interactive, "ws-flag", "ws-project", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, "ws-saved")
			got := Resolve(Request{Workspace: tt.flag, Project: tt.project})
			err, req := got.Disagrees(), got.Require()
			if tt.want == "" {
				if err != nil || req != nil {
					t.Fatalf("Disagrees() = %v, Require() = %v, want none", err, req)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Disagrees() = %v, want %q", err, tt.want)
			}
			if !errors.Is(req, err) {
				t.Fatalf("Require() = %v, want the same refusal", req)
			}
			var disagree *WorkspaceDisagreeError
			if !errors.As(err, &disagree) || !disagree.IsUsageError() || !disagree.OmitsHelpPointer() {
				t.Fatalf("%v is not a usage error with no help pointer", err)
			}
		})
	}
}

// CheckWorkspace is the same refusal for a reader with no Target: the core
// tree's manifest link, and a Workspace id written in a path.
func TestCheckWorkspace(t *testing.T) {
	useRun(t, wsToken, "")
	if err := CheckWorkspace("", SourceFlag); err != nil {
		t.Fatalf("nothing named: %v", err)
	}
	if err := CheckWorkspace("ws-claim", SourcePath); err != nil {
		t.Fatalf("the token's own: %v", err)
	}
	err := CheckWorkspace("ws-other", SourcePath)
	want := "ASTRO_API_TOKEN is a Workspace token for ws-claim, but the request path names ws-other: use a token for ws-other"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	useRun(t, orgToken, "")
	if err := CheckWorkspace("ws-other", SourcePath); err != nil {
		t.Fatalf("an Organization token: %v", err)
	}
	useRun(t, interactive, "ws-saved")
	if err := CheckWorkspace("ws-other", SourceProject); err != nil {
		t.Fatalf("no token: %v", err)
	}
}
