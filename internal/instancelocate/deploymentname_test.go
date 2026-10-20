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
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const testDeploymentID = "ck05r3bor07h40d02y2hw4n4v"

// fakeDeployments is the control plane holding deployments, in every
// workspace: a listing filters by the one it asks for, as the API does, and
// records it. A lookup by id is a failure here unless byID is set.
type fakeDeployments struct {
	t           *testing.T
	deployments []astrov1.Deployment
	listed      []string
	byID        func(id string) *astrov1.GetDeploymentResponse
	listStatus  int
	// pageSize, when set, pages the listing; offsets records each page asked.
	pageSize int
	offsets  []int
}

func (f *fakeDeployments) GetDeploymentWithResponse(_ context.Context, _, id string, _ ...astrov1.RequestEditorFn) (*astrov1.GetDeploymentResponse, error) {
	if f.byID == nil {
		f.t.Errorf("looked up %s by id", id)
		return nil, errors.New("unexpected lookup")
	}
	return f.byID(id), nil
}

func (f *fakeDeployments) ListDeploymentsWithResponse(ctx context.Context, org string, params *astrov1.ListDeploymentsParams, editors ...astrov1.RequestEditorFn) (*astrov1.ListDeploymentsResponse, error) {
	if org != "cl-org" {
		f.t.Errorf("listed in organization %q", org)
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
	id, err := finder(fake).DeploymentID(context.Background(), testDeploymentID, "ws-given", "ws-project")
	if err != nil || id != testDeploymentID {
		t.Fatalf("id, err = %q, %v", id, err)
	}
	if len(fake.listed) != 0 {
		t.Fatalf("an id listed %v", fake.listed)
	}
}

// A name is looked up in --workspace, else the project's, else the login's,
// each of which holds a Deployment of that name.
func TestDeploymentIDLooksANameUpInTheFirstWorkspaceGiven(t *testing.T) {
	everywhere := []astrov1.Deployment{
		dep("dep-given", "etl-prod", "ws-given"),
		dep("dep-project", "etl-prod", "ws-project"),
		dep("dep-login", "etl-prod", "ws-login"),
	}
	for _, tc := range []struct{ workspace, project, want, listed string }{
		{"ws-given", "ws-project", "dep-given", "ws-given"},
		{"", "ws-project", "dep-project", "ws-project"},
		{"", "", "dep-login", "ws-login"},
	} {
		fake := &fakeDeployments{t: t, deployments: everywhere}
		id, err := finder(fake).DeploymentID(context.Background(), "etl-prod", tc.workspace, tc.project)
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
	if !strings.Contains(err.Error(), "pass --workspace") || len(fake.listed) != 0 {
		t.Errorf("err = %v, listed %v", err, fake.listed)
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

// An id given with --workspace has to be in it, read off the lookup the
// address needs anyway; a link's own workspace is not this check's.
func TestAstroDeploymentIDOutsideTheWorkspaceGivenIsRefused(t *testing.T) {
	fake := &fakeDeployments{t: t, byID: func(id string) *astrov1.GetDeploymentResponse {
		return &astrov1.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.Deployment{Id: id, WorkspaceId: "ws-login", WebServerAirflowApiUrl: "https://a.example"},
		}
	}}
	l := finder(fake)
	given := instances.Instance{
		Name: testDeploymentID, Kind: instances.KindAstro, Source: instances.SourceDeploymentID,
		Link: manifest.Link{Deployment: testDeploymentID, Workspace: "ws-given"},
	}
	_, err := l.BaseURL(context.Background(), given)
	if err == nil || err.Error() != "deployment "+testDeploymentID+" is in workspace ws-login, not --workspace ws-given" {
		t.Fatalf("err = %v", err)
	}

	given.Link.Workspace = "ws-login"
	if url, err := l.BaseURL(context.Background(), given); err != nil || url != "https://a.example" {
		t.Fatalf("in its workspace: url, err = %q, %v", url, err)
	}
	given.Link.Workspace = ""
	if _, err := l.BaseURL(context.Background(), given); err != nil {
		t.Fatalf("with no --workspace: %v", err)
	}

	link := given
	link.Source, link.Link.Workspace = instances.SourceManifest, "ws-elsewhere"
	if _, err := l.BaseURL(context.Background(), link); err != nil {
		t.Fatalf("a link: %v", err)
	}
	if len(fake.listed) != 0 {
		t.Fatalf("a lookup by id listed %v", fake.listed)
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
