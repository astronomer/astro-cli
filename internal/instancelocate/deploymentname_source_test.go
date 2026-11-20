package instancelocate

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/internal/astrosession"
)

// A name looked up in a workspace says where that workspace came from, for the
// token's claim as for the project's link; the saved login's is nobody's
// surprise and is not named. Its kind is unchanged: it is still the name error.
func TestDeploymentIDNameErrorsNameTheWorkspacesSource(t *testing.T) {
	for _, tt := range []struct {
		name    string
		project string
		login   func() (string, astrosession.Source)
		want    string
	}{
		{"the token's claim", "", func() (string, astrosession.Source) { return "ws-claim", astrosession.SourceToken }, `no Deployment named "etl" in workspace ws-claim (from ASTRO_API_TOKEN)`},
		{"the project's link", "ws-link", nil, `no Deployment named "etl" in workspace ws-link (from the project link)`},
		{"the saved login", "", func() (string, astrosession.Source) { return "ws-saved", astrosession.SourceContext }, `no Deployment named "etl" in workspace ws-saved`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := finder(&fakeDeployments{t: t})
			if tt.login != nil {
				l.workspace = tt.login
			}
			_, err := l.DeploymentID(context.Background(), "etl", tt.project, "")
			var nameErr *DeploymentNameError
			if !errors.As(err, &nameErr) || !nameErr.NotFound() {
				t.Fatalf("err = %v, want a name that found nothing", err)
			}
			if err.Error() != tt.want {
				t.Errorf("err = %q, want %q", err, tt.want)
			}
		})
	}
}

// A refusal to list names it too, and keeps the answer's own words after it.
func TestDeploymentIDListRefusalNamesTheWorkspacesSource(t *testing.T) {
	l := finder(&fakeDeployments{t: t, listStatus: http.StatusForbidden})
	l.workspace = func() (string, astrosession.Source) { return "ws-claim", astrosession.SourceToken }
	_, err := l.DeploymentID(context.Background(), "etl", "", "")
	want := "you do not have access to workspace ws-claim (from ASTRO_API_TOKEN); ask a Workspace admin to grant it"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}
