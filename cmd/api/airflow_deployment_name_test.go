package api

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/instances"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// -d for a value no link declares reads the way astro af reads it: a
// Deployment id, else a Deployment name, looked up in the project's
// [tool.astro] workspace, else the context's.

const (
	testDeploymentID = "ck05r3bor07h40d02y2hw4n4v"
	contextWS        = "ws-context"
	projectWS        = "ws-project"
)

// astroLookups logs in to a cloud context in organization test-org whose
// workspace is contextWS, and stands in for the two Deployment lookups:
// deployments is every Deployment there is, which the listing filters by
// workspace as the API does. listed records each listing as org/workspace.
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
	deployment.GetDeploymentByID = func(_, id string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		for i := range deployments {
			if deployments[i].Id == id {
				return deployments[i], nil
			}
		}
		return astrov1.Deployment{}, errors.New("deployment not found")
	}
	listed = new([]string)
	deployment.ListDeployments = func(ws, orgID string, _ astrov1.APIClient) ([]astrov1.Deployment, error) {
		*listed = append(*listed, orgID+"/"+ws)
		var in []astrov1.Deployment
		for i := range deployments {
			if deployments[i].WorkspaceId == ws && deployments[i].OrganizationId == orgID {
				in = append(in, deployments[i])
			}
		}
		return in, nil
	}
	return listed
}

func astroDeployment(id, name, org, ws string) astrov1.Deployment {
	return astrov1.Deployment{Id: id, Name: name, OrganizationId: org, WorkspaceId: ws, WebServerAirflowApiUrl: id + ".airflow.astronomer.io/api/v2"}
}

func targetOf(opts *AirflowOptions) (*airflowTarget, error) {
	opts.ErrOut = new(bytes.Buffer)
	return resolveAirflowTarget(context.Background(), opts)
}

// projectIn writes a project whose [tool.astro] says astro, and links one
// Deployment, prod.
func projectIn(t *testing.T, astro string) {
	t.Helper()
	t.Setenv("ASTRO_DOMAIN", "")
	writeAPIProject(t, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
`+astro+`

[tool.astro.deployments.prod]
deployment = "clinked00000000000000000a"
workspace = "ws-linked"
`)
}

// Outside a project a name is looked up in the context's workspace.
func TestDeploymentNameInTheContextsWorkspace(t *testing.T) {
	writeAPIProject(t, "")
	listed := astroLookups(t,
		astroDeployment("dep-context", "etl-prod", "test-org", contextWS),
		astroDeployment("dep-other", "etl-prod", "test-org", "ws-other"),
	)
	target, err := targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.NoError(t, err)
	assert.Equal(t, "https://dep-context.airflow.astronomer.io", target.hostRoot)
	assert.Equal(t, []string{"test-org/" + contextWS}, *listed)
}

// Inside a project a name is looked up in the project's workspace, not the
// context's, which has one by the same name; and in the project's
// organization when it names one.
func TestDeploymentNameInTheProjectsWorkspace(t *testing.T) {
	deps := []astrov1.Deployment{
		astroDeployment("dep-context", "etl-prod", "test-org", contextWS),
		astroDeployment("dep-project", "etl-prod", "test-org", projectWS),
		astroDeployment("dep-project-org", "etl-prod", "project-org", projectWS),
	}
	for _, tc := range []struct{ astro, host, listed string }{
		{`workspace = "` + projectWS + `"`, "dep-project", "test-org/" + projectWS},
		{"workspace = \"" + projectWS + "\"\norganization = \"project-org\"", "dep-project-org", "project-org/" + projectWS},
		{"", "dep-context", "test-org/" + contextWS},
	} {
		listed := astroLookups(t, deps...)
		projectIn(t, tc.astro)
		target, err := targetOf(&AirflowOptions{Deployment: "etl-prod"})
		require.NoError(t, err, tc.astro)
		assert.Equal(t, "https://"+tc.host+".airflow.astronomer.io", target.hostRoot, tc.astro)
		assert.Equal(t, []string{tc.listed}, *listed, tc.astro)
	}
}

// An id is fetched, with no listing, wherever it is.
func TestDeploymentIDIsFetchedNotListed(t *testing.T) {
	writeAPIProject(t, "")
	listed := astroLookups(t, astroDeployment(testDeploymentID, "etl-prod", "test-org", "ws-other"))
	target, err := targetOf(&AirflowOptions{Deployment: testDeploymentID})
	require.NoError(t, err)
	assert.Equal(t, "https://"+testDeploymentID+".airflow.astronomer.io", target.hostRoot)
	assert.Empty(t, *listed)
}

func TestDeploymentNameMissingOrAmbiguous(t *testing.T) {
	writeAPIProject(t, "")
	astroLookups(t,
		astroDeployment("dep-a", "twin", "test-org", contextWS),
		astroDeployment("dep-b", "twin", "test-org", contextWS),
	)
	_, err := targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.EqualError(t, err, `no Deployment named "etl-prod" in workspace ws-context`)
	_, err = targetOf(&AirflowOptions{Deployment: "twin"})
	require.EqualError(t, err, `2 Deployments are named "twin" in workspace ws-context: pass -d with one's id (dep-a, dep-b)`)
}

// With no workspace anywhere a name has nowhere to be looked up.
func TestDeploymentNameWithNoWorkspace(t *testing.T) {
	writeAPIProject(t, "")
	listed := astroLookups(t)
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Workspace = ""
	require.NoError(t, ctx.SetContext())
	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.EqualError(t, err, `"etl-prod" is not a Deployment id, and there is no workspace to find a Deployment of that name in: pass -d with the Deployment id, or run 'astro workspace switch'`)
	assert.Empty(t, *listed)
}

// -O for another organization than the searched workspace's refuses a name,
// asking for the id; -O naming that organization changes nothing; and an id
// is still fetched in -O's organization.
func TestDeploymentNameUnderAnotherOrganization(t *testing.T) {
	writeAPIProject(t, "")
	listed := astroLookups(t, astroDeployment("dep-context", "etl-prod", "test-org", contextWS))
	_, err := targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "other-org"})
	require.EqualError(t, err, `"etl-prod" is not a Deployment id, and its name is looked up in workspace ws-context, which is not in --organization-id other-org: pass -d with the Deployment id`)
	assert.Empty(t, *listed)

	target, err := targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "test-org"})
	require.NoError(t, err)
	assert.Equal(t, "https://dep-context.airflow.astronomer.io", target.hostRoot)

	// In a project naming its organization, that is the one -O has to name.
	projectIn(t, "workspace = \""+projectWS+"\"\norganization = \"project-org\"")
	*listed = nil
	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "test-org"})
	require.ErrorContains(t, err, "which is not in --organization-id test-org")
	_, err = targetOf(&AirflowOptions{Deployment: "etl-prod", OrganizationID: "project-org"})
	require.ErrorContains(t, err, `no Deployment named "etl-prod" in workspace ws-project`)
	assert.Equal(t, []string{"project-org/" + projectWS}, *listed)

	var asked string
	deployment.GetDeploymentByID = func(orgID, id string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		asked = orgID
		return astroDeployment(id, "etl-prod", orgID, "ws-other"), nil
	}
	_, err = targetOf(&AirflowOptions{Deployment: testDeploymentID, OrganizationID: "other-org"})
	require.NoError(t, err)
	assert.Equal(t, "other-org", asked)
}

// Inside a project, a name that is neither a link nor a Deployment is most
// likely a mistyped link, so the links lead the message; a name several
// Deployments share is not, so the links follow it.
func TestDeploymentNameInAProjectLeadsWithTheLinks(t *testing.T) {
	astroLookups(t,
		astroDeployment("dep-a", "twin", "test-org", projectWS),
		astroDeployment("dep-b", "twin", "test-org", projectWS),
	)
	projectIn(t, `workspace = "`+projectWS+`"`)
	_, err := targetOf(&AirflowOptions{Deployment: "prdo"})
	require.EqualError(t, err, "no link or Deployment named \"prdo\". Deployments this project links: prod\nno Deployment named \"prdo\" in workspace ws-project")
	_, err = targetOf(&AirflowOptions{Deployment: "twin"})
	require.EqualError(t, err, "2 Deployments are named \"twin\" in workspace ws-project: pass -d with one's id (dep-a, dep-b)\nDeployments this project links: prod")
}

// The flags reach the lookup through the command: -W and --workspace-id are
// accepted and change nothing.
func TestAirflowCmdIgnoresTheWorkspaceIDFlag(t *testing.T) {
	for _, flags := range [][]string{nil, {"-W", "ws-other"}, {"--workspace-id", "ws-other"}} {
		writeAPIProject(t, "")
		listed := astroLookups(t,
			astroDeployment("dep-context", "etl-prod", "test-org", contextWS),
			astroDeployment("dep-other", "etl-prod", "test-org", "ws-other"),
		)
		out := new(bytes.Buffer)
		cmd := NewAirflowCmd(out)
		cmd.SetArgs(append([]string{"-d", "etl-prod", "--airflow-version", "3.0.3", "--generate", "/dags"}, flags...))
		cmd.SetOut(out)
		cmd.SetErr(new(bytes.Buffer))
		require.NoError(t, cmd.Execute(), flags)
		assert.Equal(t, []string{"test-org/" + contextWS}, *listed, flags)
		assert.Contains(t, out.String(), "https://dep-context.airflow.astronomer.io/api/v2/dags", flags)
	}
}

// locatorAt answers every coordinate lookup with url.
type locatorAt string

func (l locatorAt) BaseURL(context.Context, instances.Instance) (string, error) {
	return string(l), nil
}

// A Deployment a link carries answers as that link, with the link's name and
// auth, whether -d named it by id or by name: not as a bare Deployment under
// the context's token. By id it needs no lookup of its own.
func TestDeploymentALinkCarriesIsThatLink(t *testing.T) {
	const linkedID = "clinked00000000000000000b"
	listed := astroLookups(t, astroDeployment(linkedID, "orders-prod", "test-org", projectWS))
	deployment.GetDeploymentByID = func(_, id string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		t.Errorf("fetched %s by id: the link's own lookup reads it", id)
		return astrov1.Deployment{}, errors.New("unexpected")
	}
	t.Setenv("ASTRO_DOMAIN", "")
	writeAPIProject(t, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "`+projectWS+`"

[tool.astro.deployments.prod]
deployment = "`+linkedID+`"
auth = { method = 'token', token-env = 'PROD_AIRFLOW_TOKEN' }
`)
	t.Setenv("PROD_AIRFLOW_TOKEN", "from-the-link")
	orig := newLocator
	t.Cleanup(func() { newLocator = orig })
	newLocator = func(string) instances.Locator { return locatorAt("https://linked.example/api/v2") }

	for _, ref := range []string{"orders-prod", linkedID} {
		*listed = nil
		target, err := targetOf(&AirflowOptions{Deployment: ref})
		require.NoError(t, err, ref)
		assert.Equal(t, "prod", target.name, ref)
		assert.Equal(t, "https://linked.example", target.hostRoot, ref)
		assert.Equal(t, "Bearer from-the-link", target.authorization, ref)
		if ref == linkedID {
			assert.Empty(t, *listed, "an id a link carries is not looked up")
		} else {
			assert.Equal(t, []string{"test-org/" + projectWS}, *listed)
		}
	}
}

// A project on another Astro host than the context's has its workspace
// there, so a name is refused up front, naming the host, rather than listed
// on the wrong one.
func TestDeploymentNameInAProjectOnAnotherHost(t *testing.T) {
	listed := astroLookups(t, astroDeployment("dep-project", "etl-prod", "test-org", projectWS))
	projectIn(t, "workspace = \""+projectWS+"\"\ndomain = \"astronomer-stage.io\"")
	_, err := targetOf(&AirflowOptions{Deployment: "etl-prod"})
	require.ErrorContains(t, err, `"etl-prod" is not a Deployment id, and the project's workspace ws-project, where its name is looked up, is on astronomer-stage.io, not the current context's localhost: run 'astro context switch astronomer-stage.io', or pass -d with the Deployment id`)
	assert.Empty(t, *listed)
}
