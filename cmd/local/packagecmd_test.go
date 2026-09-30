package local

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// writeManifest drops a minimal valid v2 manifest in dir and points the deps'
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
	// json mode still fails, and the shared wrapper writes one error object.
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
// no artifact, so package warns about it beside the target's own warnings.
func TestPackageWarnsAboutUndeclaredLocalValues(t *testing.T) {
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
	line, ok := lineContaining(out.String(), "AIRFLOW_VAR_REGION")
	if !ok || !strings.Contains(line, "will not follow it to a Deployment") {
		t.Errorf("want a warning naming AIRFLOW_VAR_REGION:\n%s", out.String())
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
