package astrosession

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// statusLike is a cause whose kind and exit code are found by what wraps it.
type statusLike struct{ code int }

func (e *statusLike) Error() string { return fmt.Sprintf("status %d", e.code) }

// An error about looking in a Workspace is built with the Workspace's id and
// source, so its message names the source a person can see; the saved login
// is the run's own and is not named.
func TestWorkspaceErrorNamesWhereTheWorkspaceCameFrom(t *testing.T) {
	tests := []struct {
		name   string
		source Source
		want   string
	}{
		{"flag", SourceFlag, "no access to workspace wsB (from --workspace): ask an admin"},
		{"project link", SourceProject, "no access to workspace wsB (from the project link): ask an admin"},
		{"token claim", SourceToken, "no access to workspace wsB (from ASTRO_API_TOKEN): ask an admin"},
		{"request path", SourcePath, "no access to workspace wsB (from the request path): ask an admin"},
		{"the saved login is nobody's surprise", SourceContext, "no access to workspace wsB: ask an admin"},
		{"no source", SourceNone, "no access to workspace wsB: ask an admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &WorkspaceError{Workspace: "wsB", Source: tt.source, Before: "no access to ", After: ": ask an admin"}
			if got := err.Error(); got != tt.want {
				t.Fatalf("Error = %q, want %q", got, tt.want)
			}
		})
	}
}

// The error unwraps to its cause, so the kind and exit code, which are found
// by what it wraps, do not depend on the words; errors.As finds the Workspace
// and the source it carries, and a message about another Workspace is not
// labeled with this one's source.
func TestWorkspaceErrorKeepsTheCauseAndCarriesTheSource(t *testing.T) {
	cause := &statusLike{403}
	err := fmt.Errorf("listing: %w", &WorkspaceError{Workspace: "wsB", Source: SourceFlag, Before: "no access to ", Err: cause})
	var s *statusLike
	if !errors.As(err, &s) || s.code != 403 || !errors.Is(err, cause) {
		t.Fatalf("%v does not unwrap to its cause", err)
	}
	var w *WorkspaceError
	if !errors.As(err, &w) || w.Workspace != "wsB" || w.Source != SourceFlag {
		t.Fatalf("errors.As = %+v", w)
	}
	other := &WorkspaceError{Workspace: "wsC", Source: SourceNone, Before: "no access to "}
	if got := other.Error(); got != "no access to workspace wsC" {
		t.Fatalf("another Workspace's error = %q", got)
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
	if got := Annotate("wsB", SourceProject); got != "wsB (from the project link)" {
		t.Errorf("Annotate = %q", got)
	}
	if got := Annotate("wsB", SourceNone); got != "wsB" {
		t.Errorf("Annotate of no source = %q", got)
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
			logger.SetLevel(slog.LevelDebug)
			buf.Reset()
			Resolve(tt.req)
			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			if len(lines) != 1 || !strings.Contains(lines[0], tt.want) {
				t.Fatalf("debug log = %q, want one line with %q", buf.String(), tt.want)
			}
			logger.SetLevel(slog.LevelInfo)
			buf.Reset()
			Resolve(tt.req)
			if buf.Len() != 0 {
				t.Fatalf("a line was logged above debug: %q", buf.String())
			}
		})
	}
}
