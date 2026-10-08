package apc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// What the access commands publish under -o json, and what they refuse.
// Every run here goes through runAccess, whose root binds no writer: a
// result that left by anything but the tree's out would not be on stdout.

const apiKey = "60f2f4f3fa006e3e135dbe99b1391d84"

// createdSA is a service account as a create answers with it: its key whole.
var createdSA = houston.ServiceAccount{ID: "sa-1", Label: "ci", Category: "default", APIKey: apiKey, Active: true, CreatedAt: "2026-10-01T00:00:00.000Z"}

func TestServiceAccountJSON(t *testing.T) {
	t.Run("create publishes the key, and nothing else anywhere", func(t *testing.T) {
		for _, tc := range []struct {
			name, method string
			args         []string
			ret          any
		}{
			{"deployment", "CreateDeploymentServiceAccount", []string{"deployment", "service-account", "create", "--deployment-id", "dep-1", "--label", "ci"}, &houston.DeploymentServiceAccount{ServiceAccount: createdSA}},
			{"workspace", "CreateWorkspaceServiceAccount", []string{"workspace", "service-account", "create", "--workspace-id", accessWorkspaceID, "--label", "ci"}, &houston.WorkspaceServiceAccount{ServiceAccount: createdSA}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				testUtil.InitTestConfig(testUtil.SoftwarePlatform)
				api := newAccessClient()
				api.On(tc.method, mock.Anything).Return(tc.ret, nil)

				run := runAccess(t, api, "", append(tc.args, "-o", "json")...)
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				var got serviceAccountJSON
				decodeAccess(t, run.stdout, &got)
				assert.Equal(t, serviceAccountJSON{
					ID: "sa-1", Label: sp("ci"), Category: sp("default"), Active: true,
					CreatedAt: sp("2026-10-01T00:00:00.000Z"), APIKey: sp(apiKey),
				}, got, "last_used_at is null: the account has never been used")
				assert.Empty(t, run.stderr)
			})
		}
	})

	t.Run("a failed create publishes the error, and no key", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("CreateDeploymentServiceAccount", mock.Anything).Return(nil, errMockHouston)

		run := runAccess(t, api, "", "deployment", "service-account", "create", "--deployment-id", "dep-1", "--label", "ci", "-o", "json")
		require.Equal(t, 1, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, errMockHouston.Error(), got.Error)
		assert.Empty(t, run.stderr)
	})

	// Houston returns the key whole for ten minutes after the create; a list
	// publishes none, whole or masked.
	t.Run("list publishes no key", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListWorkspaceServiceAccounts", accessWorkspaceID).Return([]houston.ServiceAccount{
			{ID: "sa-1", Label: "ci", APIKey: apiKey, Active: true, LastUsedAt: "2026-10-02T00:00:00.000Z"},
		}, nil)

		run := runAccess(t, api, "", "workspace", "service-account", "list", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got serviceAccountListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []serviceAccountJSON{{ID: "sa-1", Label: sp("ci"), Active: true, LastUsedAt: sp("2026-10-02T00:00:00.000Z")}}, got.ServiceAccounts)
		assert.NotContains(t, run.stdout+run.stderr, apiKey)
		assert.Contains(t, run.stdout, `"api_key":null`)
	})

	t.Run("an empty list is []", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentServiceAccounts", "dep-1").Return([]houston.ServiceAccount{}, nil)

		run := runAccess(t, api, "", "deployment", "service-account", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.JSONEq(t, `{"service_accounts":[]}`, run.stdout)
	})

	t.Run("delete", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: "dep-1", ServiceAccountID: "sa-1"}).Return(&houston.ServiceAccount{ID: "sa-1", Label: "ci"}, nil)

		run := runAccess(t, api, "", "deployment", "service-account", "delete", "sa-1", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentServiceAccountRemovalJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, deploymentServiceAccountRemovalJSON{ID: "sa-1", Label: sp("ci"), DeploymentID: "dep-1", Action: "deleted"}, got)

		api = newAccessClient()
		api.On("DeleteWorkspaceServiceAccount", houston.DeleteServiceAccountRequest{WorkspaceID: accessWorkspaceID, ServiceAccountID: "sa-1"}).Return(&houston.ServiceAccount{ID: "sa-1", Label: "ci"}, nil)
		run = runAccess(t, api, "", "workspace", "service-account", "delete", "sa-1", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var ws workspaceServiceAccountRemovalJSON
		decodeAccess(t, run.stdout, &ws)
		assert.Equal(t, workspaceServiceAccountRemovalJSON{ID: "sa-1", Label: sp("ci"), WorkspaceID: accessWorkspaceID, Action: "deleted"}, ws)
	})
}

func TestDeploymentUserJSON(t *testing.T) {
	bound := func(role string) *houston.RoleBinding {
		return &houston.RoleBinding{Role: role, User: houston.RoleBindingUser{ID: "u-1", Username: "a@b.io"}, Deployment: houston.Deployment{ID: "dep-1"}}
	}

	t.Run("list: the users with a role on the Deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentUsers", houston.ListDeploymentUsersRequest{DeploymentID: "dep-1"}).Return([]houston.DeploymentUser{
			{ID: "u-1", FullName: "Ann", Username: "a@b.io", RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentEditorRole, Deployment: houston.Deployment{ID: "dep-1"}}}},
			{ID: "u-2", Username: "b@b.io", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: accessWorkspaceID}}}},
		}, nil)

		run := runAccess(t, api, "", "deployment", "user", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got userListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []userJSON{{ID: sp("u-1"), Username: sp("a@b.io"), FullName: sp("Ann"), DeploymentRole: sp(houston.DeploymentEditorRole)}}, got.Users)
	})

	t.Run("list: none is [], where the text says so in a sentence", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentUsers", mock.Anything).Return([]houston.DeploymentUser{}, nil)

		run := runAccess(t, api, "", "deployment", "user", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.JSONEq(t, `{"users":[]}`, run.stdout)
	})

	// It used to print the error to stdout as well as returning it.
	t.Run("list: a failure is the error object alone", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentUsers", mock.Anything).Return(nil, errMockHouston)

		run := runAccess(t, api, "", "deployment", "user", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 1, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, errMockHouston.Error(), got.Error)
	})

	t.Run("add and update: the user and the role they now hold", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("AddDeploymentUser", mock.Anything).Return(bound(houston.DeploymentEditorRole), nil)
		api.On("UpdateDeploymentUser", mock.Anything).Return(bound(houston.DeploymentAdminRole), nil)

		run := runAccess(t, api, "", "deployment", "user", "add", "--deployment-id", "dep-1", "--email", "a@b.io", "--role", houston.DeploymentEditorRole, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got userJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, userJSON{ID: sp("u-1"), Username: sp("a@b.io"), DeploymentRole: sp(houston.DeploymentEditorRole)}, got)

		run = runAccess(t, api, "", "deployment", "user", "update", "a@b.io", "--deployment-id", "dep-1", "--role", houston.DeploymentAdminRole, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, sp(houston.DeploymentAdminRole), got.DeploymentRole)
	})

	t.Run("remove: who, as Houston names them, and the role they held", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("DeleteDeploymentUser", houston.DeleteDeploymentUserRequest{DeploymentID: "dep-1", Email: "A@b.io"}).Return(bound(houston.DeploymentViewerRole), nil)

		run := runAccess(t, api, "", "deployment", "user", "remove", "A@b.io", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentUserRemovalJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, deploymentUserRemovalJSON{ID: sp("u-1"), Username: sp("a@b.io"), DeploymentID: "dep-1", Role: sp(houston.DeploymentViewerRole), Action: "removed"}, got)
	})
}

func TestWorkspaceUserJSON(t *testing.T) {
	t.Run("list ignores the interactive setting: the whole list is the result", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		require.NoError(t, config.CFG.Interactive.SetHomeString("true"))
		api := newAccessClient()
		api.On("ListWorkspaceUserAndRoles", accessWorkspaceID).Return([]houston.WorkspaceUserRoleBindings{
			{ID: "u-1", Username: "a@b.io", FullName: "Ann", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceEditorRole, Workspace: houston.Workspace{ID: accessWorkspaceID}}}},
		}, nil)

		run := runAccess(t, api, "q\n", "workspace", "user", "list", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got userListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []userJSON{{ID: sp("u-1"), Username: sp("a@b.io"), FullName: sp("Ann"), WorkspaceRole: sp(houston.WorkspaceEditorRole)}}, got.Users)
		assert.False(t, run.stdinRead, "it asked which page to show")
		api.AssertNotCalled(t, "ListWorkspacePaginatedUserAndRoles", mock.Anything)
	})

	t.Run("--paginated is refused under json, as a usage error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()

		run := runAccess(t, api, "q\n", "workspace", "user", "list", "--workspace-id", accessWorkspaceID, "--paginated", "-o", "json")
		require.Equal(t, 2, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, "usage", got.Kind)
		assert.Contains(t, got.Error, "--paginated")
		assert.False(t, run.stdinRead)
	})

	t.Run("add names the user Houston added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("AddWorkspaceUser", houston.AddWorkspaceUserRequest{WorkspaceID: accessWorkspaceID, Email: "A@b.io", Role: houston.WorkspaceEditorRole}).
			Return(&houston.Workspace{ID: accessWorkspaceID, Label: "airflow", Users: []houston.User{{ID: "u-1", Username: "a@b.io"}}}, nil)

		run := runAccess(t, api, "", "workspace", "user", "add", "--workspace-id", accessWorkspaceID, "--email", "A@b.io", "--role", houston.WorkspaceEditorRole, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got userJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, userJSON{ID: sp("u-1"), Username: sp("a@b.io"), WorkspaceRole: sp(houston.WorkspaceEditorRole)}, got)
	})

	t.Run("update: the user and the role they now hold", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceUserRole", mock.Anything).Return(houston.WorkspaceUserRoleBindings{ID: "u-1", Username: "a@b.io", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: accessWorkspaceID}}}}, nil)
		api.On("UpdateWorkspaceUserRole", mock.Anything).Return(houston.WorkspaceAdminRole, nil)

		run := runAccess(t, api, "", "workspace", "user", "update", "a@b.io", "--workspace-id", accessWorkspaceID, "--role", houston.WorkspaceAdminRole, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got userJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, userJSON{ID: sp("u-1"), Username: sp("a@b.io"), WorkspaceRole: sp(houston.WorkspaceAdminRole)}, got)
	})

	t.Run("remove", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceUserRole", mock.Anything).Return(houston.WorkspaceUserRoleBindings{ID: "u-1", Username: "a@b.io", RoleBindings: []houston.RoleBinding{{Role: houston.WorkspaceViewerRole, Workspace: houston.Workspace{ID: accessWorkspaceID}}}}, nil)
		api.On("DeleteWorkspaceUser", houston.DeleteWorkspaceUserRequest{WorkspaceID: accessWorkspaceID, UserID: "u-1"}).Return(&houston.Workspace{ID: accessWorkspaceID, Label: "airflow"}, nil)

		run := runAccess(t, api, "", "workspace", "user", "remove", "a@b.io", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got workspaceUserRemovalJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, workspaceUserRemovalJSON{ID: "u-1", Username: sp("a@b.io"), WorkspaceID: accessWorkspaceID, Action: "removed"}, got)
	})

	// workspaceUser answers for any active user with the email, with no
	// bindings when they hold none in the Workspace: that user is refused
	// before anything is sent, and an unknown email never sends an empty user
	// ID.
	t.Run("remove: a user with no role on the Workspace is refused", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceUserRole", mock.Anything).Return(houston.WorkspaceUserRoleBindings{ID: "u-1", Username: "a@b.io"}, nil)

		run := runAccess(t, api, "", "workspace", "user", "remove", "a@b.io", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 1, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Contains(t, got.Error, "not part of this workspace")
		api.AssertNotCalled(t, "DeleteWorkspaceUser", mock.Anything)
	})
}

func TestUserCreateJSON(t *testing.T) {
	t.Run("the user, and not the session token Houston returns with it", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("CreateUser", houston.CreateUserRequest{Email: "a@b.io", Password: "pw"}).
			Return(&houston.AuthUser{User: houston.User{ID: "u-1", Username: "a@b.io", Status: "pending"}, Token: houston.Token{Value: "session-token"}}, nil)

		run := runAccess(t, api, "", "user", "create", "--email", "a@b.io", "--password", "pw", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got createdUserJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, createdUserJSON{ID: sp("u-1"), Username: sp("a@b.io"), Status: sp("pending")}, got)
		assert.NotContains(t, run.stdout+run.stderr, "session-token")
	})

	t.Run("the username is Houston's, null when it gave none", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("CreateUser", mock.Anything).Return(&houston.AuthUser{User: houston.User{ID: "u-1", Status: "active"}}, nil)

		run := runAccess(t, api, "", "user", "create", "--email", "a@b.io", "--password", "pw", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stdout, `"username":null`)
	})

	t.Run("an email it would ask for is refused, naming the flag", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()

		run := runAccess(t, api, "a@b.io\n", "user", "create", "--password", "pw", "-o", "json")
		require.Equal(t, 1, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, "input_required", got.Kind)
		assert.Contains(t, got.Error, "--email")
		assert.False(t, run.stdinRead)
		api.AssertNotCalled(t, "CreateUser", mock.Anything)
	})
}

func TestTeamJSON(t *testing.T) {
	team := houston.Team{ID: "team-1", Name: "Data", CreatedAt: "2026-01-01T00:00:00.000Z", RoleBindings: []houston.RoleBinding{
		{Role: houston.SystemEditorRole},
		{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: accessWorkspaceID, Label: "airflow"}},
		{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "dep-1", Label: "prod"}},
	}}

	t.Run("get: the team and its roles; users only when asked for", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetTeam", "team-1").Return(&team, nil)

		run := runAccess(t, api, "", "team", "get", "team-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got teamDetailJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, teamDetailJSON{
			ID: "team-1", Name: sp("Data"), SystemRole: houston.SystemEditorRole, CreatedAt: sp("2026-01-01T00:00:00.000Z"),
			WorkspaceRoles:  []teamWorkspaceRoleJSON{{WorkspaceID: accessWorkspaceID, Label: sp("airflow"), Role: houston.WorkspaceAdminRole}},
			DeploymentRoles: []teamDeploymentRoleJSON{{DeploymentID: "dep-1", Label: sp("prod"), Role: houston.DeploymentViewerRole}},
		}, got)
		assert.Contains(t, run.stdout, `"users":null`)
		api.AssertNotCalled(t, "GetTeamUsers", mock.Anything)

		api.On("GetTeamUsers", "team-1").Return([]houston.User{{ID: "u-1", Username: "a@b.io"}}, nil)
		run = runAccess(t, api, "", "team", "get", "team-1", "--all", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []teamUserJSON{{ID: "u-1", Username: sp("a@b.io")}}, got.Users)
	})

	// The text printed the team, and then the heading of the users, before
	// asking for them, so a failure left half a team behind it.
	t.Run("get: a failure fetching the users prints nothing of the team", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetTeam", "team-1").Return(&team, nil)
		api.On("GetTeamUsers", "team-1").Return(nil, errMockHouston)

		run := runAccess(t, api, "", "team", "get", "team-1", "--users")
		require.Equal(t, 1, run.code)
		assert.Empty(t, run.stdout)
	})

	t.Run("list ignores the interactive setting and reads every team", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		require.NoError(t, config.CFG.Interactive.SetHomeString("true"))
		api := newAccessClient()
		api.On("ListTeams", houston.ListTeamsRequest{Take: 20}).Return(houston.ListTeamsResp{Count: 2, Teams: []houston.Team{team, {ID: "team-2", Name: "Ops"}}}, nil)

		run := runAccess(t, api, "q\n", "team", "list", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got teamListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []teamJSON{
			{ID: "team-1", Name: sp("Data"), SystemRole: sp(houston.SystemEditorRole)},
			{ID: "team-2", Name: sp("Ops"), SystemRole: sp(houston.NoneRole)},
		}, got.Teams)
		assert.False(t, run.stdinRead, "it asked which page to show")
	})

	t.Run("list --paginated is refused under json, as a usage error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		run := runAccess(t, newAccessClient(), "q\n", "team", "list", "--paginated", "-o", "json")
		require.Equal(t, 2, run.code)
		var got accessError
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, "usage", got.Kind)
	})

	t.Run("update: the system role the team now holds", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetTeam", "team-1").Return(&team, nil)
		api.On("DeleteTeamSystemRoleBinding", houston.SystemRoleBindingRequest{TeamID: "team-1", Role: houston.SystemEditorRole}).Return(houston.SystemEditorRole, nil)

		run := runAccess(t, api, "", "team", "update", "team-1", "--role", houston.NoneRole, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got teamJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, teamJSON{ID: "team-1", SystemRole: sp(houston.NoneRole)}, got)
	})
}

func TestWorkspaceAndDeploymentTeamJSON(t *testing.T) {
	data := houston.Team{ID: "team-1", Name: "Data", RoleBindings: []houston.RoleBinding{
		{Role: houston.WorkspaceAdminRole, Workspace: houston.Workspace{ID: accessWorkspaceID}},
		{Role: houston.DeploymentEditorRole, Deployment: houston.Deployment{ID: "dep-1"}},
	}}

	t.Run("workspace list", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListWorkspaceTeamsAndRoles", accessWorkspaceID).Return([]houston.Team{data}, nil)

		run := runAccess(t, api, "", "workspace", "team", "list", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got teamListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []teamJSON{{ID: "team-1", Name: sp("Data"), WorkspaceRole: sp(houston.WorkspaceAdminRole)}}, got.Teams)
	})

	t.Run("deployment list", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentTeamsAndRoles", "dep-1").Return([]houston.Team{data}, nil)

		run := runAccess(t, api, "", "deployment", "team", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got teamListJSON
		decodeAccess(t, run.stdout, &got)
		assert.Equal(t, []teamJSON{{ID: "team-1", Name: sp("Data"), DeploymentRole: sp(houston.DeploymentEditorRole)}}, got.Teams)
	})

	// Houston answers [] for a Deployment with no team and for one that does
	// not exist alike, so the text's refusal never told them apart; a list is
	// [] when empty.
	t.Run("deployment list: none is [] under json, and still refused in text", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("ListDeploymentTeamsAndRoles", "dep-1").Return([]houston.Team{}, nil)

		run := runAccess(t, api, "", "deployment", "team", "list", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.JSONEq(t, `{"teams":[]}`, run.stdout)

		run = runAccess(t, api, "", "deployment", "team", "list", "--deployment-id", "dep-1")
		assert.Equal(t, 1, run.code)
	})

	t.Run("adds and updates: the team and the role it now holds", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("AddDeploymentTeam", mock.Anything).Return(&houston.RoleBinding{Role: houston.DeploymentEditorRole}, nil)
		api.On("UpdateDeploymentTeamRole", mock.Anything).Return(&houston.RoleBinding{Role: houston.DeploymentAdminRole}, nil)
		api.On("AddWorkspaceTeam", mock.Anything).Return(&houston.Workspace{ID: accessWorkspaceID, Label: "airflow"}, nil)
		api.On("GetWorkspaceTeamRole", mock.Anything).Return(&data, nil)
		api.On("UpdateWorkspaceTeamRole", mock.Anything).Return(houston.WorkspaceEditorRole, nil)

		for _, tc := range []struct {
			args []string
			want teamJSON
		}{
			{[]string{"deployment", "team", "add", "--deployment-id", "dep-1", "--team-id", "team-1", "--role", houston.DeploymentEditorRole}, teamJSON{ID: "team-1", DeploymentRole: sp(houston.DeploymentEditorRole)}},
			{[]string{"deployment", "team", "update", "team-1", "--deployment-id", "dep-1", "--role", houston.DeploymentAdminRole}, teamJSON{ID: "team-1", DeploymentRole: sp(houston.DeploymentAdminRole)}},
			{[]string{"workspace", "team", "add", "--workspace-id", accessWorkspaceID, "--team-id", "team-1", "--role", houston.WorkspaceViewerRole}, teamJSON{ID: "team-1", WorkspaceRole: sp(houston.WorkspaceViewerRole)}},
			{[]string{"workspace", "team", "update", "team-1", "--workspace-id", accessWorkspaceID, "--role", houston.WorkspaceEditorRole}, teamJSON{ID: "team-1", Name: sp("Data"), WorkspaceRole: sp(houston.WorkspaceEditorRole)}},
		} {
			run := runAccess(t, api, "", append(tc.args, "-o", "json")...)
			require.Equal(t, 0, run.code, "%v stderr:\n%s", tc.args, run.stderr)
			var got teamJSON
			decodeAccess(t, run.stdout, &got)
			assert.Equal(t, tc.want, got, "%v", tc.args)
		}
	})

	t.Run("removes", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("RemoveDeploymentTeam", mock.Anything).Return(&houston.RoleBinding{Role: houston.DeploymentEditorRole}, nil)
		api.On("GetWorkspaceTeamRole", houston.GetWorkspaceTeamRoleRequest{WorkspaceID: accessWorkspaceID, TeamID: "team-1"}).Return(&data, nil)
		api.On("DeleteWorkspaceTeam", mock.Anything).Return(&houston.Workspace{ID: accessWorkspaceID, Label: "airflow"}, nil)

		run := runAccess(t, api, "", "deployment", "team", "remove", "team-1", "--deployment-id", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var dep deploymentTeamRemovalJSON
		decodeAccess(t, run.stdout, &dep)
		assert.Equal(t, deploymentTeamRemovalJSON{ID: "team-1", DeploymentID: "dep-1", Action: "removed"}, dep)

		run = runAccess(t, api, "", "workspace", "team", "remove", "team-1", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var ws workspaceTeamRemovalJSON
		decodeAccess(t, run.stdout, &ws)
		assert.Equal(t, workspaceTeamRemovalJSON{ID: "team-1", WorkspaceID: accessWorkspaceID, Action: "removed", MembershipVerified: true}, ws)
		assert.Empty(t, run.stderr)
	})

	// Houston removes nothing for a team with no binding in the Workspace
	// and reports no error: the command said it had removed it, and exited 0.
	// A team whose only binding there is on a Deployment, or that holds only a
	// custom role assignment, holds no Workspace role, and is refused the same
	// way.
	t.Run("workspace remove of a team with no Workspace role fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceTeamRole", mock.Anything).Return(&houston.Team{ID: "team-1", RoleBindings: []houston.RoleBinding{
			{Role: houston.DeploymentEditorRole, Workspace: houston.Workspace{ID: accessWorkspaceID}, Deployment: houston.Deployment{ID: "dep-1"}},
		}}, nil)

		for _, format := range [][]string{nil, {"-o", "json"}} {
			run := runAccess(t, api, "", append([]string{"workspace", "team", "remove", "team-1", "--workspace-id", accessWorkspaceID}, format...)...)
			assert.Equal(t, 1, run.code, "%v", format)
			assert.NotContains(t, run.stdout, "Successfully removed")
		}
		api.AssertNotCalled(t, "DeleteWorkspaceTeam", mock.Anything)
	})

	// Looking the team up needs workspace.teams.get, which removing it does
	// not, and Houston refuses the lookup the same way for a team with no
	// binding in the Workspace. So the removal is sent anyway, and the result
	// says it was not verified.
	t.Run("workspace remove when the lookup is refused sends the removal, unverified", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceTeamRole", mock.Anything).Return(nil, errors.New("Insufficient permissions."))
		api.On("DeleteWorkspaceTeam", houston.DeleteWorkspaceTeamRequest{WorkspaceID: accessWorkspaceID, TeamID: "team-1"}).Return(&houston.Workspace{ID: accessWorkspaceID, Label: "airflow"}, nil)

		run := runAccess(t, api, "", "workspace", "team", "remove", "team-1", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got workspaceTeamRemovalJSON
		decodeAccess(t, run.stdout, &got)
		assert.False(t, got.MembershipVerified)
		assert.Contains(t, run.stderr, "Could not confirm that team team-1")

		run = runAccess(t, api, "", "workspace", "team", "remove", "team-1", "--workspace-id", accessWorkspaceID)
		require.Equal(t, 0, run.code)
		assert.Contains(t, run.stdout, "Could not confirm that team team-1")
		assert.Contains(t, run.stdout, "Successfully removed team from workspace")
	})

	t.Run("workspace remove when the lookup fails otherwise sends nothing", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		api := newAccessClient()
		api.On("GetWorkspaceTeamRole", mock.Anything).Return(nil, errMockHouston)

		run := runAccess(t, api, "", "workspace", "team", "remove", "team-1", "--workspace-id", accessWorkspaceID, "-o", "json")
		require.Equal(t, 1, run.code)
		api.AssertNotCalled(t, "DeleteWorkspaceTeam", mock.Anything)
	})
}

func TestAccessOutputRefusesAnUnknownFormat(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := newAccessClient()

	run := runAccess(t, api, "", "workspace", "team", "list", "--workspace-id", accessWorkspaceID, "-o", "yaml")
	assert.Equal(t, 2, run.code)
	api.AssertNotCalled(t, "ListWorkspaceTeamsAndRoles", mock.Anything)
}
