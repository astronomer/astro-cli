package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
	written, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "dockerfile") {
		t.Fatalf("the case needs init to keep and declare the Dockerfile:\n%s", written)
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
func write1xProject(t *testing.T, dir string) {
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
}

// underAPC is d with an Astro Private Cloud context on domain, as the root
// wires it (withAPCLinking): astro init reads only the domain.
func underAPC(d Deps, domain string) Deps {
	d.APC = &APCLinking{Domain: domain}
	return d
}

// Ids in the shape Astro CLI 1.x saved, which Houston and Astro share.
const (
	savedDeployment1x = "cdeploy000000000000000001"
	savedWorkspace1x  = "cworksp000000000000000001"
)

// Under an Astro Private Cloud context init converts a 1.x project exactly
// as under Astro, in text and json: a pin-only Dockerfile is retired into the
// manifest, and nothing is said about APC, on stdout or stderr.
func TestInitConverts1xProjectUnderAPC(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			d, dir, stdout := initDeps(t)
			stderr := &strings.Builder{}
			d.Stderr = stderr
			write1xProject(t, dir)
			args := []string{"init"}
			if format == "json" {
				args = append(args, "-o", "json")
			}
			if err := execute(t, underAPC(d, "astro.corp.example"), args...); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			m := loadManifest(t, filepath.Join(dir, manifest.Marker))
			if !slices.Contains(m.Project.Dependencies, "pandas==2.1.0") {
				t.Errorf("requirements.txt was not carried: %v", m.Project.Dependencies)
			}
			if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
				t.Error("a pin-only Dockerfile is retired under APC, as under Astro")
			}
			if out := stdout.String() + stderr.String(); strings.Contains(out, "Private Cloud") || strings.Contains(out, "Note:") {
				t.Errorf("a conversion under APC says something of APC:\n%s", out)
			}
		})
	}
}

// A conversion under APC writes what one under Astro writes, byte for byte,
// for a project with no saved deploy target: the context decides nothing
// else.
func TestInitUnderAPCWritesWhatAstroWrites(t *testing.T) {
	convert := func(apc bool) string {
		d, base, _ := initDeps(t)
		dir := filepath.Join(base, "orders")
		write1xProject(t, dir)
		if apc {
			d = underAPC(d, "astro.corp.example")
		}
		if err := execute(t, d, "init", dir); err != nil {
			t.Fatalf("astro init (apc %v): %v", apc, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if astro, apc := convert(false), convert(true); astro != apc {
		t.Errorf("the manifests differ:\nastro:\n%s\napc:\n%s", astro, apc)
	}
}

// A Dockerfile FROM a mirror of Astro Runtime is adopted under APC as under
// Astro: declared as the project's build, since the mirror keeps Runtime's
// repository name and tag, and moved onto the mirror's own -base tag rather
// than onto Astronomer's, so an air-gapped project keeps building from it.
func TestInitAdoptsAMirroredBaseUnderAPC(t *testing.T) {
	for _, from := range []string{
		"registry.corp.example/astronomer/astro-runtime:12.1.0",
		"mirror.corp.example/runtime:3.1-12",
	} {
		t.Run(from, func(t *testing.T) {
			d, dir, _ := initDeps(t)
			write1xProject(t, dir)
			body := "FROM " + from + "\nENV TZ=UTC\n"
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := execute(t, underAPC(d, "astro.corp.example"), "init", "-o", "json"); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			if m := loadManifest(t, filepath.Join(dir, manifest.Marker)); m.Astro.Dockerfile != "Dockerfile" {
				t.Errorf("[tool.astro] dockerfile = %q, want the mirrored-base Dockerfile declared", m.Astro.Dockerfile)
			}
			got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			stages := airflowrt.DockerfileStages(got)
			if final := stages[len(stages)-1]; final.Ref != from+"-base" {
				t.Errorf("the final stage builds FROM %q, want the mirror's %s-base", final.Ref, from)
			}
			if !strings.HasSuffix(string(got), "\nENV TZ=UTC\n") {
				t.Errorf("the Dockerfile's own step is not kept last:\n%s", got)
			}
		})
	}
}

// Under APC a 1.x project's saved deploy target becomes an apc link on the
// context's domain, marked default; under Astro, the same ids become an Astro
// link. The ids cannot say which: both platforms use cuids.
func TestInitLinksTheSavedDeployTargetForTheContext(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + savedDeployment1x + "\n  workspace: " + savedWorkspace1x + "\n"
	for _, tc := range []struct {
		name, domain string
		want         manifest.Link
	}{
		{"apc", "astro.corp.example", manifest.Link{
			Target: "apc", Domain: "astro.corp.example", Workspace: savedWorkspace1x, Deployment: savedDeployment1x,
			Auth: manifest.Auth{Method: manifest.AuthAPC}, Default: true,
		}},
		{"astro", "", manifest.Link{
			Target: "astro", Workspace: savedWorkspace1x, Deployment: savedDeployment1x,
			Auth: manifest.Auth{Method: manifest.AuthAstro}, Default: true,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, dir, _ := initDeps(t)
			write1xProject(t, dir)
			if err := os.WriteFile(filepath.Join(dir, ".astro", "config.yaml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.domain != "" {
				d = underAPC(d, tc.domain)
			}
			if err := execute(t, d, "init", "-o", "json"); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			m := loadManifest(t, filepath.Join(dir, manifest.Marker))
			if got := m.Astro.Deployments[scaffold.Link1xName]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("link = %+v\nwant   %+v", got, tc.want)
			}
		})
	}
}

// Astro CLI 1.x's APC deploy saved project.deployment and never
// project.workspace, and looked the id up in the context's current
// workspace. So under APC init links that pair, asking the context for its
// workspace only then; a workspace it cannot read keeps the note.
func TestInitLinksASavedDeploymentInTheAPCContextsWorkspace(t *testing.T) {
	deploymentOnly := "project:\n  name: orders\n  deployment: " + savedDeployment1x + "\n"
	for _, tc := range []struct {
		name, config string
		ws           string
		wsErr        error
		wantCalls    int
		wantLink     bool
	}{
		{"the context's workspace", deploymentOnly, savedWorkspace1x, nil, 1, true},
		{"an unreadable workspace", deploymentOnly, "", errors.New("no context"), 1, false},
		{"a workspace beside an error", deploymentOnly, savedWorkspace1x, errors.New("half read"), 1, false},
		{"no current workspace", deploymentOnly, "", nil, 1, false},
		{"a saved workspace", "project:\n  name: orders\n  deployment: " + savedDeployment1x + "\n  workspace: " + savedWorkspace1x + "\n", "", nil, 0, true},
		{"nothing saved", "project:\n  name: orders\n", savedWorkspace1x, nil, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, dir, stdout := initDeps(t)
			write1xProject(t, dir)
			if err := os.WriteFile(filepath.Join(dir, ".astro", "config.yaml"), []byte(tc.config), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			d.APC = &APCLinking{Domain: "astro.corp.example", CurrentWorkspace: func() (string, error) {
				calls++
				return tc.ws, tc.wsErr
			}}
			if err := execute(t, d, "init", "-o", "json"); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			if calls != tc.wantCalls {
				t.Errorf("the context's workspace was read %d times, want %d", calls, tc.wantCalls)
			}
			link, linked := loadManifest(t, filepath.Join(dir, manifest.Marker)).Astro.Deployments[scaffold.Link1xName]
			if linked != tc.wantLink {
				t.Fatalf("linked = %v, want %v: %s", linked, tc.wantLink, stdout)
			}
			if linked && (link.Target != "apc" || link.Workspace != savedWorkspace1x || link.Deployment != savedDeployment1x || !link.Default) {
				t.Errorf("link = %+v", link)
			}
			if tc.wsErr != nil && !strings.Contains(stdout.String(), "astro link add prod --deployment "+savedDeployment1x) {
				t.Errorf("an unreadable workspace keeps the note:\n%s", stdout)
			}
		})
	}
}

// devPayload is the part of the astro dev stub's json these tests read,
// decoded by key rather than into the payload's Go type.
type devPayload struct {
	Replacement string           `json:"replacement"`
	Mapping     []devReplacement `json:"mapping"`
	Is1xProject bool             `json:"v1_project"`
	Notes       []string         `json:"notes"`
	V1Dir       string           `json:"v1_dir"`
	Convert     string           `json:"convert"`
}

// Every hint about a 1.x project says to convert it with astro init, under
// APC as under Astro, and init then converts it: a command that discovers
// the project, the astro dev stub wherever it runs in or below the project,
// and the spellings that redirect to astro init.
func TestEvery1xHintOffersAstroInitUnderAPC(t *testing.T) {
	for _, where := range []string{"root", "subdir", "fresh"} {
		t.Run(where, func(t *testing.T) {
			d, root, stdout := initDeps(t)
			d = underAPC(d, "astro.corp.example")
			wd := layOut1x(t, root, where)
			d.WorkingDir = func() (string, error) { return wd, nil }

			requireStubOffersInit(t, devStubJSON(t, d, stdout), root, where)
			if where != "fresh" {
				err := execute(t, d, "local", "status")
				if err == nil || !strings.Contains(err.Error(), "Run astro init in "+scaffold.ShellQuote(root)) {
					t.Errorf("discovery's hint under APC: %v", err)
				}
			}
			for _, args := range [][]string{{"local", "init"}, {"af", "init"}} {
				if err := execute(t, d, args...); err == nil || !cliout.IsUsage(err) || !strings.Contains(err.Error(), replaceInit) {
					t.Errorf("%v under APC: %v", args, err)
				}
			}

			if err := execute(t, d, "init", root); err != nil {
				t.Fatalf("astro init %s under APC: %v", root, err)
			}
			if _, err := os.Stat(filepath.Join(root, manifest.Marker)); err != nil {
				t.Errorf("missing pyproject.toml: %v", err)
			}
		})
	}
}

// requireStubOffersInit fails unless the astro dev stub's payload p, run where
// layOut1x put it, offers astro init for the 1.x project at root (naming root
// from below it) beside the usual replacement, or says nothing of one in a
// fresh directory.
func requireStubOffersInit(t *testing.T, p devPayload, root, where string) {
	t.Helper()
	if where == "fresh" {
		if p.Is1xProject || p.Convert != "" {
			t.Errorf("a directory in no 1.x project: %+v", p)
		}
		return
	}
	want := replaceInit
	if where == "subdir" {
		want += " " + scaffold.ShellQuote(root)
	}
	if !p.Is1xProject || p.V1Dir != root || p.Convert != want || p.Replacement != replaceStart {
		t.Errorf("payload = %+v, want convert %q and v1_dir %q", p, want, root)
	}
	if len(p.Mapping) == 0 {
		t.Error("the mapping is empty")
	}
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

// From below a 1.x root, the command that converts it names the root, quoted
// when it holds a space, in json and text alike; astro dev init's replacement
// stays astro init, which makes a project in the working directory, as 1.x's
// did, in the mapping and the example row too, and the text offers both.
func TestTheDevStubConvertsTheRootFromBelow(t *testing.T) {
	d, base, stdout := initDeps(t)
	root := filepath.Join(base, "my project")
	write1xProject(t, root)
	sub := filepath.Join(root, "dags")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return sub, nil }
	want := replaceInit + " " + scaffold.ShellQuote(root)
	if scaffold.ShellQuote(root) == root {
		t.Fatalf("a path with a space is quoted: %s", scaffold.ShellQuote(root))
	}

	stdout.Reset()
	if err := execute(t, d, "dev", "init", "-o", "json"); err == nil {
		t.Fatal("astro dev must fail")
	}
	var p devPayload
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not the payload: %v\n%s", err, stdout)
	}
	if p.Replacement != replaceInit || p.Convert != want || p.V1Dir != root {
		t.Errorf("payload = %+v, want replacement %q and convert %q", p, replaceInit, want)
	}
	for _, m := range p.Mapping {
		if m.Command == "init" && m.Replacement != replaceInit {
			t.Errorf("the mapping's init = %q, want %q", m.Replacement, replaceInit)
		}
	}

	err := execute(t, d, "dev", "init")
	if err == nil {
		t.Fatal("astro dev must fail")
	}
	line := "Use " + replaceInit + " to make a new project here, or " + want + " to convert the 1.x project at " + root + " in place"
	if !strings.Contains(err.Error(), line) {
		t.Errorf("text lacks %q:\n%v", line, err)
	}
	if n := strings.Count(err.Error(), want); n != 1 {
		t.Errorf("the convert command is named %d times, want once:\n%v", n, err)
	}
}
