package apc

import (
	"bytes"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// noWorkspaceContext leaves the current context with no workspace.
func (s *Suite) noWorkspaceContext() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	c, err := config.GetCurrentContext()
	s.Require().NoError(err)
	s.Require().NoError(c.SetContextKey("workspace", ""))
	s.T().Cleanup(func() {
		workspaceID = ""
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	})
}

// An explicit --workspace-id is the workspace, whatever the context holds: a
// context with none does not fail a command that names one.
func (s *Suite) TestCoalesceWorkspaceReadsTheFlagFirst() {
	s.noWorkspaceContext()
	workspaceID = "ckexplicitworkspace0001"
	ws, err := coalesceWorkspace()
	s.NoError(err)
	s.Equal("ckexplicitworkspace0001", ws)

	workspaceID = ""
	_, err = coalesceWorkspace()
	s.ErrorContains(err, "failed to get current workspace")
}

// --all lists every workspace's Deployments, so a context with no workspace
// still lists them; without --all the workspace is still needed.
func (s *Suite) TestDeploymentListAllNeedsNoWorkspace() {
	s.noWorkspaceContext()
	api := new(mocks.ClientInterface)
	api.On("ListPaginatedDeployments", houston.PaginatedDeploymentsRequest{Take: -1}).Return([]houston.Deployment{*mockDeployment}, nil).Once()
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api

	output, err := execDeploymentCmd("list", "--all")
	s.NoError(err)
	s.Contains(output, mockDeployment.ID)
	api.AssertExpectations(s.T())

	_, err = execDeploymentCmd("list")
	s.ErrorContains(err, "failed to find a valid workspace")
}

// --workspace-id is on the commands that read it, create, list and adopt.
// 1.x gave it to the other deployment commands, through the group, and none
// read it: they keep it, hidden and setting nothing. hibernate and wake-up,
// newer than 1.x, have none.
func (s *Suite) TestDeploymentWorkspaceIDIsOnlyWhereItIsRead() {
	prev := appConfig
	appConfig = &houston.AppConfig{Flags: houston.FeatureFlags{AstroRuntimeEnabled: true, TriggererEnabled: true}}
	s.T().Cleanup(func() { appConfig = prev; workspaceID = "" })
	root := newDeploymentRootCmd(new(bytes.Buffer))
	find := func(path string) *cobra.Command {
		c, rest, err := root.Find(strings.Fields(path))
		s.Require().NoError(err, path)
		s.Require().Empty(rest, path)
		s.Require().Equal("deployment "+path, c.CommandPath(), path)
		return c
	}
	for _, path := range []string{"create", "list", "adopt"} {
		if f := find(path).Flag("workspace-id"); s.NotNil(f, path) {
			s.False(f.Hidden, path)
		}
	}
	for _, path := range []string{
		"update", "delete", "unadopt", "airflow upgrade",
		"logs webserver", "logs scheduler", "logs workers", "logs triggerer",
		"service-account create", "service-account list", "service-account delete",
		"user list", "user add", "user remove", "user update",
		"team list", "team add", "team remove", "team update",
		"runtime upgrade", "runtime migrate",
	} {
		c := find(path)
		if f := c.Flag("workspace-id"); s.NotNil(f, path) {
			s.True(f.Hidden, path)
			s.Equal("Has no effect; accepted so scripts written for Astro CLI 1.x keep working", f.Usage, path)
		}
		s.NoError(c.ParseFlags([]string{"--workspace-id", "ckexplicitworkspace0001"}), path)
		s.Empty(workspaceID, "%s: the no-op sets nothing", path)
	}
	for _, path := range []string{"hibernate", "wake-up"} {
		s.Nil(find(path).Flag("workspace-id"), path)
	}
}
