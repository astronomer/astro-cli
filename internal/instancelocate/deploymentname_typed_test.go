package instancelocate

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/internal/astrosession"
)

// The refusal is a typed error carrying the Workspace and its source, so a
// caller reads them from the error, not from its words; and the name error of a
// Workspace with no source names none.
func TestDeploymentIDRefusalCarriesTheWorkspaceAndSource(t *testing.T) {
	l := finder(&fakeDeployments{t: t, listStatus: http.StatusForbidden})
	l.workspace = func() (string, astrosession.Source) { return "ws-claim", astrosession.SourceToken }
	_, err := l.DeploymentID(context.Background(), "etl", "", "")
	var wsErr *astrosession.WorkspaceError
	if !errors.As(err, &wsErr) || wsErr.Workspace != "ws-claim" || wsErr.Source != astrosession.SourceToken {
		t.Fatalf("err = %#v, want a WorkspaceError for ws-claim from the token", err)
	}

	got := (&DeploymentNameError{Name: "etl", Workspace: "ws-other"}).Error()
	if want := `no Deployment named "etl" in workspace ws-other`; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}
