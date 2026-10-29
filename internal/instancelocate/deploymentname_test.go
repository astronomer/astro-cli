package instancelocate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const testDeploymentID = "ck05r3bor07h40d02y2hw4n4v"

// fakeDeployments is the control plane holding deployments, in every
// workspace: a listing filters by the one it asks for, as the API does, and
// records it. A lookup by id is a failure here.
type fakeDeployments struct {
	t           *testing.T
	deployments []astrov1.Deployment
	listed      []string
	listStatus  int
	// pageSize, when set, pages the listing; offsets records each page asked.
	pageSize int
	offsets  []int
	// org is the organization a listing must go to, the login's when empty.
	org string
}

func (f *fakeDeployments) GetDeploymentWithResponse(_ context.Context, _, id string, _ ...astrov1.RequestEditorFn) (*astrov1.GetDeploymentResponse, error) {
	f.t.Errorf("looked up %s by id", id)
	return nil, errors.New("unexpected lookup")
}

func (f *fakeDeployments) ListDeploymentsWithResponse(ctx context.Context, org string, params *astrov1.ListDeploymentsParams, editors ...astrov1.RequestEditorFn) (*astrov1.ListDeploymentsResponse, error) {
	wantOrg := f.org
	if wantOrg == "" {
		wantOrg = "cl-org"
	}
	if org != wantOrg {
		f.t.Errorf("listed in organization %q, want %q", org, wantOrg)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example", http.NoBody)
	for _, e := range editors {
		if err := e(ctx, req); err != nil {
			return nil, err
		}
	}
	if got := req.Header.Get("Authorization"); got != "Bearer session" {
		f.t.Errorf("listed with Authorization %q, want the session's bearer", got)
	}
	ws := (*params.WorkspaceIds)[0]
	f.listed = append(f.listed, ws)
	if f.listStatus != 0 {
		return &astrov1.ListDeploymentsResponse{HTTPResponse: &http.Response{StatusCode: f.listStatus}}, nil
	}
	var in []astrov1.Deployment
	for i := range f.deployments {
		if f.deployments[i].WorkspaceId == ws {
			in = append(in, f.deployments[i])
		}
	}
	total := len(in)
	if f.pageSize > 0 {
		// A page starts at the offset asked for and holds at most pageSize,
		// whatever limit was asked, as a server capping its pages does.
		offset := *params.Offset
		f.offsets = append(f.offsets, offset)
		in = in[min(offset, total):min(offset+f.pageSize, total)]
	}
	return &astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: in, TotalCount: total},
	}, nil
}

// finder is a locator over fake whose login's workspace is ws-login.
func finder(fake *fakeDeployments) *locator {
	return &locator{
		deployments:  fake,
		session:      func(context.Context) (string, error) { return "Bearer session", nil },
		organization: func() (string, error) { return "cl-org", nil },
		workspace:    func() string { return "ws-login" },
	}
}

func dep(id, name, ws string) astrov1.Deployment {
	return astrov1.Deployment{Id: id, Name: name, WorkspaceId: ws}
}

// A Deployment id is returned as it is, with nothing called: the lookup that
// reads its address is the one that checks it.
func TestDeploymentIDTakesAnIDWithNoCall(t *testing.T) {
	fake := &fakeDeployments{t: t}
	id, err := finder(fake).DeploymentID(context.Background(), testDeploymentID, "ws-project", "")
	if err != nil || id != testDeploymentID {
		t.Fatalf("id, err = %q, %v", id, err)
	}
	if len(fake.listed) != 0 {
		t.Fatalf("an id listed %v", fake.listed)
	}
}

// A name is looked up in the project's workspace, else the login's, each of
// which holds a Deployment of that name.
func TestDeploymentIDLooksANameUpInTheProjectsWorkspaceElseTheLogins(t *testing.T) {
	everywhere := []astrov1.Deployment{
		dep("dep-project", "etl-prod", "ws-project"),
		dep("dep-login", "etl-prod", "ws-login"),
	}
	for _, tc := range []struct{ project, want, listed string }{
		{"ws-project", "dep-project", "ws-project"},
		{"", "dep-login", "ws-login"},
	} {
		fake := &fakeDeployments{t: t, deployments: everywhere}
		id, err := finder(fake).DeploymentID(context.Background(), "etl-prod", tc.project, "")
		if err != nil || id != tc.want {
			t.Errorf("%+v: id, err = %q, %v", tc, id, err)
		}
		if len(fake.listed) != 1 || fake.listed[0] != tc.listed {
			t.Errorf("%+v: listed %v", tc, fake.listed)
		}
	}
}

func TestDeploymentIDNameMissingOrAmbiguous(t *testing.T) {
	fake := &fakeDeployments{t: t, deployments: []astrov1.Deployment{
		dep("dep-a", "twin", "ws-given"),
		dep("dep-b", "twin", "ws-given"),
		dep("dep-c", "etl-prod", "ws-other"),
	}}
	l := finder(fake)

	_, err := l.DeploymentID(context.Background(), "etl-prod", "ws-given", "")
	var nameErr *DeploymentNameError
	if !errors.As(err, &nameErr) || !nameErr.NotFound() {
		t.Fatalf("err = %v, want a name that found nothing", err)
	}
	if err.Error() != `no Deployment named "etl-prod" in workspace ws-given` {
		t.Errorf("err = %q", err)
	}

	_, err = l.DeploymentID(context.Background(), "twin", "ws-given", "")
	if !errors.As(err, &nameErr) || nameErr.NotFound() {
		t.Fatalf("err = %v, want a name several Deployments share", err)
	}
	if err.Error() != `2 Deployments are named "twin" in workspace ws-given: pass -d with one's id (dep-a, dep-b)` {
		t.Errorf("err = %q", err)
	}
}

// With no workspace anywhere a name has nowhere to be looked up, and nothing
// is called.
func TestDeploymentIDNameWithNoWorkspace(t *testing.T) {
	fake := &fakeDeployments{t: t}
	l := finder(fake)
	l.workspace = func() string { return "" }
	_, err := l.DeploymentID(context.Background(), "etl-prod", "", "")
	var nameErr *DeploymentNameError
	if !errors.As(err, &nameErr) || !nameErr.NotFound() {
		t.Fatalf("err = %v, want a name with nowhere to look", err)
	}
	if !strings.Contains(err.Error(), "pass -d with the Deployment id") || len(fake.listed) != 0 {
		t.Errorf("err = %v, listed %v", err, fake.listed)
	}
}

// The project's workspace is listed under the project's organization when it
// names one; the login's workspace always under the login's.
func TestDeploymentIDListsUnderTheProjectsOrganization(t *testing.T) {
	deps := []astrov1.Deployment{dep("dep-project", "etl-prod", "ws-project"), dep("dep-login", "etl-prod", "ws-login")}
	fake := &fakeDeployments{t: t, deployments: deps, org: "cl-project-org"}
	id, err := finder(fake).DeploymentID(context.Background(), "etl-prod", "ws-project", "cl-project-org")
	if err != nil || id != "dep-project" {
		t.Fatalf("id, err = %q, %v", id, err)
	}
	// With no workspace of its own the project's organization has no say.
	fake = &fakeDeployments{t: t, deployments: deps}
	if id, err := finder(fake).DeploymentID(context.Background(), "etl-prod", "", "cl-project-org"); err != nil || id != "dep-login" {
		t.Fatalf("id, err = %q, %v", id, err)
	}
}

// A listing that ignored its workspace filter cannot answer from another
// workspace: only the Deployments of the one asked for count.
func TestNamedDeploymentCountsOnlyTheWorkspaceAskedFor(t *testing.T) {
	deps := []astrov1.Deployment{dep("dep-elsewhere", "etl-prod", "ws-other"), dep("dep-a", "twin", "ws-given"), dep("dep-b", "twin", "ws-other")}
	_, err := NamedDeployment(deps, "etl-prod", "ws-given")
	var nameErr *DeploymentNameError
	if !errors.As(err, &nameErr) || !nameErr.NotFound() {
		t.Fatalf("err = %v, want none in ws-given", err)
	}
	if got, err := NamedDeployment(deps, "twin", "ws-given"); err != nil || got.Id != "dep-a" {
		t.Fatalf("got, err = %q, %v; want the one twin in ws-given", got.Id, err)
	}
}

// A workspace listed over several pages is read whole: a name on the last
// page is found, and one shared across pages is ambiguous rather than
// settled by the first page.
func TestDeploymentIDReadsEveryPage(t *testing.T) {
	ws := []astrov1.Deployment{
		dep("dep-1", "twin", "ws-given"),
		dep("dep-2", "a", "ws-given"),
		dep("dep-3", "b", "ws-given"),
		dep("dep-4", "twin", "ws-given"),
		dep("dep-5", "last", "ws-given"),
	}
	fake := &fakeDeployments{t: t, deployments: ws, pageSize: 2}
	id, err := finder(fake).DeploymentID(context.Background(), "last", "ws-given", "")
	if err != nil || id != "dep-5" {
		t.Fatalf("id, err = %q, %v", id, err)
	}
	if got := fmt.Sprint(fake.offsets); got != "[0 2 4]" {
		t.Fatalf("pages asked at %s, want [0 2 4]", got)
	}

	fake = &fakeDeployments{t: t, deployments: ws, pageSize: 2}
	_, err = finder(fake).DeploymentID(context.Background(), "twin", "ws-given", "")
	var nameErr *DeploymentNameError
	if !errors.As(err, &nameErr) || fmt.Sprint(nameErr.IDs) != "[dep-1 dep-4]" {
		t.Fatalf("err = %v, want both twins", err)
	}
}

// Logged out, the fix is logging in, which also brings a workspace, so that
// is what a name with no workspace is told.
func TestDeploymentIDLoggedOutSaysLogIn(t *testing.T) {
	l := finder(&fakeDeployments{t: t})
	l.workspace = func() string { return "" }
	l.session = func(context.Context) (string, error) { return "", nil }
	_, err := l.DeploymentID(context.Background(), "etl-prod", "", "")
	if !errors.Is(err, astrosession.ErrLoggedOut) {
		t.Fatalf("err = %v, want logged out", err)
	}
}

// A listing the control plane refuses says so, with the login that fixes it.
func TestDeploymentIDNamesTheListingsOutage(t *testing.T) {
	fake := &fakeDeployments{t: t, listStatus: http.StatusUnauthorized}
	l := finder(fake)
	l.domain = "astronomer-dev.io"
	_, err := l.DeploymentID(context.Background(), "etl-prod", "ws-given", "")
	if err == nil || !strings.Contains(err.Error(), "astro login astronomer-dev.io") || !strings.Contains(err.Error(), "workspace ws-given") {
		t.Fatalf("err = %v", err)
	}
}

// The login's workspace comes from the login for the project's host.
func TestWorkspaceReadsTheDomainsLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prod := config.Context{Domain: "astronomer.io"}
	if err := prod.SetContextKey("workspace", "ws-prod"); err != nil {
		t.Fatal(err)
	}
	if ws := Workspace("astronomer.io"); ws != "ws-prod" {
		t.Fatalf("Workspace = %q, want the astronomer.io login's", ws)
	}
	if ws := Workspace("astronomer-stage.io"); ws != "" {
		t.Fatalf("Workspace = %q for a host with no login", ws)
	}
}
