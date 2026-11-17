package astrosession

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// An error about looking in a Workspace names where the run took it from, for
// a source a person can see; the saved login is the run's own and is not named.
func TestExplainNamesWhereTheWorkspaceCameFrom(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		err    string
		want   string
	}{
		{"flag, not found", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, `no Deployment named "etl" in workspace wsB`, `no Deployment named "etl" in workspace wsB (from --workspace)`},
		{"flag, permission", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "you do not have access to workspace wsB: ask an admin", "you do not have access to workspace wsB (from --workspace): ask an admin"},
		{"project link", Target{Workspace: "wsB", WorkspaceSource: SourceProject}, "no Deployment named x in workspace wsB", "no Deployment named x in workspace wsB (from the project link)"},
		{"token claim", Target{Workspace: "wsB", WorkspaceSource: SourceToken}, "no Deployment named x in workspace wsB", "no Deployment named x in workspace wsB (from ASTRO_API_TOKEN)"},
		{"a capital W", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "list failed in Workspace wsB", "list failed in Workspace wsB (from --workspace)"},
		{"no id in the text", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "the Deployment specified was not found in this workspace. Your account may not have access", "the Deployment specified was not found in workspace wsB (from --workspace). Your account may not have access"},
		{"only the first mention", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "workspace wsB, and workspace wsB again", "workspace wsB (from --workspace), and workspace wsB again"},
		{"already explained", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "no Deployment in workspace wsB (from --workspace)", "no Deployment in workspace wsB (from --workspace)"},
		{"the saved login is nobody's surprise", Target{Workspace: "wsB", WorkspaceSource: SourceContext}, "no Deployment in workspace wsB", "no Deployment in workspace wsB"},
		{"no workspace", Target{}, "no Deployment in workspace wsB", "no Deployment in workspace wsB"},
		{"another workspace's text", Target{Workspace: "wsB", WorkspaceSource: SourceFlag}, "no Deployment in workspace wsC", "no Deployment in workspace wsC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.target.Explain(errors.New(tt.err))
			if got.Error() != tt.want {
				t.Fatalf("Explain = %q, want %q", got, tt.want)
			}
		})
	}
	if (Target{Workspace: "wsB", WorkspaceSource: SourceFlag}).Explain(nil) != nil {
		t.Fatal("Explain(nil) is not nil")
	}
}

// The reworded error is the original underneath, so its kind and exit code,
// which are found by what it wraps, do not change.
type statusLike struct{ code int }

func (e *statusLike) Error() string { return fmt.Sprintf("status %d", e.code) }

func TestExplainKeepsTheOriginalErrorUnderneath(t *testing.T) {
	orig := fmt.Errorf("no access to workspace wsB: %w", &statusLike{403})
	got := (Target{Workspace: "wsB", WorkspaceSource: SourceFlag}).Explain(orig)
	if got.Error() == orig.Error() {
		t.Fatal("the error was not reworded")
	}
	var s *statusLike
	if !errors.Is(got, orig) || !errors.As(got, &s) || s.code != 403 {
		t.Fatalf("%v does not unwrap to what it was", got)
	}
}

func TestSourceLabelsAreTheWordsOfTheNotes(t *testing.T) {
	for _, tt := range []struct {
		source Source
		flag   string
		want   string
	}{
		{SourceFlag, "--workspace", "--workspace"},
		{SourceFlag, "--deployment", "--deployment"},
		{SourceProject, "--workspace", "the project link"},
		{SourceToken, "--workspace", "ASTRO_API_TOKEN"},
		{SourcePath, "--workspace", "the request path"},
		{SourceContext, "--workspace", ""},
		{SourceNone, "--workspace", ""},
	} {
		if got := tt.source.Label(tt.flag); got != tt.want {
			t.Errorf("Label(%q) of %v = %q, want %q", tt.flag, tt.source, got, tt.want)
		}
	}
	if got := DescribeWorkspace("wsB", SourceFlag); got != "workspace wsB (from --workspace)" {
		t.Errorf("DescribeWorkspace = %q", got)
	}
	if got := DescribeWorkspace("wsB", SourceContext); got != "workspace wsB" {
		t.Errorf("DescribeWorkspace of the saved login = %q", got)
	}
}

// Every resolution writes one debug line, through the CLI's logger, with the
// Workspace and the Deployment and where each came from, and none above debug.
func TestResolveWritesOneDebugLine(t *testing.T) {
	var buf bytes.Buffer
	prevLevel := logger.GetLevel()
	logger.SetOutput(&buf)
	t.Cleanup(func() {
		logger.SetLevel(prevLevel)
		logger.SetOutput(os.Stderr)
	})

	tests := []struct {
		name string
		kind runKind
		req  Request
		want string
	}{
		{"flags", interactive, Request{Workspace: "ws-flag", Deployment: "dep-flag"}, "workspace ws-flag (--workspace), deployment dep-flag (--deployment)"},
		{"project", interactive, Request{Project: "ws-project"}, "workspace ws-project (the project link), deployment none (no source)"},
		{"token claims", depToken, Request{WantDeployment: true}, "workspace ws-claim (ASTRO_API_TOKEN), deployment dep-claim (ASTRO_API_TOKEN)"},
		{"saved login", interactive, Request{}, "workspace ws-saved (the saved login), deployment none (no source)"},
		{"nothing", orgToken, Request{}, "workspace none (no source), deployment none (no source)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRun(t, tt.kind, "ws-saved")
			logger.SetLevel(logrus.DebugLevel)
			buf.Reset()
			Resolve(tt.req)
			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			if len(lines) != 1 || !strings.Contains(lines[0], tt.want) {
				t.Fatalf("debug log = %q, want one line with %q", buf.String(), tt.want)
			}
			logger.SetLevel(logrus.InfoLevel)
			buf.Reset()
			Resolve(tt.req)
			if buf.Len() != 0 {
				t.Fatalf("a line was logged above debug: %q", buf.String())
			}
		})
	}
}
