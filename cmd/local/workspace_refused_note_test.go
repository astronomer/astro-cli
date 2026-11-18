package local

import (
	"bytes"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The note that the Workspace's values were not read is for a person: one line
// on stderr in text mode, nothing at all under -o json, and nothing when
// nothing was refused.
func TestNoteWorkspaceRefusedIsTextOnly(t *testing.T) {
	const note = "workspace B not read (the API token is for another Workspace): starting without its values. ASTRO_API_TOKEN is a Workspace token for A, but the project's link names B"

	var out, errw bytes.Buffer
	noteWorkspaceRefused(cliout.Renderer{Format: cliout.FormatText, Out: &out, Err: &errw}, note)
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
	if got := errw.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, note) {
		t.Errorf("stderr = %q, want the note on one line", got)
	}

	out.Reset()
	errw.Reset()
	noteWorkspaceRefused(cliout.Renderer{Format: cliout.FormatJSON, Out: &out, Err: &errw}, note)
	if out.Len() != 0 || errw.Len() != 0 {
		t.Errorf("under json: stdout %q, stderr %q, want nothing", out.String(), errw.String())
	}

	noteWorkspaceRefused(cliout.Renderer{Format: cliout.FormatText, Out: &out, Err: &errw}, "")
	if out.Len() != 0 || errw.Len() != 0 {
		t.Errorf("with no refusal: stdout %q, stderr %q, want nothing", out.String(), errw.String())
	}
}
