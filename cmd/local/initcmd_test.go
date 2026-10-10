package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/scaffold"
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

// write1xProject lays out the 1.x shape scaffold.Is1xProject recognizes: a
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

// APC deploys the 1.x layout (with Astro CLI 1.x), so under an APC context init
// refuses to convert a 1.x project and writes nothing: the project keeps
// deploying with Astro CLI 1.x until APC deploys pyproject.toml projects. The
// refusal is no mistake in the command line, so it is not usage: kind
// unsupported_on_platform, exit 1.
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
			requireRefused(t, err)
			requireAPCAdvice(t, err, dir)
			requireUnchanged(t, dir, before, files)
			if format == "json" {
				obj := errorObjectOf(t, stdout.String())
				if obj.Kind != KindUnsupportedOnPlatform || obj.Code != 1 || obj.Error != project.Blocked1xMessage(project.BlockedUnderAPC, dir) {
					t.Errorf("error object = %+v", obj)
				}
			}
		})
	}
}

// requireRefused fails unless err is init's refusal of a 1.x project for the
// platform: unsupported_on_platform, exit 1, not usage.
func requireRefused(t *testing.T, err error) {
	t.Helper()
	if err == nil || !errors.Is(err, scaffold.ErrConvert1xUnderAPC) {
		t.Fatalf("want the 1.x refusal, got %v", err)
	}
	if cliout.IsUsage(err) || ProblemKinds.Of(err) != KindUnsupportedOnPlatform {
		t.Errorf("kind = %q, usage = %v", ProblemKinds.Of(err), cliout.IsUsage(err))
	}
}

// errorObjectOf decodes the one error object stdout holds.
func errorObjectOf(t *testing.T, stdout string) cliout.ErrorObject {
	t.Helper()
	var obj cliout.ErrorObject
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("stdout is not one error object: %v\n%s", err, stdout)
	}
	return obj
}

// requireAPCAdvice fails unless err is the one account every hint under APC
// gives (project.Blocked1xMessage) of the 1.x project in dir: why it stays,
// and how to convert anyway, in plain text.
func requireAPCAdvice(t *testing.T, err error, dir string) {
	t.Helper()
	if want := project.Blocked1xMessage(project.BlockedUnderAPC, dir); err.Error() != want {
		t.Errorf("error = %q\nwant    %q", err, want)
	}
	for _, want := range []string{
		"the current context is Astro Private Cloud",
		"Leave the project as it is for now: Astro CLI 1.x keeps deploying it to Astro Private Cloud",
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

// A directory in no 1.x project is made a project under APC, or an
// unresolved context, as anywhere. In text mode a notice on stderr says the
// project does not deploy there yet, worded for the context and for whether
// ASTRO_DOMAIN chose it; it is not one of the result's notes, which are what
// is left to do, so json says nothing of it.
func TestInitScaffoldsUnderAPC(t *testing.T) {
	for _, ctx := range []string{"apc", "apc-env", "unresolved", "unresolved-env"} {
		for _, format := range []string{"text", "json"} {
			t.Run(ctx+" "+format, func(t *testing.T) {
				d, dir, stdout := initDeps(t)
				stderr := &strings.Builder{}
				d.Stderr = stderr
				setContextFor(t, ctx)
				notice := requireNotice(t, strings.HasSuffix(ctx, "-env"))
				args := []string{"init"}
				if format == "json" {
					args = append(args, "-o", "json")
				}
				if err := execute(t, d, args...); err != nil {
					t.Fatalf("astro init: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
					t.Errorf("missing pyproject.toml: %v", err)
				}
				if strings.Contains(stdout.String(), "Note:") {
					t.Errorf("stdout carries the notice:\n%s", stdout)
				}
				if got := strings.Contains(stderr.String(), notice); got != (format == "text") {
					t.Errorf("the notice on stderr = %v in %s:\n%s", got, format, stderr)
				}
			})
		}
	}
}

// requireNotice is the new-project notice for the context a case set, which
// names ASTRO_DOMAIN exactly when it chose the context, in plain text.
func requireNotice(t *testing.T, env bool) string {
	t.Helper()
	notice := project.NewProjectNotice()
	if notice == "" || strings.Contains(notice, "`") {
		t.Fatalf("notice = %q", notice)
	}
	if strings.Contains(notice, "ASTRO_DOMAIN") != env {
		t.Errorf("notice names ASTRO_DOMAIN = %v, want %v: %q", !env, env, notice)
	}
	return notice
}

// setUnderAPC records an APC context for one test, as the root does at
// startup, and puts it back. Tests that call it do not run in parallel.
func setUnderAPC(t *testing.T) {
	t.Helper()
	project.SetContext(project.Context{APC: true})
	t.Cleanup(func() { project.SetContext(project.Context{}) })
}

// Under APC, a directory inside a 1.x project is refused too, existing or
// new, naming the project: a project scaffolded there would be deployed with
// it by Astro CLI 1.x.
func TestInitUnderAPCRefusesInsideA1xProject(t *testing.T) {
	d, root, _ := initDeps(t)
	setUnderAPC(t)
	files := write1xProject(t, root)
	sub := filepath.Join(root, "dags")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return sub, nil }
	before := listTree(t, root)
	for _, args := range [][]string{{"init"}, {"init", "fresh"}} {
		err := execute(t, d, args...)
		requireRefused(t, err)
		requireAPCAdvice(t, err, root)
	}
	requireUnchanged(t, root, before, files)
}

// The check reads the directories themselves, whatever is around them: a
// 1.x project in a monorepo whose root pyproject.toml only configures tools
// is refused, and so is one that is the home directory, as in a 1.x image.
func TestInitUnderAPCRefusesA1xProjectWhereverItIs(t *testing.T) {
	t.Run("in a monorepo", func(t *testing.T) {
		d, repo, _ := initDeps(t)
		setUnderAPC(t)
		if err := os.WriteFile(filepath.Join(repo, "pyproject.toml"), []byte("[tool.ruff]\nline-length = 100\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		proj := filepath.Join(repo, "airflow")
		files := write1xProject(t, proj)
		before := listTree(t, proj)
		err := execute(t, d, "init", "airflow")
		requireRefused(t, err)
		requireAPCAdvice(t, err, proj)
		requireUnchanged(t, proj, before, files)
	})
	t.Run("as the home directory", func(t *testing.T) {
		d, home, _ := initDeps(t)
		setUnderAPC(t)
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		files := write1xProject(t, home)
		before := listTree(t, home)
		err := execute(t, d, "init")
		requireRefused(t, err)
		requireAPCAdvice(t, err, home)
		requireUnchanged(t, home, before, files)
	})
}

// A context the CLI cannot resolve may be APC's, so init refuses a 1.x
// project there as well, saying to fix the context; a directory in no 1.x
// project is made a project as anywhere.
func TestInitRefusesA1xProjectUnderAnUnresolvedContext(t *testing.T) {
	d, dir, stdout := initDeps(t)
	project.SetContext(project.Context{Unresolved: true})
	t.Cleanup(func() { project.SetContext(project.Context{}) })
	files := write1xProject(t, dir)
	before := listTree(t, dir)

	err := execute(t, d, "init", "-o", "json")
	requireRefused(t, err)
	for _, want := range []string{"the current context cannot be resolved", "astro context switch", "run astro init in " + dir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
	if obj := errorObjectOf(t, stdout.String()); obj.Kind != KindUnsupportedOnPlatform {
		t.Errorf("error object = %+v", obj)
	}
	requireUnchanged(t, dir, before, files)

	fresh := filepath.Join(t.TempDir(), "fresh")
	if err := execute(t, d, "init", fresh); err != nil {
		t.Errorf("astro init %s: %v", fresh, err)
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
			advice := project.Blocked1xMessage(project.BlockedUnderAPC, dir)

			err := execute(t, d, "local", "status")
			if err == nil {
				t.Fatal("astro local status in a 1.x project must fail")
			}
			if got := strings.Contains(err.Error(), advice); got != apc {
				t.Errorf("discovery error carries the APC advice = %v, want %v:\n%v", got, apc, err)
			}
			stdout.Reset()
			if err := execute(t, d, "local", "status", "-o", "json"); err == nil {
				t.Fatal("astro local status in a 1.x project must fail")
			}
			obj := errorObjectOf(t, stdout.String())
			if got := obj.Error == advice; got != apc {
				t.Errorf("json error is the APC advice = %v, want %v: %+v", got, apc, obj)
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

// devPayload is the part of the astro dev stub's json these tests read,
// decoded by key rather than into the payload's Go type.
type devPayload struct {
	Replacement       string           `json:"replacement"`
	Mapping           []devReplacement `json:"mapping"`
	Is1xProject       bool             `json:"v1_project"`
	Notes             []string         `json:"notes"`
	V1Dir             string           `json:"v1_dir"`
	Convert           string           `json:"convert"`
	UnderAPC          bool             `json:"under_apc"`
	ContextUnresolved bool             `json:"context_unresolved"`
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
	var p devPayload
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if !p.UnderAPC || !p.Is1xProject || p.Convert != "" || p.Replacement != "" {
		t.Errorf("payload = %+v", p)
	}
	if !slices.Contains(p.Notes, project.Blocked1xMessage(project.BlockedUnderAPC, dir)) {
		t.Errorf("notes lack the APC advice: %q", p.Notes)
	}
	if len(p.Mapping) != 0 {
		t.Errorf("the mapping names astro local commands under APC: %v", p.Mapping)
	}
	// The text is the notes and nothing else: no replacement, and no table
	// of astro local commands, none of which runs in this project.
	err = execute(t, d, "dev", "init")
	if err == nil {
		t.Fatal("astro dev must fail")
	}
	text := err.Error()
	for _, n := range p.Notes {
		if !strings.Contains(text, n) {
			t.Errorf("text lacks the note %q:\n%s", n, text)
		}
	}
	if strings.Contains(text, "Use `") || strings.Contains(text, "# was: astro dev") {
		t.Errorf("text names astro local commands under APC:\n%s", text)
	}

	// With flags too: a build secret names no replacement here.
	stdout.Reset()
	err = execute(t, d, "dev", "start", "--build-secret", "id=mysecret,src=secret.txt", "-o", "json")
	if err == nil {
		t.Fatal("astro dev must fail")
	}
	p = devPayload{}
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if p.Replacement != "" || !p.UnderAPC || !p.Is1xProject {
		t.Errorf("payload with a build secret = %+v", p)
	}

	// Below the 1.x project the stub gives the advice it gives at the root,
	// since init refuses there too.
	sub := filepath.Join(dir, "dags")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return sub, nil }
	stdout.Reset()
	if err := execute(t, d, "dev", "start", "-o", "json"); err == nil {
		t.Fatal("astro dev must fail")
	}
	p = devPayload{}
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if !p.Is1xProject || !p.UnderAPC || p.Replacement != "" || !slices.Contains(p.Notes, project.Blocked1xMessage(project.BlockedUnderAPC, dir)) {
		t.Errorf("payload below the project = %+v", p)
	}
}

// The astro dev stub and astro init give the same answer, in every context
// (ASTRO_DOMAIN choosing it, or the saved one) and wherever they run: where
// init refuses, the stub offers no astro init and gives init's own message;
// where init goes ahead, the stub suggests it for the 1.x root it is in, or
// nothing for a directory that is not in a 1.x project.
func TestTheDevStubAndInitAgree(t *testing.T) {
	for _, ctx := range []string{"apc", "apc-env", "astro", "unresolved", "unresolved-env"} {
		for _, where := range []string{"root", "subdir", "fresh"} {
			t.Run(ctx+" "+where, func(t *testing.T) {
				d, root, stdout := initDeps(t)
				setContextFor(t, ctx)
				wd := layOut1x(t, root, where)
				d.WorkingDir = func() (string, error) { return wd, nil }
				p := devStubJSON(t, d, stdout)
				blocked := ctx != "astro" && where != "fresh"
				apc, unresolved := strings.HasPrefix(ctx, "apc"), strings.HasPrefix(ctx, "unresolved")
				if p.UnderAPC != (blocked && apc) || p.ContextUnresolved != (blocked && unresolved) {
					t.Errorf("stub reasons = apc %v, unresolved %v: %+v", p.UnderAPC, p.ContextUnresolved, p)
				}
				if where != "fresh" && p.V1Dir != root {
					t.Errorf("v1_dir = %q, want the root %q", p.V1Dir, root)
				}
				// From below the root, the command converts the root, not a
				// project inside it.
				if want := replaceInit + " " + root; ctx == "astro" && where == "subdir" && p.Convert != want {
					t.Errorf("convert = %q, want %q", p.Convert, want)
				}
				requireStubMatchesInit(t, p, execute(t, d, "init"))
			})
		}
	}
}

// setContextFor records the context a case runs under, and puts it back.
func setContextFor(t *testing.T, ctx string) {
	t.Helper()
	c := project.Context{FromASTRODomain: strings.HasSuffix(ctx, "-env")}
	switch {
	case strings.HasPrefix(ctx, "apc"):
		c.APC = true
	case strings.HasPrefix(ctx, "unresolved"):
		c.Unresolved = true
	}
	project.SetContext(c)
	t.Cleanup(func() { project.SetContext(project.Context{}) })
}

// layOut1x makes root a 1.x project unless where is fresh, and returns the
// directory a case runs in: root, or a subdirectory of it.
func layOut1x(t *testing.T, root, where string) string {
	t.Helper()
	if where == "fresh" {
		return root
	}
	write1xProject(t, root)
	if where == "root" {
		return root
	}
	sub := filepath.Join(root, "dags")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	return sub
}

// devStubJSON runs astro dev start under json and decodes what it published.
func devStubJSON(t *testing.T, d Deps, stdout *strings.Builder) devPayload {
	t.Helper()
	stdout.Reset()
	if err := execute(t, d, "dev", "start", "-o", "json"); err == nil {
		t.Fatal("astro dev must fail")
	}
	var p devPayload
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	return p
}

// requireStubMatchesInit fails unless the stub refuses exactly where init
// does (err), names no command where it does, and offers astro init for a
// 1.x project where init converts it.
func requireStubMatchesInit(t *testing.T, p devPayload, err error) {
	t.Helper()
	initRefuses := errors.Is(err, scaffold.ErrConvert1xUnderAPC)
	if !initRefuses && err != nil {
		t.Fatalf("astro init: %v", err)
	}
	stubRefuses := p.UnderAPC || p.ContextUnresolved
	if stubRefuses != initRefuses {
		t.Errorf("the stub refuses = %v, init refuses = %v", stubRefuses, initRefuses)
	}
	if stubRefuses && (p.Convert != "" || p.Replacement != "" || len(p.Mapping) != 0) {
		t.Errorf("a refusing stub names a command: %+v", p)
	}
	if stubRefuses && !slices.Contains(p.Notes, err.Error()) {
		t.Errorf("the stub's advice is not init's:\nstub %q\ninit %q", p.Notes, err)
	}
	if !stubRefuses && p.Is1xProject && !strings.HasPrefix(p.Convert, replaceInit) {
		t.Errorf("a 1.x project where init converts is not offered astro init: %+v", p)
	}
}

// From below a 1.x root, the command that converts it names the root, quoted
// when it holds a space: as astro dev init's replacement, in the mapping, in
// convert, and in the text's example row alike.
func TestTheDevStubConvertsTheRootFromBelow(t *testing.T) {
	d, base, stdout := initDeps(t)
	root := filepath.Join(base, "my project")
	write1xProject(t, root)
	sub := filepath.Join(root, "dags")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return sub, nil }
	want := replaceInit + " '" + root + "'"

	stdout.Reset()
	if err := execute(t, d, "dev", "init", "-o", "json"); err == nil {
		t.Fatal("astro dev must fail")
	}
	var p devPayload
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if p.Replacement != want || p.Convert != want || p.V1Dir != root {
		t.Errorf("payload = %+v, want replacement and convert %q", p, want)
	}
	for _, m := range p.Mapping {
		if m.Command == "init" && m.Replacement != want {
			t.Errorf("the mapping's init = %q, want %q", m.Replacement, want)
		}
	}

	err := execute(t, d, "dev", "init")
	if err == nil {
		t.Fatal("astro dev must fail")
	}
	for _, line := range []string{"Use `" + want + "` instead", want + " ", "Run " + want + " to convert it in place"} {
		if !strings.Contains(err.Error(), line) {
			t.Errorf("text lacks %q:\n%v", line, err)
		}
	}
}
