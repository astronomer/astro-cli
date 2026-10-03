package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// linkManifest is a project a person wrote: a comment above [tool.astro], a
// linked workspace, one link under its own header and one inline, and a
// commented Composer section.
const linkManifest = `[project]
name = 'orders'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

# the orders team's project
[tool.astro]
workspace = 'ws_A'
domain = 'astronomer.io'

[tool.astro.deployments]
stage = { deployment = 'dep-stage' } # the staging Deployment

[tool.astro.deployments.dev]
# the one everybody queries
deployment = 'dep-dev'
default = true

# where our Composer environments live
[tool.astro.targets.composer]
project = 'acme-data'
location = 'us-central1'
`

// linkComments are the comments in linkManifest an edit elsewhere must keep.
var linkComments = []string{
	"# the orders team's project",
	"# the staging Deployment",
	"# where our Composer environments live",
}

func linkTestProject(t *testing.T, body string) (dir, path string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "orders")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path = filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	t.Setenv("ASTRO_DOMAIN", "")
	t.Setenv(instances.EnvVar, "")
	return dir, path
}

func linkDeps(t *testing.T, dir string) (d Deps, stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	d, _ = testDeps(t)
	d.Stdout, d.Stderr = stdout, stderr
	d.WorkingDir = func() (string, error) { return dir, nil }
	return d, stdout, stderr
}

// runLink runs one `astro link` command and returns what it printed.
func runLink(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	d, out, errOut := linkDeps(t, dir)
	err = execute(t, d, append([]string{"link"}, args...)...)
	return out.String(), errOut.String(), err
}

// linkJSON runs one `astro link` command with --output json, requires it to
// succeed, and decodes what it printed into v, which it zeroes first so a
// field the output omits does not keep an earlier call's value.
func linkJSON(t *testing.T, dir string, v any, args ...string) {
	t.Helper()
	reflect.ValueOf(v).Elem().SetZero()
	out, _, err := runLink(t, dir, append(args, "--output", "json")...)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), v), "decode %q", out)
}

func loadManifest(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(path)
	require.NoError(t, err)
	return m
}

func assertCommentsKept(t *testing.T, path string) {
	t.Helper()
	body := readManifest(t, filepath.Dir(path))
	for _, c := range linkComments {
		assert.Contains(t, body, c)
	}
}

// An astro link in the project's workspace does not repeat it, so it follows
// the project; one in another workspace states its own.
func TestLinkAddAnAstroLink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res linkResult
	linkJSON(t, dir, &res, "add", "prod", "--deployment", "dep-prod")
	assert.Equal(t, linkResult{Name: "prod", Kind: "astro", Status: linkStatusAdded, Manifest: path}, res)
	m := loadManifest(t, path)
	assert.Equal(t, "ws_A", m.Astro.Deployments["prod"].Workspace)
	assert.Equal(t, 1, strings.Count(readManifest(t, dir), "'ws_A'"), "the link repeated the project's workspace")
	assertCommentsKept(t, path)

	linkJSON(t, dir, &res, "add", "other", "--deployment", "dep-other", "--workspace", "ws_Z")
	assert.Equal(t, "ws_Z", loadManifest(t, path).Astro.Deployments["other"].Workspace)
}

// A name already linked is refused unless --replace, and a replace keeps the
// default mark.
func TestLinkAddRefusesAnExistingNameUnlessReplacing(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	_, _, err := runLink(t, dir, "add", "dev", "--deployment", "dep-new")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--replace")
	assert.Equal(t, linkManifest, readManifest(t, dir))

	var res linkResult
	linkJSON(t, dir, &res, "add", "dev", "--deployment", "dep-new", "--replace")
	assert.Equal(t, linkStatusReplaced, res.Status)
	dev := loadManifest(t, path).Astro.Deployments["dev"]
	assert.Equal(t, "dep-new", dev.Deployment)
	assert.True(t, dev.Default, "the replace dropped the default")
}

// An MWAA link writes the shared region when given one, and an empty --region
// removes it.
func TestLinkAddAnMWAALink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res linkResult
	linkJSON(t, dir, &res, "add", "aws", "--target", "mwaa", "--environment", "orders-prod", "--region", "eu-west-1")
	assert.Equal(t, "mwaa", res.Kind)
	m := loadManifest(t, path)
	assert.Equal(t, manifest.KindMWAA, m.Astro.Deployments["aws"].Kind())
	assert.Equal(t, "eu-west-1", m.Astro.Targets["mwaa"]["region"])

	linkJSON(t, dir, &res, "add", "aws", "--target", "mwaa", "--environment", "orders-prod", "--replace")
	assert.Equal(t, "eu-west-1", loadManifest(t, path).Astro.Targets["mwaa"]["region"], "a save with no --region cleared it")

	linkJSON(t, dir, &res, "add", "aws", "--target", "mwaa", "--environment", "orders-prod", "--region", "", "--replace")
	assert.Nil(t, loadManifest(t, path).Astro.Targets["mwaa"]["region"])
	assertCommentsKept(t, path)
}

// A Composer link takes the project's shared coordinates when it names none,
// and one that gives its own moves them, leaf by leaf.
func TestLinkAddAComposerLink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res linkResult
	linkJSON(t, dir, &res, "add", "gcp", "--target", "composer", "--environment", "orders")
	assert.Equal(t, "composer", res.Kind)
	m := loadManifest(t, path)
	assert.Equal(t, "acme-data", m.Astro.Targets["composer"]["project"])

	linkJSON(t, dir, &res, "add", "gcp2", "--target", "composer", "--environment", "orders2", "--location", "europe-west1")
	m = loadManifest(t, path)
	assert.Equal(t, "acme-data", m.Astro.Targets["composer"]["project"])
	assert.Equal(t, "europe-west1", m.Astro.Targets["composer"]["location"])
	assertCommentsKept(t, path)

	// With no section to take them from, both are required.
	bare, _ := linkTestProject(t, "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n")
	_, _, err := runLink(t, bare, "add", "gcp", "--target", "composer", "--environment", "orders")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Google Cloud project")
}

// A url link names its auth method and the env vars its credentials are read
// from, and an exec command comes after --.
func TestLinkAddAURLLink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res linkResult
	linkJSON(t, dir, &res, "add", "oss", "--url", "https://airflow.example.com", "--auth", "token", "--token-env", "AIRFLOW_TOKEN")
	assert.Equal(t, "endpoint", res.Kind)
	oss := loadManifest(t, path).Astro.Deployments["oss"]
	assert.Equal(t, manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AIRFLOW_TOKEN"}, oss.Auth)
	assert.NotContains(t, readManifest(t, dir), "target = 'astro'", "an endpoint that inherits astro wrote it")

	_, _, err := runLink(t, dir, "add", "iap", "--url", "https://iap.example.com", "--auth", "exec", "--", "gcloud", "auth", "print-identity-token")
	require.NoError(t, err)
	assert.Equal(t, []string{"gcloud", "auth", "print-identity-token"}, loadManifest(t, path).Astro.Deployments["iap"].Auth.Command)
}

// A credential pasted where an env var name belongs is refused, and appears in
// neither output stream nor the file, in text or json.
func TestLinkAddNeverEchoesAPastedCredential(t *testing.T) {
	const secret = "hunter2-s3cr3t.v4lu3"
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			dir, _ := linkTestProject(t, linkManifest)
			out, errOut, err := runLink(t, dir, "add", "oss", "--url", "https://airflow.example.com",
				"--auth", "basic", "--username-env", "AF_USER", "--password-env", secret, "--output", format)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "NAME of an environment variable")
			for _, s := range []string{err.Error(), out, errOut, readManifest(t, dir)} {
				assert.NotContains(t, s, secret)
			}
		})
	}
}

// A flag that does not belong to the kind the others pick is refused rather
// than dropped, and nothing is written.
func TestLinkAddRefusesAFlagTheKindDoesNotTake(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--deployment", "d", "--region", "eu-west-1"}, "--region does not apply to an astro link"},
		{[]string{"--target", "mwaa", "--environment", "e", "--deployment", "d"}, "--deployment does not apply to an mwaa link"},
		{[]string{"--target", "composer", "--environment", "e", "--region", "r"}, "--region does not apply to a composer link"},
		{[]string{"--url", "https://a.example.com", "--auth", "none", "--workspace", "w"}, "--workspace does not apply to a url link"},
		{[]string{"--url", "https://a.example.com", "--target", "mwaa"}, "cannot be combined"},
		{[]string{"--environment", "e"}, "pass --target mwaa or --target composer"},
		{[]string{"--target", "endpoint", "--deployment", "d"}, "must be astro, mwaa or composer"},
		{[]string{"--deployment", "d", "--", "tool"}, "is for --auth exec"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			dir, _ := linkTestProject(t, linkManifest)
			_, _, err := runLink(t, dir, append([]string{"add", "x"}, tc.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, linkManifest, readManifest(t, dir))
		})
	}
}

// In a project whose target is mwaa, an environment alone makes an mwaa link
// and a deployment makes an astro one, which then states its target.
func TestLinkAddFollowsTheProjectTarget(t *testing.T) {
	body := strings.Replace(linkManifest, "workspace = 'ws_A'\n", "workspace = 'ws_A'\ntarget = 'mwaa'\n", 1)
	body = strings.Replace(body, "stage = { deployment = 'dep-stage' }", "stage = { deployment = 'dep-stage', target = 'astro' }", 1)
	body = strings.Replace(body, "deployment = 'dep-dev'\n", "deployment = 'dep-dev'\ntarget = 'astro'\n", 1)
	dir, path := linkTestProject(t, body)
	var res linkResult
	linkJSON(t, dir, &res, "add", "aws", "--environment", "orders")
	assert.Equal(t, "mwaa", res.Kind)
	linkJSON(t, dir, &res, "add", "prod", "--deployment", "dep-prod")
	assert.Equal(t, "astro", res.Kind)
	m := loadManifest(t, path)
	assert.Equal(t, manifest.KindMWAA, m.Astro.Deployments["aws"].Kind())
	assert.Equal(t, manifest.KindAstro, m.Astro.Deployments["prod"].Kind())
}

// Remove takes a link out in either spelling, refuses a name that is not
// there, and says when the user's pin still names the removed link.
func TestLinkRemove(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	require.NoError(t, savePin(dir, "stage"))
	var res linkResult
	d, out, errOut := linkDeps(t, dir)
	require.NoError(t, execute(t, d, "link", "remove", "stage", "--output", "json"))
	require.NoError(t, json.Unmarshal(out.Bytes(), &res))
	assert.Equal(t, linkResult{Name: "stage", Kind: "astro", Status: linkStatusRemoved, Manifest: path}, res)
	assert.Contains(t, errOut.String(), "astro use --unset")
	m := loadManifest(t, path)
	assert.NotContains(t, m.Astro.Deployments, "stage")
	assert.Contains(t, m.Astro.Deployments, "dev")
	assert.Contains(t, readManifest(t, dir), "# the orders team's project")

	linkJSON(t, dir, &res, "remove", "dev")
	assert.NotContains(t, loadManifest(t, path).Astro.Deployments, "dev")

	before := readManifest(t, dir)
	_, _, err := runLink(t, dir, "remove", "dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no link named dev")
	assert.Equal(t, before, readManifest(t, dir))

	state, err := userstate.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "stage", state.Instance, "remove changed the user's pin")
}

// Default moves the mark, clears it with --unset, and refuses a name that is
// not linked.
func TestLinkDefault(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res linkResult
	linkJSON(t, dir, &res, "default", "stage")
	assert.Equal(t, linkResult{Name: "stage", Kind: "astro", Status: linkStatusDefault, Manifest: path}, res)
	m := loadManifest(t, path)
	assert.True(t, m.Astro.Deployments["stage"].Default)
	assert.False(t, m.Astro.Deployments["dev"].Default)
	assertCommentsKept(t, path)

	linkJSON(t, dir, &res, "default", "stage")
	assert.Equal(t, linkStatusUnchanged, res.Status)

	linkJSON(t, dir, &res, "default", "--unset")
	assert.Equal(t, linkResult{Status: linkStatusCleared, Manifest: path}, res)
	for name, l := range loadManifest(t, path).Astro.Deployments {
		assert.False(t, l.Default, "%s is still the default", name)
	}

	before := readManifest(t, dir)
	_, _, err := runLink(t, dir, "default", "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no link named nope")
	assert.Equal(t, before, readManifest(t, dir))

	_, _, err = runLink(t, dir, "default")
	require.Error(t, err)
}

// Outside a project there is nothing to link, and no manifest is created.
func TestLinkOutsideAProject(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runLink(t, dir, "add", "prod", "--deployment", "d", "--workspace", "w")
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "pyproject.toml"))
	assert.True(t, os.IsNotExist(statErr))
}
