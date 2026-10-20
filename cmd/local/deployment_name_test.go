package local

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/util"
)

// -d for a value no link declares reads the way --deployment reads on the
// astro deployment commands: a Deployment id, else a Deployment name, looked
// up in --workspace, else the project's workspace, else the login's.

const (
	unlinkedID     = "ck05r3bor07h40d02y2hw4n4v"
	projectWS      = "ws_abc123" // instancestest.Preamble's [tool.astro] workspace
	loginWS        = "ws-login"
	nameLinkedProd = "clm2xk9dq000108l7a2b3c4d6" // twoLinkManifest's prod
)

// findingLocator is a control plane holding deployments: it finds a name the
// way the production locator does, through instancelocate.NamedDeployment,
// records each lookup, and points every Airflow at url.
type findingLocator struct {
	url         string
	deployments []astrov1.Deployment
	// asked is each name looked up, with the workspace it was looked up in.
	asked *[]string
	// reached is each instance an address was read for.
	reached *[]instances.Instance
}

// BaseURL refuses a Deployment no link declares that is not in the workspace
// it carries, as the production lookup does off the Deployment it reads.
func (l findingLocator) BaseURL(_ context.Context, i instances.Instance) (string, error) {
	*l.reached = append(*l.reached, i)
	for j := range l.deployments {
		d := &l.deployments[j]
		if ws := i.Link.Workspace; d.Id == i.Link.Deployment && i.Source == instances.SourceDeploymentID && ws != "" && d.WorkspaceId != ws {
			return "", errors.New("deployment " + d.Id + " is in workspace " + d.WorkspaceId + ", not --workspace " + ws)
		}
	}
	return l.url, nil
}

func (l findingLocator) DeploymentID(_ context.Context, ref, workspace, projectWorkspace string) (string, error) {
	if util.IsCUID(ref) {
		return ref, nil
	}
	ws := workspace
	if ws == "" {
		ws = projectWorkspace
	}
	if ws == "" {
		ws = loginWS
	}
	*l.asked = append(*l.asked, ref+"@"+ws)
	var in []astrov1.Deployment
	for i := range l.deployments {
		if l.deployments[i].WorkspaceId == ws {
			in = append(in, l.deployments[i])
		}
	}
	dep, err := instancelocate.NamedDeployment(in, ref, ws)
	return dep.Id, err
}

// byNameRun is one `astro af pools list` in a project linking twoLinkManifest,
// against a control plane holding deployments.
type byNameRun struct {
	err     error
	stderr  string
	asked   []string
	reached []instances.Instance
}

func runByName(t *testing.T, manifestBody string, deployments []astrov1.Deployment, args ...string) byNameRun {
	t.Helper()
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/pools", `{"pools":[],"total_entries":0}`)
	dir := instanceProject(t, manifestBody)
	d, _, errOut := queryDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Session = func(context.Context, string) (string, error) { return "Bearer session", nil }
	var run byNameRun
	d.Locator = anyDomain(findingLocator{url: stub.URL, deployments: deployments, asked: &run.asked, reached: &run.reached})
	run.err = execute(t, d, append([]string{"af", "pools", "list"}, args...)...)
	run.stderr = errOut.String()
	return run
}

func astroDep(id, name, ws string) astrov1.Deployment {
	return astrov1.Deployment{Id: id, Name: name, WorkspaceId: ws}
}

// A link name is the link: nothing is looked up by name.
func TestDeploymentFlagLinkIsTheLink(t *testing.T) {
	run := runByName(t, twoLinkManifest, nil, "-d", "prod")
	if run.err != nil {
		t.Fatal(run.err)
	}
	if len(run.asked) != 0 || len(run.reached) != 1 || run.reached[0].Name != "prod" || run.reached[0].Source != instances.SourceManifest {
		t.Fatalf("asked %v, reached %+v", run.asked, run.reached)
	}
}

// A Deployment id is that Deployment, reached by id with no name lookup.
func TestDeploymentFlagIDIsTheDeployment(t *testing.T) {
	run := runByName(t, twoLinkManifest, nil, "-d", unlinkedID)
	if run.err != nil {
		t.Fatal(run.err)
	}
	if len(run.asked) != 0 {
		t.Fatalf("an id was looked up by name: %v", run.asked)
	}
	got := run.reached[0]
	if got.Source != instances.SourceDeploymentID || got.Link.Deployment != unlinkedID || got.Name != unlinkedID || got.Link.Workspace != "" {
		t.Fatalf("reached %+v", got)
	}
}

// An id given with --workspace carries it to the lookup that reads its
// address, which refuses a Deployment in another workspace: the flag is not
// dropped for an id any more than for a name.
func TestDeploymentFlagIDOutsideTheWorkspaceGivenIsRefused(t *testing.T) {
	inProject := []astrov1.Deployment{astroDep(unlinkedID, "etl-prod", projectWS)}
	run := runByName(t, twoLinkManifest, inProject, "-d", unlinkedID, "--workspace", "ws-other")
	want := "deployment " + unlinkedID + " is in workspace " + projectWS + ", not --workspace ws-other"
	if run.err == nil || !strings.HasPrefix(run.err.Error(), want) {
		t.Fatalf("err = %v, want %q", run.err, want)
	}
	if len(run.reached) != 1 || run.reached[0].Link.Workspace != "ws-other" || len(run.asked) != 0 {
		t.Fatalf("reached %+v, asked %v", run.reached, run.asked)
	}

	if run := runByName(t, twoLinkManifest, inProject, "-d", unlinkedID, "--workspace", projectWS); run.err != nil {
		t.Fatalf("in its workspace: %v", run.err)
	}
}

// A name is the Deployment of that name in the project's workspace, not the
// login's, which has one by the same name; it is announced by the name.
func TestDeploymentFlagNameInTheProjectsWorkspace(t *testing.T) {
	run := runByName(t, twoLinkManifest, []astrov1.Deployment{
		astroDep(unlinkedID, "etl-prod", projectWS),
		astroDep("cdifferent000000000000000", "etl-prod", loginWS),
	}, "-d", "etl-prod")
	if run.err != nil {
		t.Fatal(run.err)
	}
	if strings.Join(run.asked, ",") != "etl-prod@"+projectWS {
		t.Fatalf("asked %v", run.asked)
	}
	got := run.reached[0]
	if got.Link.Deployment != unlinkedID || got.Source != instances.SourceDeploymentID || got.Name != "etl-prod" {
		t.Fatalf("reached %+v", got)
	}
	if want := "→ etl-prod (astro deployment " + unlinkedID + ")"; !strings.Contains(run.stderr, want) {
		t.Fatalf("stderr = %q, want %q", run.stderr, want)
	}
}

// --workspace is where a name is looked up, over the project's, and the
// Deployment it finds carries it.
func TestDeploymentFlagNameInTheWorkspaceGiven(t *testing.T) {
	run := runByName(t, twoLinkManifest, []astrov1.Deployment{
		astroDep("cdifferent000000000000000", "etl-prod", projectWS),
		astroDep(unlinkedID, "etl-prod", "ws-given"),
	}, "-d", "etl-prod", "--workspace", "ws-given")
	if run.err != nil {
		t.Fatal(run.err)
	}
	if strings.Join(run.asked, ",") != "etl-prod@ws-given" {
		t.Fatalf("asked %v", run.asked)
	}
	if got := run.reached[0]; got.Link.Deployment != unlinkedID || got.Link.Workspace != "ws-given" {
		t.Fatalf("reached %+v", got)
	}
}

// A name a link's Deployment has is that link, with its name and auth.
func TestDeploymentFlagNameOfALinkedDeploymentIsTheLink(t *testing.T) {
	run := runByName(t, twoLinkManifest, []astrov1.Deployment{astroDep(nameLinkedProd, "orders-prod", projectWS)}, "-d", "orders-prod")
	if run.err != nil {
		t.Fatal(run.err)
	}
	if got := run.reached[0]; got.Name != "prod" || got.Source != instances.SourceManifest {
		t.Fatalf("reached %+v", got)
	}
}

// Inside a project a name that is neither a link nor a Deployment is most
// likely a mistyped link, so the links lead the message.
func TestDeploymentFlagUnknownNameLeadsWithTheLinks(t *testing.T) {
	run := runByName(t, twoLinkManifest, nil, "-d", "prdo")
	want := "no link or Deployment named \"prdo\". Deployments this project links: dev, prod\nno Deployment named \"prdo\" in workspace " + projectWS
	if run.err == nil || run.err.Error() != want {
		t.Fatalf("err = %v, want %q", run.err, want)
	}
	if len(run.reached) != 0 {
		t.Fatalf("reached %+v", run.reached)
	}
}

// A name several Deployments share is refused with their ids, and the links
// follow, as for any lookup that failed.
func TestDeploymentFlagAmbiguousNameListsTheIDs(t *testing.T) {
	run := runByName(t, twoLinkManifest, []astrov1.Deployment{
		astroDep("ctwina0000000000000000000", "twin", projectWS),
		astroDep("ctwinb0000000000000000000", "twin", projectWS),
	}, "-d", "twin")
	want := "2 Deployments are named \"twin\" in workspace " + projectWS + ": pass -d with one's id (ctwina0000000000000000000, ctwinb0000000000000000000)\nDeployments this project links: dev, prod"
	if run.err == nil || run.err.Error() != want {
		t.Fatalf("err = %v, want %q", run.err, want)
	}
}

// --workspace has to agree with a link, which already says where it lives,
// and cannot apply to one in no Astro workspace, or to --url; nor may it be
// empty.
func TestDeploymentFlagWorkspaceAndLinks(t *testing.T) {
	withWorkspaces := twoLinkManifest + "workspace = 'ws-prod'\n" + `
[tool.astro.deployments.staging]
url = 'https://airflow.staging.corp.dev'
auth = { method = 'none' }
`
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-d", "prod", "--workspace", "ws-other"}, "deployment prod links a Deployment in workspace ws-prod (pyproject.toml), not --workspace ws-other"},
		{[]string{"-d", "staging", "--workspace", "ws-other"}, "deployment staging is a url link, which is in no Astro workspace, so --workspace ws-other cannot apply to it"},
		{[]string{"--url", "https://a.dev", "--workspace", "ws-other"}, "--url and --workspace cannot be used together"},
		{[]string{"-d", "prod", "--workspace", ""}, "--workspace cannot be empty"},
	} {
		run := runByName(t, withWorkspaces, nil, tc.args...)
		if run.err == nil || !strings.Contains(run.err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, run.err, tc.want)
		}
		if len(run.reached) != 0 || len(run.asked) != 0 {
			t.Errorf("%v: reached %+v, asked %v", tc.args, run.reached, run.asked)
		}
	}

	// The default link a bare run resolves to is held to it too: dev is in
	// the project's workspace, which it inherits.
	if run := runByName(t, withWorkspaces, nil, "--workspace", "ws-other"); run.err == nil ||
		!strings.Contains(run.err.Error(), "deployment dev links a Deployment in workspace "+projectWS) {
		t.Errorf("bare run: err = %v", run.err)
	}

	// Agreeing with the link it is no obstacle.
	for _, args := range [][]string{
		{"-d", "prod", "--workspace", "ws-prod"},
		{"-d", "dev", "--workspace", projectWS},
		{"--workspace", projectWS},
	} {
		if run := runByName(t, withWorkspaces, nil, args...); run.err != nil {
			t.Errorf("%v: %v", args, run.err)
		}
	}
}

// An apc link is in a workspace of its own (a Houston one), so --workspace
// has to agree with it as with an Astro link, rather than be told the link is
// in none; agreeing, the link goes on to the refusal every apc link meets.
func TestDeploymentFlagWorkspaceAndAnAPCLink(t *testing.T) {
	withAPC := twoLinkManifest + `
[tool.astro.deployments.onprem]
target = 'apc'
domain = 'astro.corp.example'
workspace = 'ws-houston'
deployment = 'capcdeployment00000000001'
`
	run := runByName(t, withAPC, nil, "-d", "onprem", "--workspace", "ws-other")
	want := "deployment onprem links a Deployment in workspace ws-houston (pyproject.toml), not --workspace ws-other"
	if run.err == nil || run.err.Error() != want {
		t.Fatalf("err = %v, want %q", run.err, want)
	}

	run = runByName(t, withAPC, nil, "-d", "onprem", "--workspace", "ws-houston")
	if !errors.Is(run.err, instances.ErrAPCUnsupported) {
		t.Fatalf("agreeing: err = %v, want the apc refusal", run.err)
	}
}

// A fix the command suggests looks the name up in the same workspace.
func TestDeploymentTargetSuggestsTheWorkspace(t *testing.T) {
	tgt := &deploymentTarget{f: deploymentFlags{deployment: "etl-prod", workspace: "ws-given"}}
	if got := tgt.suggest("dags list"); got != "astro af dags list -d etl-prod --workspace ws-given" {
		t.Fatalf("suggest = %q", got)
	}
}

// Without a locator that can find a name, -d reads as it always did.
func TestDeploymentFlagWithNoFinderKeepsTheID(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir)
	d.Locator = anyDomain(locatorFunc(func(context.Context, instances.Instance) (string, error) {
		return "", errors.New("no address")
	}))
	c := &cli{d: d}
	_, err := c.deploymentClient(context.Background(), deploymentFlags{deployment: "etl-prod"})
	if err == nil || !strings.HasPrefix(err.Error(), "no address") || !strings.Contains(err.Error(), "Deployments this project links: dev, prod") {
		t.Fatalf("err = %v", err)
	}
}
