package astro

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
)

// What `astro workspace create|update|delete|switch`, `astro organization
// switch` and `astro organization role list` print.
//
// The json shapes are pinned once, by the goldens in testdata/schema. These
// tests decode what a run printed and assert what it means; in text they
// assert the messages, in the order a person reads them, and each table cell
// under its header.

const otherOrgID = "org-other"

// lifecycleWorkspaces are the current Organization's Workspaces: Production,
// and Development, the current one (the test config's).
func lifecycleWorkspaces() (prod, dev astrov1.Workspace) {
	desc := "Prod pipelines"
	prod = astrov1.Workspace{Id: "ws-prod", Name: "Production", Description: &desc, CicdEnforcedDefault: true, OrganizationId: "test-org-id"}
	dev = astrov1.Workspace{Id: curWorkspaceID, Name: "Development", OrganizationId: "test-org-id"}
	return prod, dev
}

func listWorkspacesResp(ws ...astrov1.Workspace) *astrov1.ListWorkspacesResponse {
	if ws == nil {
		ws = []astrov1.Workspace{}
	}
	return &astrov1.ListWorkspacesResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.WorkspacesPaginated{Workspaces: ws, TotalCount: len(ws), Limit: 1000},
	}
}

// workspacesMock answers the Workspace list with ws, and whatever more adds.
func workspacesMock(ws []astrov1.Workspace, more ...func(m *astrov1_mocks.ClientWithResponsesInterface)) func(t *testing.T) astrov1.APIClient {
	return func(t *testing.T) astrov1.APIClient {
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listWorkspacesResp(ws...), nil).Maybe()
		for _, f := range more {
			f(m)
		}
		return m
	}
}

func createsWorkspace(made astrov1.Workspace) func(m *astrov1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("CreateWorkspaceWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateWorkspaceResponse{HTTPResponse: ok200(), JSON200: &made}, nil).Once()
	}
}

func updatesWorkspace(id string, after astrov1.Workspace) func(m *astrov1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(&astrov1.UpdateWorkspaceResponse{HTTPResponse: ok200(), JSON200: &after}, nil).Once()
	}
}

func deletesWorkspace(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("DeleteWorkspaceWithResponse", mock.Anything, mock.Anything, id).Return(&astrov1.DeleteWorkspaceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil).Once()
	}
}

// lifecycleOrgs are the Organizations the caller belongs to: the test
// config's current one, and another.
func lifecycleOrgs() (cur, other astrov1.Organization) {
	return astrov1.Organization{Id: "test-org-id", Name: "Test Org"}, astrov1.Organization{Id: otherOrgID, Name: "Other Org"}
}

// orgSwitchMock answers what a switch to another Organization asks: the
// Organizations, the caller, the target by id (the login check reads the
// context's Organization), and the target's Workspaces, ws.
func orgSwitchMock(ws []astrov1.Workspace) func(t *testing.T) astrov1.APIClient {
	cur, other := lifecycleOrgs()
	return workspacesMock(ws, func(m *astrov1_mocks.ClientWithResponsesInterface) {
		orgs := []astrov1.Organization{cur, other}
		m.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&astrov1.ListOrganizationsResponse{
			HTTPResponse: ok200(),
			JSON200:      &astrov1.OrganizationsPaginated{Organizations: orgs, TotalCount: len(orgs), Limit: 100},
		}, nil).Maybe()
		m.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&astrov1.GetSelfUserResponse{HTTPResponse: ok200()}, nil).Maybe()
		m.On("GetOrganizationWithResponse", mock.Anything, otherOrgID, mock.Anything).Return(&astrov1.GetOrganizationResponse{HTTPResponse: ok200(), JSON200: &other}, nil).Maybe()
	})
}

// rolesMock answers the role list: one custom role, and the default ones
// when they are asked for.
func rolesMock(t *testing.T) astrov1.APIClient {
	t.Helper()
	desc, memberDesc := "Reads deployments", "Member of the Organization"
	resp := func(withDefaults bool) *astrov1.ListRolesResponse {
		page := &astrov1.RolesPaginated{
			Roles:      []astrov1.Role{{Id: "role-viewer", Name: "Viewer", Description: &desc, ScopeType: astrov1.RoleScopeTypeDEPLOYMENT}},
			TotalCount: 1,
			Limit:      100,
		}
		if withDefaults {
			page.DefaultRoles = &[]astrov1.DefaultRole{{Name: "ORGANIZATION_MEMBER", Description: &memberDesc, ScopeType: astrov1.DefaultRoleScopeTypeORGANIZATION}}
		}
		return &astrov1.ListRolesResponse{HTTPResponse: ok200(), JSON200: page}
	}
	// The first page, with the default roles when they are asked for; a later
	// one is past the end, and empty, as the API answers it.
	first := func(withDefaults bool) any {
		return mock.MatchedBy(func(p *astrov1.ListRolesParams) bool {
			return *p.Offset == 0 && *p.IncludeDefaultRoles == withDefaults
		})
	}
	later := mock.MatchedBy(func(p *astrov1.ListRolesParams) bool { return *p.Offset > 0 })
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListRolesWithResponse", mock.Anything, mock.Anything, first(true)).Return(resp(true), nil).Maybe()
	m.On("ListRolesWithResponse", mock.Anything, mock.Anything, first(false)).Return(resp(false), nil).Maybe()
	m.On("ListRolesWithResponse", mock.Anything, mock.Anything, later).Return(&astrov1.ListRolesResponse{
		HTTPResponse: ok200(), JSON200: &astrov1.RolesPaginated{Roles: []astrov1.Role{}, TotalCount: 1, Offset: 100, Limit: 100},
	}, nil).Maybe()
	return m
}

// sgr is a color code: a picker highlights the current row with one.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// picks reads a picker's table with its colors taken out, so the highlighted
// current row reads as the others do.
func picks(rows ...map[string]string) func(t *testing.T, out string) {
	return func(t *testing.T, out string) { listsRows("#", rows...)(t, sgr.ReplaceAllString(out, "")) }
}

// contextRow is the context table a Workspace switch prints.
func contextRow(ws string) map[string]string {
	return map[string]string{"CONTROLPLANE": "localhost", "WORKSPACE": ws}
}

func listsContext(ws string) func(t *testing.T, out string) {
	return listsRows("CONTROLPLANE", contextRow(ws))
}

// deleteProdQuestion is the confirmation a delete of Production asks, as
// plain reads it.
const deleteProdQuestion = "\nAre you sure you want to delete the Production Workspace (ws-prod)? This cannot be undone. (y/n) "

// plain runs check on out with its colors taken out: the name in a question
// is bold on a terminal.
func plain(check func(t *testing.T, out string)) func(t *testing.T, out string) {
	return func(t *testing.T, out string) { check(t, sgr.ReplaceAllString(out, "")) }
}

// lacks fails if out holds s.
func lacks(s string) func(t *testing.T, out string) {
	return func(t *testing.T, out string) { assert.NotContains(t, out, s) }
}

// What each command prints in text: the same messages, in the same order, as
// before it gained --output.
func TestWorkspaceOrganizationLifecycleText(t *testing.T) {
	prod, dev := lifecycleWorkspaces()
	both := []astrov1.Workspace{prod, dev}
	staging := astrov1.Workspace{Id: "ws-staging", Name: "Staging"}
	renamed := prod
	renamed.Name = "Prod"
	ws, org := newWorkspaceCmd, newOrganizationCmd
	pickRows := func(t *testing.T, out string) {
		assert := picks(
			map[string]string{"#": "1", "WORKSPACENAME": "Production", "ID": "ws-prod", "CICD ENFORCEMENT": "true"},
			map[string]string{"#": "2", "WORKSPACENAME": "Development", "ID": curWorkspaceID, "CICD ENFORCEMENT": "false"})
		assert(t, out)
	}
	then := func(checks ...func(t *testing.T, out string)) func(t *testing.T, out string) {
		return func(t *testing.T, out string) {
			for _, c := range checks {
				c(t, out)
			}
		}
	}

	runTokenCases(t, []tokenCase{
		{name: "workspace create", root: ws, client: workspacesMock(nil, createsWorkspace(staging)), args: []string{"workspace", "create", "--name", "Staging"}, check: says("Astro Workspace Staging was successfully created\n")},
		{name: "workspace create with no name", root: ws, client: workspacesMock(nil), args: []string{"workspace", "create"}, check: says(""), wantErr: "no name provided for the workspace. Retry with a valid name"},
		{name: "workspace create with a bad --enforce-cicd", root: ws, client: workspacesMock(nil), args: []string{"workspace", "create", "--name", "Staging", "--enforce-cicd", "on"}, check: says(""), wantErr: "the input to the `--enforce-cicd` flag"},
		// The line names the Workspace as it was called before the update.
		{name: "workspace update", root: ws, client: workspacesMock(both, updatesWorkspace("ws-prod", renamed)), args: []string{"workspace", "update", "ws-prod", "--name", "Prod"}, check: says("Astro Workspace Production was successfully updated\n")},
		{
			name: "workspace update through the picker", root: ws, client: workspacesMock(both, updatesWorkspace("ws-prod", renamed)), answers: "1\n",
			args:  []string{"workspace", "update", "--name", "Prod"},
			check: then(pickRows, says("Please select the workspace you would like to update:", "> ", "Astro Workspace Production was successfully updated\n")),
		},
		{
			name: "workspace update of the only Workspace", root: ws, client: workspacesMock([]astrov1.Workspace{prod}, updatesWorkspace("ws-prod", renamed)),
			args: []string{"workspace", "update", "--name", "Prod"},
			check: says("Only one Workspace was found. Using the following Workspace by default: \n",
				"Workspace Name: ", "Production", "Workspace ID: ", "ws-prod",
				"Astro Workspace Production was successfully updated\n"),
		},
		{name: "workspace update of an unknown id", root: ws, client: workspacesMock(both), args: []string{"workspace", "update", "ws-nope", "--name", "Prod"}, check: says(""), wantErr: "no workspace was found for the ID you provided"},
		{name: "workspace update with no Workspaces", root: ws, client: workspacesMock(nil), args: []string{"workspace", "update", "--name", "Prod"}, check: says(""), wantErr: "no workspace was found in your organization"},
		{
			name: "workspace delete, confirmed", root: ws, client: workspacesMock(both, deletesWorkspace("ws-prod")), answers: "y\n",
			args:  []string{"workspace", "delete", "ws-prod"},
			check: plain(says(deleteProdQuestion, "Astro Workspace Production was successfully deleted\n")),
		},
		// Declined, nothing is deleted (the client mocks no delete) and the
		// run exits 0, as a declined Deployment or token delete does.
		{
			name: "workspace delete, declined", root: ws, client: workspacesMock(both), answers: "n\n",
			args:  []string{"workspace", "delete", "ws-prod"},
			check: then(plain(says(deleteProdQuestion, "Canceling Workspace deletion\n")), lacks("successfully deleted")),
		},
		{
			name: "workspace delete --yes", root: ws, client: workspacesMock(both, deletesWorkspace("ws-prod")),
			args:  []string{"workspace", "delete", "ws-prod", "--yes"},
			check: then(says("Astro Workspace Production was successfully deleted\n"), lacks("Are you sure")),
		},
		{
			name: "workspace delete through the picker", root: ws, client: workspacesMock(both, deletesWorkspace(curWorkspaceID)), answers: "2\n",
			args:  []string{"workspace", "delete", "-y"},
			check: then(pickRows, says("Please select the workspace you would like to delete:", "> ", "Astro Workspace Development was successfully deleted\n")),
		},
		// The pick and the confirmation answered from one pipe, as a script
		// answers them: the confirmation gets the "y" the picker read ahead.
		{
			name: "workspace delete through the picker, confirmed", root: ws, client: workspacesMock(both, deletesWorkspace(curWorkspaceID)), answers: "2\ny\n",
			args:  []string{"workspace", "delete"},
			check: then(pickRows, plain(says("Please select the workspace you would like to delete:", "> ", "\nAre you sure you want to delete the Development Workspace ("+curWorkspaceID+")? This cannot be undone. (y/n) ", "Astro Workspace Development was successfully deleted\n"))),
		},
		{
			name: "workspace delete through the picker, declined", root: ws, client: workspacesMock(both), answers: "2\nn\n",
			args:  []string{"workspace", "delete"},
			check: then(pickRows, says("Canceling Workspace deletion\n"), lacks("successfully deleted")),
		},
		// The only Workspace is found, not named, and still asked about.
		{
			name: "workspace delete of the only Workspace", root: ws, client: workspacesMock([]astrov1.Workspace{prod}, deletesWorkspace("ws-prod")), answers: "y\n",
			args: []string{"workspace", "delete"},
			check: plain(says("Only one Workspace was found. Using the following Workspace by default: \n",
				deleteProdQuestion, "Astro Workspace Production was successfully deleted\n")),
		},
		{
			name: "workspace delete of the only Workspace, declined", root: ws, client: workspacesMock([]astrov1.Workspace{prod}), answers: "n\n",
			args:  []string{"workspace", "delete"},
			check: plain(says("Only one Workspace was found.", deleteProdQuestion, "Canceling Workspace deletion\n")),
		},
		{name: "workspace delete of an unknown id", root: ws, client: workspacesMock(both), args: []string{"workspace", "delete", "ws-nope"}, check: says(""), wantErr: "no workspace was found for the ID you provided"},
		{name: "workspace switch by name", root: ws, client: workspacesMock(both), args: []string{"workspace", "switch", "Production"}, check: listsContext("ws-prod")},
		{name: "workspace switch by id", root: ws, client: workspacesMock(both), args: []string{"workspace", "switch", curWorkspaceID}, check: listsContext(curWorkspaceID)},
		{
			name: "workspace switch through the picker", root: ws, client: workspacesMock(both), answers: "1\n",
			args: []string{"workspace", "switch"},
			check: then(
				picks(
					map[string]string{"#": "1", "NAME": "Production", "ID": "ws-prod"},
					map[string]string{"#": "2", "NAME": "Development", "ID": curWorkspaceID}),
				// The table follows the prompt on its line: the answer came
				// from a pipe, so no newline was echoed after it.
				says("\n>  CONTROLPLANE"),
				func(t *testing.T, out string) {
					listsContext("ws-prod")(t, strings.Replace(out, "\n> ", "\n", 1))
				}),
		},
		{name: "workspace switch to an unknown Workspace", root: ws, client: workspacesMock(both), args: []string{"workspace", "switch", "nope"}, check: says(""), wantErr: "workspace id/name could not be found"},
		{name: "organization switch", root: org, client: orgSwitchMock([]astrov1.Workspace{prod}), args: []string{"organization", "switch", "Other Org"}, check: says("\nSuccessfully switched organization\n")},
		{name: "organization switch to the current one", root: org, client: orgSwitchMock(nil), args: []string{"organization", "switch", "Test Org"}, check: says("You selected the same organization as the current one. No switch was made\n")},
		{
			name: "organization switch through the picker", root: org, client: orgSwitchMock([]astrov1.Workspace{prod}), answers: "2\n",
			args: []string{"organization", "switch"},
			check: then(
				picks(
					map[string]string{"#": "1", "NAME": "Test Org", "ID": "test-org-id"},
					map[string]string{"#": "2", "NAME": "Other Org", "ID": otherOrgID}),
				says("Successfully switched organization\n")),
		},
		{
			name: "organization switch with --workspace", root: org, client: orgSwitchMock(both),
			args:  []string{"organization", "switch", "Other Org", "--workspace", "ws-prod"},
			check: then(says("\nSuccessfully switched organization\n", "CONTROLPLANE"), listsContext("ws-prod")),
		},
		{name: "organization switch to an unknown Organization", root: org, client: orgSwitchMock(nil), args: []string{"organization", "switch", "Nope"}, check: says(""), wantErr: "invalid organization name"},
		{
			name: "organization role list", root: org, client: rolesMock,
			args:  []string{"organization", "role", "list"},
			check: listsRows("NAME", map[string]string{"NAME": "Viewer", "ID": "role-viewer", "DESCRIPTION": "Reads deployments"}),
		},
		{
			name: "organization role list --include-default-roles", root: org, client: rolesMock,
			args: []string{"organization", "role", "list", "--include-default-roles"},
			check: listsRows("NAME",
				map[string]string{"NAME": "ORGANIZATION_MEMBER", "ID": "", "DESCRIPTION": "Member of the Organization"},
				map[string]string{"NAME": "Viewer", "ID": "role-viewer", "DESCRIPTION": "Reads deployments"}),
		},
	})
}

// The Organization and the Workspaces as they are published.
func orgJSON(name, id string) map[string]any {
	return map[string]any{"name": name, "id": id, "is_current": true}
}

func wsJSON(name, id string, current bool) map[string]any {
	return map[string]any{"name": name, "id": id, "is_current": current}
}

// What each command publishes under --output json, and that it publishes
// nothing else: stdout is the one object, stderr is empty, the exit is 0.
func TestWorkspaceOrganizationLifecycleJSON(t *testing.T) {
	prod, dev := lifecycleWorkspaces()
	both := []astrov1.Workspace{prod, dev}
	staging := astrov1.Workspace{Id: "ws-staging", Name: "Staging"}
	renamed := prod
	renamed.Name = "Prod"
	renamedDev := dev
	renamedDev.Name = "Dev"
	elsewhere := astrov1.Workspace{Id: "ws-elsewhere", Name: "Elsewhere"}
	ws, org := newWorkspaceCmd, newOrganizationCmd

	runJSONCases(t, []tokenCase{
		{name: "workspace create", root: ws, client: workspacesMock(nil, createsWorkspace(staging)), args: []string{"workspace", "create", "--name", "Staging"}, check: jsonIs(wsJSON("Staging", "ws-staging", false))},
		{name: "workspace update", root: ws, client: workspacesMock(both, updatesWorkspace("ws-prod", renamed)), args: []string{"workspace", "update", "ws-prod", "--name", "Prod"}, check: jsonIs(wsJSON("Prod", "ws-prod", false))},
		{name: "workspace update of the current Workspace", root: ws, client: workspacesMock(both, updatesWorkspace(curWorkspaceID, renamedDev)), args: []string{"workspace", "update", curWorkspaceID, "--name", "Dev"}, check: jsonIs(wsJSON("Dev", curWorkspaceID, true))},
		{name: "workspace delete --yes", root: ws, client: workspacesMock(both, deletesWorkspace("ws-prod")), args: []string{"workspace", "delete", "ws-prod", "--yes"}, check: jsonIs(map[string]any{"workspace_id": "ws-prod", "name": "Production", "action": "deleted"})},
		{name: "workspace switch", root: ws, client: workspacesMock(both), args: []string{"workspace", "switch", "Production"}, check: jsonIs(wsJSON("Production", "ws-prod", true))},
		{
			// The target's one Workspace becomes current on the way.
			name: "organization switch", root: org, client: orgSwitchMock([]astrov1.Workspace{prod}),
			args:  []string{"organization", "switch", "Other Org"},
			check: jsonIs(map[string]any{"organization": orgJSON("Other Org", otherOrgID), "workspace": wsJSON("Production", "ws-prod", true)}),
		},
		{
			// The target has two Workspaces, neither the last one used, so
			// the switch would ask which, and under json it cannot: the
			// Organization switches, and the Workspace the context still
			// names is the old Organization's, so none is current.
			name: "organization switch that leaves no Workspace current", root: org, client: orgSwitchMock([]astrov1.Workspace{prod, elsewhere}),
			args:  []string{"organization", "switch", "Other Org"},
			check: jsonIs(map[string]any{"organization": orgJSON("Other Org", otherOrgID), "workspace": nil}),
		},
		{
			name: "organization switch with --workspace", root: org, client: orgSwitchMock(both),
			args:  []string{"organization", "switch", "Other Org", "--workspace", "ws-prod"},
			check: jsonIs(map[string]any{"organization": orgJSON("Other Org", otherOrgID), "workspace": wsJSON("Production", "ws-prod", true)}),
		},
		{
			name: "organization switch to the current one", root: org, client: orgSwitchMock(both),
			args:  []string{"organization", "switch", "Test Org"},
			check: jsonIs(map[string]any{"organization": orgJSON("Test Org", "test-org-id"), "workspace": wsJSON("Development", curWorkspaceID, true)}),
		},
		{
			name: "organization role list", root: org, client: rolesMock,
			args: []string{"organization", "role", "list"},
			check: jsonIs(map[string]any{"roles": []any{
				map[string]any{"name": "Viewer", "id": "role-viewer", "description": "Reads deployments", "scope_type": "DEPLOYMENT", "is_default": false},
			}}),
		},
		{
			// A default role has no id, and the key is left out.
			name: "organization role list --include-default-roles", root: org, client: rolesMock,
			args: []string{"organization", "role", "list", "--include-default-roles"},
			check: jsonIs(map[string]any{"roles": []any{
				map[string]any{"name": "ORGANIZATION_MEMBER", "description": "Member of the Organization", "scope_type": "ORGANIZATION", "is_default": true},
				map[string]any{"name": "Viewer", "id": "role-viewer", "description": "Reads deployments", "scope_type": "DEPLOYMENT", "is_default": false},
			}}),
		},
	})
}

// An update or a delete naming no Workspace, in an Organization with one,
// uses it and says so. Under json that note goes to stderr, and stdout is the
// one object.
func TestWorkspaceOnlyOneNoteGoesToStderrUnderJSON(t *testing.T) {
	prod, _ := lifecycleWorkspaces()
	renamed := prod
	renamed.Name = "Prod"
	for _, run := range []struct {
		name   string
		client func(t *testing.T) astrov1.APIClient
		args   []string
		want   map[string]any
	}{
		{"update", workspacesMock([]astrov1.Workspace{prod}, updatesWorkspace("ws-prod", renamed)), []string{"workspace", "update", "--name", "Prod", "-o", "json"}, wsJSON("Prod", "ws-prod", false)},
		{"delete", workspacesMock([]astrov1.Workspace{prod}, deletesWorkspace("ws-prod")), []string{"workspace", "delete", "--yes", "-o", "json"}, map[string]any{"workspace_id": "ws-prod", "name": "Production", "action": "deleted"}},
	} {
		t.Run(run.name, func(t *testing.T) {
			r := execAstroCmd(t, run.client(t), "", newWorkspaceCmd, run.args...)
			require.NoError(t, r.err)
			jsonIs(run.want)(t, r.stdout)
			assert.Contains(t, r.stderr, "Only one Workspace was found. Using the following Workspace by default:")
		})
	}
}

// An update sends the CI/CD setting --enforce-cicd gives, and without the
// flag the one the Workspace has: the request has no way to leave it out, and
// a default sent in its place would switch enforcement off on a Workspace
// that had it on. Production has it on, Development off.
func TestWorkspaceUpdateKeepsCICDEnforcementUnlessAsked(t *testing.T) {
	prod, dev := lifecycleWorkspaces()
	both := []astrov1.Workspace{prod, dev}
	for _, run := range []struct {
		name    string
		args    []string
		id      string
		enforce bool
	}{
		{"name only, enforcement on", []string{"workspace", "update", "ws-prod", "--name", "Prod"}, "ws-prod", true},
		{"name only, enforcement off", []string{"workspace", "update", curWorkspaceID, "--name", "Dev"}, curWorkspaceID, false},
		{"--enforce-cicd OFF", []string{"workspace", "update", "ws-prod", "--enforce-cicd", "OFF"}, "ws-prod", false},
		{"-e ON", []string{"workspace", "update", curWorkspaceID, "-e", "ON"}, curWorkspaceID, true},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(run.name+" "+format, func(t *testing.T) {
				var sent *astrov1.UpdateWorkspaceRequest
				client := workspacesMock(both, func(m *astrov1_mocks.ClientWithResponsesInterface) {
					m.On("UpdateWorkspaceWithResponse", mock.Anything, mock.Anything, run.id, mock.Anything).
						Run(func(a mock.Arguments) {
							req := a.Get(3).(astrov1.UpdateWorkspaceRequest) //nolint:forcetypeassert // the mock's own signature
							sent = &req
						}).
						Return(&astrov1.UpdateWorkspaceResponse{HTTPResponse: ok200(), JSON200: &prod}, nil).Once()
				})
				r := execAstroCmd(t, client(t), "", newWorkspaceCmd, append(run.args, "-o", format)...)
				require.NoError(t, r.err)
				require.NotNil(t, sent, "no update was sent")
				assert.Equal(t, run.enforce, sent.CicdEnforcedDefault)
			})
		}
	}
}

// --enforce-cicd given with no value is not a way to leave it unchanged.
func TestWorkspaceUpdateRefusesAnEmptyEnforceCICD(t *testing.T) {
	prod, dev := lifecycleWorkspaces()
	r := execAstroCmd(t, workspacesMock([]astrov1.Workspace{prod, dev})(t), "", newWorkspaceCmd, "workspace", "update", "ws-prod", "--enforce-cicd", "")
	require.ErrorIs(t, r.err, workspace.ErrWrongEnforceInput)
}

// An Organization switch whose --workspace names no Workspace of the new
// Organization fails after the Organization switched. Text says it switched,
// then the error, as it always has; under json the error object is all of
// stdout.
func TestOrganizationSwitchToAnUnknownWorkspace(t *testing.T) {
	prod, _ := lifecycleWorkspaces()
	args := []string{"organization", "switch", "Other Org", "--workspace", "nope"}

	r := execAstroCmd(t, orgSwitchMock([]astrov1.Workspace{prod})(t), "", newOrganizationCmd, args...)
	require.EqualError(t, r.err, "workspace id/name could not be found")
	assert.Equal(t, cliout.ExitFailure, r.code)
	assert.Equal(t, "\nSuccessfully switched organization\n", r.stdout)

	r = execAstroCmd(t, orgSwitchMock([]astrov1.Workspace{prod})(t), "", newOrganizationCmd, append(args, "-o", "json")...)
	require.Error(t, r.err)
	assert.Equal(t, cliout.ExitFailure, r.code)
	var got errorJSON
	decodeOne(t, r.stdout, &got)
	assert.Equal(t, "workspace id/name could not be found", got.Error)
}

// Under --output json a command that would ask which Workspace or
// Organization it means fails as input_required, naming what answers it,
// with that object as the whole of stdout. The clients mock nothing that
// changes a Workspace, so a refused question that went on to act would panic.
func TestWorkspaceOrganizationLifecycleJSONNeverAsks(t *testing.T) {
	prod, dev := lifecycleWorkspaces()
	both := []astrov1.Workspace{prod, dev}
	cases := []struct {
		name     string
		root     func(io.Writer) *cobra.Command
		client   func(t *testing.T) astrov1.APIClient
		args     []string
		answered string
	}{
		{"workspace update naming no Workspace", newWorkspaceCmd, workspacesMock(both), []string{"workspace", "update", "--name", "Prod"}, "pass the workspace ID as an argument"},
		{"workspace delete naming no Workspace", newWorkspaceCmd, workspacesMock(both), []string{"workspace", "delete", "--yes"}, "pass the workspace ID as an argument"},
		// A delete asks before it deletes, and only --yes answers that.
		{"workspace delete without --yes", newWorkspaceCmd, workspacesMock(both), []string{"workspace", "delete", "ws-prod"}, "confirmation to delete the Workspace; with --output json it cannot — pass --yes"},
		{"workspace delete of the only Workspace without --yes", newWorkspaceCmd, workspacesMock([]astrov1.Workspace{prod}), []string{"workspace", "delete"}, "pass --yes"},
		{"workspace switch naming no Workspace", newWorkspaceCmd, workspacesMock(both), []string{"workspace", "switch"}, "pass the workspace name or ID as an argument"},
		{"organization switch naming no Organization", newOrganizationCmd, orgSwitchMock(both), []string{"organization", "switch"}, "pass the organization name or ID as an argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execAstroCmd(t, tc.client(t), "1\n", tc.root, append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, tc.answered)
			assert.Empty(t, r.stderr)
		})
	}
}

// --output takes text or json on each of these commands: anything else is a
// usage error, exit 2, before anything is asked or done.
func TestWorkspaceOrganizationLifecycleOutputUsage(t *testing.T) {
	for _, run := range []struct {
		root func(io.Writer) *cobra.Command
		args []string
	}{
		{newWorkspaceCmd, []string{"workspace", "create", "--name", "Staging", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "update", "ws-prod", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "delete", "ws-prod", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "switch", "ws-prod", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "switch", "Other Org", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "role", "list", "-o", "yaml"}},
	} {
		// A client that answers nothing: a usage error makes no request.
		r := execAstroCmd(t, new(astrov1_mocks.ClientWithResponsesInterface), "", run.root, run.args...)
		require.Error(t, r.err, run.args)
		assert.Equal(t, cliout.ExitUsage, r.code, run.args)
	}
}

// Under json an Organization switch reads back which Workspace it left
// current, after the switch is written. That read failing does not undo the
// switch, so the run still publishes it, with no Workspace known to be
// current, says on stderr why, and exits 0.
func TestOrganizationSwitchJSONWhenTheWorkspaceCannotBeRead(t *testing.T) {
	prod, _ := lifecycleWorkspaces()
	cur, other := lifecycleOrgs()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	orgs := []astrov1.Organization{cur, other}
	m.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&astrov1.ListOrganizationsResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.OrganizationsPaginated{Organizations: orgs, TotalCount: len(orgs), Limit: 100},
	}, nil).Maybe()
	m.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&astrov1.GetSelfUserResponse{HTTPResponse: ok200()}, nil).Maybe()
	m.On("GetOrganizationWithResponse", mock.Anything, otherOrgID, mock.Anything).Return(&astrov1.GetOrganizationResponse{HTTPResponse: ok200(), JSON200: &other}, nil).Maybe()
	// The switch's own list, then the read-back, which fails.
	m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listWorkspacesResp(prod), nil).Once()
	m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("connection reset")).Once()

	r := execAstroCmd(t, m, "", newOrganizationCmd, "organization", "switch", "Other Org", "-o", "json")
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
	jsonIs(map[string]any{"organization": orgJSON("Other Org", otherOrgID), "workspace": nil})(t, r.stdout)
	assert.Equal(t, "The Organization was switched, but the current Workspace could not be read: connection reset\n", r.stderr)
	m.AssertExpectations(t)
}
