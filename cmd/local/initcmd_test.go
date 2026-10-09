package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
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

// --deploy-target decides the platform a conversion is for when it is given,
// and the current context when it is not. APC's deploy builds the 1.x
// Dockerfile, so for APC a pin-only one is kept, and for Astro it is retired.
// The note on what was kept says what decided and how to choose the other.
func TestInitDeployTargetDecidesTheBuild(t *testing.T) {
	for _, tc := range []struct {
		name         string
		apcContext   bool
		flag         string
		wantKept     bool
		wantDecision string
	}{
		{
			name: "an APC context", apcContext: true, wantKept: true,
			wantDecision: "because the current context is Astro Private Cloud (apc.example.com); " +
				"to convert for Astro instead, pass --deploy-target astro",
		},
		{name: "an Astro context"},
		{
			name: "--deploy-target apc under an Astro context", flag: "apc", wantKept: true,
			wantDecision: "because of --deploy-target apc; to convert for Astro instead, pass --deploy-target astro",
		},
		{name: "--deploy-target astro under an APC context", apcContext: true, flag: "astro"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, dir, stdout := initDeps(t)
			d.DeploysToAPC = tc.apcContext
			d.ContextDomain = "apc.example.com"
			if !tc.apcContext {
				d.ContextDomain = "astronomer.io"
			}
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"init"}
			if tc.flag != "" {
				args = append(args, "--deploy-target", tc.flag)
			}
			if err := execute(t, d, args...); err != nil {
				t.Fatalf("astro init: %v", err)
			}
			_, err := os.Stat(filepath.Join(dir, "Dockerfile"))
			if kept := err == nil; kept != tc.wantKept {
				t.Errorf("Dockerfile kept = %v, want %v", kept, tc.wantKept)
			}
			out := stdout.String()
			if tc.wantDecision != "" && !strings.Contains(out, "This run converted the project for Astro Private Cloud "+tc.wantDecision) {
				t.Errorf("want the note to say what decided:\n%s", out)
			}
			if tc.wantDecision == "" && strings.Contains(out, "Astro Private Cloud") {
				t.Errorf("an Astro conversion with nothing kept says nothing of APC:\n%s", out)
			}
		})
	}
}

// Any other value is refused as a usage error while flags are parsed, before
// anything is written.
func TestInitRefusesAnUnknownDeployTarget(t *testing.T) {
	d, dir, _ := initDeps(t)
	err := execute(t, d, "init", "--deploy-target", "software")
	if err == nil || !cliout.IsUsage(err) || !strings.Contains(err.Error(), "must be astro or apc") {
		t.Fatalf("want a usage error naming the values, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		t.Error("a refused flag wrote a manifest")
	}
}

// The reason each decision gives, including no context at all, which is Astro.
func TestInitDeployTargetBasis(t *testing.T) {
	for _, tc := range []struct {
		flag         deployTargetValue
		apcCtx       bool
		domain       string
		apc          bool
		why, instead string
	}{
		{"", false, "", false, "no context is current", "pass --deploy-target apc"},
		{"", false, "astronomer.io", false, "the current context is Astro (astronomer.io)", "pass --deploy-target apc"},
		{"", true, "apc.example.com", true, "the current context is Astro Private Cloud (apc.example.com)", "pass --deploy-target astro"},
		{"", true, "", true, "the current context is Astro Private Cloud", "pass --deploy-target astro"},
		{"apc", false, "astronomer.io", true, "of --deploy-target apc", "pass --deploy-target astro"},
		{"astro", true, "apc.example.com", false, "of --deploy-target astro", "pass --deploy-target apc"},
	} {
		apc, basis := initDeployTarget(tc.flag, &Deps{DeploysToAPC: tc.apcCtx, ContextDomain: tc.domain})
		if apc != tc.apc || basis.Why != tc.why || basis.Instead != tc.instead {
			t.Errorf("flag %q, APC context %v, domain %q: got %v %+v, want %v %q %q",
				tc.flag, tc.apcCtx, tc.domain, apc, basis, tc.apc, tc.why, tc.instead)
		}
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
