package apc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// APC takes Astro's spellings where 1.x had its own: the Astro spelling is
// the one help shows, and the 1.x one, and its shorthand, reach the same
// value.

func TestWorkspaceNameFlag(t *testing.T) {
	created := func(label string) any {
		return mock.MatchedBy(func(r houston.CreateWorkspaceRequest) bool { return r.Label == label })
	}
	for _, args := range [][]string{
		{"--name", "alpha"}, {"-n", "alpha"}, {"--label", "alpha"}, {"-l", "alpha"}, {"--name", "alpha", "--label", "alpha"},
	} {
		t.Run("create "+args[0], func(t *testing.T) {
			api := newAPCClient()
			api.On("CreateWorkspace", created("alpha")).Return(&houston.Workspace{ID: "ws-1", Label: "alpha"}, nil).Once()
			run := runAPC(t, api, "", append([]string{"workspace", "create"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
		t.Run("update "+args[0], func(t *testing.T) {
			api := newAPCClient()
			api.On("UpdateWorkspace", mock.MatchedBy(func(r houston.UpdateWorkspaceRequest) bool {
				return r.WorkspaceID == "ws-1" && r.Args["label"] == "alpha"
			})).Return(&houston.Workspace{ID: "ws-1", Label: "alpha"}, nil).Once()
			run := runAPC(t, api, "", append([]string{"workspace", "update", "ws-1"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
	}

	t.Run("the two spellings disagreeing is refused", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "workspace", "create", "--name", "alpha", "--label", "beta")
		assert.NotEqual(t, 0, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), "disagree")
		api.AssertNotCalled(t, "CreateWorkspace", mock.Anything)
	})

	t.Run("--label \"\" is no name, as in 1.x", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "workspace", "create", "--label", "")
		assert.NotEqual(t, 0, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), errCreateWorkspaceMissingLabel.Error())
		api.AssertNotCalled(t, "CreateWorkspace", mock.Anything)
	})

	for _, sub := range []string{"create", "update"} {
		t.Run(sub+" help shows --name only", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", "workspace", sub, "--help")
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stdout, "--name")
			assert.NotContains(t, run.stdout, "--label")
		})
	}
}

// deployment create and update take --name, as Astro's do, with no shorthand
// (-n is 1.x's --nfs-location); adopt's 1.x --name, the custom resource's, is
// --cr-name's alias, and the adopted Deployment's name keeps --label.
func TestDeploymentNameFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--name", "alpha"}, {"--label", "alpha"}, {"-l", "alpha"}, {"--name", "alpha", "--label", "alpha"},
	} {
		t.Run("create "+args[0], func(t *testing.T) {
			api := newAPCClient()
			api.On("CreateDeployment", mock.MatchedBy(func(v map[string]interface{}) bool { return v["label"] == "alpha" })).Return(&runtimeDep, nil).Once()
			run := runAPC(t, api, "", append([]string{"deployment", "create", "--cluster-id", "cl-1"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
		t.Run("update "+args[0], func(t *testing.T) {
			api := newAPCClient()
			api.On("GetDeployment", "dep-ac").Return(&certifiedDep, nil)
			api.On("UpdateDeployment", mock.MatchedBy(func(v map[string]interface{}) bool {
				p, ok := v["payload"].(map[string]string)
				return ok && p["label"] == "alpha"
			})).Return(&certifiedDep, nil).Once()
			run := runAPC(t, api, "", append([]string{"deployment", "update", "dep-ac"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
	}

	t.Run("create: the two spellings disagreeing is refused", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "deployment", "create", "--cluster-id", "cl-1", "--name", "alpha", "--label", "beta")
		assert.NotEqual(t, 0, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), "disagree")
		api.AssertNotCalled(t, "CreateDeployment", mock.Anything)
	})

	t.Run("create: neither is the required-flag error, naming --name", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "deployment", "create", "--cluster-id", "cl-1")
		assert.NotEqual(t, 0, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), `required flag(s) "name" not set`)
	})

	t.Run("update: --label \"\" changes nothing, as in 1.x", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&certifiedDep, nil)
		api.On("UpdateDeployment", mock.MatchedBy(func(v map[string]interface{}) bool {
			p, _ := v["payload"].(map[string]string)
			_, renamed := p["label"]
			return !renamed
		})).Return(&certifiedDep, nil).Once()
		run := runAPC(t, api, "", "deployment", "update", "dep-ac", "--label", "", "--description", "d")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		api.AssertExpectations(t)
	})

	for _, args := range [][]string{{"--cr-name", "cr"}, {"--name", "cr"}} {
		t.Run("adopt "+args[0]+" names the custom resource", func(t *testing.T) {
			api := newAPCClient()
			api.On("AdoptDeployment", mock.MatchedBy(func(r *houston.AdoptDeploymentRequest) bool {
				return r.CRName == "cr" && r.Label == "shown"
			})).Return(&houston.Deployment{ID: "adopted"}, nil).Once()
			run := runAPC(t, api, "", append([]string{"deployment", "adopt", "--cluster-id", "cl-1", "--namespace", "ns", "--label", "shown"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
	}

	for _, sub := range []string{"create", "update"} {
		t.Run(sub+" help shows --name only", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", "deployment", sub, "--help")
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stdout, "--name")
			assert.NotContains(t, run.stdout, "--label")
		})
	}
	t.Run("adopt help shows --cr-name, not --name", func(t *testing.T) {
		run := runAPC(t, newAPCClient(), "", "deployment", "adopt", "--help")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stdout, "--cr-name")
		assert.NotContains(t, run.stdout, "--name ")
	})
}

func TestDeploymentLogsKeywordFlag(t *testing.T) {
	searched := func(term string) any {
		return mock.MatchedBy(func(r houston.ListDeploymentLogsRequest) bool { return r.Search == term })
	}
	for _, args := range [][]string{
		{"--keyword", "boom"}, {"-k", "boom"}, {"--search", "boom"}, {"-s", "boom"}, {"--keyword", "boom", "--search", "boom"},
	} {
		t.Run(args[0], func(t *testing.T) {
			api := newAPCClient()
			api.On("ListDeploymentLogs", searched("boom")).Return([]houston.DeploymentLog{}, nil).Once()
			run := runAPC(t, api, "", append([]string{"deployment", "logs", "scheduler", "dep-1"}, args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
	}

	t.Run("--search \"\" still searches for nothing", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", searched("")).Return([]houston.DeploymentLog{}, nil).Once()
		run := runAPC(t, api, "", "deployment", "logs", "workers", "dep-1", "--search", "")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		api.AssertExpectations(t)
	})

	t.Run("the two spellings disagreeing is refused", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "deployment", "logs", "webserver", "dep-1", "--keyword", "a", "--search", "b")
		assert.NotEqual(t, 0, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), "disagree")
		api.AssertNotCalled(t, "ListDeploymentLogs", mock.Anything)
	})

	// logs itself, and 1.x's hidden subcommands.
	for _, path := range [][]string{{"logs"}, {"logs", "scheduler"}, {"logs", "webserver"}, {"logs", "workers"}} {
		t.Run(strings.Join(path, " ")+" help shows --keyword only", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", append(append([]string{"deployment"}, path...), "--help")...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stdout, "--keyword")
			assert.NotContains(t, run.stdout, "--search")
		})
	}
}

// user add and team add take the email or the team as the argument, as
// Astro's do, or as the flag they always took.
func TestAddTakesItsSubjectAsTheArgument(t *testing.T) {
	for _, c := range []struct {
		name   string
		cmd    []string
		flag   string
		method string
		sent   any
		ret    any
	}{
		{
			"workspace user add",
			[]string{"workspace", "user", "add", "--workspace", "ws-1"},
			"--email", "AddWorkspaceUser",
			mock.MatchedBy(func(r houston.AddWorkspaceUserRequest) bool { return r.Email == "a@b.co" && r.WorkspaceID == "ws-1" }),
			&houston.Workspace{ID: "ws-1"},
		},
		{
			"deployment user add",
			[]string{"deployment", "user", "add", "--deployment", "dep-1"},
			"--email", "AddDeploymentUser",
			mock.MatchedBy(func(r houston.UpdateDeploymentUserRequest) bool {
				return r.Email == "a@b.co" && r.DeploymentID == "dep-1"
			}),
			&houston.RoleBinding{Role: houston.DeploymentViewerRole},
		},
		{
			"workspace team add",
			[]string{"workspace", "team", "add", "--workspace", "ws-1"},
			"--team-id", "AddWorkspaceTeam",
			mock.MatchedBy(func(r houston.AddWorkspaceTeamRequest) bool { return r.TeamID == "a@b.co" && r.WorkspaceID == "ws-1" }),
			&houston.Workspace{ID: "ws-1"},
		},
		{
			"deployment team add",
			[]string{"deployment", "team", "add", "--deployment", "dep-1"},
			"--team-id", "AddDeploymentTeam",
			mock.MatchedBy(func(r houston.AddDeploymentTeamRequest) bool {
				return r.TeamID == "a@b.co" && r.DeploymentID == "dep-1"
			}),
			&houston.RoleBinding{Role: houston.DeploymentViewerRole},
		},
	} {
		// The value is the same for both, so one matcher serves: an email
		// for a user, a team id for a team.
		const value = "a@b.co"
		for _, given := range [][]string{{value}, {c.flag, value}, {value, c.flag, value}} {
			t.Run(c.name+" "+given[0], func(t *testing.T) {
				api := newAPCClient()
				api.On(c.method, c.sent).Return(c.ret, nil).Once()
				run := runAPC(t, api, "", append(append([]string{}, c.cmd...), given...)...)
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				api.AssertExpectations(t)
			})
		}

		t.Run(c.name+": the argument and the flag disagreeing is refused", func(t *testing.T) {
			api := newAPCClient()
			run := runAPC(t, api, "", append(append([]string{}, c.cmd...), value, c.flag, "other")...)
			assert.NotEqual(t, 0, run.code)
			require.Error(t, run.err)
			assert.Contains(t, run.err.Error(), "disagree")
			api.AssertNotCalled(t, c.method, mock.Anything)
		})

		t.Run(c.name+": neither is the required-flag error it always was", func(t *testing.T) {
			api := newAPCClient()
			run := runAPC(t, api, "", c.cmd...)
			assert.NotEqual(t, 0, run.code)
			require.Error(t, run.err)
			assert.Contains(t, run.err.Error(), `required flag(s) "`+c.flag[2:]+`" not set`)
			api.AssertNotCalled(t, c.method, mock.Anything)
		})

		t.Run(c.name+": more than one argument is refused", func(t *testing.T) {
			api := newAPCClient()
			run := runAPC(t, api, "", append(append([]string{}, c.cmd...), value, "another")...)
			assert.NotEqual(t, 0, run.code)
			api.AssertNotCalled(t, c.method, mock.Anything)
		})
	}
}
