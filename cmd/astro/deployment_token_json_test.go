package astro

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
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

// What the `astro deployment token` family does, in both formats.
//
// The json shapes are pinned once, by the goldens in testdata/schema
// (deployment-token*.json, `make update-schemas`). These tests decode what a
// run printed and assert what it means: the exit code, which tokens a list
// holds and with what role, that a list never carries a secret and a create
// or a rotate does, what a removal did. In text they assert the messages, in
// the order a person reads them, and each table cell under its header, not
// how the table pads it: nothing parses that spacing. The one text pinned
// byte for byte is --clean-output, which a script captures whole as the
// token.

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
	// asked is what reached the process's stderr directly: every question
	// (a y/n, a picker drawn on os.Stderr, a prompt for a value) and the
	// notes written beside them. Questions never go to stdout, which carries
	// only the command's result.
	asked string
	err   error
	code  int
}

// noTokenQuestionOnStdout fails when an API token picker's question reached
// stdout: a question's table goes with its question, on stderr.
func noTokenQuestionOnStdout(t *testing.T, stdout string) {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "Please select the") && strings.Contains(line, "API token") {
			t.Errorf("a token picker's question went to stdout: %q", line)
		}
	}
}

// terminal is what a person running the command sees, questions first: the
// order the text tests read prompts and results in.
func (r tokenRun) terminal() string {
	return r.asked + r.stdout
}

// execTokenCmd runs `astro deployment <args>` the way execAstroCmd does.
func execTokenCmd(t *testing.T, client astrov1.APIClient, answers string, args ...string) tokenRun {
	t.Helper()
	return execAstroCmd(t, client, answers, newDeploymentRootCmd, append([]string{"deployment"}, args...)...)
}

// execAstroCmd runs `astro <args>` under the one command newRoot builds, the
// way the CLI does: through cliout.Execute, with os.Stdout captured whole (the
// command's writer is the same stdout, as in production) and stdin answering
// any question with answers.
func execAstroCmd(t *testing.T, client astrov1.APIClient, answers string, newRoot func(io.Writer) *cobra.Command, args ...string) tokenRun {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.SetupOSArgsForGinkgo()
	prevClient := astroV1Client
	astroV1Client = client
	t.Cleanup(func() { astroV1Client = prevClient })
	// The token ids are package variables a positional argument sets and no
	// flag registration resets, so one run's id would answer the next.
	tokenID, orgTokenID, workspaceTokenID = "", "", ""
	// --workspace-id, and deploy and dbt when they resolve one, set the
	// package's workspaceID, which coalesceWorkspace reads before the context.
	// Clear what an earlier run left, and what this one leaves.
	workspaceID = ""
	t.Cleanup(func() { workspaceID = "" })

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(answers)
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	askedR, askedW, err := os.Pipe()
	require.NoError(t, err)

	prevOut, prevErr, prevIn := os.Stdout, os.Stderr, os.Stdin
	os.Stdout, os.Stderr, os.Stdin = outW, askedW, inR
	captured, capturedAsked := make(chan string), make(chan string)
	go func() {
		b, _ := io.ReadAll(outR)
		captured <- string(b)
	}()
	go func() {
		b, _ := io.ReadAll(askedR)
		capturedAsked <- string(b)
	}()

	var errBuf bytes.Buffer
	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	root.AddCommand(newRoot(outW))
	root.SetOut(outW)
	root.SetErr(&errBuf)
	ctx := context.Background()
	runErr := cliout.Execute(ctx, root, args, outW, nil)

	os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn
	require.NoError(t, outW.Close())
	require.NoError(t, askedW.Close())
	stdout, asked := <-captured, <-capturedAsked
	_ = inR.Close()
	_ = outR.Close()
	_ = askedR.Close()
	assert.NotContains(t, stdout, "(y/n)", "a question went to stdout, which carries only the result")
	return tokenRun{stdout: stdout, stderr: errBuf.String(), asked: asked, err: runErr, code: cliout.ExitCode(ctx, runErr)}
}

// headerCell is one column title in a table header: words joined by single
// spaces, so DEPLOYMENT ROLE is one title and the run of spaces after it is
// the padding.
var headerCell = regexp.MustCompile(`\S+(?: \S+)*`)

// tableRows reads the table in out whose header starts with first the way a
// person does: each cell is whatever sits under its column's title, trimmed.
// It fails if there is no such header. The rows end at the first blank line.
// So a cell in the wrong column, or a missing row, fails, while a change to
// how wide the padding is does not.
func tableRows(t *testing.T, out, first string) []map[string]string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, header := range lines {
		if f := strings.Fields(header); len(f) == 0 || f[0] != first {
			continue
		}
		spans := headerCell.FindAllStringIndex(header, -1)
		rows := []map[string]string{}
		for _, line := range lines[i+1:] {
			if strings.TrimSpace(line) == "" {
				break
			}
			row := map[string]string{}
			for c, span := range spans {
				end := len(line)
				if c+1 < len(spans) {
					end = min(spans[c+1][0], len(line))
				}
				if start := span[0]; start < end {
					row[header[span[0]:span[1]]] = strings.TrimSpace(line[start:end])
				} else {
					row[header[span[0]:span[1]]] = ""
				}
			}
			rows = append(rows, row)
		}
		return rows
	}
	t.Fatalf("no table headed %q in:\n%s", first, out)
	return nil
}

// requireInOrder fails unless each of parts appears in out, each after the
// one before it.
func requireInOrder(t *testing.T, out string, parts ...string) {
	t.Helper()
	rest := out
	for _, p := range parts {
		i := strings.Index(rest, p)
		require.GreaterOrEqual(t, i, 0, "%q missing, or out of order, in:\n%s", p, out)
		rest = rest[i+len(p):]
	}
}

// requireLine fails unless line is a whole line of out: the secret a person
// copies stands alone, with nothing they would have to trim.
func requireLine(t *testing.T, out, line string) {
	t.Helper()
	require.Contains(t, strings.Split(out, "\n"), line, "no line %q in:\n%s", line, out)
}

// The rows each kind of token shows in a Deployment's list: the role is its
// role on this Deployment, whatever else it holds.
func listRow(id, name, desc, scope, role, created, by string) map[string]string {
	return map[string]string{
		"ID": id, "NAME": name, "DESCRIPTION": desc, "SCOPE": scope,
		"DEPLOYMENT ROLE": role, "CREATED": created, "CREATED BY": by,
	}
}

var (
	depRow = listRow("tok-dep", "ci-deploy", "Deploys from CI", "DEPLOYMENT", "DEPLOYMENT_ADMIN", "2 days ago", "Ada Lovelace")
	wsRow  = listRow("tok-ws", "ws-token", "", "WORKSPACE", "DEPLOYMENT_MEMBER", "3 hours ago", "bootstrap")
	orgRow = listRow("tok-org", "org-token", "Org wide", "ORGANIZATION", "DEPLOYMENT_ADMIN", "10 minutes ago", "")
)

// What each command prints in text: the same messages, in the same order, as
// before it gained --output, except where three quirks were since fixed:
// an update without --role keeps the role without sending it, an update
// refuses a role the token already holds before changing anything, and a
// rotate by id names the token.
func TestDeploymentTokenText(t *testing.T) {
	dep, ws, org := tokenFixtures()
	d := "--deployment=" + tokDeploymentID

	// What a create or a rotate prints around the secret.
	secretShown := func(verb string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) {
			requireInOrder(t, out,
				"Astro Deployment API token ci-deploy was successfully "+verb,
				"Copy and paste this API token for your records.",
				tokSecret,
				"You will not be shown this API token value again.")
			requireLine(t, out, tokSecret)
		}
	}
	says := func(parts ...string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) { requireInOrder(t, out, parts...) }
	}
	lists := func(rows ...map[string]string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) {
			assert.Equal(t, append([]map[string]string{}, rows...), tableRows(t, out, "ID"))
		}
	}
	// --clean-output exists to be captured whole as the token
	// (`TOKEN=$(astro deployment token create ... --clean-output)`), so here
	// the bytes are the contract: the secret and a newline, nothing else.
	cleanOutput := func(t *testing.T, out string) { assert.Equal(t, tokSecret+"\n", out) }

	cases := []struct {
		name    string
		client  func(t *testing.T) astrov1.APIClient
		answers string
		args    []string
		check   func(t *testing.T, stdout string)
		wantErr string
	}{
		{
			name:   "list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "list", d},
			check:  lists(depRow, wsRow, orgRow),
		},
		{
			name:   "list empty",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t) },
			args:   []string{"token", "list", d},
			check:  lists(),
		},
		{
			name:   "organization-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "organization-token", "list", d},
			check:  lists(orgRow),
		},
		{
			name:   "workspace-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "workspace-token", "list", d},
			check:  lists(wsRow),
		},
		{
			name: "create",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN"},
			check: secretShown("created"),
		},
		{
			name: "create --clean-output",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN", "--clean-output"},
			check: cleanOutput,
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
			args:  []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--role", "DEPLOYMENT_MEMBER"},
			check: says("Astro Deployment API token ci-deploy was successfully updated"),
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
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"Please select the Deployment API token:",
					"> Astro Deployment API token ci-deploy was successfully updated")
				picker := map[string]string{"#": "1"}
				for k, v := range depRow {
					picker[k] = v
				}
				assert.Equal(t, []map[string]string{picker}, tableRows(t, out, "#"))
			},
		},
		{
			// That no role change is sent: TestDeploymentTokenUpdateWithoutRole.
			name: "update without --role keeps the role",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &dep}, nil)
				return m
			},
			args:  []string{"token", "update", "tok-dep", d, "--description", "new words"},
			check: says("Astro Deployment API token ci-deploy was successfully updated"),
		},
		{
			// That nothing is sent: TestDeploymentTokenUpdateRefusesBeforeChanging.
			name:    "update to the role it already has",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			args:    []string{"token", "update", "tok-dep", d, "--new-name", "ci-deploy-2", "--role", "DEPLOYMENT_ADMIN"},
			check:   func(t *testing.T, out string) { assert.Empty(t, out) },
			wantErr: "this Deployment API token already has that role on the Deployment",
		},
		{
			// Named by its id, the token is still reported by its name.
			name: "rotate --yes by id",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "rotate", "tok-dep", d, "--yes"},
			check: secretShown("rotated"),
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
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"WARNING: API Token rotation will invalidate the current token and cannot be undone.",
					"Are you sure you want to rotate the ci-deploy API token? (y/n)",
					"Astro Deployment API token ci-deploy was successfully rotated")
				secretShown("rotated")(t, out)
			},
		},
		{
			name: "rotate --clean-output",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "rotate", "tok-dep", d, "--yes", "--clean-output"},
			check: cleanOutput,
		},
		{
			// The client mocks no rotate, so going on after "n" would panic.
			name:    "rotate declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			answers: "n\n",
			args:    []string{"token", "rotate", "tok-dep", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"WARNING: API Token rotation will invalidate the current token and cannot be undone.",
					"Are you sure you want to rotate the ci-deploy API token? (y/n)",
					"Canceling token rotation")
				assert.NotContains(t, out, tokSecret)
			},
		},
		{
			name: "delete --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
				return m
			},
			args: []string{"token", "delete", "tok-dep", d, "--yes"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Astro Deployment API token ci-deploy was successfully deleted")
				assert.NotContains(t, out, "Are you sure", "--yes answers the question")
			},
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
			check: says(
				"WARNING: API token deletion cannot be undone.",
				"Are you sure you want to delete the ci-deploy API token? (y/n)",
				"Astro Deployment API token ci-deploy was successfully deleted"),
		},
		{
			name:    "delete declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			answers: "n\n",
			args:    []string{"token", "delete", "tok-dep", d},
			check: says(
				"WARNING: API token deletion cannot be undone.",
				"Are you sure you want to delete the ci-deploy API token? (y/n)",
				"Canceling API Token deletion"),
		},
		{
			// A Workspace token is not the Deployment's to delete: deleting it
			// here removes it from the Deployment, and says so.
			name: "delete a workspace token removes it",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			answers: "y\n",
			args:    []string{"token", "delete", "tok-ws", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"Are you sure you want to remove the ws-token API token from the Deployment? (y/n)",
					"Astro API token ws-token was successfully removed from the Deployment")
				assert.NotContains(t, out, "WARNING", "a removal can be undone")
			},
		},
		{
			name:    "remove declined",
			client:  func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) },
			answers: "n\n",
			args:    []string{"token", "delete", "tok-ws", d},
			check: says(
				"Are you sure you want to remove the ws-token API token from the Deployment? (y/n)",
				"Canceling API Token removal"),
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
			args:  []string{"token", "organization-token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN"},
			check: says("Astro Organization API token org-token was successfully added/updated to the Deployment"),
		},
		{
			name: "organization-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "organization-token", "update", "tok-org", d, "--role", "DEPLOYMENT_MEMBER"},
			check: says("Astro Organization API token org-token was successfully added/updated to the Deployment"),
		},
		{
			name: "organization-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "organization-token", "remove", "tok-org", d},
			check: says("Astro Organization API token org-token was successfully removed from the Deployment"),
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
			args:  []string{"token", "workspace-token", "add", "tok-ws", d, "--role", "DEPLOYMENT_MEMBER"},
			check: says("Astro Workspace API token ws-token was successfully added/updated to the Deployment"),
		},
		{
			name: "workspace-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "workspace-token", "update", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN"},
			check: says("Astro Workspace API token ws-token was successfully added/updated to the Deployment"),
		},
		{
			name: "workspace-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "workspace-token", "remove", "tok-ws", d},
			check: says("Astro Workspace API token ws-token was successfully removed from the Deployment"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execTokenCmd(t, tc.client(t), tc.answers, tc.args...)
			if tc.wantErr != "" {
				require.Error(t, r.err)
				assert.Equal(t, tc.wantErr, r.err.Error())
				assert.Equal(t, cliout.ExitFailure, r.code)
			} else {
				require.NoError(t, r.err)
				assert.Equal(t, 0, r.code)
			}
			noTokenQuestionOnStdout(t, r.stdout)
			tc.check(t, r.terminal())
		})
	}
}

// The json as a consumer reads it. The pointers are the fields that may be
// absent: a token's secret, only after a create or a rotate, and when a
// token expires, only for one that does.
type (
	tokenJSON struct {
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Scope       string     `json:"scope"`
		Role        string     `json:"role"`
		CreatedAt   time.Time  `json:"created_at"`
		CreatedBy   string     `json:"created_by"`
		ExpiresAt   *time.Time `json:"expires_at"`
		Token       *string    `json:"token"`
	}
	tokenListJSON struct {
		Tokens []tokenJSON `json:"tokens"`
	}
	tokenRemovalJSON struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Scope        string `json:"scope"`
		DeploymentID string `json:"deployment_id"`
		Action       string `json:"action"`
	}
	errorJSON struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
		Kind  string `json:"kind"`
	}
)

var tokCreated = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// tokenKeys is the set of keys a token publishes, which decoding cannot show:
// an absent key and a null one decode alike. description is always there;
// role, created_by, expires_at and token only when they have a value, so a
// dropped omitempty turns up here as an unexpected key.
func tokenKeys(tok *tokenJSON) []string {
	keys := []string{"created_at", "description", "id", "name", "scope"}
	if tok.Role != "" {
		keys = append(keys, "role")
	}
	if tok.CreatedBy != "" {
		keys = append(keys, "created_by")
	}
	if tok.ExpiresAt != nil {
		keys = append(keys, "expires_at")
	}
	if tok.Token != nil {
		keys = append(keys, "token")
	}
	slices.Sort(keys)
	return keys
}

// objectKeys is the keys of one decoded json object, sorted.
func objectKeys(fields map[string]json.RawMessage) []string {
	return slices.Sorted(maps.Keys(fields))
}

// jsonTokenFixtures are tokenFixtures at a fixed time, so a decoded token
// compares equal. The Workspace token expires; the others do not.
func jsonTokenFixtures() (dep, ws, org astrov1.ApiToken) {
	dep, ws, org = tokenFixtures()
	dep.CreatedAt, ws.CreatedAt, org.CreatedAt = tokCreated, tokCreated, tokCreated
	ws.EndAt = tokPtr(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC))
	return dep, ws, org
}

// What jsonTokenFixtures publish, each with its role on this Deployment.
var (
	depTokenJSON = tokenJSON{
		ID: "tok-dep", Name: "ci-deploy", Description: "Deploys from CI", Scope: "DEPLOYMENT",
		Role: "DEPLOYMENT_ADMIN", CreatedAt: tokCreated, CreatedBy: "Ada Lovelace",
	}
	wsTokenJSON = tokenJSON{
		ID: "tok-ws", Name: "ws-token", Scope: "WORKSPACE",
		Role: "DEPLOYMENT_MEMBER", CreatedAt: tokCreated, CreatedBy: "bootstrap",
		ExpiresAt: tokPtr(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)),
	}
	orgTokenJSON = tokenJSON{
		ID: "tok-org", Name: "org-token", Description: "Org wide", Scope: "ORGANIZATION",
		Role: "DEPLOYMENT_ADMIN", CreatedAt: tokCreated,
	}
)

// with returns tok changed by edit, leaving the shared fixture alone.
func (tok tokenJSON) with(edit func(*tokenJSON)) tokenJSON { //nolint:gocritic // a test fixture
	edit(&tok)
	return tok
}

// What each command publishes under --output json, and that it publishes
// nothing else: stdout is the one object, stderr is empty, the exit is 0.
func TestDeploymentTokenJSON(t *testing.T) {
	dep, ws, org := jsonTokenFixtures()
	d := "--deployment=" + tokDeploymentID

	lists := func(want ...tokenJSON) func(t *testing.T, stdout string) {
		return func(t *testing.T, stdout string) {
			var got tokenListJSON
			fields := decodeOne(t, stdout, &got)
			assert.Equal(t, want, got.Tokens)
			var raw []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(fields["tokens"], &raw))
			require.Len(t, raw, len(want))
			for i := range want {
				assert.Equal(t, tokenKeys(&want[i]), objectKeys(raw[i]), "keys of token %d", i)
			}
		}
	}
	isToken := func(want tokenJSON) func(t *testing.T, stdout string) {
		return func(t *testing.T, stdout string) {
			var got tokenJSON
			fields := decodeOne(t, stdout, &got)
			assert.Equal(t, want, got)
			assert.Equal(t, tokenKeys(&want), objectKeys(fields), "keys of the token")
		}
	}
	removal := func(id, name, scope, action string) func(t *testing.T, stdout string) {
		return func(t *testing.T, stdout string) {
			var got tokenRemovalJSON
			decodeOne(t, stdout, &got)
			assert.Equal(t, tokenRemovalJSON{ID: id, Name: name, Scope: scope, DeploymentID: tokDeploymentID, Action: action}, got)
		}
	}
	withSecretJSON := func(tok *tokenJSON) { tok.Token = tokPtr(tokSecret) }

	cases := []struct {
		name   string
		client func(t *testing.T) astrov1.APIClient
		args   []string
		check  func(t *testing.T, stdout string)
	}{
		{
			name:   "list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "list", d},
			check:  lists(depTokenJSON, wsTokenJSON, orgTokenJSON),
		},
		{
			name:   "list empty",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t) },
			args:   []string{"token", "list", d},
			check: func(t *testing.T, stdout string) {
				var got tokenListJSON
				fields := decodeOne(t, stdout, &got)
				assert.JSONEq(t, `[]`, string(fields["tokens"]), "an empty array, not null and not a missing key")
			},
		},
		{
			name:   "organization-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "organization-token", "list", d},
			check:  lists(orgTokenJSON),
		},
		{
			name:   "workspace-token list",
			client: func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep, ws, org) },
			args:   []string{"token", "workspace-token", "list", d},
			check:  lists(wsTokenJSON),
		},
		{
			name: "create carries the secret",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t)
				m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "create", d, "--name", "ci-deploy", "--role", "DEPLOYMENT_ADMIN"},
			check: isToken(depTokenJSON.with(withSecretJSON)),
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
			check: isToken(depTokenJSON.with(func(tok *tokenJSON) {
				tok.Name, tok.Role = "ci-deploy-2", "DEPLOYMENT_MEMBER"
			})),
		},
		{
			name: "rotate carries the new secret",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(dep)}, nil)
				return m
			},
			args:  []string{"token", "rotate", "tok-dep", d, "--yes"},
			check: isToken(depTokenJSON.with(withSecretJSON)),
		},
		{
			name: "delete",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, dep)
				m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
				return m
			},
			args:  []string{"token", "delete", "tok-dep", d, "--yes"},
			check: removal("tok-dep", "ci-deploy", "DEPLOYMENT", "deleted"),
		},
		{
			name: "delete of a workspace token removes it",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "delete", "tok-ws", d, "--yes"},
			check: removal("tok-ws", "ws-token", "WORKSPACE", "removed"),
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
			args:  []string{"token", "organization-token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN"},
			check: isToken(orgTokenJSON),
		},
		{
			name: "organization-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "organization-token", "update", "tok-org", d, "--role", "DEPLOYMENT_MEMBER"},
			check: isToken(orgTokenJSON.with(func(tok *tokenJSON) { tok.Role = "DEPLOYMENT_MEMBER" })),
		},
		{
			name: "organization-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, org)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-org", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "organization-token", "remove", "tok-org", d},
			check: removal("tok-org", "org-token", "ORGANIZATION", "removed"),
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
			args:  []string{"token", "workspace-token", "add", "tok-ws", d, "--role", "DEPLOYMENT_MEMBER"},
			check: isToken(wsTokenJSON),
		},
		{
			name: "workspace-token update",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "workspace-token", "update", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN"},
			check: isToken(wsTokenJSON.with(func(tok *tokenJSON) { tok.Role = "DEPLOYMENT_ADMIN" })),
		},
		{
			name: "workspace-token remove",
			client: func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			args:  []string{"token", "workspace-token", "remove", "tok-ws", d},
			check: removal("tok-ws", "ws-token", "WORKSPACE", "removed"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execTokenCmd(t, tc.client(t), "", append(tc.args, "-o", "json")...)
			require.NoError(t, r.err)
			assert.Equal(t, 0, r.code)
			tc.check(t, r.stdout)
			assert.Empty(t, r.stderr)
		})
	}
}

// A list never carries a secret, even when the API sent one: no element has
// a token key at all.
func TestDeploymentTokenListCarriesNoSecret(t *testing.T) {
	dep, _, _ := jsonTokenFixtures()
	r := execTokenCmd(t, tokenMock(t, *withSecret(dep)), "", "token", "list", "--deployment="+tokDeploymentID, "-o", "json")
	require.NoError(t, r.err)
	var got struct {
		Tokens []map[string]json.RawMessage `json:"tokens"`
	}
	decodeOne(t, r.stdout, &got)
	require.Len(t, got.Tokens, 1)
	assert.NotContains(t, got.Tokens[0], "token")
	assert.NotContains(t, r.stdout, tokSecret)
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
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Equal(t, cliout.ExitFailure, got.Code, "the code it reports is the one it exits with")
			assert.Contains(t, got.Error, tc.answered)
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
		assert.Equal(t, cliout.ExitUsage, r.code, args)
	}
}

// An update changes only what it was given. Without --role it sends no role
// change at all, and --role has no default for a help text to misstate.
func TestDeploymentTokenUpdateWithoutRole(t *testing.T) {
	dep, _, _ := jsonTokenFixtures()
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
				var got tokenJSON
				decodeOne(t, r.stdout, &got)
				assert.Equal(t, "DEPLOYMENT_ADMIN", got.Role, "the role it kept")
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
		assert.Equal(t, cliout.ExitFailure, r.code)
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
