package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
)

// initDeps pins WorkingDir to one directory (testDeps mints a fresh temp dir
// per call, which init tests cannot use).
func initDeps(t *testing.T) (d Deps, dir string, stdout *strings.Builder) {
	t.Helper()
	d, _ = testDeps(t)
	dir = t.TempDir()
	stdout = &strings.Builder{}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Stdout = stdout
	return d, dir, stdout
}

func TestInitScaffoldsTheWorkingDir(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	for _, f := range []string{"pyproject.toml", ".gitignore", "AGENTS.md", "dags", "include", "plugins", "tests"} {
		if _, err := os.Lstat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	out := stdout.String()
	if !strings.Contains(out, "Created Astro project") || !strings.HasSuffix(out, "Next: "+replaceStart+"\n") {
		t.Errorf("text output incomplete:\n%s", out)
	}
}

// A kept Dockerfile is built only in Docker mode, so that is the start to
// suggest: a plain start runs standalone and leaves the file out.
func TestInitSuggestsDockerForAKeptDockerfile(t *testing.T) {
	d, dir, stdout := initDeps(t)
	dockerfile := "FROM astrocrpublic.azurecr.io/runtime:3.1-1\nRUN apt-get update && apt-get install -y libpq-dev\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "dockerfile") {
		t.Fatalf("the case needs init to keep and declare the Dockerfile:\n%s", manifest)
	}
	if out := stdout.String(); !strings.HasSuffix(out, "Next: "+replaceStart+" --docker\n") {
		t.Errorf("want the Docker-mode start suggested:\n%s", out)
	}
}

// OS packages are the other thing only Docker mode applies.
func TestInitSuggestsDockerForOSPackages(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := os.WriteFile(filepath.Join(dir, "packages.txt"), []byte("libpq-dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if out := stdout.String(); !strings.HasSuffix(out, "Next: "+replaceStart+" --docker\n") {
		t.Errorf("want the Docker-mode start suggested:\n%s", out)
	}
}

func TestInitScaffoldsARelativeDirectory(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := execute(t, d, "init", "pipelines"); err != nil {
		t.Fatalf("astro init pipelines: %v", err)
	}
	m, err := os.ReadFile(filepath.Join(dir, "pipelines", "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// The surgical TOML editor may quote with ' or ".
	if !strings.Contains(string(m), `name = 'pipelines'`) && !strings.Contains(string(m), `name = "pipelines"`) {
		t.Errorf("manifest does not carry the derived name:\n%s", m)
	}
}

func TestInitJSONOutput(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := execute(t, d, "init", "--output", "json"); err != nil {
		t.Fatalf("astro init --output json: %v", err)
	}
	var payload struct {
		Dir     string   `json:"dir"`
		Name    string   `json:"name"`
		Airflow string   `json:"airflow"`
		Created []string `json:"created"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout.String())
	}
	if payload.Dir != dir {
		t.Errorf("dir = %q, want %q", payload.Dir, dir)
	}
	if payload.Name != filepath.Base(dir) {
		t.Errorf("name = %q, want %q", payload.Name, filepath.Base(dir))
	}
	if payload.Airflow == "" || len(payload.Created) == 0 {
		t.Errorf("payload missing airflow or created: %+v", payload)
	}
}

func TestInitAdoptsAnExistingManifest(t *testing.T) {
	d, dir, stdout := initDeps(t)
	existing := "[project]\nname = 'orders'\ndependencies = ['requests']\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init over an existing manifest: %v", err)
	}
	m, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(m), "[tool.astro]") {
		t.Errorf("manifest gained no astro section:\n%s", m)
	}
	if !strings.Contains(string(m), "requests") {
		t.Errorf("existing dependency was lost:\n%s", m)
	}
	if out := stdout.String(); !strings.Contains(out, "Adopted Astro project orders") {
		t.Errorf("adoption not reported:\n%s", out)
	}
}

func TestInitRefusesAnAstroProject(t *testing.T) {
	d, dir, _ := initDeps(t)
	existing := "[project]\nname = 'orders'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	err := execute(t, d, "init")
	if err == nil || !strings.Contains(err.Error(), "already an Astro project") {
		t.Errorf("want a clear refusal, got: %v", err)
	}
}

func TestInitListsWhatItCouldNotCarry(t *testing.T) {
	d, dir, stdout := initDeps(t)
	// Every one of these is READ now, so the hand-off list is what is left over
	// rather than the files themselves — listing a carried file would be
	// telling the user to redo work init just did.
	//
	// airflow_settings.yaml is here for its pools, which move into
	// [tool.astro.pools], so the file goes and nothing about it is left to do.
	for name, body := range map[string]string{
		"requirements.txt":      "flask==2.0\n",
		"airflow_settings.yaml": "airflow:\n  pools:\n    - pool_name: heavy\n      pool_slot: 4\n",
		"Dockerfile":            "FROM quay.io/astronomer/astro-runtime:9\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	out := stdout.String()
	_, handoff, found := strings.Cut(out, "Left to do:")
	if !found {
		t.Fatalf("no hand-off list:\n%s", out)
	}
	if strings.Contains(handoff, "heavy") || strings.Contains(handoff, "airflow_settings.yaml") {
		t.Errorf("the carried pool is still left to do:\n%s", out)
	}
	if !strings.Contains(out, "migrated 1 pool from airflow_settings.yaml into [tool.astro.pools]") {
		t.Errorf("the run did not say the pool moved:\n%s", out)
	}
	// And not the old instruction to move the whole file by hand, which now
	// contradicts the same run's report of what it carried.
	if strings.Contains(handoff, "airflow_settings.yaml: move") {
		t.Errorf("still asking for work the conversion did:\n%s", out)
	}
	// The Dockerfile still appears, but saying something different: its runtime
	// 9 tag names Airflow 2 without a minor, so the pin is the coarse "2".
	if !strings.Contains(handoff, "does not name the Airflow minor") {
		t.Errorf("the coarse Airflow 2 pin was not explained:\n%s", out)
	}
	// Scoped to the hand-off section, not the whole output, and deliberately.
	// This assertion was written against `out` and passed only because the
	// greenfield arm printed no label at all; now that it reports "migrated 1
	// from requirements.txt into dependencies", the file is legitimately named
	// in the CREATED list. What must not happen is it appearing as work left to
	// do, which is a different claim about the same string.
	if strings.Contains(handoff, "requirements.txt") {
		t.Errorf("requirements.txt was carried, so it must not be work left to do:\n%s", out)
	}
	if !strings.Contains(out, "migrated 1 from requirements.txt") {
		t.Errorf("the conversion did not say it carried requirements.txt:\n%s", out)
	}
}

// A greenfield init in a repo that already has a .gitignore updates that file,
// which must not make the command claim it adopted a manifest it wrote itself.
func TestInitSaysCreatedWhenItWroteTheManifest(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.pyc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Created Astro project") {
		t.Errorf("want Created, got:\n%s", out)
	}
	if !strings.Contains(out, ".gitignore (added the .env rule)") {
		t.Errorf("gitignore edit not reported:\n%s", out)
	}
}

// astro local init is gone. `astro local` means the Airflow running on this
// machine, and init writes a manifest — it never belonged to that family. The
// spelling still names where the command went rather than reading as a typo.
func TestLocalInitPointsAtAstroInit(t *testing.T) {
	d, _, _ := initDeps(t)
	err := execute(t, d, "local", "init")
	if err == nil {
		t.Fatal("astro local init should no longer resolve")
	}
	if !strings.Contains(err.Error(), "astro init") {
		t.Errorf("error should name astro init; got %q", err)
	}
}

func TestInitStillWorksAtTheRoot(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Errorf("missing pyproject.toml: %v", err)
	}
}

// write1xProject lays out the 1.x shape project.Is1xProject recognizes: a
// Dockerfile beside .astro/config.yaml, with no manifest.
func write1xProject(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{
		"Dockerfile":         "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n",
		"requirements.txt":   "pandas==2.1.0\n",
		".astro/config.yaml": "project:\n  name: orders\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// listTree is every path under dir, for asserting a run changed nothing.
func listTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		out = append(out, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// APC's deploy still builds the 1.x layout, so under an APC context init
// refuses to convert a 1.x project, as a usage error, and writes nothing: the
// project keeps deploying as it is until APC deploys pyproject.toml projects.
func TestInitRefusesA1xProjectUnderAPC(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			d, dir, stdout := initDeps(t)
			setUnderAPC(t)
			files := write1xProject(t, dir)
			before := listTree(t, dir)

			args := []string{"init"}
			if format == "json" {
				args = append(args, "-o", "json")
			}
			err := execute(t, d, args...)
			if err == nil || !cliout.IsUsage(err) {
				t.Fatalf("want a usage error, got %v", err)
			}
			requireAPCAdvice(t, err, dir)
			requireUnchanged(t, dir, before, files)
			if format == "json" {
				var obj cliout.ErrorObject
				if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
					t.Fatalf("stdout is not one error object: %v\n%s", err, stdout)
				}
				if obj.Kind != cliout.KindUsage || obj.Code != 2 || !strings.Contains(obj.Error, "Astro Private Cloud") {
					t.Errorf("error object = %+v", obj)
				}
			}
		})
	}
}

// requireAPCAdvice fails unless err is the one account every hint under APC
// gives (project.Project1xUnderAPC) of the 1.x project in dir: why it stays,
// and how to convert anyway, in plain text.
func requireAPCAdvice(t *testing.T, err error, dir string) {
	t.Helper()
	if want := project.Project1xUnderAPC(dir); err.Error() != want {
		t.Errorf("error = %q\nwant    %q", err, want)
	}
	for _, want := range []string{
		"the current context is Astro Private Cloud",
		"Leave the project as it is for now",
		"once Astro Private Cloud deploys pyproject.toml projects",
		"switch to an Astro context first (astro context switch astronomer.io",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "`") {
		t.Errorf("the advice is plain text:\n%v", err)
	}
}

// requireUnchanged fails unless dir holds exactly the paths before listed and
// each of files with its contents as written.
func requireUnchanged(t *testing.T, dir string, before []string, files map[string]string) {
	t.Helper()
	if after := listTree(t, dir); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Errorf("the tree changed:\nbefore %v\nafter  %v", before, after)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || string(got) != body {
			t.Errorf("%s changed: %q, %v", name, got, err)
		}
	}
}

// Under an Astro context, or none, a 1.x project converts as it always has.
func TestInitConverts1xProjectUnderAstro(t *testing.T) {
	d, dir, stdout := initDeps(t)
	write1xProject(t, dir)
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Errorf("missing pyproject.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
		t.Error("a pin-only Dockerfile is retired under Astro")
	}
	if strings.Contains(stdout.String(), "Astro Private Cloud") {
		t.Errorf("an Astro conversion says nothing of APC:\n%s", stdout)
	}
}

// A directory that is not a 1.x project is made a project under APC too, with
// a note that APC does not deploy it yet. The note comes with the run that
// made the project, and only that run: init again is refused, as anywhere,
// and says nothing of APC.
func TestInitScaffoldsUnderAPCWithANote(t *testing.T) {
	d, dir, stdout := initDeps(t)
	setUnderAPC(t)
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Errorf("missing pyproject.toml: %v", err)
	}
	if !strings.Contains(stdout.String(), apcDeployNote) {
		t.Errorf("want the APC note:\n%s", stdout)
	}

	stdout.Reset()
	err := execute(t, d, "init")
	if err == nil || !strings.Contains(err.Error(), "is already an Astro project") {
		t.Fatalf("want init again refused, got %v", err)
	}
	if strings.Contains(stdout.String()+err.Error(), "Astro Private Cloud") {
		t.Errorf("a rerun repeats the APC note:\n%s\n%v", stdout, err)
	}
}

// setUnderAPC records an APC context for one test, as the root does at
// startup, and puts it back. Tests that call it do not run in parallel.
func setUnderAPC(t *testing.T) {
	t.Helper()
	project.SetUnderAPC(true)
	t.Cleanup(func() { project.SetUnderAPC(false) })
}

// Under APC, init refuses a directory inside a 1.x project too, as discovery
// would find it, naming the project rather than the directory it ran in.
func TestInitRefusesInsideA1xProjectUnderAPC(t *testing.T) {
	for _, args := range [][]string{{"init"}, {"init", "fresh"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, root, _ := initDeps(t)
			setUnderAPC(t)
			files := write1xProject(t, root)
			sub := filepath.Join(root, "dags")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return sub, nil }
			before := listTree(t, root)
			err := execute(t, d, args...)
			if err == nil || !cliout.IsUsage(err) {
				t.Fatalf("want a usage error, got %v", err)
			}
			requireAPCAdvice(t, err, root)
			requireUnchanged(t, root, before, files)
		})
	}
}

// Under APC, every other hint about a 1.x project says what init's refusal
// says rather than to run astro init, in text and json: a command that
// discovers the project, and the astro dev stub. Under Astro they still say
// to convert.
func TestTheAPCAdviceOn1xProjectsIsOne(t *testing.T) {
	for _, apc := range []bool{true, false} {
		t.Run(map[bool]string{true: "apc", false: "astro"}[apc], func(t *testing.T) {
			d, dir, stdout := initDeps(t)
			if apc {
				setUnderAPC(t)
			}
			write1xProject(t, dir)
			advice := project.Project1xUnderAPC(dir)

			err := execute(t, d, "local", "status")
			if err == nil {
				t.Fatal("astro local status in a 1.x project must fail")
			}
			if got := strings.Contains(err.Error(), advice); got != apc {
				t.Errorf("discovery error carries the APC advice = %v, want %v:\n%v", got, apc, err)
			}
			if err := execute(t, d, "local", "status", "-o", "json"); err == nil ||
				strings.Contains(stdout.String(), "astro init` here") == apc {
				t.Errorf("json discovery error under apc = %v:\n%s", apc, stdout)
			}

			err = execute(t, d, "dev", "start")
			if err == nil {
				t.Fatal("astro dev must fail")
			}
			if got := strings.Contains(err.Error(), advice); got != apc {
				t.Errorf("dev stub carries the APC advice = %v, want %v:\n%v", got, apc, err)
			}
			if converts := strings.Contains(err.Error(), "to convert it in place"); converts == apc {
				t.Errorf("dev stub says to convert = %v under apc = %v:\n%v", converts, apc, err)
			}
		})
	}
}

// Under APC the astro dev stub in a 1.x project says in json what it says in
// text: under_apc, the advice in notes, and no astro init, neither as the
// replacement for astro dev init nor in the mapping. A build secret still
// means the project builds in Docker mode once converted.
func TestTheDevStubUnderAPC(t *testing.T) {
	d, dir, stdout := initDeps(t)
	setUnderAPC(t)
	write1xProject(t, dir)

	err := execute(t, d, "dev", "init", "-o", "json")
	if err == nil {
		t.Fatal("astro dev must fail")
	}
	var p devRemoved
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if !p.UnderAPC || !p.Is1xProject || p.Convert != "" || p.Replacement != "" {
		t.Errorf("payload = %+v", p)
	}
	if !slices.Contains(p.Notes, project.Project1xUnderAPC(dir)) {
		t.Errorf("notes lack the APC advice: %q", p.Notes)
	}
	for _, m := range p.Mapping {
		if m.Replacement == replaceInit {
			t.Errorf("the mapping offers %s under APC", replaceInit)
		}
	}
	text := renderDevRemoved(p)
	for _, n := range p.Notes {
		if !strings.Contains(text, n) {
			t.Errorf("text lacks the note %q:\n%s", n, text)
		}
	}
	if strings.Contains(text, "# was: astro dev init") || strings.Contains(text, "Use `"+replaceInit) {
		t.Errorf("text offers %s under APC:\n%s", replaceInit, text)
	}

	stdout.Reset()
	err = execute(t, d, "dev", "start", "--build-secret", "id=mysecret,src=secret.txt")
	if err == nil || !strings.Contains(err.Error(), replaceStart+" --docker") {
		t.Errorf("a build secret still means Docker mode under APC:\n%v", err)
	}
}
