package astro

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const (
	projectWS  = "clprojectworkspace0000001"
	projectDep = "clprojectdeployment000001"
	otherDep   = "clotherdeployment00000001"
)

var testProjectManifest = fmt.Sprintf(`[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = %q
domain = "astronomer.io"

[tool.astro.deployments.test]
deployment = %q
default = true
`, projectWS, projectDep)

// inProject runs the test from inside a project holding manifest, and puts
// back every value the project hook and the commands under it set.
func inProject(t *testing.T, pyproject string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0o600))
	sub := filepath.Join(dir, "dags")
	require.NoError(t, os.Mkdir(sub, 0o755))
	orig := config.WorkingPath
	config.WorkingPath = sub
	t.Setenv("ASTRO_DOMAIN", "")
	t.Cleanup(func() {
		config.WorkingPath = orig
		projectWorkspaceID, workspaceID, deploymentID, deploymentName = "", "", "", ""
		forceDelete = false
		resetEnvFlags()
	})
}

func expectWorkspaceName(mc *astrov1_mocks.ClientWithResponsesInterface) {
	mc.On("GetWorkspaceWithResponse", mock.Anything, mock.Anything, projectWS).Return(&astrov1.GetWorkspaceResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.Workspace{Id: projectWS, Name: "Example"},
	}, nil)
}

func expectEnvList(mc *astrov1_mocks.ClientWithResponsesInterface, match func(*astrov1.ListEnvironmentObjectsParams) bool) {
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(match)).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{},
	}, nil).Once()
}

func TestEnvFollowsTheProjectWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == projectWS && p.DeploymentId == nil
	})
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "using workspace Example from pyproject.toml\n")
	mc.AssertExpectations(t)
}

// The workspace's name is looked up in the organization the manifest names.
func TestEnvNamesTheWorkspaceFromTheProjectsOrganization(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, strings.Replace(testProjectManifest, "domain = \"astronomer.io\"\n", "domain = \"astronomer.io\"\norganization = \"clother\"\n", 1))
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool { return true })
	mc.On("GetWorkspaceWithResponse", mock.Anything, "clother", projectWS).Return(&astrov1.GetWorkspaceResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.Workspace{Id: projectWS, Name: "Example"},
	}, nil)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "using workspace Example from pyproject.toml\n")
}

func TestEnvWorkspaceFlagWinsOverTheProject(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == "clexplicitworkspace000001"
	})
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "--workspace-id", "clexplicitworkspace000001")
	require.NoError(t, err)
	assert.NotContains(t, out, "pyproject.toml")
	mc.AssertExpectations(t)
}

func TestEnvTakesALinkNameAsTheDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.DeploymentId != nil && *p.DeploymentId == projectDep && p.WorkspaceId == nil
	})
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "--deployment-id", "test")
	require.NoError(t, err)
	assert.Contains(t, out, "using workspace Example from pyproject.toml (link test)\n")
	mc.AssertExpectations(t)
}

func TestEnvRefusesAnUnknownDeploymentAndNamesTheLinks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("variable", "list", "--deployment-id", "prod")
	require.EqualError(t, err, `"prod" is not a Deployment id or a Deployment link in pyproject.toml. Links: test`)
	mc.AssertExpectations(t)
}

func TestEnvOutsideAProjectKeepsTheContextWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	orig := config.WorkingPath
	config.WorkingPath = t.TempDir()
	t.Cleanup(func() { config.WorkingPath = orig; resetEnvFlags() })
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == "ck05r3bor07h40d02y2hw4n4v"
	})
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list")
	require.NoError(t, err)
	assert.NotContains(t, out, "pyproject.toml")
	mc.AssertExpectations(t)
}

func TestEnvSwitchesToTheProjectDomainLogin(t *testing.T) {
	writeTestConfig(t, `context: astronomer-dev.io
contexts:
  astronomer-dev_io:
    domain: astronomer-dev.io
    token: Bearer dev-token
    workspace: cldevworkspace00000000001
    organization: dev-org
  astronomer_io:
    domain: astronomer.io
    token: Bearer prod-token
    workspace: clprodworkspace0000000001
    organization: prod-org
`)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, "prod-org", mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == projectWS
	})).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{},
	}, nil).Once()
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list")
	require.NoError(t, err)
	assert.Equal(t, "astronomer.io", os.Getenv("ASTRO_DOMAIN"))
	assert.Contains(t, out, "using workspace Example on astronomer.io from pyproject.toml\n")
	assert.Equal(t, "astronomer-dev.io", config.CFG.Context.GetHomeString())
	mc.AssertExpectations(t)
}

func TestEnvWithNoLoginForTheProjectDomainSaysSo(t *testing.T) {
	writeTestConfig(t, `context: astronomer-dev.io
contexts:
  astronomer-dev_io:
    domain: astronomer-dev.io
    token: Bearer dev-token
    organization: dev-org
`)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("variable", "list")
	require.ErrorContains(t, err, "not logged in to astronomer.io")
	assert.Empty(t, os.Getenv("ASTRO_DOMAIN"))
	mc.AssertExpectations(t)
}

func TestEnvWithAnAPITokenStaysOnTheCurrentHost(t *testing.T) {
	writeTestConfig(t, `context: astronomer-dev.io
contexts:
  astronomer-dev_io:
    domain: astronomer-dev.io
    token: Bearer dev-token
    workspace: cldevworkspace00000000001
    organization: dev-org
`)
	inProject(t, testProjectManifest)
	t.Setenv("ASTRO_API_TOKEN", "api-token")
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == projectWS
	})
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list")
	require.NoError(t, err)
	assert.Empty(t, os.Getenv("ASTRO_DOMAIN"))
	assert.Contains(t, out, "using workspace Example from pyproject.toml\n")
	mc.AssertExpectations(t)
}

func writeTestConfig(t *testing.T, yaml string) {
	t.Helper()
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, []byte(yaml), 0o600))
	config.InitConfig(fs)
}

func projectDeploymentsResponse(ids ...string) *astrov1.ListDeploymentsResponse {
	deployments := make([]astrov1.Deployment, 0, len(ids))
	for _, id := range ids {
		deployments = append(deployments, astrov1.Deployment{Id: id, Name: "dep-" + id, WorkspaceId: projectWS, OrganizationId: "test-org-id"})
	}
	return &astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: deployments},
	}
}

func inWorkspace(ws string) any {
	return mock.MatchedBy(func(p *astrov1.ListDeploymentsParams) bool {
		return p.WorkspaceIds != nil && len(*p.WorkspaceIds) == 1 && (*p.WorkspaceIds)[0] == ws
	})
}

func TestDeploymentVariableListFindsAnIDInTheProjectWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, inWorkspace(projectWS)).Return(projectDeploymentsResponse(otherDep), nil).Once()
	value := "v"
	mc.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, otherDep).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.Deployment{Id: otherDep, WorkspaceId: projectWS, EnvironmentVariables: &[]astrov1.DeploymentEnvironmentVariable{
			{Key: "FROM_PROJECT", Value: &value},
		}},
	}, nil).Once()
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execDeploymentCmd("variable", "list", "--deployment-id", otherDep)
	require.NoError(t, err)
	assert.Contains(t, out, "FROM_PROJECT")
	mc.AssertExpectations(t)
}

func TestDeploymentDeleteOfALinkStillAsks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	_, err = w.WriteString("n\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, inWorkspace(projectWS)).Return(projectDeploymentsResponse(projectDep), nil).Once()
	mc.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, projectDep).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.Deployment{Id: projectDep, Name: "dep", WorkspaceId: projectWS},
	}, nil).Once()
	expectWorkspaceName(mc)
	astroV1Client = mc

	_, err = execDeploymentCmd("delete", "test")
	require.NoError(t, err)
	mc.AssertNotCalled(t, "DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
	mc.AssertExpectations(t)
}

func TestDeploymentCreateDefaultsToTheProjectWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectWorkspaceName(mc)
	astroV1Client = mc

	group := &cobra.Command{Use: "deployment"}
	group.PersistentPreRunE = followProjectPreRun(group)
	var ws string
	create := &cobra.Command{Use: "create", RunE: func(*cobra.Command, []string) (err error) {
		ws, err = coalesceWorkspace()
		return err
	}}
	create.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "")
	group.AddCommand(create)
	group.SetArgs([]string{"create"})
	group.SetErr(new(bytes.Buffer))

	require.NoError(t, group.Execute())
	assert.Equal(t, projectWS, ws)
	mc.AssertExpectations(t)
}

func TestFollowProjectResolvesALinkWhereverADeploymentGoes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("argument", func(t *testing.T) {
		inProject(t, testProjectManifest)
		cmd := &cobra.Command{Use: "inspect", Annotations: map[string]string{deploymentArgAnnotation: "true"}}
		args := []string{"test"}
		pick, err := followProject(cmd, args)
		require.NoError(t, err)
		assert.Equal(t, []string{projectDep}, args)
		assert.Equal(t, projectPick{workspace: projectWS, domain: "astronomer.io", link: "test"}, pick)
	})

	t.Run("deployment name", func(t *testing.T) {
		inProject(t, testProjectManifest)
		cmd := &cobra.Command{Use: "logs"}
		cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"-n", "test"}))
		pick, err := followProject(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, projectDep, deploymentID)
		assert.Empty(t, deploymentName)
		assert.Equal(t, "test", pick.link)
	})

	t.Run("a deployment name that is no link stays a name", func(t *testing.T) {
		inProject(t, testProjectManifest)
		cmd := &cobra.Command{Use: "logs"}
		cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"-n", "my deployment"}))
		pick, err := followProject(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, "my deployment", deploymentName)
		assert.Empty(t, deploymentID)
		assert.Equal(t, projectPick{workspace: projectWS, domain: "astronomer.io"}, pick)
	})

	t.Run("an explicit workspace keeps the context", func(t *testing.T) {
		inProject(t, testProjectManifest)
		cmd := &cobra.Command{Use: "list"}
		cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"--workspace-id", "clexplicitworkspace000001"}))
		pick, err := followProject(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, projectPick{}, pick)
	})

	t.Run("a project with no workspace keeps the context", func(t *testing.T) {
		inProject(t, "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n")
		cmd := &cobra.Command{Use: "list"}
		pick, err := followProject(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, projectPick{}, pick)
	})
}

func TestProjectDeployment(t *testing.T) {
	m := &manifest.Manifest{Astro: manifest.Astro{Deployments: map[string]manifest.Link{
		"test":   {Target: "astro", Deployment: projectDep, Workspace: projectWS},
		"prod":   {Target: "astro", Deployment: otherDep, Workspace: projectWS},
		"orders": {Target: "mwaa", Environment: "orders-prod"},
	}}}

	id, link, err := projectDeployment(m, "test")
	require.NoError(t, err)
	assert.Equal(t, [2]string{projectDep, "test"}, [2]string{id, link})

	id, link, err = projectDeployment(m, otherDep)
	require.NoError(t, err)
	assert.Equal(t, [2]string{otherDep, "prod"}, [2]string{id, link})

	id, link, err = projectDeployment(m, "clunlinkeddeployment00001")
	require.NoError(t, err)
	assert.Equal(t, [2]string{"clunlinkeddeployment00001", ""}, [2]string{id, link})

	_, _, err = projectDeployment(m, "orders")
	require.EqualError(t, err, "link orders in pyproject.toml points at mwaa, not at an Astro Deployment")

	_, _, err = projectDeployment(m, "staging")
	require.EqualError(t, err, `"staging" is not a Deployment id or a Deployment link in pyproject.toml. Links: prod, test`)

	_, _, err = projectDeployment(&manifest.Manifest{}, "staging")
	require.EqualError(t, err, `"staging" is not a Deployment id, and pyproject.toml has no Astro Deployment links`)
}
