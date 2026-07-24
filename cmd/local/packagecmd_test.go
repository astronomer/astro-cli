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
)

// writeManifest drops a minimal valid v2 manifest in dir and points the deps'
// WorkingDir at it, so `astro package` discovers a project.
func writeManifest(t *testing.T, d *Deps, dir string) {
	t.Helper()
	toml := "" +
		"[project]\n" +
		"name = 'demo'\n" +
		"dependencies = ['pandas']\n\n" +
		"[tool.astro]\n" +
		"airflow = '3.1'\n"
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
	err := execute(t, d, "package", "mwaa")
	if err == nil {
		t.Fatal("want a staged error for mwaa")
	}
	var staged *pack.StagedError
	if !errors.As(err, &staged) {
		t.Fatalf("want a StagedError, got %T: %v", err, err)
	}
	if staged.Target != "mwaa" {
		t.Errorf("staged error names the wrong target: %+v", staged)
	}
}

func TestPackageStagedTargetJSONError(t *testing.T) {
	d, out := testDeps(t)
	writeManifest(t, &d, t.TempDir())
	// json mode still fails, and the shared wrapper writes one error object.
	_ = execute(t, d, "package", "composer", "--output", "json")
	var obj struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &obj); err != nil {
		t.Fatalf("json error object did not decode: %v: %q", err, out.String())
	}
	if !strings.Contains(obj.Error, "composer") || obj.Code != 1 {
		t.Errorf("json error object wrong: %+v", obj)
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
	if err := renderPackage(&buf, res); err != nil {
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
