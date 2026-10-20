package api

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// -W/--workspace is where -d's Deployment is looked up.

const (
	testDeploymentID = "ck05r3bor07h40d02y2hw4n4v"
	contextWS        = "ws-context"
	givenWS          = "ws-given"
)

// astroLookups logs in to a cloud context whose workspace is contextWS, and
// stands in for the two Deployment lookups: deployments is every Deployment in
// the organization, which the listing filters by workspace as the API does.
// listed records the workspace each listing asked for, prefixed with its
// organization when that is not the context's.
func astroLookups(t *testing.T, deployments ...astrov1.Deployment) (listed *[]string) {
	t.Helper()
	isolateSpecCache(t)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token, ctx.Organization, ctx.Workspace = "test-token", "test-org", contextWS
	require.NoError(t, ctx.SetContext())

	origByID, origList := deployment.GetDeploymentByID, deployment.ListDeployments
	t.Cleanup(func() { deployment.GetDeploymentByID, deployment.ListDeployments = origByID, origList })
	deployment.GetDeploymentByID = func(orgID, id string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		assert.Equal(t, "test-org", orgID)
		for i := range deployments {
			if deployments[i].Id == id {
				return deployments[i], nil
			}
		}
		return astrov1.Deployment{}, errors.New("deployment not found")
	}
	listed = new([]string)
	deployment.ListDeployments = func(ws, orgID string, _ astrov1.APIClient) ([]astrov1.Deployment, error) {
		if orgID != "test-org" {
			ws = orgID + "/" + ws
		}
		*listed = append(*listed, ws)
		var in []astrov1.Deployment
		for i := range deployments {
			if deployments[i].WorkspaceId == ws {
				in = append(in, deployments[i])
			}
		}
		return in, nil
	}
	return listed
}

func astroDeployment(id, name, ws string) astrov1.Deployment {
	return astrov1.Deployment{Id: id, Name: name, WorkspaceId: ws, WebServerAirflowApiUrl: id + ".airflow.astronomer.io/api/v2"}
}

func targetOf(opts *AirflowOptions) (*airflowTarget, error) {
	opts.ErrOut = new(bytes.Buffer)
	return resolveAirflowTarget(context.Background(), opts)
}

// A name no link declares is the Deployment of that name in --workspace's
// workspace, not in the context's, which has one by the same name.
func TestResolveAirflowTarget_DeploymentNameInTheWorkspaceGiven(t *testing.T) {
	listed := astroLookups(t,
		astroDeployment("dep-in-context", "etl-prod", contextWS),
		astroDeployment("dep-in-given", "etl-prod", givenWS),
	)
	target, err := targetOf(&AirflowOptions{Deployment: "etl-prod", WorkspaceID: givenWS})
	require.NoError(t, err)
	assert.Equal(t, "https://dep-in-given.airflow.astronomer.io", target.hostRoot)
	assert.Equal(t, []string{givenWS}, *listed)
}

// Without --workspace a name is looked up in the context's workspace.
func TestResolveAirflowTarget_DeploymentNameInTheContextsWorkspace(t *testing.T) {
	listed := astroLookups(t,
		astroDeployment("dep-in-context", "etl-prod", contextWS),
		astroDeployment("dep-in-given", "etl-prod", givenWS),
	)
	target, err := targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.NoError(t, err)
	assert.Equal(t, "https://dep-in-context.airflow.astronomer.io", target.hostRoot)
	assert.Equal(t, []string{contextWS}, *listed)
}

func TestResolveAirflowTarget_DeploymentNameMissingOrAmbiguous(t *testing.T) {
	astroLookups(t,
		astroDeployment("dep-a", "twin", givenWS),
		astroDeployment("dep-b", "twin", givenWS),
	)
	_, err := targetOf(&AirflowOptions{Deployment: "etl-prod", WorkspaceID: givenWS})
	require.EqualError(t, err, `no Deployment named "etl-prod" in workspace ws-given`)
	_, err = targetOf(&AirflowOptions{Deployment: "twin", WorkspaceID: givenWS})
	require.EqualError(t, err, `2 Deployments are named "twin" in workspace ws-given: pass -d with one's id (dep-a, dep-b)`)
}

// An id is found across the organization, but with --workspace it has to be
// in that workspace: the flag is not dropped.
func TestResolveAirflowTarget_DeploymentIDInTheWorkspaceGiven(t *testing.T) {
	listed := astroLookups(t, astroDeployment(testDeploymentID, "etl-prod", contextWS))
	want := "https://" + testDeploymentID + ".airflow.astronomer.io"

	for _, ws := range []string{"", contextWS} {
		target, err := targetOf(&AirflowOptions{Deployment: testDeploymentID, WorkspaceID: ws})
		require.NoError(t, err, ws)
		assert.Equal(t, want, target.hostRoot, ws)
	}

	_, err := targetOf(&AirflowOptions{Deployment: testDeploymentID, WorkspaceID: givenWS})
	require.EqualError(t, err, "deployment "+testDeploymentID+" is in workspace ws-context, not --workspace ws-given")
	assert.Empty(t, *listed, "an id is fetched, not listed")
}

// A link says which Deployment it is and where it lives, so --workspace has
// to agree with it rather than be dropped.
func TestResolveAirflowTarget_LinkAndWorkspace(t *testing.T) {
	writeAPIProject(t, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-linked"

[tool.astro.deployments.prod]
deployment = "`+testDeploymentID+`"

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "from-the-environment")

	_, err := targetOf(&AirflowOptions{Deployment: "prod", WorkspaceID: givenWS})
	require.EqualError(t, err, "-d prod links a Deployment in workspace ws-linked (pyproject.toml), not --workspace ws-given")

	_, err = targetOf(&AirflowOptions{Deployment: "staging", WorkspaceID: givenWS})
	require.EqualError(t, err, "-d staging is a url link, which is in no workspace, so --workspace ws-given cannot apply to it")

	// Agreeing with the link, or not given, it is no obstacle.
	m, err := manifest.Load(filepath.Join(config.WorkingPath, manifest.Marker))
	require.NoError(t, err)
	prod, ok := instances.Build(m).Lookup("prod")
	require.True(t, ok)
	require.NoError(t, linkInWorkspace(&prod, "ws-linked"))
	require.NoError(t, linkInWorkspace(&prod, ""))
	target, err := targetOf(&AirflowOptions{Deployment: "staging"})
	require.NoError(t, err)
	assert.Equal(t, "https://airflow.staging.corp.dev", target.hostRoot)
}

func TestResolveAirflowTarget_RefusesURLAndWorkspace(t *testing.T) {
	_, err := targetOf(&AirflowOptions{URL: "https://a.dev", WorkspaceID: givenWS})
	require.ErrorContains(t, err, "--url and --workspace cannot be used together")
}

// The flag reaches the lookup through the command: --workspace-id (1.x's
// spelling) the same as --workspace and -W, and an empty --workspace-id is
// the context's workspace, as in 1.x. An empty --workspace is refused.
func TestAirflowCmdLooksTheDeploymentUpInTheWorkspaceFlag(t *testing.T) {
	for _, tc := range []struct {
		flags    []string
		ws, host string
	}{
		{[]string{"--workspace", givenWS}, givenWS, "dep-in-given"},
		{[]string{"-W", givenWS}, givenWS, "dep-in-given"},
		{[]string{"--workspace-id", givenWS}, givenWS, "dep-in-given"},
		{[]string{"--workspace-id", ""}, contextWS, "dep-in-context"},
		{nil, contextWS, "dep-in-context"},
	} {
		listed := astroLookups(t,
			astroDeployment("dep-in-context", "etl-prod", contextWS),
			astroDeployment("dep-in-given", "etl-prod", givenWS),
		)
		out := new(bytes.Buffer)
		cmd := NewAirflowCmd(out)
		cmd.SetArgs(append([]string{"-d", "etl-prod", "--airflow-version", "3.0.3", "--generate", "/dags"}, tc.flags...))
		cmd.SetOut(out)
		cmd.SetErr(new(bytes.Buffer))
		require.NoError(t, cmd.Execute(), tc.flags)
		assert.Equal(t, []string{tc.ws}, *listed, tc.flags)
		assert.Contains(t, out.String(), "https://"+tc.host+".airflow.astronomer.io/api/v2/dags", tc.flags)
	}

	cmd := NewAirflowCmd(new(bytes.Buffer))
	cmd.SetArgs([]string{"-d", "etl-prod", "--workspace", "", "/dags"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	require.EqualError(t, cmd.Execute(), "--workspace cannot be empty")
}

// Under -O for another organization the context's workspace is not one of
// its workspaces, so a name needs --workspace; an id is still found across
// that organization.
func TestResolveAirflowTarget_OtherOrganizationNeedsAWorkspaceForAName(t *testing.T) {
	listed := astroLookups(t, astroDeployment("dep-in-given", "etl-prod", givenWS))
	deployment.GetDeploymentByID = func(orgID, id string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		assert.Equal(t, "other-org", orgID)
		return astroDeployment(id, "etl-prod", givenWS), nil
	}

	_, err := targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "other-org"})
	require.EqualError(t, err, `"etl-prod" is not a Deployment id, and the context's workspace is not in --organization-id other-org: pass --workspace to find a Deployment of that name there`)
	assert.Empty(t, *listed, "the context's workspace is not listed in another organization")

	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "other-org", WorkspaceID: givenWS})
	require.EqualError(t, err, `no Deployment named "etl-prod" in workspace ws-given`, "the listing went to the other organization")
	assert.Equal(t, []string{"other-org/" + givenWS}, *listed)

	target, err := targetOf(&AirflowOptions{Deployment: testDeploymentID, OrganizationID: "other-org"})
	require.NoError(t, err)
	assert.Equal(t, "https://"+testDeploymentID+".airflow.astronomer.io", target.hostRoot)

	// -O naming the context's own organization changes nothing.
	*listed = nil
	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "test-org"})
	require.EqualError(t, err, `no Deployment named "etl-prod" in workspace ws-context`)
	assert.Equal(t, []string{contextWS}, *listed)
}

// A name with no --workspace and no workspace on the context has nowhere to
// be looked up.
func TestResolveAirflowTarget_DeploymentNameWithNoWorkspace(t *testing.T) {
	listed := astroLookups(t)
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Workspace = ""
	require.NoError(t, ctx.SetContext())

	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.EqualError(t, err, `"etl-prod" is not a Deployment id, and there is no workspace to find a Deployment of that name in: pass --workspace, or run 'astro workspace switch'`)
	assert.Empty(t, *listed)
}

// Inside a project, a name that is neither a link nor a Deployment is most
// likely a mistyped link, so the links lead the message.
func TestResolveAirflowTarget_UnknownNameInAProjectLeadsWithTheLinks(t *testing.T) {
	astroLookups(t, astroDeployment("dep-a", "twin", contextWS), astroDeployment("dep-b", "twin", contextWS))
	writeAPIProject(t, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
url = "https://airflow.prod.corp.dev"
auth = { method = 'none' }

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev"
auth = { method = 'none' }
`)
	_, err := targetOf(&AirflowOptions{Deployment: "prdo"})
	require.EqualError(t, err, "no link or Deployment named \"prdo\". Deployments this project links: prod, staging\n"+
		`no Deployment named "prdo" in workspace ws-context`)

	// Any other failure keeps the links after it.
	_, err = targetOf(&AirflowOptions{Deployment: "twin"})
	require.EqualError(t, err, `2 Deployments are named "twin" in workspace ws-context: pass -d with one's id (dep-a, dep-b)`+
		"\nDeployments this project links: prod, staging")
}
