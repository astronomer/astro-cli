package local

import (
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
