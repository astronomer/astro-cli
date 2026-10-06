package astro

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The `astro deployment token` family: what each command publishes under
// --output json, and that its text is the bytes it printed before it gained
// one. The text cases were recorded on v2 before the platform package stopped
// printing, with stdout captured whole (the old code wrote some lines to the
// command's writer and some to bare stdout), so a line that moved or changed
// fails here.

const (
	tokDeploymentID = "cldep000000000000000000001"
	tokWorkspaceID  = "clws0000000000000000000001"
	tokSecret       = "eyJhbGciOi.secret-value"
)

func tokPtr[T any](v T) *T { return &v }

// tokenFixtures builds the three kinds of token a Deployment can hold, created
// at times whose "ago" rendering is stable for the length of a test run.
func tokenFixtures() (dep, ws, org astrov1.ApiToken) {
	now := time.Now()
	dep = astrov1.ApiToken{
		Id: "tok-dep", Name: "ci-deploy", Description: "Deploys from CI",
		Scope:     astrov1.ApiTokenScopeDEPLOYMENT,
		Roles:     &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: tokDeploymentID, Role: "DEPLOYMENT_ADMIN"}},
		CreatedAt: now.Add(-50 * time.Hour),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: tokPtr("Ada Lovelace")},
	}
	ws = astrov1.ApiToken{
		Id: "tok-ws", Name: "ws-token",
		Scope: astrov1.ApiTokenScopeWORKSPACE,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: tokWorkspaceID, Role: "WORKSPACE_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: tokDeploymentID, Role: "DEPLOYMENT_MEMBER"},
		},
		CreatedAt: now.Add(-3*time.Hour - time.Minute),
		CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: tokPtr("bootstrap")},
	}
	org = astrov1.ApiToken{
		Id: "tok-org", Name: "org-token", Description: "Org wide",
		Scope: astrov1.ApiTokenScopeORGANIZATION,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: tokDeploymentID, Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: now.Add(-10*time.Minute - 30*time.Second),
	}
	return dep, ws, org
}

func ok200() *http.Response { return &http.Response{StatusCode: http.StatusOK} }

func listTokensResp(tokens ...astrov1.ApiToken) *astrov1.ListApiTokensResponse {
	if tokens == nil {
		tokens = []astrov1.ApiToken{}
	}
	return &astrov1.ListApiTokensResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.ApiTokensPaginated{Tokens: tokens, Limit: 100, TotalCount: len(tokens)},
	}
}

func getTokenResp(t astrov1.ApiToken) *astrov1.GetApiTokenResponse { //nolint:gocritic // a test fixture
	return &astrov1.GetApiTokenResponse{HTTPResponse: ok200(), JSON200: &t}
}

func withSecret(t astrov1.ApiToken) *astrov1.ApiToken { //nolint:gocritic // a test fixture
	t.Token = tokPtr(tokSecret)
	return &t
}

// tokenMock is a client that answers List with tokens and Get with whichever
// of them is asked for.
func tokenMock(t *testing.T, tokens ...astrov1.ApiToken) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listTokensResp(tokens...), nil).Maybe()
	for i := range tokens {
		m.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, tokens[i].Id).Return(getTokenResp(tokens[i]), nil).Maybe()
	}
	return m
}

func rolesOK() *astrov1.UpdateApiTokenRolesResponse {
	return &astrov1.UpdateApiTokenRolesResponse{HTTPResponse: ok200(), JSON200: &astrov1.SubjectRoles{}}
}

// tokenRun is one run of `astro deployment ...` through the root's reporting,
// with every stream the test needs apart.
type tokenRun struct {
	stdout string
	stderr string
	err    error
}

// execTokenCmd runs `astro deployment <args>` the way the CLI does: through
// cliout.Execute, with os.Stdout captured whole (the command's writer is the
// same stdout, as in production) and stdin answering any question with answers.
func execTokenCmd(t *testing.T, client astrov1.APIClient, answers string, args ...string) tokenRun {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.SetupOSArgsForGinkgo()
	prevClient := astroV1Client
	astroV1Client = client
	t.Cleanup(func() { astroV1Client = prevClient })
	// The token ids are package variables a positional argument sets and no
	// flag registration resets, so one run's id would answer the next.
	tokenID, orgTokenID, workspaceTokenID = "", "", ""

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(answers)
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	prevOut, prevIn := os.Stdout, os.Stdin
	os.Stdout, os.Stdin = outW, inR
	captured := make(chan string)
	go func() {
		b, _ := io.ReadAll(outR)
		captured <- string(b)
	}()

	var errBuf bytes.Buffer
	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	root.AddCommand(newDeploymentRootCmd(outW))
	root.SetOut(outW)
	root.SetErr(&errBuf)
	runErr := cliout.Execute(context.Background(), root, append([]string{"deployment"}, args...), outW, nil)

	os.Stdout, os.Stdin = prevOut, prevIn
	require.NoError(t, outW.Close())
	stdout := <-captured
	_ = inR.Close()
	_ = outR.Close()
	return tokenRun{stdout: stdout, stderr: errBuf.String(), err: runErr}
}

// The text each command printed on v2 before the conversion, byte for byte,
// except where three quirks were since fixed: an update without --role keeps
// the role without sending it, an update refuses a role the token already
// holds before changing anything, and a rotate by id names the token.
func TestDeploymentTokenTextIsUnchanged(t *testing.T) {
	dep, ws, org := tokenFixtures()
	d := "--deployment=" + tokDeploymentID

	listAll := "" +
		" ID          NAME          DESCRIPTION         SCOPE            DEPLOYMENT ROLE       CREATED            CREATED BY       \n" +
		" tok-dep     ci-deploy     Deploys from CI     DEPLOYMENT       DEPLOYMENT_ADMIN      2 days ago         Ada Lovelace     \n" +
		" tok-ws      ws-token                          WORKSPACE        DEPLOYMENT_MEMBER     3 hours ago        bootstrap        \n" +
		" tok-org     org-token     Org wide            ORGANIZATION     DEPLOYMENT_ADMIN      10 minutes ago                      \n"

	cases := []struct {
		name    string
		client  func(t *testing.T) astrov1.APIClient
		answers string
		args    []string
		want    string
		wantErr string
	}{
		{
			name:   "list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "list", d},
			want:   listAll,
		},
		{
			name:   "list empty",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t) },
			args:   []string{"token", "list", d},
			want:   " ID     NAME     DESCRIPTION     SCOPE     DEPLOYMENT ROLE     CREATED     CREATED BY     \n",
		},
		{
			name:   "organization-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "organization-token", "list", d},
			want: "" +
				" ID          NAME          DESCRIPTION     SCOPE            DEPLOYMENT ROLE      CREATED            CREATED BY     \n" +
				" tok-org     org-token     Org wide        ORGANIZATION     DEPLOYMENT_ADMIN     10 minutes ago                    \n",
		},
		{
			name:   "workspace-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "workspace-token", "list", d},
			want: "" +
				" ID         NAME         DESCRIPTION     SCOPE         DEPLOYMENT ROLE       CREATED         CREATED BY     \n" +
				" tok-ws     ws-token                     WORKSPACE     DEPLOYMENT_MEMBER     3 hours ago     bootstrap      \n",
		},
		{
			name: "create",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN"},
			want: "\nAstro Deployment API token ci-deploy was successfully created\n" +
				"Copy and paste this API token for your records.\n" +
				"\n" + tokSecret + "\n" +
				"\nYou will not be shown this API token value again.\n",
		},
		{
			name: "create --clean-output",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN", "--clean-output"},
			want: tokSecret + "\n",
		},
		{
			name: "update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				renamed := dep
				renamed.Name = "ci-deploy-2"
				m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &renamed}, nil)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--role", "DEPLOYMENT_MEMBER"},
			want: "Astro Deployment API token ci-deploy was successfully updated\n",
		},
		{
			name: "update through the picker",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			answers: "1\n",
			args:    []string{"token", "update", d, "--role", "DEPLOYMENT_MEMBER"},
			want: "\nPlease select the Deployment API token:\n" +
				" #     ID          NAME          DESCRIPTION         SCOPE          DEPLOYMENT ROLE      CREATED        CREATED BY       \n" +
				" 1     tok-dep     ci-deploy     Deploys from CI     DEPLOYMENT     DEPLOYMENT_ADMIN     2 days ago     Ada Lovelace     \n" +
				"\n> Astro Deployment API token ci-deploy was successfully updated\n",
		},
		{
			// That no role change is sent: TestDeploymentTokenUpdateWithoutRole.
			name: "update without --role keeps the role",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil)
				return m
			},
			args: []string{"token", "update", "tok-dep", d, "--description", "new words"},
			want: "Astro Deployment API token ci-deploy was successfully updated\n",
		},
		{
			// That nothing is sent: TestDeploymentTokenUpdateRefusesBeforeChanging.
			name:    "update to the role it already has",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			args:    []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--role", "DEPLOYMENT_ADMIN"},
			wantErr: "this Deployment API token already has that role on the Deployment",
		},
		{
			name: "rotate --yes by id",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "rotate", "tok-dep", d, "--yes"},
			// Named by its id, the token is still reported by its name.
			want: "\nAstro Deployment API token ci-deploy was successfully rotated\n" +
				"Copy and paste this API token for your records.\n" +
				"\n" + tokSecret + "\n" +
				"\nYou will not be shown this API token value again.\n",
		},
		{
			name: "rotate confirmed by name",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			answers: "y\n",
			args:    []string{"token", "rotate", d, "--name", "ci-deploy"},
			want: "WARNING: API Token rotation will invalidate the current token and cannot be undone.\n" +
				"\nAre you sure you want to rotate the ci-deploy API token? (y/n) " +
				"\nAstro Deployment API token ci-deploy was successfully rotated\n" +
				"Copy and paste this API token for your records.\n" +
				"\n" + tokSecret + "\n" +
				"\nYou will not be shown this API token value again.\n",
		},
		{
			name: "rotate --clean-output",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "rotate", "tok-dep", d, "--yes", "--clean-output"},
			want: tokSecret + "\n",
		},
		{
			name:    "rotate declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			answers: "n\n",
			args:    []string{"token", "rotate", "tok-dep", d},
			want: "WARNING: API Token rotation will invalidate the current token and cannot be undone.\n" +
				"\nAre you sure you want to rotate the ci-deploy API token? (y/n) " +
				"Canceling token rotation\n",
		},
		{
			name: "delete --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
				return m
			},
			args: []string{"token", "delete", "tok-dep", d, "--yes"},
			want: "Astro Deployment API token ci-deploy was successfully deleted\n",
		},
		{
			name: "delete confirmed",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
				return m
			},
			answers: "y\n",
			args:    []string{"token", "delete", "tok-dep", d},
			want: "WARNING: API token deletion cannot be undone.\n" +
				"\nAre you sure you want to delete the ci-deploy API token? (y/n) " +
				"Astro Deployment API token ci-deploy was successfully deleted\n",
		},
		{
			name:    "delete declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			answers: "n\n",
			args:    []string{"token", "delete", "tok-dep", d},
			want: "WARNING: API token deletion cannot be undone.\n" +
				"\nAre you sure you want to delete the ci-deploy API token? (y/n) " +
				"Canceling API Token deletion\n",
		},
		{
			name: "delete a workspace token removes it",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			answers: "y\n",
			args:    []string{"token", "delete", "tok-ws", d},
			want: "\nAre you sure you want to remove the ws-token API token from the Deployment? (y/n) " +
				"Astro API token ws-token was successfully removed from the Deployment\n",
		},
		{
			name:    "remove declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) },
			answers: "n\n",
			args:    []string{"token", "delete", "tok-ws", d},
			want: "\nAre you sure you want to remove the ws-token API token from the Deployment? (y/n) " +
				"Canceling API Token removal\n",
		},
		{
			name: "organization-token add",
			client: func(t *testing.T) astrov1.APIClient {
				_, _, bare := tokenFixtures()
				bare.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}}
				m := tokenMock(t, bare)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN"},
			want: "Astro Organization API token org-token was successfully added/updated to the Deployment\n",
		},
		{
			name: "organization-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "update", "tok-org", d, "--role", "DEPLOYMENT_MEMBER"},
			want: "Astro Organization API token org-token was successfully added/updated to the Deployment\n",
		},
		{
			name: "organization-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "remove", "tok-org", d},
			want: "Astro Organization API token org-token was successfully removed from the Deployment\n",
		},
		{
			name: "workspace-token add",
			client: func(t *testing.T) astrov1.APIClient {
				_, bare, _ := tokenFixtures()
				bare.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: tokWorkspaceID, Role: "WORKSPACE_MEMBER"}}
				m := tokenMock(t, bare)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "add", "tok-ws", d, "--role", "DEPLOYMENT_MEMBER"},
			want: "Astro Workspace API token ws-token was successfully added/updated to the Deployment\n",
		},
		{
			name: "workspace-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "update", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN"},
			want: "Astro Workspace API token ws-token was successfully added/updated to the Deployment\n",
		},
		{
			name: "workspace-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "remove", "tok-ws", d},
			want: "Astro Workspace API token ws-token was successfully removed from the Deployment\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execTokenCmd(t, tc.client(t), tc.answers, tc.args...)
			if tc.wantErr != "" {
				require.Error(t, r.err)
				assert.Equal(t, tc.wantErr, r.err.Error())
			} else {
				require.NoError(t, r.err)
			}
			assert.Equal(t, tc.want, r.stdout)
		})
	}
}

// jsonTokenFixtures are tokenFixtures at fixed times, so their json is exact.
// The Workspace token expires; the others do not.
func jsonTokenFixtures() (dep, ws, org astrov1.ApiToken) {
	dep, ws, org = tokenFixtures()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dep.CreatedAt, ws.CreatedAt, org.CreatedAt = created, created, created
	ws.EndAt = tokPtr(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC))
	return dep, ws, org
}

const (
	depJSON = `{"id":"tok-dep","name":"ci-deploy","description":"Deploys from CI","scope":"DEPLOYMENT","role":"DEPLOYMENT_ADMIN","created_at":"2026-01-02T03:04:05Z","created_by":"Ada Lovelace"`
	wsJSON  = `{"id":"tok-ws","name":"ws-token","description":"","scope":"WORKSPACE","role":"DEPLOYMENT_MEMBER","created_at":"2026-01-02T03:04:05Z","created_by":"bootstrap","expires_at":"2027-01-02T00:00:00Z"}`
	orgJSON = `{"id":"tok-org","name":"org-token","description":"Org wide","scope":"ORGANIZATION","role":"DEPLOYMENT_ADMIN","created_at":"2026-01-02T03:04:05Z"}`
)

// What each command publishes under --output json, byte for byte, and that it
// publishes nothing else: stdout is the one object, stderr is empty.
func TestDeploymentTokenJSON(t *testing.T) {
	dep, ws, org := jsonTokenFixtures()
	d := "--deployment=" + tokDeploymentID

	cases := []struct {
		name   string
		client func(t *testing.T) astrov1.APIClient
		args   []string
		want   string
	}{
		{
			name:   "list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "list", d},
			want:   `{"tokens":[` + depJSON + `},` + wsJSON + `,` + orgJSON + `]}`,
		},
		{
			name:   "list empty",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t) },
			args:   []string{"token", "list", d},
			want:   `{"tokens":[]}`,
		},
		{
			name:   "organization-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "organization-token", "list", d},
			want:   `{"tokens":[` + orgJSON + `]}`,
		},
		{
			name:   "workspace-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "workspace-token", "list", d},
			want:   `{"tokens":[` + wsJSON + `]}`,
		},
		{
			name: "create carries the secret",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN"},
			want: depJSON + `,"token":"` + tokSecret + `"}`,
		},
		{
			name: "update is the token as it now is",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				renamed := dep
				renamed.Name = "ci-deploy-2"
				m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &renamed}, nil)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--role", "DEPLOYMENT_MEMBER"},
			want: `{"id":"tok-dep","name":"ci-deploy-2","description":"Deploys from CI","scope":"DEPLOYMENT","role":"DEPLOYMENT_MEMBER","created_at":"2026-01-02T03:04:05Z","created_by":"Ada Lovelace"}`,
		},
		{
			name: "rotate carries the new secret",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args: []string{"token", "rotate", "tok-dep", d, "--yes"},
			want: depJSON + `,"token":"` + tokSecret + `"}`,
		},
		{
			name: "delete",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
				return m
			},
			args: []string{"token", "delete", "tok-dep", d, "--yes"},
			want: `{"id":"tok-dep","name":"ci-deploy","scope":"DEPLOYMENT","deployment_id":"` + tokDeploymentID + `","action":"deleted"}`,
		},
		{
			name: "delete of a workspace token removes it",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "delete", "tok-ws", d, "--yes"},
			want: `{"id":"tok-ws","name":"ws-token","scope":"WORKSPACE","deployment_id":"` + tokDeploymentID + `","action":"removed"}`,
		},
		{
			name: "organization-token add",
			client: func(t *testing.T) astrov1.APIClient {
				_, _, bare := jsonTokenFixtures()
				bare.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}}
				m := tokenMock(t, bare)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN"},
			want: orgJSON,
		},
		{
			name: "organization-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "update", "tok-org", d, "--role", "DEPLOYMENT_MEMBER"},
			want: `{"id":"tok-org","name":"org-token","description":"Org wide","scope":"ORGANIZATION","role":"DEPLOYMENT_MEMBER","created_at":"2026-01-02T03:04:05Z"}`,
		},
		{
			name: "organization-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "organization-token", "remove", "tok-org", d},
			want: `{"id":"tok-org","name":"org-token","scope":"ORGANIZATION","deployment_id":"` + tokDeploymentID + `","action":"removed"}`,
		},
		{
			name: "workspace-token add",
			client: func(t *testing.T) astrov1.APIClient {
				_, bare, _ := jsonTokenFixtures()
				bare.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: tokWorkspaceID, Role: "WORKSPACE_MEMBER"}}
				m := tokenMock(t, bare)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "add", "tok-ws", d, "--role", "DEPLOYMENT_MEMBER"},
			want: wsJSON,
		},
		{
			name: "workspace-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "update", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN"},
			want: `{"id":"tok-ws","name":"ws-token","description":"","scope":"WORKSPACE","role":"DEPLOYMENT_ADMIN","created_at":"2026-01-02T03:04:05Z","created_by":"bootstrap","expires_at":"2027-01-02T00:00:00Z"}`,
		},
		{
			name: "workspace-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args: []string{"token", "workspace-token", "remove", "tok-ws", d},
			want: `{"id":"tok-ws","name":"ws-token","scope":"WORKSPACE","deployment_id":"` + tokDeploymentID + `","action":"removed"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execTokenCmd(t, tc.client(t), "", append(tc.args, "-o", "json")...)
			require.NoError(t, r.err)
			assert.Equal(t, tc.want+"\n", r.stdout)
			assert.Empty(t, r.stderr)
		})
	}
}

// A list never carries a secret, even when the API sent one.
func TestDeploymentTokenListCarriesNoSecret(t *testing.T) {
	dep, _, _ := jsonTokenFixtures()
	r := execTokenCmd(t, tokenMock(t, *withSecret(dep)), "", "token", "list", "--deployment="+tokDeploymentID, "-o", "json")
	require.NoError(t, r.err)
	assert.NotContains(t, r.stdout, tokSecret)
	assert.NotContains(t, r.stdout, `"token"`)
}

// Under --output json a command that would ask something fails as
// input_required, naming what answers it, with that object as the whole of
// stdout: no warning, no table, no prompt ahead of it. The client mocks
// nothing that changes a token, so a refused question that went on to act
// would panic.
func TestDeploymentTokenJSONNeverAsks(t *testing.T) {
	dep, ws, _ := jsonTokenFixtures()
	d := "--deployment=" + tokDeploymentID

	cases := []struct {
		name     string
		client   func(t *testing.T) astrov1.APIClient
		args     []string
		answered string
	}{
		{"rotate without --yes", func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) }, []string{"token", "rotate", "tok-dep", d}, "pass --yes"},
		{"delete without --yes", func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) }, []string{"token", "delete", "tok-dep", d}, "pass --yes"},
		{"remove without --yes", func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) }, []string{"token", "delete", "tok-ws", d}, "pass --yes"},
		{"update naming no token", func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) }, []string{"token", "update", d, "--role", "DEPLOYMENT_MEMBER"}, "pass the token ID or --name"},
		{"rotate naming no token", func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) }, []string{"token", "rotate", d, "--yes"}, "pass the token ID or --name"},
		{"create naming no name", func(t *testing.T) astrov1.APIClient { return tokenMock(t) }, []string{"token", "create", d, "--role", "DEPLOYMENT_ADMIN"}, "pass --name"},
		{"workspace-token add naming no token", func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) }, []string{"token", "workspace-token", "add", d, "--role", "DEPLOYMENT_ADMIN"}, "pass the token ID or --workspace-token-name"},
		{"workspace-token remove naming no token", func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) }, []string{"token", "workspace-token", "remove", d}, "pass the token ID or --workspace-token-name"},
		{"organization-token add naming no token", func(t *testing.T) astrov1.APIClient { return tokenMock(t) }, []string{"token", "organization-token", "add", d, "--role", "DEPLOYMENT_ADMIN"}, "pass the token ID or --org-token-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execTokenCmd(t, tc.client(t), "y\n1\n", append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, cliout.ExitCode(context.Background(), r.err))
			m := decodeOneJSON(t, r.stdout)
			assert.Equal(t, string(cliout.KindInputRequired), m["kind"])
			assert.Contains(t, m["error"], tc.answered)
			assert.Empty(t, r.stderr)
		})
	}
}

// --output takes text or json, and --clean-output is a text format of its own:
// either mistake is a usage error, exit 2, before anything is asked or done.
func TestDeploymentTokenOutputUsage(t *testing.T) {
	d := "--deployment=" + tokDeploymentID
	for _, args := range [][]string{
		{"token", "list", d, "-o", "yaml"},
		{"token", "create", d, "-o", "yaml"},
		{"token", "create", d, "--name", "n", "--role", "r", "--clean-output", "-o", "json"},
		{"token", "rotate", "tok-dep", d, "--yes", "--clean-output", "-o", "json"},
	} {
		r := execTokenCmd(t, tokenMock(t), "", args...)
		require.Error(t, r.err, args)
		assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(context.Background(), r.err), args)
	}
}

// An update changes only what it was given. Without --role it sends no role
// change at all, and --role has no default for a help text to misstate.
func TestDeploymentTokenUpdateWithoutRole(t *testing.T) {
	dep, _, _ := tokenFixtures()
	d := "--deployment=" + tokDeploymentID

	update, _, err := newDeploymentRootCmd(io.Discard).Find([]string{"token", "update"})
	require.NoError(t, err)
	role := update.Flags().Lookup("role")
	require.NotNil(t, role)
	assert.Empty(t, role.DefValue, "--role has no default")
	assert.Contains(t, role.Usage, "Without it, the token keeps its role")

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			m := tokenMock(t, dep)
			m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(rolesOK(), nil).Maybe()

			r := execTokenCmd(t, m, "", "token", "update", "tok-dep", d, "--description", "new words", "-o", format)
			require.NoError(t, r.err)
			m.AssertCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything)
			m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			if format == "json" {
				assert.Equal(t, "DEPLOYMENT_ADMIN", decodeOneJSON(t, r.stdout)["role"], "the role it kept")
			}
		})
	}
}

// A refused role leaves the token as it was: a role it already holds is
// refused before anything is sent, and a role the API refuses is refused
// before the name and description are sent.
func TestDeploymentTokenUpdateRefusesBeforeChanging(t *testing.T) {
	dep, _, _ := tokenFixtures()
	d := "--deployment=" + tokDeploymentID
	rename := []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--description", "new words"}

	t.Run("a role it already holds", func(t *testing.T) {
		m := tokenMock(t, dep)
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil).Maybe()
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(rolesOK(), nil).Maybe()

		r := execTokenCmd(t, m, "", append(rename, "--role", "DEPLOYMENT_ADMIN")...)
		require.Error(t, r.err)
		assert.Equal(t, "this Deployment API token already has that role on the Deployment", r.err.Error())
		assert.Equal(t, cliout.ExitFailure, cliout.ExitCode(context.Background(), r.err))
		m.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		assert.Empty(t, r.stdout)
	})

	t.Run("a role the API refuses", func(t *testing.T) {
		m := tokenMock(t, dep)
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil).Maybe()
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenRolesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
			Body:         []byte(`{"message":"role NOT_A_ROLE does not exist"}`),
		}, nil)

		r := execTokenCmd(t, m, "", append(rename, "--role", "NOT_A_ROLE")...)
		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), "NOT_A_ROLE")
		m.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		assert.Empty(t, r.stdout)
	})
}
