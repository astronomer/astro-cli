package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/internal/localenv"
)

// envProject writes a project with the given [tool.astro.env] body and points
// ASTRO_HOME at a scratch dir, so project and global files are both isolated.
func envProject(t *testing.T, envBody string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	manifest := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\n" + envBody
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASTRO_HOME", t.TempDir())
	return dir
}

func envDeps(t *testing.T, dir, stdin string) (d Deps, stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	d, _ = testDeps(t)
	d.Stdin = strings.NewReader(stdin)
	d.Stdout = stdout
	d.Stderr = stderr
	d.WorkingDir = func() (string, error) { return dir, nil }
	return d, stdout, stderr
}

func TestEnvSetGetRoundTrip(t *testing.T) {
	dir := envProject(t, "")

	// Set an env var from stdin (a non-TTY stdin reads the value).
	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	// The value landed in the project .env at 0600.
	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "API_TOKEN=s3cr3t") {
		t.Fatalf(".env missing the value:\n%s", content)
	}

	// Get it back through the chain.
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "s3cr3t" {
		t.Fatalf("get printed %q, want s3cr3t", out.String())
	}
}

func TestEnvGetJSONShape(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "http://api\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_URL"); err != nil {
		t.Fatal(err)
	}
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "API_URL", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Kind, Name, Source, Value string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Kind != "env" || got.Name != "API_URL" || got.Source != "project" || got.Value != "http://api" {
		t.Fatalf("get json = %+v", got)
	}
}

func TestEnvSetConnFromURI(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "postgres://u:p@host:5432/db\n")
	if err := execute(t, d, "local", "env", "connection", "set", "warehouse"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "AIRFLOW_CONN_WAREHOUSE=") || !strings.Contains(string(content), `"conn_type":"postgres"`) {
		t.Fatalf("conn not stored as AIRFLOW_CONN JSON:\n%s", content)
	}
}

func TestEnvSetConnAndVarWithValueFlag(t *testing.T) {
	dir := envProject(t, "")
	// The conn/var subcommands inherit --value (persistent on `set`); no stdin.
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "http_api", "--value", `{"conn_type":"http","host":"api"}`); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "airflow-variable", "set", "region", "--value", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	for _, want := range []string{`AIRFLOW_CONN_HTTP_API=`, `"conn_type":"http"`, "AIRFLOW_VAR_REGION=us-east-1"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf(".env missing %q:\n%s", want, content)
		}
	}
}

func TestEnvListJSONShapeAndOrphan(t *testing.T) {
	envBody := "\n[tool.astro.env]\nAPI_URL = {}\n"
	dir := envProject(t, envBody)
	// A declared value plus an orphan hand-added to the file.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_URL=http://x\nLEFTOVER=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	// NDJSON: one object per line.
	var declared, orphan bool
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var it struct {
			Kind, Name, Source string
			Orphan             bool
		}
		if err := json.Unmarshal([]byte(line), &it); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if it.Name == "API_URL" && it.Source == "project" && !it.Orphan {
			declared = true
		}
		if it.Name == "LEFTOVER" && it.Orphan {
			orphan = true
		}
		// No line ever carries a value key.
		if strings.Contains(line, "\"value\"") {
			t.Errorf("list output leaked a value: %s", line)
		}
	}
	if !declared || !orphan {
		t.Fatalf("declared=%v orphan=%v\n%s", declared, orphan, out.String())
	}
}

func TestEnvDeleteRemovesValue(t *testing.T) {
	dir := envProject(t, "")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "A"); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if strings.Contains(string(content), "A=1") || !strings.Contains(string(content), "B=2") {
		t.Fatalf("delete wrong: %s", content)
	}
	// Deleting an absent value is an error (non-zero exit).
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "NOPE"); err == nil {
		t.Fatal("expected an error deleting an absent value")
	}
}

func TestEnvGlobalScope(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "globalval\n")
	if err := execute(t, d, "local", "env", "variable", "set", "SHARED", "--global"); err != nil {
		t.Fatal(err)
	}
	// It went to the global file, not the project .env. Through GlobalEnvPath
	// rather than by joining a path here: the layout is that function's to own,
	// and restating it is how this test agreed with the bug where ASTRO_HOME was
	// read as .astro itself rather than as its parent.
	gp, err := localenv.GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	gc, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gc), "SHARED=globalval") {
		t.Fatalf("global file missing value:\n%s", gc)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Fatalf("project .env should not exist for a --global set")
	}
}

func TestEnvSetWarnsWhenNotGitignored(t *testing.T) {
	dir := envProject(t, "")
	// Remove the .gitignore so .env is not covered.
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	d, _, stderr := envDeps(t, dir, "x\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "gitignore") {
		t.Fatalf("expected a gitignore warning on stderr, got: %q", stderr.String())
	}
}

func TestStartMissingEnvHint(t *testing.T) {
	envBody := "\n[tool.astro.env]\nAPI_TOKEN = {}\n"
	dir := envProject(t, envBody)
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "start")
	if err == nil {
		t.Fatal("expected start to fail on a missing required value")
	}
	if !strings.Contains(err.Error(), "astro local env variable set API_TOKEN --project") {
		t.Fatalf("missing hint not in error:\n%s", err.Error())
	}
}

// The NOTE column for the three shapes of orphan row.
//
// An orphan in another project (which only --all reaches) carries no remove
// command on purpose: `astro local env <noun> delete` acts on the working directory's
// project, so any command offered here would edit the wrong file. The note
// printed the label anyway, so the row read "orphan in /path; remove: " and
// trailed off — an instruction with nothing to follow.
func TestEnvListOrphanNote(t *testing.T) {
	var out strings.Builder
	err := renderEnvList(&out, []localenv.ListItem{
		{
			Kind: localenv.KindEnv, Name: "LEFTOVER", Source: "project",
			Orphan: true, RemoveHint: "astro local env variable delete LEFTOVER --project",
		},
		{
			Kind: localenv.KindEnv, Name: "ELSEWHERE", Source: "project",
			Orphan: true, Project: "/projects/other",
		},
		{Kind: localenv.KindEnv, Name: "DECLARED", Source: "project"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		for _, name := range []string{"LEFTOVER", "ELSEWHERE", "DECLARED"} {
			if strings.Contains(line, name) {
				lines[name] = line
			}
		}
	}

	if !strings.Contains(lines["LEFTOVER"], "orphan; remove: astro local env variable delete LEFTOVER --project") {
		t.Errorf("an orphan in this project should offer the command: %q", lines["LEFTOVER"])
	}

	if !strings.Contains(lines["ELSEWHERE"], "orphan in /projects/other") {
		t.Errorf("an orphan elsewhere should name the project: %q", lines["ELSEWHERE"])
	}
	if strings.Contains(lines["ELSEWHERE"], "remove:") {
		t.Errorf("an orphan elsewhere must not offer a command that would edit "+
			"the wrong project's file: %q", lines["ELSEWHERE"])
	}

	// A declared value is not an orphan, so its note stays empty.
	if strings.Contains(lines["DECLARED"], "orphan") {
		t.Errorf("a declared value should carry no note: %q", lines["DECLARED"])
	}
}

// The three kinds are peer nouns, and each noun's aliases are the ones
// `astro env` uses for the same object. The aliases are the part worth
// pinning: they are what makes `conn` and `var` mean one thing across both
// halves of the CLI, and nothing else in the tree fails if one is dropped.
func TestEnvNounsAndAliasesMatchTheCloudTree(t *testing.T) {
	for _, tc := range []struct {
		noun string
		name string
		kind localenv.Kind
	}{
		{"variable", "ALIASED_ENV", localenv.KindEnv},
		{"var", "ALIASED_VAR", localenv.KindEnv},
		{"vars", "ALIASED_VARS", localenv.KindEnv},
		{"variables", "ALIASED_VARIABLES", localenv.KindEnv},
		{"connection", "conn_a", localenv.KindConn},
		{"conn", "conn_b", localenv.KindConn},
		{"connections", "conn_c", localenv.KindConn},
		{"airflow-variable", "av_a", localenv.KindVar},
		{"airflow-var", "av_b", localenv.KindVar},
		{"airflow-vars", "av_c", localenv.KindVar},
		{"airflow-variables", "av_d", localenv.KindVar},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			dir := envProject(t, "")
			value := "sqlite:///x.db"
			d, out, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.name,
				"--value", value, "--output", "json"); err != nil {
				t.Fatalf("set through %q: %v", tc.noun, err)
			}
			var got envResult
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode %q: %v", out.String(), err)
			}
			if got.Kind != tc.kind {
				t.Errorf("%q set kind = %q, want %q", tc.noun, got.Kind, tc.kind)
			}
		})
	}
}

// `var` names a plain environment variable here and on the cloud side, not an
// Airflow Variable on one and a plain env var on the other. This is the whole
// reason the noun words were copied from `astro env` rather than shortened,
// and a rename that quietly flipped it would otherwise pass every other test.
func TestVarMeansAPlainEnvVarAsItDoesOnTheCloudSide(t *testing.T) {
	dir := envProject(t, "")
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "var", "set", "SAME_TOKEN",
		"--value", "v", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got envResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != localenv.KindEnv {
		t.Errorf("`env var set` kind = %q, want %q: `var` is a plain environment "+
			"variable in `astro env`, so it has to be one here too",
			got.Kind, localenv.KindEnv)
	}
}

// With no bare-NAME form there is no default kind, so a name that happens to
// spell a noun is reachable. Under the old verb-first tree `env set conn`
// routed to the conn subcommand and died on "accepts 1 arg(s), received 0",
// with no escape hatch — `--` did not rescue it either.
func TestANameThatSpellsANounIsStillReachable(t *testing.T) {
	for _, name := range []string{"conn", "var", "variable", "connection", "list", "set"} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", "variable", "set", name, "--value", "v"); err != nil {
				t.Fatalf("set env var %q: %v", name, err)
			}
			d, out, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", "variable", "get", name, "--output", "json"); err != nil {
				t.Fatalf("get env var %q: %v", name, err)
			}
			var got envValue
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Value != "v" || got.Kind != localenv.KindEnv {
				t.Errorf("get %q = %+v, want the env var this test set", name, got)
			}
		})
	}
}

// The bare verb forms are gone, and they name their replacement rather than
// dead-ending.
//
// `astro local env set API_TOKEN` was the shape in the docs, the demo
// scripts, the start-time hint and pkg/instances' error text. Those were all
// updated; shell history and a teammate's notes were not. A plain "unknown
// command" does not help there, because cobra's suggestions never fire for
// these words — "set" is too far from variable/connection/airflow-variable by
// edit distance and shares no prefix with any of them.
//
// A form carrying a set-only flag reports the flag rather than the command,
// because cobra parses flags against `env` before it reaches the RunE. That is
// worth knowing rather than working around: --project and --global are `env`'s
// own, so the forms a user is most likely to retype still get the guidance.
func TestTheBareVerbFormsNameTheirReplacement(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		namesVerb bool
	}{
		{[]string{"local", "env", "set", "API_URL"}, true},
		{[]string{"local", "env", "set", "API_URL", "--project"}, true},
		{[]string{"local", "env", "get", "API_URL"}, true},
		{[]string{"local", "env", "delete", "API_URL"}, true},
		{[]string{"local", "env", "rm", "API_URL"}, true},
		{[]string{"local", "env", "set", "conn", "warehouse"}, true},
		// Cobra reports the unknown flag first for this one.
		{[]string{"local", "env", "set", "API_URL", "--value", "x"}, false},
	} {
		t.Run(strings.Join(tc.args[2:], " "), func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, tc.args...)
			if err == nil {
				t.Fatalf("%v: want an error, the bare verb form was removed", tc.args)
			}
			if !tc.namesVerb {
				return
			}
			if !strings.Contains(err.Error(), "was removed in v2") {
				t.Errorf("%v: error = %v, want it to say the form was removed", tc.args, err)
			}
			for _, noun := range []string{"variable", "connection", "airflow-variable"} {
				if !strings.Contains(err.Error(), "astro local env "+noun+" ") {
					t.Errorf("%v: error should name the %s form, got: %v", tc.args, noun, err)
				}
			}
		})
	}
}

// A noun's list is the cross-kind list narrowed to that kind, and the
// cross-kind list beside the nouns still reports every kind.
func TestListNarrowsByNounAndStillHasACrossKindForm(t *testing.T) {
	dir := envProject(t, "")
	for _, s := range [][]string{
		{"variable", "AN_ENV", "v"},
		{"connection", "a_conn", "sqlite:///x.db"},
		{"airflow-variable", "a_var", "v"},
	} {
		d, _, _ := envDeps(t, dir, "")
		if err := execute(t, d, "local", "env", s[0], "set", s[1], "--value", s[2]); err != nil {
			t.Fatal(err)
		}
	}

	kinds := func(args ...string) []localenv.Kind {
		t.Helper()
		d, out, _ := envDeps(t, dir, "")
		if err := execute(t, d, append([]string{"local", "env"}, append(args, "--output", "json")...)...); err != nil {
			t.Fatal(err)
		}
		var seen []localenv.Kind
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			if line == "" {
				continue
			}
			var it localenv.ListItem
			if err := json.Unmarshal([]byte(line), &it); err != nil {
				t.Fatalf("decode %q: %v", line, err)
			}
			seen = append(seen, it.Kind)
		}
		return seen
	}

	for _, tc := range []struct {
		noun string
		want localenv.Kind
	}{
		{"variable", localenv.KindEnv},
		{"connection", localenv.KindConn},
		{"airflow-variable", localenv.KindVar},
	} {
		got := kinds(tc.noun, "list")
		if len(got) == 0 {
			t.Errorf("%s list returned nothing", tc.noun)
		}
		for _, k := range got {
			if k != tc.want {
				t.Errorf("%s list returned a %q row, want only %q", tc.noun, k, tc.want)
			}
		}
	}

	all := map[localenv.Kind]bool{}
	for _, k := range kinds("list") {
		all[k] = true
	}
	for _, want := range []localenv.Kind{localenv.KindEnv, localenv.KindConn, localenv.KindVar} {
		if !all[want] {
			t.Errorf("the cross-kind list is missing %q rows: %v", want, all)
		}
	}
}

// A connection can be given whole or field by field, and the two have to
// produce the same stored record — otherwise offering both shapes is worse
// than offering one. The cloud side asserts the same equivalence about the API
// request it builds; this asserts it about the .env line.
func TestConnFieldFlagsAndURIStoreTheSameRecord(t *testing.T) {
	stored := func(t *testing.T, args ...string) string {
		t.Helper()
		dir := envProject(t, "")
		d, _, _ := envDeps(t, dir, "")
		set := append([]string{"local", "env", "connection", "set", "warehouse"}, args...)
		if err := execute(t, d, set...); err != nil {
			t.Fatalf("set %v: %v", args, err)
		}
		d, out, _ := envDeps(t, dir, "")
		if err := execute(t, d, "local", "env", "connection", "get", "warehouse", "--output", "json"); err != nil {
			t.Fatalf("get after %v: %v", args, err)
		}
		var v envValue
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("decode %q: %v", out.String(), err)
		}
		return v.Value
	}

	viaURI := stored(t, "--value", "postgres://admin:pw@db.example.com:5432/warehouse")
	viaFields := stored(t,
		"--type", "postgres", "--host", "db.example.com", "--login", "admin",
		"--password", "pw", "--port", "5432", "--schema", "warehouse")

	if viaURI == "" {
		t.Fatal("the URI form stored nothing")
	}
	if viaURI != viaFields {
		t.Errorf("the two shapes stored different records:\n  uri:    %s\n  fields: %s", viaURI, viaFields)
	}
}

// The field flags are the cloud sibling's, long-name for long-name, so one
// invocation describes the same connection in both trees. Only the shorthands
// differ, deliberately.
func TestConnFieldFlagNamesMatchTheCloudTree(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "http_api",
		"--type", "http", "--host", "api.example.com",
		"--extra", `{"timeout":30}`); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "get", "http_api", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var v envValue
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"conn_type":"http"`, `"host":"api.example.com"`, `"timeout":30`} {
		if !strings.Contains(v.Value, want) {
			t.Errorf("stored record is missing %s:\n%s", want, v.Value)
		}
	}
}

// The field flags describe a whole connection, not a patch, so they cannot be
// combined with a whole value — and a connection still needs a type from
// somewhere.
func TestConnSetRejectsMixedOrTypelessInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			"value with a field flag",
			[]string{"--value", "postgres://h/db", "--host", "other.example.com"},
			"none of the others can be",
		},
		{
			"fields with no type",
			[]string{"--host", "db.example.com"},
			"a connection needs a type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, append([]string{"local", "env", "connection", "set", "warehouse"}, tc.args...)...)
			if err == nil {
				t.Fatalf("%v: want an error", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%v: error = %v, want it to contain %q", tc.args, err, tc.want)
			}
		})
	}
}

// Only a connection takes field flags: a variable and an Airflow Variable are
// a name and a value, so there is nothing to break out.
func TestOnlyConnectionsTakeFieldFlags(t *testing.T) {
	for _, noun := range []string{"variable", "airflow-variable"} {
		t.Run(noun, func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, "local", "env", noun, "set", "NAME", "--type", "postgres")
			if err == nil {
				t.Fatalf("%s set accepted --type", noun)
			}
			if !strings.Contains(err.Error(), "unknown flag") {
				t.Errorf("%s: error = %v, want an unknown-flag error", noun, err)
			}
		})
	}
}

// A password given field-wise has to have a path that is not argv. It did not:
// the field branch returned before the piped-stdin branch, so a piped password
// was read by nobody and the set still reported success — writing a connection
// that cannot authenticate, for a reason nothing named.
func TestConnFieldSetTakesThePasswordFromStdin(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []string
	}{
		{"piped", nil},
		{"explicit --stdin", []string{"--stdin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "s3cr3t\n")
			args := append([]string{
				"local", "env", "connection", "set", "db",
				"--type", "postgres", "--host", "h", "--login", "u",
			}, tc.extra...)
			if err := execute(t, d, args...); err != nil {
				t.Fatal(err)
			}

			d, out, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", "connection", "get", "db", "--output", "json"); err != nil {
				t.Fatal(err)
			}
			var v envValue
			if err := json.Unmarshal(out.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(v.Value, `"password":"s3cr3t"`) {
				t.Errorf("the piped password did not reach the stored connection:\n%s", v.Value)
			}
		})
	}
}

// An empty read means "no password", not "the password is the empty string".
// Field mode builds a whole connection, so this is what stops a stray pipe —
// stdin is not a terminal in CI — from writing a passwordless connection over
// a deliberate one.
func TestConnFieldSetTreatsAnEmptyPipeAsNoPassword(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "api",
		"--type", "http", "--host", "api.example.com"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "get", "api", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var v envValue
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(v.Value, "password") {
		t.Errorf("a passwordless connection should carry no password field:\n%s", v.Value)
	}
}

// The narrowed list's empty line has to be about the kind asked for. The
// cross-kind wording claims nothing is in any .env, which is false whenever
// the project holds values of the other kinds.
func TestNarrowedListSaysWhichKindIsEmpty(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "A_VAR", "--value", "x"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "list"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "No connections declared") {
		t.Errorf("narrowed empty list should name the kind, got:\n%s", got)
	}
	if strings.Contains(got, "no entries in any .env") {
		t.Errorf("narrowed empty list still claims the whole project is empty:\n%s", got)
	}
}

// Prose names the noun, not the wire token. `var` is a plain environment
// variable in this tree, so an Airflow Variable command answering with "var"
// asserts the opposite of the grammar — the very collision this tree removed.
func TestMessagesNameTheNounNotTheWireToken(t *testing.T) {
	dir := envProject(t, "")
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "airflow-variable", "set", "region", "--value", "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "set airflow-variable region") {
		t.Errorf("set message should name the noun, got: %s", out.String())
	}

	d, _, _ = envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "airflow-variable", "get", "nope", "--project")
	if err == nil {
		t.Fatal("want an error for a missing Airflow Variable")
	}
	if !strings.Contains(err.Error(), "airflow-variable") {
		t.Errorf("error should name the noun, got: %v", err)
	}
}

// An unknown subcommand must fail, and should say what was probably meant.
// Dropping the RunE entirely gets the suggestions from cobra but makes the
// group print help and exit 0, so a typo in CI would read as success.
func TestUnknownSubcommandFailsAndSuggests(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"local", "env", "connection", "seet", "x"}, "set"},
		//nolint:misspell // a deliberate typo: the case exists to prove the suggestion fires, and misspell --fix would otherwise silently repair it into a valid command
		{[]string{"local", "env", "conection", "set", "x"}, "connection"},
	} {
		t.Run(strings.Join(tc.args[2:], " "), func(t *testing.T) {
			dir := envProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, tc.args...)
			if err == nil {
				t.Fatalf("%v: want an error, not help with a zero exit", tc.args)
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Errorf("%v: error = %v", tc.args, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%v: error should suggest %q, got: %v", tc.args, tc.want, err)
			}
		})
	}
}

// Every noun lists its verbs in the order `astro env` lists the same four:
// reads, then the write, then the destructive one. The two trees shipped in
// different orders until this was pinned, and nothing failed when they drifted
// — the order is AddCommand order, which no other test looks at.
//
// The cross-kind `list` sits last under `env`, after the three nouns, because
// it is the odd one out rather than a fourth noun.
func TestEnvVerbOrderMatchesTheCloudTree(t *testing.T) {
	// AddCommand order only reaches help because cmd/root.go sets
	// cobra.EnableCommandSorting = false process-wide. This test builds the
	// tree directly, so with sorting left on Commands() comes back
	// alphabetical and the assertion measures nothing a user sees.
	sorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	defer func() { cobra.EnableCommandSorting = sorting }()

	d, _ := testDeps(t)
	root := NewRootCmd(d)

	names := func(path ...string) []string {
		t.Helper()
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		var got []string
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				got = append(got, sub.Name())
			}
		}
		return got
	}

	for _, noun := range []string{"variable", "connection", "airflow-variable"} {
		t.Run(noun, func(t *testing.T) {
			assert.Equal(t, []string{"list", "get", "set", "delete"}, names("local", "env", noun))
		})
	}

	t.Run("the env group", func(t *testing.T) {
		assert.Equal(t,
			[]string{"variable", "connection", "airflow-variable", "list"},
			names("local", "env"))
	})
}

// --extra carries account and project ids, which must survive being decoded
// and re-encoded. Decoded into map[string]any every number becomes a float64,
// and re-marshaling one above 2^53 writes a different number: a Snowflake
// account id came back eleven off with nothing reporting it.
func TestConnExtraKeepsLargeIntegersExact(t *testing.T) {
	const id = "1234567890123456789"
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "wh",
		"--type", "snowflake", "--extra", `{"account_id":`+id+`}`); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "get", "wh", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var v envValue
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.Value, id) {
		t.Errorf("the account id was rewritten:\n%s", v.Value)
	}
}

// `[1,2]` is valid JSON, so telling the user it is not sends them looking for
// a syntax error that is not there.
func TestConnExtraRejectsNonObjectsByName(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "connection", "set", "wh",
		"--type", "http", "--extra", "[1,2]")
	if err == nil {
		t.Fatal("want an error for a non-object --extra")
	}
	if !strings.Contains(err.Error(), "must be a JSON object") {
		t.Errorf("error = %v, want it to say --extra must be an object", err)
	}
}

// --stdin names where a secret comes from, so pairing it with a flag that also
// supplies one threw half the input away in silence.
func TestConnSetRefusesStdinTogetherWithPassword(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "piped\n")
	err := execute(t, d, "local", "env", "connection", "set", "db",
		"--type", "postgres", "--password", "flagpw", "--stdin")
	if err == nil {
		t.Fatal("want an error: --stdin and --password both supply the password")
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Errorf("error = %v, want cobra's mutual-exclusion error", err)
	}
}

// The subcommand word and the word every hint is composed from have to be the
// same word. Spelled twice, a rename leaves the hints naming a command that no
// longer exists — the failure localenv.Noun exists to prevent.
func TestNounSubcommandMatchesTheHintWord(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, kind := range []localenv.Kind{localenv.KindEnv, localenv.KindConn, localenv.KindVar} {
		noun := localenv.Noun(kind)
		t.Run(noun, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"local", "env", noun})
			if err != nil {
				t.Fatalf("find %q: %v", noun, err)
			}
			if cmd.Name() != noun {
				t.Errorf("localenv.Noun(%q) = %q but no such subcommand; found %q",
					kind, noun, cmd.Name())
			}
		})
	}
}

// A login with no password is almost always the mistake it looks like, and
// the set otherwise reports plain success for a connection that cannot
// authenticate. It is a warning rather than an error because passwordless
// connections are legitimate — http, fs, an aws one on an instance role.
func TestConnFieldSetWarnsOnALoginWithNoPassword(t *testing.T) {
	dir := envProject(t, "")
	d, _, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db",
		"--type", "postgres", "--host", "h", "--login", "admin"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "login but no password") {
		t.Errorf("want a warning about the missing password, got: %q", stderr.String())
	}

	// And none when the connection genuinely has no login.
	d, _, stderr = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "api",
		"--type", "http", "--host", "api.example.com"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "login but no password") {
		t.Errorf("a passwordless connection with no login should warn nothing, got: %q", stderr.String())
	}
}
