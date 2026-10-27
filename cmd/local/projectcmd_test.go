package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
)

// inspectManifest is a project with most of what the view publishes: an
// inherited workspace on an astro link marked default, an endpoint link with
// its own auth, OS packages, a pool and two env declarations. The preamble
// pins Airflow 3.1 and names workspace ws_abc123.
var inspectManifest = instancestest.Preamble + `
packages = ['git']

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.staging]
url = 'https://airflow.staging.corp.dev'
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }

[tool.astro.pools]
heavy = { slots = 2, description = 'big jobs' }

[tool.astro.env.variables]
REGION = { type = 'string', default = 'eu-west-1', description = 'where the data is' }
OPENAI_API_KEY = { secret = true }

# Shown as written, so a quoted key may hold a dot, and an integer above
# 2^53 has to survive the read.
[tool.astro.targets.mwaa]
region = 'eu-west-1'
"max.bytes" = 9007199254740993
`

// projectFile writes body as a project's whole pyproject.toml and isolates
// the run from the user's machine: the cache root and home at scratch dirs,
// so the pin and any runtime records land there, and the deployment
// selection unset. ASTRO_DOMAIN is left alone, since a case may set it
// before writing its project; a test that asserts the default host clears
// it itself. instanceProject writes the preamble itself and comes here.
func projectFile(t *testing.T, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orders")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	t.Setenv(instances.EnvVar, "")
	return dir
}

// inspectJSON runs `astro project inspect --output json` in dir and decodes
// what it published.
func inspectJSON(t *testing.T, dir string, args ...string) (view projectView, raw string) {
	t.Helper()
	d, stdout, _ := instanceDeps(t, dir)
	if err := execute(t, d, append([]string{"project", "inspect", "--output", "json"}, args...)...); err != nil {
		t.Fatalf("inspect: %v\n%s", err, stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	return view, stdout.String()
}

func TestProjectInspectPublishesTheManifestAsTheCLIReadsIt(t *testing.T) {
	dir := projectFile(t, inspectManifest)
	t.Setenv("ASTRO_DOMAIN", "")
	view, raw := inspectJSON(t, dir)

	if view.Path != filepath.Join(dir, "pyproject.toml") {
		t.Errorf("path = %q, want the manifest's", view.Path)
	}
	if view.Name != "demo" || view.RequiresPython != ">=3.10" {
		t.Errorf("[project] = %q %q, want demo >=3.10", view.Name, view.RequiresPython)
	}
	// The Airflow version is the requirement's pin, not a key of its own.
	if view.Airflow != "3.1" {
		t.Errorf("airflow = %q, want the pin 3.1", view.Airflow)
	}
	if len(view.Dependencies) != 1 || !strings.HasPrefix(view.Dependencies[0], "apache-airflow==3.1") {
		t.Errorf("dependencies = %q, want the requirement", view.Dependencies)
	}
	if len(view.Packages) != 1 || view.Packages[0] != "git" {
		t.Errorf("packages = %q, want [git]", view.Packages)
	}
	// Resolved past the file: the workspace's default host, and the
	// built-in default target.
	if view.Workspace != "ws_abc123" || view.Domain != "astronomer.io" || view.Target != "astro" {
		t.Errorf("workspace/domain/target = %q %q %q, want ws_abc123 astronomer.io astro", view.Workspace, view.Domain, view.Target)
	}
	// Nothing names an organization: no seam was wired.
	if view.Organization != "" {
		t.Errorf("organization = %q, want none", view.Organization)
	}

	prod, ok := view.Deployments["prod"]
	if !ok {
		t.Fatalf("deployments = %v, want prod", view.Deployments)
	}
	// The link inherits the workspace and target it does not set, takes its
	// kind's auth method, and keeps its default mark.
	want := projectLink{Kind: "astro", Target: "astro", Workspace: "ws_abc123", Deployment: "clm2xk9dq000108l7a2b3c4d5", AuthMethod: "astro", Default: true}
	if prod != want {
		t.Errorf("prod = %+v, want %+v", prod, want)
	}
	staging := view.Deployments["staging"]
	want = projectLink{Kind: "endpoint", Target: "astro", URL: "https://airflow.staging.corp.dev", AuthMethod: "token"}
	if staging != want {
		t.Errorf("staging = %+v, want %+v", staging, want)
	}

	heavy := view.Pools["heavy"]
	if heavy.Slots != 2 || heavy.Description != "big jobs" || heavy.IncludeDeferred != nil {
		t.Errorf("pools.heavy = %+v, want 2 slots, big jobs, include_deferred unset", heavy)
	}

	// [tool.astro.env] is published as written.
	variables, _ := view.Env["variables"].(map[string]any)
	region, _ := variables["REGION"].(map[string]any)
	if region["default"] != "eu-west-1" || region["type"] != "string" {
		t.Errorf("env.variables.REGION = %v, want the declaration as written", region)
	}

	// Omitted only when unset: runtime, dockerfile, organization.
	for _, key := range []string{`"runtime"`, `"dockerfile"`, `"organization"`} {
		if strings.Contains(raw, key) {
			t.Errorf("published object has %s, which the project does not set:\n%s", key, raw)
		}
	}
}

// What a project does not use is present and empty, never null or missing,
// so a reader meets the same keys on every project.
func TestProjectInspectPublishesEmptyCollections(t *testing.T) {
	dir := projectFile(t, instancestest.Preamble)
	_, raw := inspectJSON(t, dir)
	for _, key := range []string{`"packages":[]`, `"build_secrets":[]`, `"deployments":{}`, `"env":{}`, `"pools":{}`, `"targets":{}`} {
		if !strings.Contains(raw, key) {
			t.Errorf("published object lacks %s:\n%s", key, raw)
		}
	}
}

// Text mode is the same object as YAML, under the same keys, so --key reads
// one spelling whichever format someone looked at first.
func TestProjectInspectTextIsTheSameObjectAsYAML(t *testing.T) {
	dir := projectFile(t, inspectManifest)
	d, stdout, _ := instanceDeps(t, dir)
	if err := execute(t, d, "project", "inspect"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "path: ") {
		t.Errorf("text starts with %q, want the path line first", strings.SplitN(stdout.String(), "\n", 2)[0])
	}
	var got map[string]any
	if err := yaml.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("text is not YAML: %v\n%s", err, stdout.String())
	}
	deployments, _ := got["deployments"].(map[string]any)
	prod, _ := deployments["prod"].(map[string]any)
	if got["name"] != "demo" || prod["deployment"] != "clm2xk9dq000108l7a2b3c4d5" || got["requires_python"] != ">=3.10" {
		t.Errorf("YAML = %v, want the same keys and values as json", got)
	}
}

func TestProjectInspectKeyPrintsOneValue(t *testing.T) {
	dir := projectFile(t, inspectManifest)
	cases := []struct {
		name string
		args []string
		// want is the whole stdout for a bare value, or a substring of it
		// for a table.
		want      string
		substring bool
	}{
		{"a string, bare", []string{"--key", "deployments.prod.deployment"}, "clm2xk9dq000108l7a2b3c4d5\n", false},
		{"a number, bare", []string{"--key", "pools.heavy.slots"}, "2\n", false},
		{"a boolean, bare", []string{"--key", "deployments.prod.default"}, "true\n", false},
		{"a string under json is quoted", []string{"--key", "name", "--output", "json"}, "\"demo\"\n", false},
		{"a table as YAML", []string{"--key", "deployments.staging"}, "url: https://airflow.staging.corp.dev\n", true},
		{"a list as YAML", []string{"--key", "packages"}, "- git\n", true},
		// A table named by --key is read back from the json encoding, so its
		// keys come out sorted rather than in the view's order.
		{"a table under json", []string{"--key", "pools", "--output", "json"}, `{"heavy":{"description":"big jobs","slots":2}}`, true},
		{"an as-written table", []string{"--key", "env.variables.REGION.default"}, "eu-west-1\n", false},
		// A quoted TOML key holds a dot: the shortest run of parts that
		// names a key is the key.
		{"a key with a dot in it", []string{"--key", "targets.mwaa.max.bytes"}, "9007199254740993\n", false},
		{"an integer above 2^53, bare", []string{"--key", "targets.mwaa.max.bytes", "--output", "json"}, "9007199254740993\n", false},
		{"an integer above 2^53, in a table", []string{"--key", "targets.mwaa"}, "max.bytes: 9007199254740993\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, stdout, _ := instanceDeps(t, dir)
			if err := execute(t, d, append([]string{"project", "inspect"}, c.args...)...); err != nil {
				t.Fatalf("inspect %v: %v", c.args, err)
			}
			got := stdout.String()
			if c.substring && !strings.Contains(got, c.want) {
				t.Errorf("stdout = %q, want it to contain %q", got, c.want)
			}
			if !c.substring && got != c.want {
				t.Errorf("stdout = %q, want %q", got, c.want)
			}
		})
	}
}

// A key the project cannot answer is a usage error: exit 2, so a script
// reading the value stops rather than carrying on with an empty one, and a
// message that names what the table does have.
func TestProjectInspectRefusesAKeyTheProjectDoesNotHave(t *testing.T) {
	dir := projectFile(t, inspectManifest)
	cases := []struct {
		name, key, want string
	}{
		{"no such key", "org", "no such key. The keys are: path, name, "},
		{"an unset key", "organization", "this project does not set organization, and nothing supplies one"},
		{"no such nested key", "deployments.prod.region", "deployments.prod has no region. Its keys are: auth_method, default, deployment, kind, target, workspace"},
		{"no such link", "deployments.dev.deployment", "deployments has no dev. Its keys are: prod, staging"},
		{"a path through a value", "name.first", "name is a value, not a table, so it has no first"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, stdout, _ := instanceDeps(t, dir)
			err := execute(t, d, "project", "inspect", "--key", c.key)
			if err == nil || !cliout.IsUsage(err) {
				t.Fatalf("err = %v, want a usage error", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %q, want it to say %q", err.Error(), c.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing in text mode", stdout.String())
			}

			d, stdout, _ = instanceDeps(t, dir)
			_ = execute(t, d, "project", "inspect", "--key", c.key, "--output", "json")
			var got cliout.ErrorObject
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("json stdout is not the error object: %v\n%s", err, stdout.String())
			}
			if got.Kind != cliout.KindUsage || got.Code != 2 {
				t.Errorf("error object = %+v, want kind usage, code 2", got)
			}
		})
	}
}

// domain and organization are the two keys a script most wants and the two
// the file need not spell out, so they are resolved the way the workspace
// reads resolve them: the file, then ASTRO_DOMAIN, then the default host;
// the file, then the login that host uses.
func TestProjectInspectResolvesDomainAndOrganization(t *testing.T) {
	withHost := instancestest.Preamble + "domain = 'https://cloud.astronomer-dev.io/'\norganization = 'org-file'\n"
	const noWorkspace = "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
	cases := []struct {
		name     string
		manifest string
		env      string
		login    string
		// wantAsked is the domain the login seam is asked for.
		wantAsked, wantDomain, wantOrg string
	}{
		// The file's organization is the answer, so the login is not asked.
		{"the file's host and organization", withHost, "", "org-login", "unasked", "astronomer-dev.io", "org-file"},
		{"a workspace with no host is on astronomer.io", instancestest.Preamble, "", "org-login", "astronomer.io", "astronomer.io", "org-login"},
		{"ASTRO_DOMAIN stands in for a host", instancestest.Preamble, "astronomer-dev.io", "org-login", "astronomer-dev.io", "astronomer-dev.io", "org-login"},
		{"no host and no workspace is the current login", noWorkspace, "", "org-login", "", "", "org-login"},
		{"a login with no organization", instancestest.Preamble, "", "", "astronomer.io", "astronomer.io", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := projectFile(t, c.manifest)
			t.Setenv("ASTRO_DOMAIN", c.env)
			d, stdout, _ := instanceDeps(t, dir)
			asked := "unasked"
			d.LoginOrganization = func(domain string) string {
				asked = domain
				return c.login
			}
			if err := execute(t, d, "project", "inspect", "--output", "json"); err != nil {
				t.Fatalf("inspect: %v", err)
			}
			var view projectView
			if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
				t.Fatal(err)
			}
			if asked != c.wantAsked {
				t.Errorf("the login seam was asked for %q, want %q", asked, c.wantAsked)
			}
			if view.Domain != c.wantDomain || view.Organization != c.wantOrg {
				t.Errorf("domain, organization = %q, %q, want %q, %q", view.Domain, view.Organization, c.wantDomain, c.wantOrg)
			}
		})
	}
}

func TestProjectInspectFindsTheProjectFromBelow(t *testing.T) {
	dir := projectFile(t, inspectManifest)
	sub := filepath.Join(dir, "dags", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	view, _ := inspectJSON(t, sub)
	if view.Path != filepath.Join(dir, "pyproject.toml") {
		t.Errorf("path = %q, want the project root's manifest", view.Path)
	}
}

// Outside a project the failure is discovery's own, with its kind, so a
// script tells "not a project" from "a project with a broken manifest".
func TestProjectInspectOutsideAProjectIsNamed(t *testing.T) {
	plain := "[project]\nname = 'plain'\nrequires-python = '>=3.10'\ndependencies = []\n\n[tool.ruff]\nline-length = 100\n"
	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"no pyproject.toml", t.TempDir(), "no Astro project found"},
		{"a pyproject.toml with no [tool.astro]", projectFile(t, plain), "has no [tool.astro] section"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, stdout, _ := instanceDeps(t, c.dir)
			if err := execute(t, d, "project", "inspect", "--output", "json"); err == nil {
				t.Fatal("inspect outside a project succeeded")
			}
			var got cliout.ErrorObject
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("not the error object: %v\n%s", err, stdout.String())
			}
			if got.Kind != KindNoProject || !strings.Contains(got.Error, c.want) {
				t.Errorf("error object = %+v, want kind %s saying %q", got, KindNoProject, c.want)
			}
		})
	}
}

// A manifest that does not load is reported the way a start reports it: every
// problem, each by its key, rather than a partial view of a file in a state
// nothing else would act on.
func TestProjectInspectReportsAManifestThatDoesNotLoad(t *testing.T) {
	dir := projectFile(t, instancestest.Preamble+"org = 'typo'\nruntime = 'not-a-build'\n")
	d, stdout, _ := instanceDeps(t, dir)
	err := execute(t, d, "project", "inspect")
	if err == nil {
		t.Fatal("a manifest with an unknown key loaded")
	}
	for _, want := range []string{"2 problems", "tool.astro.org: unknown key", "tool.astro.runtime:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to say %q", err.Error(), want)
		}
	}
	if cliout.IsUsage(err) {
		t.Errorf("a broken manifest is reported as a usage error; it is the project's failure, exit 1")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}
