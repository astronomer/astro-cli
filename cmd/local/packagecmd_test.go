package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Every cmd/local warning (emitWarning) reaches the user one way: a "Warning:"
// line on stderr in text, and a warning event in json, never on stdout as if
// it were the command's output.
func TestEmitWarningIsAWarningOnStderr(t *testing.T) {
	const msg = "the image will carry 5,510 files that .gitignore ignores and .dockerignore does not, in .claude/. If they do not belong in the image, add them to .dockerignore"
	var out, stderr bytes.Buffer
	emitWarning(cliout.Renderer{Format: cliout.FormatText, Out: &out, Err: &stderr}, event{Event: "warning", Text: msg})
	if got, want := stderr.String(), "Warning: "+msg+"\n"; got != want {
		t.Errorf("text: stderr = %q, want %q", got, want)
	}
	if out.Len() != 0 {
		t.Errorf("text: a warning is not output, but stdout got %q", out.String())
	}

	stderr.Reset()
	out.Reset()
	emitWarning(cliout.Renderer{Format: cliout.FormatJSON, Out: &out, Err: &stderr}, event{Event: "warning", Text: msg})
	var e map[string]any
	if err := json.Unmarshal(out.Bytes(), &e); err != nil {
		t.Fatalf("json: want one event, got %q: %v", out.String(), err)
	}
	if e["event"] != "warning" || e["text"] != msg {
		t.Errorf("json: event = %v", e)
	}
	if stderr.Len() != 0 {
		t.Errorf("json: the event carries it, but stderr got %q", stderr.String())
	}
}

// A Renderer whose Format nobody mapped (an unbound flag's "", an extra like
// yaml) is a programming error, and a warning through it panics rather than
// render as text.
func TestEmitWarningPanicsOnAnUnmappedFormat(t *testing.T) {
	for _, format := range []cliout.Format{"", "yaml"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("format %q: emitWarning did not panic", format)
				}
			}()
			var out bytes.Buffer
			emitWarning(cliout.Renderer{Format: format, Out: &out}, event{Event: "warning", Text: "x"})
		}()
	}
}

// writeManifest drops a minimal valid manifest in dir and points the deps'
// WorkingDir at it, so `astro package` discovers a project.
func writeManifest(t *testing.T, d *Deps, dir string) {
	t.Helper()
	toml := "" +
		"[project]\n" +
		"name = 'demo'\n" +
		"dependencies = ['apache-airflow==3.1.*', 'pandas']\n\n" +
		"[tool.astro]\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
}

func TestPackageUnknownTargetLists(t *testing.T) {
	d, _ := testDeps(t)
	// No project needed: the target is resolved before the manifest is loaded.
	err := execute(t, d, "package", "bogus")
	if err == nil {
		t.Fatal("want an error for an unknown target")
	}
	for _, want := range []string{"bogus", "astro", "mwaa", "composer", "oss"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q: %v", want, err)
		}
	}
}

func TestPackageStagedTargetErrors(t *testing.T) {
	d, _ := testDeps(t)
	writeManifest(t, &d, t.TempDir())
	// oss is the one target still staged.
	err := execute(t, d, "package", "oss")
	if err == nil {
		t.Fatal("want a staged error for oss")
	}
	var staged *pack.StagedError
	if !errors.As(err, &staged) {
		t.Fatalf("want a StagedError, got %T: %v", err, err)
	}
	if staged.Target != "oss" {
		t.Errorf("staged error names the wrong target: %+v", staged)
	}
}

func TestPackageStagedTargetJSONError(t *testing.T) {
	d, out := testDeps(t)
	writeManifest(t, &d, t.TempDir())
	// json mode still fails, and the root's error report writes one error object.
	_ = execute(t, d, "package", "oss", "--output", "json")
	var obj struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &obj); err != nil {
		t.Fatalf("json error object did not decode: %v: %q", err, out.String())
	}
	if !strings.Contains(obj.Error, "oss") || obj.Code != 1 {
		t.Errorf("json error object wrong: %+v", obj)
	}
}

func TestPackageMWAABuildsTree(t *testing.T) {
	d, out := testDeps(t)
	dir := t.TempDir()
	writeManifest(t, &d, dir)
	if err := os.MkdirAll(filepath.Join(dir, "dags"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dags", "d.py"), []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "mwaa-artifact")
	if err := execute(t, d, "package", "mwaa", "--out-dir", outDir); err != nil {
		t.Fatalf("package mwaa: %v", err)
	}
	// The command reports the tree and the upload hand-off in text.
	for _, want := range []string{"tree:", outDir, "requirements.txt", "aws s3 sync"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("text output missing %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "dags", "d.py")); err != nil {
		t.Errorf("dag not copied into the artifact: %v", err)
	}
}

// A value the project gets from this machine without declaring it is carried by
// no artifact, but that is the project's setup, not the artifact's: astro local
// check warns about it, and package says nothing.
func TestPackageSaysNothingAboutUndeclaredLocalValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ASTRO_HOME", "")
	d, out := testDeps(t)
	dir := t.TempDir()
	writeManifest(t, &d, dir)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("AIRFLOW_VAR_REGION=us\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "mwaa-artifact")
	if err := execute(t, d, "package", "mwaa", "--out-dir", outDir); err != nil {
		t.Fatalf("package mwaa: %v", err)
	}
	if got := stderrOf(d) + out.String(); strings.Contains(got, "region") {
		t.Errorf("package named the undeclared value:\n%s", got)
	}

	d, out = testDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	if err := execute(t, d, "package", "mwaa", "--out-dir", outDir, "--output", "json"); err != nil {
		t.Fatalf("package mwaa: %v", err)
	}
	if strings.Contains(out.String(), "region") {
		t.Errorf("package -o json named the undeclared value:\n%s", out.String())
	}
}

// An image's result lists the values the manifest declares under the deploy
// command, for the Deployment to set, the required ones first, aligned.
func TestPackageListsTheDeploymentEnvUnderTheDeployCommand(t *testing.T) {
	var b strings.Builder
	err := renderImageResult(&b, pack.Result{
		Image: "astro-package/p:3.1-2-abc",
		DeploymentEnv: []pack.DeploymentValue{
			{Kind: "connection", Name: "llm_gateway", Status: "required"},
			{Kind: "connection", Name: "github_default", Status: "optional"},
			{Kind: "airflow variable", Name: "region", Status: "has a default"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "\nDeploy it with:\n  astro deploy --image-name astro-package/p:3.1-2-abc\n" +
		"and set these on the Deployment, as environment variables or linked Environment Manager objects:\n" +
		"  connection llm_gateway     required\n" +
		"  connection github_default  optional\n" +
		"  airflow variable region    has a default\n"
	if !strings.HasSuffix(b.String(), want) {
		t.Errorf("got:\n%s\nwant suffix:\n%s", b.String(), want)
	}

	b.Reset()
	if err := renderImageResult(&b, pack.Result{Image: "i"}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "set these") {
		t.Errorf("no declared values, yet:\n%s", b.String())
	}
}

// A package linked to a workspace says nothing about it: an image carries
// none of the workspace's values, and a Deployment gets its own workspace's.
func TestPackageSaysNothingAboutALinkedWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ASTRO_HOME", "")
	d, out := testDeps(t)
	dir := t.TempDir()
	writeManifest(t, &d, dir)
	m := "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'cmws'\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "package", "mwaa", "--out-dir", filepath.Join(t.TempDir(), "mwaa")); err != nil {
		t.Fatalf("package mwaa: %v", err)
	}
	if all := out.String() + stderrOf(d); strings.Contains(all, "cmws") {
		t.Errorf("package named the workspace:\n%s", all)
	}
}

// fakePackageTarget stands in for the astro target: it finds a warning,
// streams a line of build output, finds another warning, then returns
// res or err.
type fakePackageTarget struct {
	res pack.Result
	err error
}

func (fakePackageTarget) Name() string { return pack.TargetAstro }

func (f fakePackageTarget) Build(_ context.Context, req pack.Request, cb localrt.Callbacks) (pack.Result, error) {
	req.Warn("found before the build")
	cb.OnLine(localrt.LogLine{Component: "build", Text: "step 1/9"})
	req.Warn("found after the build")
	return f.res, f.err
}

// packageWithFake runs `astro package` over fakePackageTarget, with stdout and
// stderr in one buffer so the order the two interleave in is what a terminal
// shows.
func packageWithFake(t *testing.T, target fakePackageTarget, args ...string) (string, error) {
	t.Helper()
	d, out := testDeps(t)
	d.Stderr = out
	writeManifest(t, &d, t.TempDir())
	d.PackageAstro = target
	err := execute(t, d, append([]string{"package", "astro"}, args...)...)
	return out.String(), err
}

// In text, each warning a package finds prints once, after the
// build's output, just above the result.
func TestPackageShowsWarningsOnceAfterTheBuild(t *testing.T) {
	got, err := packageWithFake(t, fakePackageTarget{res: pack.Result{
		Target: pack.TargetAstro, Kind: pack.KindImage, Image: "astro-package/demo:x",
		Warnings: []string{"found before the build", "found after the build"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	requireInOrder(t, got, "[build] step 1/9", "Warning: found before the build",
		"Warning: found after the build", "target: astro")
	for _, msg := range []string{"found before the build", "found after the build"} {
		if n := strings.Count(got, msg); n != 1 {
			t.Errorf("%q printed %d times, want once:\n%s", msg, n, got)
		}
	}
}

// A build that fails prints what it found once, before the error the command
// returns.
func TestPackageShowsWarningsOnceWhenTheBuildFails(t *testing.T) {
	boom := errors.New("boom")
	got, err := packageWithFake(t, fakePackageTarget{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	requireInOrder(t, got, "[build] step 1/9", "Warning: found before the build",
		"Warning: found after the build")
	if n := strings.Count(got, "found before the build"); n != 1 {
		t.Errorf("printed %d times, want once:\n%s", n, got)
	}
}

// Under json a warning is an event emitted once, as it is found, between the
// build's lines, as before.
func TestPackageStreamsWarningsInJSON(t *testing.T) {
	got, err := packageWithFake(t, fakePackageTarget{res: pack.Result{Target: pack.TargetAstro, Kind: pack.KindImage, Image: "i"}},
		"--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	requireInOrder(t, got, `"text":"found before the build"`, `step 1/9`, `"text":"found after the build"`)
	if n := strings.Count(got, `"text":"found before the build"`); n != 1 {
		t.Errorf("json emitted the warning %d times, want once:\n%s", n, got)
	}
}

// `astro package astro` refuses a [tool.astro.env] declaration the parser
// refuses, as `astro local start` does, before it needs Docker.
func TestPackageAstroRefusesAnEnvDeclarationThatDoesNotParse(t *testing.T) {
	d, _ := testDeps(t)
	dir := t.TempDir()
	writeManifest(t, &d, dir)
	m := "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n\n[tool.astro.env]\nASTRO_TEST_BAD = 5\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	err := execute(t, d, "package", "astro")
	if err == nil || !strings.Contains(err.Error(), "ASTRO_TEST_BAD") {
		t.Fatalf("want the declaration named in an error, got %v", err)
	}
}

func TestRenderPackageImageText(t *testing.T) {
	var buf bytes.Buffer
	res := pack.Result{
		Target:         "astro",
		Kind:           pack.KindImage,
		Image:          "astro-package/demo:3.1-2-abc1234",
		RuntimeVersion: "3.1-2",
		SavedPath:      "image.tar",
	}
	if err := renderPackage(&buf, res, nil); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{
		"target: astro",
		"image:  astro-package/demo:3.1-2-abc1234",
		"runtime: 3.1-2",
		"saved:  image.tar",
		"astro deploy --image-name astro-package/demo:3.1-2-abc1234",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text output missing %q:\n%s", want, text)
		}
	}
}

func TestDeployImageHintNamesATarget(t *testing.T) {
	const image = "astro-package/demo:3.1-2-abc1234"
	astro := func(isDefault bool) manifest.Link {
		return manifest.Link{Deployment: "clx123", Default: isDefault}
	}
	cases := []struct {
		name  string
		links map[string]manifest.Link
		want  string
	}{
		{name: "no links", want: "  astro deploy --image-name " + image},
		{name: "the default", links: map[string]manifest.Link{"dev": astro(false), "prod": astro(true)}, want: "  astro deploy prod --image-name " + image},
		{name: "the only one", links: map[string]manifest.Link{"prod": astro(false)}, want: "  astro deploy prod --image-name " + image},
		{
			name:  "several and no default",
			links: map[string]manifest.Link{"prod": astro(false), "dev": astro(false)},
			want:  "  astro deploy <link> --image-name " + image + "\nDeployable links: dev, prod",
		},
		{
			name:  "a default that deploy cannot ship to",
			links: map[string]manifest.Link{"prod": astro(false), "dev": astro(false), "mwaa": {Target: "mwaa", Environment: "e", Default: true}},
			want:  "  astro deploy <link> --image-name " + image + "\nDeployable links: dev, prod",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deployImageHint(image, tc.links); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A context lists the commands that build it, and nothing more.
func TestContextResultListsItsCommands(t *testing.T) {
	var out bytes.Buffer
	res := pack.Result{ContextDir: "/b", NextSteps: []string{"docker build x", "podman build x"}}
	if err := renderContextResult(&out, res); err != nil {
		t.Fatal(err)
	}
	want := "context: /b\n\nBuild it with either of:\n  docker build x\n  podman build x\n"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --base prints the install base's tag alone, so $(astro package --base) is
// the tag.
func TestInstallBaseResultIsTheTag(t *testing.T) {
	var out bytes.Buffer
	if err := renderPackage(&out, pack.Result{Target: "astro", Kind: pack.KindInstallBase, Image: "localhost/p-base:abc"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "localhost/p-base:abc\n" {
		t.Errorf("got %q", got)
	}
}

// --base's stdout is the install base's tag and nothing else, so
// $(astro package --base) is the tag: the build's progress and the warnings
// go to stderr.
func TestPackageBaseWritesOnlyTheTagToStdout(t *testing.T) {
	d, out := testDeps(t)
	writeManifest(t, &d, t.TempDir())
	d.PackageAstro = fakePackageTarget{res: pack.Result{Target: pack.TargetAstro, Kind: pack.KindInstallBase, Image: "localhost/demo-base:abc"}}
	if err := execute(t, d, "package", "--base"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "localhost/demo-base:abc\n" {
		t.Errorf("stdout = %q, want the tag alone", got)
	}
	if errOut := stderrOf(d); !strings.Contains(errOut, "step 1/9") || !strings.Contains(errOut, "found before the build") {
		t.Errorf("stderr lacks the build's progress and warnings:\n%s", errOut)
	}
	for _, args := range [][]string{{"package", "--base", "--context-dir", "x"}, {"package", "--base", "--tag", "x"}, {"package", "mwaa", "--base"}} {
		if err := execute(t, d, args...); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}
