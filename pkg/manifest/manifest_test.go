package manifest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pyproject.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// validationError asserts that err is a *ValidationError and returns it.
func validationError(t *testing.T, err error) *ValidationError {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ValidationError, got %T: %v", err, err)
	}
	return ve
}

// problemKeys is what the validation tests assert on: every problem is
// addressed by the dotted TOML key it concerns, and that key is the contract.
func problemKeys(ve *ValidationError) []string {
	var keys []string
	for _, p := range ve.Problems {
		keys = append(keys, p.Key)
	}
	return keys
}

const full = `
[project]
name = "my-pipelines"
requires-python = ">=3.11"
dependencies = ["pandas>=2.1", "apache-airflow-providers-snowflake"]

[tool.astro]
airflow = "3.1"
packages = ["libpq-dev", "build-essential"]

[tool.astro.targets.astro]
image = { os = "ubi", python = "3.12" }
system-packages = ["libaio"]

[tool.astro.deployments.preview]
target = "astro"
workspace = "ws-abc"
deployment = "dep-preview"

[tool.astro.deployments.prod]
target = "astro"
workspace = "ws-abc"
deployment = "dep-xyz"

[tool.astro.env.connections.warehouse]
conn_type = "snowflake"
required = true
`

func TestLoadFull(t *testing.T) {
	m, err := Load(write(t, full))
	if err != nil {
		t.Fatal(err)
	}

	wantProject := Project{
		Name:           "my-pipelines",
		RequiresPython: ">=3.11",
		Dependencies:   []string{"pandas>=2.1", "apache-airflow-providers-snowflake"},
	}
	if !reflect.DeepEqual(m.Project, wantProject) {
		t.Errorf("Project = %#v, want %#v", m.Project, wantProject)
	}

	if m.Astro.AirflowVersion != "3.1" {
		t.Errorf("AirflowVersion = %q, want %q", m.Astro.AirflowVersion, "3.1")
	}

	wantPackages := []string{"libpq-dev", "build-essential"}
	if !reflect.DeepEqual(m.Astro.Packages, wantPackages) {
		t.Errorf("Packages = %#v, want %#v", m.Astro.Packages, wantPackages)
	}

	wantLinks := map[string]Link{
		"preview": {Target: "astro", Workspace: "ws-abc", Deployment: "dep-preview", Auth: Auth{Method: AuthAstro}},
		"prod":    {Target: "astro", Workspace: "ws-abc", Deployment: "dep-xyz", Auth: Auth{Method: AuthAstro}},
	}
	if !reflect.DeepEqual(m.Astro.Deployments, wantLinks) {
		t.Errorf("Deployments = %#v, want %#v", m.Astro.Deployments, wantLinks)
	}

	wantTarget := map[string]any{
		"image":           map[string]any{"os": "ubi", "python": "3.12"},
		"system-packages": []any{"libaio"},
	}
	if !reflect.DeepEqual(m.Astro.Targets["astro"], wantTarget) {
		t.Errorf("Targets[astro] = %#v, want %#v", m.Astro.Targets["astro"], wantTarget)
	}

	wantEnv := map[string]any{
		"connections": map[string]any{
			"warehouse": map[string]any{"conn_type": "snowflake", "required": true},
		},
	}
	if !reflect.DeepEqual(m.Astro.Env, wantEnv) {
		t.Errorf("Env = %#v, want %#v", m.Astro.Env, wantEnv)
	}
}

func TestLoadMinimal(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\n\n[tool.astro]\nairflow = \"3\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Project.Name != "etl" || m.Astro.AirflowVersion != "3" {
		t.Errorf("got %#v", m)
	}
	if m.Astro.Deployments != nil || m.Astro.Env != nil || m.Astro.Targets != nil || m.Astro.Packages != nil {
		t.Errorf("absent sections should stay nil, got %#v", m.Astro)
	}
}

func TestResolveDefaults(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		wantWorkspace string // top-level Astro.Workspace
		wantTarget    string // top-level Astro.Target
		wantLinks     map[string]Link
	}{
		{
			name: "top-level workspace inherited, implicit astro target",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-default"

[tool.astro.deployments.prod]
deployment = "dep-prod"
`,
			wantWorkspace: "ws-default",
			wantLinks: map[string]Link{
				"prod": {Target: "astro", Workspace: "ws-default", Deployment: "dep-prod", Auth: Auth{Method: AuthAstro}},
			},
		},
		{
			name: "link workspace overrides the top-level default",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-default"

[tool.astro.deployments.prod]
workspace = "ws-own"
deployment = "dep-prod"
`,
			wantWorkspace: "ws-default",
			wantLinks: map[string]Link{
				"prod": {Target: "astro", Workspace: "ws-own", Deployment: "dep-prod", Auth: Auth{Method: AuthAstro}},
			},
		},
		{
			name: "top-level target inherited, link target wins, workspace stays astro-only",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-default"
target = "mwaa"

[tool.astro.deployments.cloud]
target = "astro"
deployment = "dep-cloud"

[tool.astro.deployments.aws]
environment = "orders-prod"
`,
			wantWorkspace: "ws-default",
			wantTarget:    "mwaa",
			wantLinks: map[string]Link{
				"cloud": {Target: "astro", Workspace: "ws-default", Deployment: "dep-cloud", Auth: Auth{Method: AuthAstro}},
				"aws":   {Target: "mwaa", Environment: "orders-prod", Auth: Auth{Method: AuthAWS}},
			},
		},
		{
			name: "minimal link is deployment plus a workspace default",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-default"

[tool.astro.deployments.prod]
deployment = "dep-prod"
default = true
`,
			wantWorkspace: "ws-default",
			wantLinks: map[string]Link{
				"prod": {Target: "astro", Workspace: "ws-default", Deployment: "dep-prod", Default: true, Auth: Auth{Method: AuthAstro}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(write(t, tc.content))
			if err != nil {
				t.Fatal(err)
			}
			if m.Astro.Workspace != tc.wantWorkspace {
				t.Errorf("Astro.Workspace = %q, want %q", m.Astro.Workspace, tc.wantWorkspace)
			}
			if m.Astro.Target != tc.wantTarget {
				t.Errorf("Astro.Target = %q, want %q", m.Astro.Target, tc.wantTarget)
			}
			if !reflect.DeepEqual(m.Astro.Deployments, tc.wantLinks) {
				t.Errorf("Deployments = %#v, want %#v", m.Astro.Deployments, tc.wantLinks)
			}
		})
	}
}

// kindsManifest carries one link of every kind, as docs/v2-instances.md
// spells them.
const kindsManifest = `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.dev]
deployment = "dep-dev"
default = true

[tool.astro.deployments.prod-mwaa]
target = "mwaa"
environment = "orders-prod"

[tool.astro.deployments.prod-composer]
target = "composer"
environment = "orders-prod"

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev"
auth = { method = "token", token-env = "STAGING_AIRFLOW_TOKEN" }
`

func TestLinkKinds(t *testing.T) {
	m, err := Load(write(t, kindsManifest))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		kind LinkKind
		auth Auth
	}{
		"dev":           {KindAstro, Auth{Method: AuthAstro}},
		"prod-mwaa":     {KindMWAA, Auth{Method: AuthAWS}},
		"prod-composer": {KindComposer, Auth{Method: AuthGoogle}},
		"staging":       {KindEndpoint, Auth{Method: AuthToken, TokenEnv: "STAGING_AIRFLOW_TOKEN"}},
	}
	if len(m.Astro.Deployments) != len(want) {
		t.Fatalf("got %d links, want %d", len(m.Astro.Deployments), len(want))
	}
	for name, w := range want {
		link := m.Astro.Deployments[name]
		if link.Kind() != w.kind {
			t.Errorf("%s: Kind() = %q, want %q", name, link.Kind(), w.kind)
		}
		if !reflect.DeepEqual(link.Auth, w.auth) {
			t.Errorf("%s: Auth = %#v, want %#v", name, link.Auth, w.auth)
		}
	}
	if url := m.Astro.Deployments["staging"].URL; url != "https://airflow.staging.corp.dev" {
		t.Errorf("staging URL = %q", url)
	}
	if env := m.Astro.Deployments["prod-mwaa"].Environment; env != "orders-prod" {
		t.Errorf("prod-mwaa Environment = %q", env)
	}
	// Only an astro link is in a workspace, whatever [tool.astro] sets.
	for _, name := range []string{"prod-mwaa", "prod-composer", "staging"} {
		if ws := m.Astro.Deployments[name].Workspace; ws != "" {
			t.Errorf("%s: Workspace = %q, want empty", name, ws)
		}
	}
}

// endpointLink is one url link with the auth table under test; url links are
// where every auth method is legal, so one shape covers the menu.
func endpointLink(auth string) string {
	return "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\n\n" +
		"[tool.astro.deployments.staging]\nurl = \"https://airflow.corp.dev\"\n" + auth + "\n"
}

// Every method on the menu, parsed with the fields it takes. A url link
// carries them here because it is the kind that must name one, but the method
// is its own axis and any of these attaches to any kind.
func TestAuthMethods(t *testing.T) {
	cases := []struct {
		name string
		auth string
		want Auth
	}{
		{
			name: "token",
			auth: `auth = { method = "token", token-env = "AIRFLOW_TOKEN" }`,
			want: Auth{Method: AuthToken, TokenEnv: "AIRFLOW_TOKEN"},
		},
		{
			name: "basic",
			auth: `auth = { method = "basic", username-env = "AF_USER", password-env = "AF_PASSWORD" }`,
			want: Auth{Method: AuthBasic, UsernameEnv: "AF_USER", PasswordEnv: "AF_PASSWORD"},
		},
		{
			name: "airflow-token with client credentials",
			auth: `auth = { method = "airflow-token", client-id-env = "AF_CLIENT_ID", client-secret-env = "AF_CLIENT_SECRET" }`,
			want: Auth{Method: AuthAirflowToken, ClientIDEnv: "AF_CLIENT_ID", ClientSecretEnv: "AF_CLIENT_SECRET"},
		},
		{
			name: "airflow-token with a username and password",
			auth: `auth = { method = "airflow-token", username-env = "AF_USER", password-env = "AF_PASSWORD" }`,
			want: Auth{Method: AuthAirflowToken, UsernameEnv: "AF_USER", PasswordEnv: "AF_PASSWORD"},
		},
		{
			name: "exec, argv style",
			auth: `auth = { method = "exec", command = ["acme-airflow-token", "--profile", "prod"] }`,
			want: Auth{Method: AuthExec, Command: []string{"acme-airflow-token", "--profile", "prod"}},
		},
		{
			name: "google, on a self-hosted Airflow behind IAP",
			auth: `auth = { method = "google" }`,
			want: Auth{Method: AuthGoogle},
		},
		{
			name: "none, for an open dev server",
			auth: `auth = { method = "none" }`,
			want: Auth{Method: AuthNone},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(write(t, endpointLink(tc.auth)))
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Astro.Deployments["staging"].Auth; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Auth = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// An auth table on a link that has a default replaces it, and says nothing
// about the link's kind.
func TestAuthOverridesKindDefault(t *testing.T) {
	m, err := Load(write(t, `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.dev]
deployment = "dep-dev"
auth = { method = "none" }
`))
	if err != nil {
		t.Fatal(err)
	}
	link := m.Astro.Deployments["dev"]
	if link.Kind() != KindAstro {
		t.Errorf("Kind() = %q, want %q", link.Kind(), KindAstro)
	}
	if link.Auth.Method != AuthNone {
		t.Errorf("Auth.Method = %q, want %q", link.Auth.Method, AuthNone)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "pyproject.toml"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want fs.ErrNotExist, got %v", err)
	}
}

func TestLoadBadTOML(t *testing.T) {
	path := write(t, "[project\nname=")
	_, err := Load(path)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Path != path {
		t.Errorf("ParseError.Path = %q, want %q", pe.Path, path)
	}
}

func TestLoadNoAstroSection(t *testing.T) {
	_, err := Load(write(t, "[project]\nname = \"plain-python\"\n"))
	if !errors.Is(err, ErrNoAstroSection) {
		t.Errorf("want ErrNoAstroSection, got %v", err)
	}
}

func TestValidation(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantKeys []string
	}{
		{
			name:     "missing project name",
			content:  "[tool.astro]\nairflow = \"3.1\"\n",
			wantKeys: []string{"project.name"},
		},
		{
			name:     "bad project name",
			content:  "[project]\nname = \"-bad-\"\n\n[tool.astro]\nairflow = \"3.1\"\n",
			wantKeys: []string{"project.name"},
		},
		{
			name:     "missing airflow",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\ndeployments = {}\n",
			wantKeys: []string{"tool.astro.airflow"},
		},
		{
			name:     "bad airflow version",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"three\"\n",
			wantKeys: []string{"tool.astro.airflow"},
		},
		{
			name:     "empty package entry",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\npackages = [\"libpq-dev\", \"  \"]\n",
			wantKeys: []string{"tool.astro.packages[1]"},
		},
		{
			name:     "unknown key in [tool.astro]",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\nairflw = \"3.1\"\n",
			wantKeys: []string{"tool.astro.airflw"},
		},
		{
			name: "unknown key on a link",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
defaults = true
`,
			wantKeys: []string{"tool.astro.deployments.prod.defaults"},
		},
		{
			name: "incomplete deployment",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "astro"
`,
			wantKeys: []string{
				"tool.astro.deployments.prod.deployment",
				"tool.astro.deployments.prod.workspace",
			},
		},
		{
			name:    "several at once",
			content: "[tool.astro]\nairflow = \"v3\"\n\n[tool.astro.deployments.d]\nworkspace = \"w\"\ndeployment = \"x\"\n",
			wantKeys: []string{
				"project.name",
				"tool.astro.airflow",
			},
		},
		{
			name: "missing workspace at both levels",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments.prod.workspace"},
		},
		{
			name: "empty per-link target rejected",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.prod]
target = ""
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments.prod.target"},
		},
		{
			name: "empty top-level target rejected",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
target = ""
`,
			wantKeys: []string{"tool.astro.target"},
		},
		{
			name: "target as a table is the old spelling of targets",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.target.astro]
image = { os = "ubi" }
`,
			wantKeys: []string{"tool.astro.target"},
		},
		{
			name: "target a link cannot use",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.prod]
target = "MWAA"
environment = "orders-prod"
`,
			wantKeys: []string{"tool.astro.deployments.prod.target"},
		},
		{
			name: "link inherits a target it cannot use",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"
target = "oss"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments.prod.target"},
		},
		{
			name: "two links marked default",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-prod"
default = true

[tool.astro.deployments.dev]
deployment = "dep-dev"
default = true
`,
			wantKeys: []string{"tool.astro.deployments"},
		},
		{
			name: "a link sets both a url and coordinates",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.mixed]
url = "https://airflow.corp.dev"
deployment = "dep-xyz"
auth = { method = "none" }
`,
			wantKeys: []string{"tool.astro.deployments.mixed"},
		},
		{
			name: "an mwaa link with a url",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "mwaa"
url = "https://airflow.corp.dev"
auth = { method = "aws" }
`,
			wantKeys: []string{"tool.astro.deployments.prod"},
		},
		{
			name: "environment on an astro link",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
environment = "orders-prod"
`,
			wantKeys: []string{"tool.astro.deployments.prod.environment"},
		},
		{
			name: "deployment id on an mwaa link",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "mwaa"
environment = "orders-prod"
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments.prod.deployment"},
		},
		{
			name: "workspace on an mwaa link",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "mwaa"
environment = "orders-prod"
workspace = "ws-abc"
`,
			wantKeys: []string{"tool.astro.deployments.prod.workspace"},
		},
		{
			name: "composer link without an environment",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "composer"
`,
			wantKeys: []string{"tool.astro.deployments.prod.environment"},
		},
		{
			name: "url with no scheme",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "airflow.staging.corp.dev"
auth = { method = "none" }
`,
			wantKeys: []string{"tool.astro.deployments.staging.url"},
		},
		{
			name: "url carrying a username and password",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "https://admin:hunter2@airflow.corp.dev"
auth = { method = "none" }
`,
			wantKeys: []string{"tool.astro.deployments.staging.url"},
		},
		{
			// `local` is the machine's own word — `astro local start`,
			// `astro local dags list`. A link may not take it, so nobody has to
			// work out which one a reader meant.
			name: "a link named local",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.local]
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments.local"},
		},
		{
			name: "a link with no name",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"
workspace = "ws-abc"

[tool.astro.deployments.""]
deployment = "dep-xyz"
`,
			wantKeys: []string{"tool.astro.deployments"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, tc.content)
			_, err := Load(path)
			ve := validationError(t, err)
			if ve.Path != path {
				t.Errorf("ValidationError.Path = %q, want %q", ve.Path, path)
			}
			if keys := problemKeys(ve); !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Errorf("problem keys = %v, want %v", keys, tc.wantKeys)
			}
		})
	}
}

func TestParseWithoutPath(t *testing.T) {
	_, err := Parse([]byte("[tool.astro]\nairflow = \"3.1\"\n"))
	ve := validationError(t, err)
	if ve.Path != "" {
		t.Errorf("Path should be empty for Parse, got %q", ve.Path)
	}
}

// The error lists every problem, because a caller that prints it is how the
// author of the file finds out what to fix.
func TestValidationErrorListsEveryProblem(t *testing.T) {
	_, err := Load(write(t, `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "astro"
`))
	ve := validationError(t, err)
	got := ve.Error()
	for _, want := range []string{
		"2 problems",
		"tool.astro.deployments.prod.deployment: required on an astro link",
		"tool.astro.deployments.prod.workspace: no workspace",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error text %q should contain %q", got, want)
		}
	}
}

func TestAuthValidation(t *testing.T) {
	const key = "tool.astro.deployments.staging.auth"
	cases := []struct {
		name       string
		auth       string
		wantKeys   []string
		wantReason string // substring of the first problem's reason
	}{
		{
			name:       "a url link with no auth table at all",
			auth:       "",
			wantKeys:   []string{key},
			wantReason: "required on a url link",
		},
		{
			name:       "auth is not a table",
			auth:       `auth = "token"`,
			wantKeys:   []string{key},
			wantReason: "must be a table",
		},
		{
			name:       "auth table names no method",
			auth:       `auth = { token-env = "AIRFLOW_TOKEN" }`,
			wantKeys:   []string{key + ".method"},
			wantReason: "required",
		},
		{
			name:       "unknown method",
			auth:       `auth = { method = "kerberos" }`,
			wantKeys:   []string{key + ".method"},
			wantReason: `"kerberos" is not an auth method`,
		},
		{
			name:       "field on the wrong method",
			auth:       `auth = { method = "basic", username-env = "U", password-env = "P", token-env = "T" }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "not a field of the basic method",
		},
		{
			name:       "field on a method that takes none",
			auth:       `auth = { method = "google", token-env = "T" }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "not a field of the google method",
		},
		{
			name:       "unknown field",
			auth:       `auth = { method = "token", token-env = "T", audience = "airflow" }`,
			wantKeys:   []string{key + ".audience"},
			wantReason: "not a field of the token method",
		},
		{
			name:       "token method without its env var",
			auth:       `auth = { method = "token" }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "required by the token method",
		},
		{
			name:     "basic method missing both env vars",
			auth:     `auth = { method = "basic" }`,
			wantKeys: []string{key + ".password-env", key + ".username-env"},
		},
		{
			name:       "a literal secret where an env-var name belongs",
			auth:       `auth = { method = "token", token-env = "eyJhbGciOi.J9 secret" }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "is not an env-var name",
		},
		{
			name:       "empty field value",
			auth:       `auth = { method = "token", token-env = "" }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "must not be empty",
		},
		{
			name:       "field value of the wrong type",
			auth:       `auth = { method = "token", token-env = 7 }`,
			wantKeys:   []string{key + ".token-env"},
			wantReason: "expected a string",
		},
		{
			name:       "exec command as a string",
			auth:       `auth = { method = "exec", command = "acme-airflow-token --profile prod" }`,
			wantKeys:   []string{key + ".command"},
			wantReason: "expected an array, argv style",
		},
		{
			name:       "exec command with nothing in it",
			auth:       `auth = { method = "exec", command = [] }`,
			wantKeys:   []string{key + ".command"},
			wantReason: "must name a program to run",
		},
		{
			name:       "exec command with a non-string argument",
			auth:       `auth = { method = "exec", command = ["acme-token", 7] }`,
			wantKeys:   []string{key + ".command[1]"},
			wantReason: "expected a non-empty string",
		},
		{
			name:       "half an airflow-token credential pair",
			auth:       `auth = { method = "airflow-token", client-id-env = "AF_CLIENT_ID" }`,
			wantKeys:   []string{key + ".client-secret-env"},
			wantReason: "required alongside client-id-env",
		},
		{
			name:       "airflow-token with both credential pairs",
			auth:       `auth = { method = "airflow-token", client-id-env = "ID", client-secret-env = "SECRET", username-env = "U", password-env = "P" }`,
			wantKeys:   []string{key},
			wantReason: "takes one credential pair, not both",
		},
		{
			name:       "airflow-token with nothing to exchange",
			auth:       `auth = { method = "airflow-token" }`,
			wantKeys:   []string{key},
			wantReason: "needs credentials to exchange",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, endpointLink(tc.auth)))
			ve := validationError(t, err)
			if keys := problemKeys(ve); !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Fatalf("problem keys = %v, want %v", keys, tc.wantKeys)
			}
			if tc.wantReason != "" && !strings.Contains(ve.Problems[0].Reason, tc.wantReason) {
				t.Errorf("first reason = %q, want it to contain %q", ve.Problems[0].Reason, tc.wantReason)
			}
		})
	}
}

func TestAirflowVersions(t *testing.T) {
	good := []string{"3", "3.1", "3.1.2", "2.10"}
	bad := []string{"three", "3.", ".1", "3.1.2.3", "v3", "3.x", ""}
	for _, v := range good {
		if !airflowVersionRe.MatchString(v) {
			t.Errorf("%q should be accepted", v)
		}
	}
	for _, v := range bad {
		if airflowVersionRe.MatchString(v) {
			t.Errorf("%q should be rejected", v)
		}
	}
}

// TestDefaultLink pins the rule both the deploy path and instance resolution
// ask for: the marked link, or a project's only link, and nothing otherwise.
func TestDefaultLink(t *testing.T) {
	marked := map[string]Link{
		"dev":  {Deployment: "clm2xk9dq000108l7a2b3c4d5", Default: true},
		"prod": {Deployment: "clm2xk9dq000108l7a2b3c4d6"},
	}
	if name, link, ok := DefaultLink(marked); !ok || name != "dev" || !link.Default {
		t.Errorf("marked default = %q, %v, %v; want dev", name, link.Default, ok)
	}

	lone := map[string]Link{"only": {Deployment: "clm2xk9dq000108l7a2b3c4d5"}}
	if name, _, ok := DefaultLink(lone); !ok || name != "only" {
		t.Errorf("lone link = %q, %v; want only", name, ok)
	}

	// Several links and none marked: no default, which is what sends a command
	// to its prompt or its error.
	several := map[string]Link{
		"dev":  {Deployment: "clm2xk9dq000108l7a2b3c4d5"},
		"prod": {Deployment: "clm2xk9dq000108l7a2b3c4d6"},
	}
	if name, _, ok := DefaultLink(several); ok {
		t.Errorf("several links defaulted to %q", name)
	}
	if _, _, ok := DefaultLink(nil); ok {
		t.Error("no links produced a default")
	}
}
