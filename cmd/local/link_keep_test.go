package local

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --replace without --workspace keeps a workspace the link sets itself, in a
// project with a workspace of its own and in one with none, where dropping it
// would leave a link the parser refuses.
func TestLinkAddReplaceKeepsTheLinksOwnWorkspace(t *testing.T) {
	for name, top := range map[string]string{
		"project workspace":    "workspace = 'ws_A'\n",
		"no project workspace": "",
	} {
		t.Run(name, func(t *testing.T) {
			body := "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n" + top +
				"\n[tool.astro.deployments.prod]\ndeployment = 'd1'\nworkspace = 'ws_B'\n"
			dir, path := linkTestProject(t, body)
			var res linkResult
			linkJSON(t, dir, &res, "add", "prod", "--deployment", "d2", "--replace")
			assert.Equal(t, linkStatusReplaced, res.Status)
			prod := loadManifest(t, path).Astro.Deployments["prod"]
			assert.Equal(t, "d2", prod.Deployment)
			assert.Equal(t, "ws_B", prod.Workspace, "the replace moved the link off its own workspace")
		})
	}
}

// A credential flag is refused unless the chosen auth method reads it, since
// the writer would drop it from a link reported as added.
func TestLinkAddRefusesACredentialFlagTheMethodDoesNotRead(t *testing.T) {
	const url = "https://airflow.example.com"
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--token-env", "TOK"}, "--token-env needs --auth"},
		{[]string{"--auth", "token", "--token-env", "TOK", "--username-env", "U"}, "--username-env does not apply to the token auth method"},
		{[]string{"--auth", "none", "--password-env", "P"}, "--password-env does not apply to the none auth method"},
		{[]string{"--auth", "basic", "--username-env", "U", "--password-env", "P", "--client-id-env", "C"}, "--client-id-env does not apply to the basic auth method"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			dir, _ := linkTestProject(t, linkManifest)
			_, _, err := runLink(t, dir, append([]string{"add", "oss", "--url", url}, tc.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, linkManifest, readManifest(t, dir))
		})
	}

	for _, args := range [][]string{
		{"--auth", "basic", "--username-env", "U", "--password-env", "P"},
		{"--auth", "airflow-token", "--client-id-env", "C", "--client-secret-env", "S"},
		{"--auth", "airflow-token", "--username-env", "U", "--password-env", "P"},
	} {
		dir, _ := linkTestProject(t, linkManifest)
		_, _, err := runLink(t, dir, append([]string{"add", "oss", "--url", url}, args...)...)
		require.NoError(t, err, "%v", args)
	}
}

// Without --domain, a committed domain is kept rather than replaced by the
// current login's host.
func TestLinkWorkspaceKeepsTheCommittedDomain(t *testing.T) {
	body := "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws_A'\ndomain = 'astronomer-dev.io'\n"
	dir, path := linkTestProject(t, body)
	d, out, _ := linkDeps(t, dir)
	d.LoginDomain = func() (string, error) { return "astronomer.io", nil }
	require.NoError(t, execute(t, d, "link", "workspace", "ws_B", "--output", "json"))
	var res workspaceLinkResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &res))
	assert.Equal(t, "astronomer-dev.io", res.Domain)
	assert.Equal(t, "astronomer-dev.io", loadManifest(t, path).Astro.Domain)

	// Re-linking the same workspace with no login needs none: the domain is in
	// the file, and nothing changes.
	after := readManifest(t, dir)
	linkJSON(t, dir, &res, "workspace", "ws_B")
	assert.Equal(t, linkStatusUnchanged, res.Status)
	assert.Equal(t, after, readManifest(t, dir))
}

// Re-linking the workspace a project already links with no committed domain
// writes nothing and needs no login: the link reads as the default host, and
// the login's host written over it would move it.
func TestLinkWorkspaceRelinkWithNoCommittedDomainIsANoOp(t *testing.T) {
	body := "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws_A'\n"
	dir, _ := linkTestProject(t, body)
	var res workspaceLinkResult
	linkJSON(t, dir, &res, "workspace", "ws_A")
	assert.Equal(t, linkStatusUnchanged, res.Status)
	assert.Equal(t, "astronomer.io", res.Domain)
	assert.Equal(t, body, readManifest(t, dir))

	// A different workspace still needs a domain from somewhere.
	_, _, err := runLink(t, dir, "workspace", "ws_B")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--domain")
}
