package apc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// APC takes Astro's spellings where 1.x had its own: the Astro spelling is
// the one help shows, and the 1.x one, and its shorthand, reach the same
// value.

func TestWorkspaceNameFlag(t *testing.T) {
	created := func(label string) any {
		return mock.MatchedBy(func(r houston.CreateWorkspaceRequest) bool { return r.Label == label })
	}
	for _, args := range [][]string{
		{"--name", "alpha"}, {"-n", "alpha"}, {"--label", "alpha"}, {"--name", "alpha", "--label", "alpha"},
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

// deployment create, update and adopt take the Deployment's name as --name,
// with 1.x's --label as its alias, and adopt as -n too. 1.x's -n on create and
// update was --nfs-location, so those two take no -n (the root refuses it by
// name, see removedFlags), and 1.x's --name on adopt was the custom
// resource's, which is --cr-name.
func TestDeploymentNameFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--name", "alpha"}, {"--label", "alpha"}, {"--name", "alpha", "--label", "alpha"},
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

	// 1.x's --label took any name, so --name does, whatever it looks like.
	t.Run("a name with :/ in it is a name", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&certifiedDep, nil)
		api.On("UpdateDeployment", mock.MatchedBy(func(v map[string]interface{}) bool {
			p, ok := v["payload"].(map[string]string)
			return ok && p["label"] == "etl:/daily"
		})).Return(&certifiedDep, nil).Once()
		run := runAPC(t, api, "", "deployment", "update", "dep-ac", "--name", "etl:/daily")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		api.AssertExpectations(t)
	})

	// 1.x's -n was the NFS mount, and --name takes no letter on these two, so a
	// 1.x line passing the mount with it never names the Deployment.
	t.Run("create and update take no -n", func(t *testing.T) {
		for _, args := range [][]string{
			{"deployment", "create", "--cluster-id", "cl-1", "--label", "prod", "-n", "nas01:/export/dags"},
			{"deployment", "update", "dep-ac", "--executor", "local", "-n", "nas01:/export/dags"},
		} {
			api := newAPCClient()
			api.On("GetDeployment", "dep-ac").Return(&certifiedDep, nil).Maybe()
			run := runAPC(t, api, "", args...)
			require.Error(t, run.err, "%v", args)
			assert.Contains(t, run.err.Error(), "unknown shorthand flag: 'n'", "%v", args)
			api.AssertNotCalled(t, "CreateDeployment", mock.Anything)
			api.AssertNotCalled(t, "UpdateDeployment", mock.Anything)
		}
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

	adopts := func(label string) *mocks.ClientInterface {
		api := newAPCClient()
		api.On("AdoptDeployment", mock.MatchedBy(func(r *houston.AdoptDeploymentRequest) bool {
			return r.CRName == "cr" && r.Label == label
		})).Return(&houston.Deployment{ID: "adopted"}, nil).Once()
		return api
	}
	adopt := []string{"deployment", "adopt", "--cluster-id", "cl-1", "--namespace", "ns", "--cr-name", "cr"}
	for _, args := range [][]string{{"--name", "shown"}, {"-n", "shown"}, {"--label", "shown"}} {
		t.Run("adopt "+args[0]+" names the Deployment, --cr-name the custom resource", func(t *testing.T) {
			api := adopts("shown")
			run := runAPC(t, api, "", append(append([]string{}, adopt...), args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			api.AssertExpectations(t)
		})
	}
	t.Run("adopt with no --name leaves the name to APC, the custom resource's", func(t *testing.T) {
		api := adopts("")
		run := runAPC(t, api, "", adopt...)
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		api.AssertExpectations(t)
	})

	// 1.x's adopt named the custom resource with --name: such a line names
	// none now, and is refused saying so, with --label beside it too, ahead
	// of --name and --label disagreeing.
	t.Run("a 1.x adopt naming the custom resource with --name is refused, naming --cr-name", func(t *testing.T) {
		for _, args := range [][]string{
			{"--name", "cr"},
			{"--name", "cr", "--label", "shown"},
			{"--label", "shown", "--name", "cr"},
		} {
			api := newAPCClient()
			run := runAPC(t, api, "", append([]string{"deployment", "adopt", "--cluster-id", "cl-1", "--namespace", "ns"}, args...)...)
			assert.Equal(t, 2, run.code, "%v", args)
			require.Error(t, run.err, "%v", args)
			assert.Contains(t, run.err.Error(), "name the custom resource with --cr-name: --name is the adopted Deployment's name in v2", "%v", args)
			assert.NotContains(t, run.err.Error(), "disagree", "%v", args)
			api.AssertNotCalled(t, "AdoptDeployment", mock.Anything)
		}
	})

	// No --name typed, so nothing to say about what it means now: the error is
	// the one naming the flag missing, --label alone given or not.
	t.Run("adopt naming no custom resource and no --name is the required-flag error", func(t *testing.T) {
		for _, args := range [][]string{{}} {
			run := runAPC(t, newAPCClient(), "", append([]string{"deployment", "adopt", "--cluster-id", "cl-1", "--namespace", "ns"}, args...)...)
			require.Error(t, run.err, "%v", args)
			assert.Contains(t, run.err.Error(), `required flag(s) "cr-name" not set`, "%v", args)
		}
	})

	for _, sub := range []string{"create", "update", "adopt"} {
		t.Run(sub+" help shows --name only", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", "deployment", sub, "--help")
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			if sub == "adopt" {
				assert.Contains(t, run.stdout, "-n, --name")
			} else {
				assert.Contains(t, run.stdout, "--name")
				assert.NotContains(t, run.stdout, "-n, --name", "1.x's -n here was --nfs-location")
			}
			assert.NotContains(t, run.stdout, "--label")
		})
	}
}

// service-account create names the account with --name and -n, as Astro's
// API tokens are named, with 1.x's --label and -l as its alias.
func TestServiceAccountNameFlag(t *testing.T) {
	named := func(name string) any {
		return mock.MatchedBy(func(r *houston.CreateServiceAccountRequest) bool { return r.Label == name })
	}
	for _, c := range []struct {
		scope  string
		args   []string
		method string
		ret    any
	}{
		{"workspace", []string{"workspace", "service-account", "create", "--workspace", "ws-1"}, "CreateWorkspaceServiceAccount", &houston.WorkspaceServiceAccount{ServiceAccount: createdSA}},
		{"deployment", []string{"deployment", "service-account", "create", "--deployment", "dep-1"}, "CreateDeploymentServiceAccount", &houston.DeploymentServiceAccount{ServiceAccount: createdSA}},
	} {
		for _, given := range [][]string{
			{"--name", "ci"},
			{"-n", "ci"},
			{"--label", "ci"},
			{"--name", "ci", "--label", "ci"},
			{"-n", "ci", "--label", "ci"},
		} {
			t.Run(c.scope+" "+strings.Join(given, " "), func(t *testing.T) {
				api := newAPCClient()
				api.On(c.method, named("ci")).Return(c.ret, nil).Once()
				run := runAPC(t, api, "", append(append([]string{}, c.args...), given...)...)
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				api.AssertExpectations(t)
			})
		}
		for _, given := range [][]string{{"--name", "ci", "--label", "other"}, {"-n", "ci", "--label", "other"}} {
			t.Run(c.scope+": "+strings.Join(given, " ")+" disagreeing is refused", func(t *testing.T) {
				api := newAPCClient()
				run := runAPC(t, api, "", append(append([]string{}, c.args...), given...)...)
				assert.NotEqual(t, 0, run.code)
				require.Error(t, run.err)
				assert.Contains(t, run.err.Error(), "disagree")
				api.AssertNotCalled(t, c.method, mock.Anything)
			})
		}
		// An unset variable is no name: refused, not an account named "" with
		// a live API key, whichever spelling handed it on.
		for _, given := range [][]string{{"--name", ""}, {"--label", ""}} {
			t.Run(c.scope+": "+strings.Join(given, " ")+`"" is refused`, func(t *testing.T) {
				api := newAPCClient()
				run := runAPC(t, api, "", append(append([]string{}, c.args...), given...)...)
				assert.Equal(t, 2, run.code)
				require.Error(t, run.err)
				assert.Contains(t, run.err.Error(), "a service account needs a name: --name cannot be empty")
				api.AssertNotCalled(t, c.method, mock.Anything)
			})
		}
		t.Run(c.scope+": neither is the required-flag error, naming --name", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", c.args...)
			require.Error(t, run.err)
			assert.Contains(t, run.err.Error(), `required flag(s) "name" not set`)
		})
		t.Run(c.scope+": help shows --name only", func(t *testing.T) {
			run := runAPC(t, newAPCClient(), "", c.scope, "service-account", "create", "--help")
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stdout, "-n, --name")
			assert.NotContains(t, run.stdout, "--label")
		})
	}
}

func TestDeploymentLogsKeywordFlag(t *testing.T) {
	searched := func(term string) any {
		return mock.MatchedBy(func(r houston.ListDeploymentLogsRequest) bool { return r.Search == term })
	}
	for _, args := range [][]string{
		{"--keyword", "boom"}, {"--search", "boom"}, {"--keyword", "boom", "--search", "boom"},
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
