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
dependencies = ["apache-airflow==3.1.*", "pandas>=2.1", "apache-airflow-providers-snowflake"]

[tool.astro]
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
		Dependencies:   []string{"apache-airflow==3.1.*", "pandas>=2.1", "apache-airflow-providers-snowflake"},
	}
	if !reflect.DeepEqual(m.Project, wantProject) {
		t.Errorf("Project = %#v, want %#v", m.Project, wantProject)
	}

	if got := m.Airflow(); got != (Airflow{Pin: "3.1"}) {
		t.Errorf("Airflow() = %#v, want the requirement's 3.1", got)
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
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\"apache-airflow==3.*\"]\n\n[tool.astro]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Project.Name != "etl" || m.Airflow().Pin != "3" {
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
	return "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n" +
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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

// validationCases is shared with code_test.go's TestEveryProblemCarriesACode,
// so a case added here is a case that check sees too — a rule added without a
// code then fails there rather than shipping silently.
var validationCases = []struct {
	name    string
	content string
	// wantKeys is which key each finding is addressed to, wantCodes which
	// rule raised it — both in the order Load returns them. The key alone
	// does not identify a rule: several keys carry two rules apiece.
	wantKeys  []string
	wantCodes []ProblemCode
}{
	{
		name:      "runtime that is not a runtime tag",
		wantCodes: []ProblemCode{CodeRuntimeInvalid},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\nruntime = 'latest'\n",
		wantKeys:  []string{"tool.astro.runtime"},
	},
	{
		name:      "runtime from another series",
		wantCodes: []ProblemCode{CodeRuntimeMismatch},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\nruntime = '3.2-10'\n",
		wantKeys:  []string{"tool.astro.runtime"},
	},
	{
		name:      "runtime beside a dockerfile",
		wantCodes: []ProblemCode{CodeRuntimeWithDockerfile},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\ndockerfile = 'Dockerfile'\nruntime = '3.3-8'\n",
		wantKeys:  []string{"tool.astro.runtime"},
	},
	{
		name:      "missing project name",
		wantCodes: []ProblemCode{CodeAirflowMissing, CodeRequired},
		content:   "[tool.astro]\n",
		wantKeys:  []string{"project.dependencies", "project.name"},
	},
	{
		name:      "bad project name",
		wantCodes: []ProblemCode{CodeProjectNameInvalid},
		content:   "[project]\nname = \"-bad-\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.name"},
	},
	{
		name:      "no airflow requirement",
		wantCodes: []ProblemCode{CodeAirflowMissing},
		content:   "[project]\nname = \"p\"\ndependencies = [\"pandas\", \"apache-airflow-providers-standard\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies"},
	},
	{
		name:      "airflow requirement with a range",
		wantCodes: []ProblemCode{CodeAirflowUnpinned},
		content:   "[project]\nname = \"p\"\ndependencies = [\"pandas\", \"apache-airflow>=3.1\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies[1]"},
	},
	{
		name:      "airflow requirement with a URL",
		wantCodes: []ProblemCode{CodeAirflowUnpinned},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow @ https://example.com/apache_airflow-3.3.2-py3-none-any.whl\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies[0]"},
	},
	{
		name:      "airflow requirement with no version",
		wantCodes: []ProblemCode{CodeAirflowUnpinned},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow-core\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies[0]"},
	},
	{
		// One entry pinned does not excuse another that is not: standalone
		// installs both, and nothing says which the marker picks.
		name:      "one airflow entry pinned and one a range",
		wantCodes: []ProblemCode{CodeAirflowUnpinned},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*; python_version >= '3.12'\", \"apache-airflow>=3.1; python_version < '3.12'\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies[1]"},
	},
	{
		name:      "both airflow distributions",
		wantCodes: []ProblemCode{CodeAirflowAmbiguous},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\", \"apache-airflow-core==3.3.*\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies"},
	},
	{
		name:      "airflow pinned twice, differently",
		wantCodes: []ProblemCode{CodeAirflowAmbiguous},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*; python_version >= '3.12'\", \"apache-airflow==3.1.*; python_version < '3.12'\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies"},
	},
	{
		name:      "apache-airflow-core pinned to an Airflow 2",
		wantCodes: []ProblemCode{CodeAirflowCoreBeforeThree},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow-core==2.10.*\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies[0]"},
	},
	{
		// With or without a static list beside it, which PEP 621 forbids.
		name:      "dependencies declared dynamic",
		wantCodes: []ProblemCode{CodeAirflowMissing, CodeDependenciesDynamic},
		content:   "[project]\nname = \"p\"\ndynamic = [\"dependencies\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dependencies", "project.dynamic"},
	},
	{
		name:      "dependencies declared dynamic beside a static list",
		wantCodes: []ProblemCode{CodeDependenciesDynamic},
		content:   "[project]\nname = \"p\"\ndynamic = [\"version\", \"dependencies\"]\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n",
		wantKeys:  []string{"project.dynamic"},
	},
	{
		name:      "leftover airflow key",
		wantCodes: []ProblemCode{CodeAirflowRemoved},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nairflow = \"3.1\"\n",
		wantKeys:  []string{"tool.astro.airflow"},
	},
	{
		name:      "leftover airflow key that is not a version",
		wantCodes: []ProblemCode{CodeAirflowRemoved},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nairflow = \"three\"\n",
		wantKeys:  []string{"tool.astro.airflow"},
	},
	{
		// An element of the wrong type and an element that is empty are two
		// different faults, and merging them under one code was the bug that
		// made this case worth writing: a number in the list reported
		// "empty_string", which is not what is wrong with it.
		name:      "package entry of the wrong type",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\npackages = [\"libpq-dev\", 3]\n",
		wantKeys:  []string{"tool.astro.packages[1]"},
		wantCodes: []ProblemCode{CodeExpectedString},
	},
	{
		name:      "empty package entry",
		wantCodes: []ProblemCode{CodeEmptyString},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\npackages = [\"libpq-dev\", \"  \"]\n",
		wantKeys:  []string{"tool.astro.packages[1]"},
	},
	{
		// The consumer joins this to the project dir and hands it to a
		// docker build, so a path climbing out of the project is refused
		// rather than resolved.
		name:      "dockerfile escaping the project",
		wantCodes: []ProblemCode{CodeDockerfileOutsideProject},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = \"../../etc/Dockerfile\"\n",
		wantKeys:  []string{"tool.astro.dockerfile"},
	},
	{
		name:      "absolute dockerfile path",
		wantCodes: []ProblemCode{CodeDockerfileOutsideProject},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = \"/etc/Dockerfile\"\n",
		wantKeys:  []string{"tool.astro.dockerfile"},
	},
	{
		// Works on Windows, is one filename with a backslash in it
		// everywhere else. Refused on every platform so the error lands on
		// the machine that wrote it, not on a colleague who pulled it.
		name:      "dockerfile with windows separators",
		wantCodes: []ProblemCode{CodeDockerfileSeparators},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = 'docker\\Dockerfile'\n",
		wantKeys:  []string{"tool.astro.dockerfile"},
	},
	{
		name:      "dockerfile of the wrong shape",
		wantCodes: []ProblemCode{CodeExpectedString},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = 3\n",
		wantKeys:  []string{"tool.astro.dockerfile"},
	},
	{
		name:      "build-secrets without a dockerfile",
		wantCodes: []ProblemCode{CodeBuildSecretsWithoutDockerfile},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nbuild-secrets = ['id=netrc,env=NETRC_CONTENT']\n",
		wantKeys:  []string{"tool.astro.build-secrets"},
	},
	{
		name: "build-secrets entries that are not specs",
		wantCodes: []ProblemCode{
			CodeBuildSecretInvalid, CodeBuildSecretInvalid, CodeBuildSecretInvalid,
			CodeBuildSecretInvalid, CodeBuildSecretInvalid, CodeExpectedString, CodeEmptyString,
		},
		content: "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n" +
			"build-secrets = ['hunter2', 'id=netrc,value=hunter2', 'env=NETRC_CONTENT', 'id=netrc,env=A,src=/b', 'id=netrc,src=.netrc', 3, ' ']\n",
		wantKeys: []string{
			"tool.astro.build-secrets[0]", "tool.astro.build-secrets[1]", "tool.astro.build-secrets[2]",
			"tool.astro.build-secrets[3]", "tool.astro.build-secrets[4]", "tool.astro.build-secrets[5]", "tool.astro.build-secrets[6]",
		},
	},
	{
		name:      "build-secrets of the wrong shape",
		wantCodes: []ProblemCode{CodeExpectedStringArray},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = 'Dockerfile'\nbuild-secrets = 'id=netrc,env=NETRC_CONTENT'\n",
		wantKeys:  []string{"tool.astro.build-secrets"},
	},
	{
		name:      "unknown key in [tool.astro]",
		wantCodes: []ProblemCode{CodeUnknownKey},
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nairflw = \"3.1\"\n",
		wantKeys:  []string{"tool.astro.airflw"},
	},
	{
		name:      "unknown key on a link",
		wantCodes: []ProblemCode{CodeUnknownKey},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
defaults = true
`,
		wantKeys: []string{"tool.astro.deployments.prod.defaults"},
	},
	{
		name:      "incomplete deployment",
		wantCodes: []ProblemCode{CodeDeploymentRequired, CodeWorkspaceRequired},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "astro"
`,
		wantKeys: []string{
			"tool.astro.deployments.prod.deployment",
			"tool.astro.deployments.prod.workspace",
		},
	},
	{
		name:      "several at once",
		wantCodes: []ProblemCode{CodeAirflowMissing, CodeRequired, CodeAirflowRemoved},
		content:   "[tool.astro]\nairflow = \"v3\"\n\n[tool.astro.deployments.d]\nworkspace = \"w\"\ndeployment = \"x\"\n",
		wantKeys: []string{
			"project.dependencies",
			"project.name",
			"tool.astro.airflow",
		},
	},
	{
		name:      "domain without a workspace",
		wantCodes: []ProblemCode{CodeDomainWithoutWorkspace},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
domain = "astronomer-dev.io"
`,
		wantKeys: []string{"tool.astro.domain"},
	},
	{
		name:      "domain with only links that skip the Astro login",
		wantCodes: []ProblemCode{CodeDomainWithoutWorkspace},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
domain = "astronomer-dev.io"

[tool.astro.deployments.staging]
url = "https://airflow.example.com"
auth = { method = "none" }
`,
		wantKeys: []string{"tool.astro.domain"},
	},
	{
		name:      "missing workspace at both levels",
		wantCodes: []ProblemCode{CodeWorkspaceRequired},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments.prod.workspace"},
	},
	{
		name:      "empty per-link target rejected",
		wantCodes: []ProblemCode{CodeEmptyString},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.prod]
target = ""
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments.prod.target"},
	},
	{
		name:      "empty top-level target rejected",
		wantCodes: []ProblemCode{CodeEmptyString},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
target = ""
`,
		wantKeys: []string{"tool.astro.target"},
	},
	{
		name:      "target as a table is the old spelling of targets",
		wantCodes: []ProblemCode{CodeTargetNotAName},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.target.astro]
image = { os = "ubi" }
`,
		wantKeys: []string{"tool.astro.target"},
	},
	{
		name:      "target a link cannot use",
		wantCodes: []ProblemCode{CodeTargetUnusable},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.prod]
target = "MWAA"
environment = "orders-prod"
`,
		wantKeys: []string{"tool.astro.deployments.prod.target"},
	},
	{
		name:      "link inherits a target it cannot use",
		wantCodes: []ProblemCode{CodeInheritedTargetUnusable},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"
target = "oss"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments.prod.target"},
	},
	{
		name:      "two links marked default",
		wantCodes: []ProblemCode{CodeMultipleDefaults},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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
		name:      "a link sets both a url and coordinates",
		wantCodes: []ProblemCode{CodeURLAndCoordinates},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.mixed]
url = "https://airflow.corp.dev"
deployment = "dep-xyz"
auth = { method = "none" }
`,
		wantKeys: []string{"tool.astro.deployments.mixed"},
	},
	{
		name:      "an mwaa link with a url",
		wantCodes: []ProblemCode{CodeTargetNeedsEnvironment},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "mwaa"
url = "https://airflow.corp.dev"
auth = { method = "aws" }
`,
		wantKeys: []string{"tool.astro.deployments.prod"},
	},
	{
		name:      "environment on an astro link",
		wantCodes: []ProblemCode{CodeEnvironmentOnAstroLink},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
environment = "orders-prod"
`,
		wantKeys: []string{"tool.astro.deployments.prod.environment"},
	},
	{
		name:      "deployment id on an mwaa link",
		wantCodes: []ProblemCode{CodeDeploymentOnEnvironmentLink},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "mwaa"
environment = "orders-prod"
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments.prod.deployment"},
	},
	{
		name:      "workspace on an mwaa link",
		wantCodes: []ProblemCode{CodeWorkspaceOnNonAstroLink},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "mwaa"
environment = "orders-prod"
workspace = "ws-abc"
`,
		wantKeys: []string{"tool.astro.deployments.prod.workspace"},
	},
	{
		name:      "composer link without an environment",
		wantCodes: []ProblemCode{CodeEnvironmentRequired},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "composer"
`,
		wantKeys: []string{"tool.astro.deployments.prod.environment"},
	},
	{
		name:      "url with no scheme",
		wantCodes: []ProblemCode{CodeURLNoScheme},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.staging]
url = "airflow.staging.corp.dev"
auth = { method = "none" }
`,
		wantKeys: []string{"tool.astro.deployments.staging.url"},
	},
	{
		name:      "url carrying a username and password",
		wantCodes: []ProblemCode{CodeURLHasCredentials},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.staging]
url = "https://admin:hunter2@airflow.corp.dev"
auth = { method = "none" }
`,
		wantKeys: []string{"tool.astro.deployments.staging.url"},
	},
	{
		// `local` is the machine's own word — `astro local start`,
		// `astro local af dags list`. A link may not take it, so nobody has to
		// work out which one a reader meant.
		name:      "a link named local",
		wantCodes: []ProblemCode{CodeLinkNameReserved},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.local]
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments.local"},
	},
	{
		name:      "a link with no name",
		wantCodes: []ProblemCode{CodeLinkNeedsName},
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.""]
deployment = "dep-xyz"
`,
		wantKeys: []string{"tool.astro.deployments"},
	},
	{
		// The four cases below exist so that every declared ProblemCode is
		// raised by some manifest; see TestEveryCodeIsReachable.
		name:      "a link default that is not a boolean",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nworkspace = \"ws\"\n\n[tool.astro.deployments.prod]\ndeployment = \"d\"\ndefault = \"yes\"\n",
		wantKeys:  []string{"tool.astro.deployments.prod.default"},
		wantCodes: []ProblemCode{CodeExpectedBool},
	},
	{
		name:      "a url that will not parse",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.deployments.prod]\nurl = \"http://[::1\"\nauth = { method = \"none\" }\n",
		wantKeys:  []string{"tool.astro.deployments.prod.url"},
		wantCodes: []ProblemCode{CodeURLInvalid},
	},
	{
		name:      "a url that is not http",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.deployments.prod]\nurl = \"ftp://airflow.example.com\"\nauth = { method = \"none\" }\n",
		wantKeys:  []string{"tool.astro.deployments.prod.url"},
		wantCodes: []ProblemCode{CodeURLNotHTTP},
	},
	{
		name:      "a url naming no host",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.deployments.prod]\nurl = \"https:///dags\"\nauth = { method = \"none\" }\n",
		wantKeys:  []string{"tool.astro.deployments.prod.url"},
		wantCodes: []ProblemCode{CodeURLNoHost},
	},
	{
		name:      "a pool with no slots",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { description = 'ETL loads' }\n",
		wantKeys:  []string{"tool.astro.pools.etl.slots"},
		wantCodes: []ProblemCode{CodeRequired},
	},
	{
		name:      "a pool whose slots are not a whole number",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = '4' }\n",
		wantKeys:  []string{"tool.astro.pools.etl.slots"},
		wantCodes: []ProblemCode{CodeExpectedInteger},
	},
	{
		name:      "a pool with zero slots",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = 0 }\n",
		wantKeys:  []string{"tool.astro.pools.etl.slots"},
		wantCodes: []ProblemCode{CodePoolSlotsInvalid},
	},
	{
		name:      "a pool with slots below -1",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = -2 }\n",
		wantKeys:  []string{"tool.astro.pools.etl.slots"},
		wantCodes: []ProblemCode{CodePoolSlotsInvalid},
	},
	{
		name:      "a pool with a key Airflow has no field for",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = 4, open_slots = 2 }\n",
		wantKeys:  []string{"tool.astro.pools.etl.open_slots"},
		wantCodes: []ProblemCode{CodeUnknownKey},
	},
	{
		name:      "a pool include_deferred that is not a boolean",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = 4, include_deferred = 'yes' }\n",
		wantKeys:  []string{"tool.astro.pools.etl.include_deferred"},
		wantCodes: []ProblemCode{CodeExpectedBool},
	},
	{
		name:      "a pool that is not a table",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = 4\n",
		wantKeys:  []string{"tool.astro.pools.etl"},
		wantCodes: []ProblemCode{CodeExpectedTable},
	},
	{
		name:      "a pool name with a slash",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\n'etl/eu' = { slots = 4 }\n",
		wantKeys:  []string{"tool.astro.pools.etl/eu"},
		wantCodes: []ProblemCode{CodePoolNameInvalid},
	},
	{
		name:      "a blank pool name",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\n' ' = { slots = 4 }\n",
		wantKeys:  []string{"tool.astro.pools. "},
		wantCodes: []ProblemCode{CodePoolNameInvalid},
	},
	{
		name:      "a description on default_pool",
		content:   "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\ndefault_pool = { slots = 64, description = 'mine' }\n",
		wantKeys:  []string{"tool.astro.pools.default_pool.description"},
		wantCodes: []ProblemCode{CodeDefaultPoolDescription},
	},
}

func TestValidation(t *testing.T) {
	for _, tc := range validationCases {
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
			if codes := problemCodesOf(ve); !reflect.DeepEqual(codes, tc.wantCodes) {
				t.Errorf("problem codes = %v, want %v", codes, tc.wantCodes)
			}
		})
	}
}

func TestParseWithoutPath(t *testing.T) {
	_, err := Parse([]byte("[tool.astro]\n"))
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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

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

// authKey is the key every auth case is addressed under: one link, so one
// prefix.
const authKey = "tool.astro.deployments.staging.auth"

// authCases is the auth table, in a function so that TestEveryCodeIsReachable
// can drive the same fixtures without a second copy of them.
func authCases() []struct {
	name      string
	auth      string
	wantKeys  []string
	wantCodes []ProblemCode
} {
	const key = authKey
	return []struct {
		name     string
		auth     string
		wantKeys []string
		// wantCodes is which rule refused, in the order Load returns the
		// problems. This used to pin a substring of the first problem's
		// prose, which asserted the English rather than the rule and went
		// quiet the moment a message was reworded; the code is the part
		// that is promised to a caller, so it is the part asserted here.
		wantCodes []ProblemCode
	}{
		{
			name:      "a url link with no auth table at all",
			auth:      "",
			wantKeys:  []string{key},
			wantCodes: []ProblemCode{CodeAuthRequired},
		},
		{
			name:      "auth is not a table",
			auth:      `auth = "token"`,
			wantKeys:  []string{key},
			wantCodes: []ProblemCode{CodeExpectedTable},
		},
		{
			name:      "auth table names no method",
			auth:      `auth = { token-env = "AIRFLOW_TOKEN" }`,
			wantKeys:  []string{key + ".method"},
			wantCodes: []ProblemCode{CodeAuthMethodRequired},
		},
		{
			name:      "unknown method",
			auth:      `auth = { method = "kerberos" }`,
			wantKeys:  []string{key + ".method"},
			wantCodes: []ProblemCode{CodeAuthMethodUnknown},
		},
		{
			name:      "field on the wrong method",
			auth:      `auth = { method = "basic", username-env = "U", password-env = "P", token-env = "T" }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeAuthFieldNotForMethod},
		},
		{
			name:      "field on a method that takes none",
			auth:      `auth = { method = "google", token-env = "T" }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeAuthFieldNotForMethod},
		},
		{
			name:      "unknown field",
			auth:      `auth = { method = "token", token-env = "T", audience = "airflow" }`,
			wantKeys:  []string{key + ".audience"},
			wantCodes: []ProblemCode{CodeAuthFieldNotForMethod},
		},
		{
			name:      "token method without its env var",
			auth:      `auth = { method = "token" }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeAuthFieldRequired},
		},
		{
			name:      "basic method missing both env vars",
			auth:      `auth = { method = "basic" }`,
			wantKeys:  []string{key + ".password-env", key + ".username-env"},
			wantCodes: []ProblemCode{CodeAuthFieldRequired, CodeAuthFieldRequired},
		},
		{
			name:      "a literal secret where an env-var name belongs",
			auth:      `auth = { method = "token", token-env = "eyJhbGciOi.J9 secret" }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeAuthEnvNameInvalid},
		},
		{
			name:      "empty field value",
			auth:      `auth = { method = "token", token-env = "" }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeEmptyString},
		},
		{
			name:      "field value of the wrong type",
			auth:      `auth = { method = "token", token-env = 7 }`,
			wantKeys:  []string{key + ".token-env"},
			wantCodes: []ProblemCode{CodeExpectedString},
		},
		{
			name:      "exec command as a string",
			auth:      `auth = { method = "exec", command = "acme-airflow-token --profile prod" }`,
			wantKeys:  []string{key + ".command"},
			wantCodes: []ProblemCode{CodeExpectedStringArray},
		},
		{
			name:      "exec command with nothing in it",
			auth:      `auth = { method = "exec", command = [] }`,
			wantKeys:  []string{key + ".command"},
			wantCodes: []ProblemCode{CodeAuthCommandEmpty},
		},
		{
			name:      "exec command with a non-string argument",
			auth:      `auth = { method = "exec", command = ["acme-token", 7] }`,
			wantKeys:  []string{key + ".command[1]"},
			wantCodes: []ProblemCode{CodeExpectedString},
		},
		{
			name:      "half an airflow-token credential pair",
			auth:      `auth = { method = "airflow-token", client-id-env = "AF_CLIENT_ID" }`,
			wantKeys:  []string{key + ".client-secret-env"},
			wantCodes: []ProblemCode{CodeAuthPairIncomplete},
		},
		{
			name:      "airflow-token with both credential pairs",
			auth:      `auth = { method = "airflow-token", client-id-env = "ID", client-secret-env = "SECRET", username-env = "U", password-env = "P" }`,
			wantKeys:  []string{key},
			wantCodes: []ProblemCode{CodeAuthTooManyPairs},
		},
		{
			name:      "airflow-token with nothing to exchange",
			auth:      `auth = { method = "airflow-token" }`,
			wantKeys:  []string{key},
			wantCodes: []ProblemCode{CodeAuthNeedsCredentials},
		},
	}
}

func TestAuthValidation(t *testing.T) {
	for _, tc := range authCases() {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, endpointLink(tc.auth)))
			ve := validationError(t, err)
			if keys := problemKeys(ve); !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Fatalf("problem keys = %v, want %v", keys, tc.wantKeys)
			}
			if codes := problemCodesOf(ve); !reflect.DeepEqual(codes, tc.wantCodes) {
				t.Errorf("problem codes = %v, want %v", codes, tc.wantCodes)
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

// A declared Dockerfile decodes, and the shapes that stay inside the project are
// accepted rather than merely not-refused.
//
// The subdirectory case is the one worth pinning: "tier 3" exists for builds a
// manifest cannot express, and a project that keeps its build files under
// docker/ is exactly that, so restricting this to a bare "Dockerfile" at the
// root would have made the escape hatch narrower than the thing it is for.
func TestParseDockerfileDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name string
		decl string
		want string
	}{
		{"root", `dockerfile = "Dockerfile"`, "Dockerfile"},
		{"named variant", `dockerfile = "Dockerfile.prod"`, "Dockerfile.prod"},
		{"subdirectory", `dockerfile = "docker/Dockerfile"`, "docker/Dockerfile"},
		{"absent", "", ""},
		// Whitespace decodes to empty, so a consumer's `declared != ""` reads it
		// as "not declared" rather than as a file named " ". Untrimmed, this
		// suppressed the desktop's presence fallback and took the project's real
		// Dockerfile away.
		{"whitespace only", `dockerfile = "   "`, ""},
		{"padded", `dockerfile = "  Dockerfile  "`, "Dockerfile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n"+tc.decl+"\n"))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if m.Astro.Dockerfile != tc.want {
				t.Errorf("Dockerfile = %q, want %q", m.Astro.Dockerfile, tc.want)
			}
		})
	}
}

func TestLoadReadsUVConstraintDependencies(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\"apache-airflow==3.*\"]\n\n[tool.astro]\n\n"+
		"[tool.uv]\nconstraint-dependencies = [\"sqlalchemy<2.1\", \"pandas<3\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"sqlalchemy<2.1", "pandas<3"}; !reflect.DeepEqual(m.UV.ConstraintDependencies, want) {
		t.Errorf("constraint-dependencies = %#v, want %#v", m.UV.ConstraintDependencies, want)
	}
}

// [tool.uv] is uv's table, so a shape uv would reject is uv's to report: the
// project still loads, and astro reads no constraints from it.
func TestLoadIgnoresAUVTableOfTheWrongShape(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\"apache-airflow==3.*\"]\n\n[tool.astro]\n\n"+
		"[tool.uv]\nconstraint-dependencies = \"sqlalchemy<2.1\"\n"))
	if err != nil {
		t.Fatalf("a [tool.uv] astro does not own must not stop the load: %v", err)
	}
	if m.UV.ConstraintDependencies != nil {
		t.Errorf("a string is not a constraint list, got %#v", m.UV.ConstraintDependencies)
	}
}
