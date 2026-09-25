package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// A project's [tool.uv] constraint-dependencies reach the scratch venvs
// `astro local check` builds. Those sit outside the project, where uv cannot
// read its [tool.uv], so the spec is the only way the constraints get there,
// for the plain check and for every --target alike.

const constrainedManifest = "[project]\nname = 'demo'\nrequires-python = '>=3.12'\n" +
	"dependencies = [\"apache-airflow==3.3.*\"]\n[tool.astro]\nairflow = '3.3'\n\n" +
	"[tool.uv]\nconstraint-dependencies = ['pandas<3', 'numpy<2.4']\n"

func TestCheckVenvsCarryTheManifestsConstraints(t *testing.T) {
	for _, args := range [][]string{
		{"local", "check"},
		{"local", "check", "--target", "composer"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			d, _ := targetDeps(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(constrainedManifest), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			d.Checks = stubParser{err: checks.ErrNoInterpreter}

			var got checks.VenvSpec
			d.Provisioner = func(context.Context) (checks.Provisioner, error) {
				return &fakeProvisioner{python: "/tmp/venv/bin/python", got: &got}, nil
			}
			if err := execute(t, d, args...); err != nil {
				t.Fatalf("check: %v", err)
			}
			if want := []string{"numpy<2.4", "pandas<3"}; !reflect.DeepEqual(slices.Sorted(slices.Values(got.Constraints)), want) {
				t.Errorf("constraints = %q, want the manifest's %q", got.Constraints, want)
			}
		})
	}
}

// And the provisioner hands them to uv as a constraints file, rather than
// installing without them.
func TestProvisionerInstallsUnderTheSpecsConstraints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	for _, tc := range []struct {
		name        string
		constraints []string
	}{
		{name: "with constraints", constraints: []string{"pandas<3", "numpy<2.4"}},
		{name: "without", constraints: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			captured := filepath.Join(tmp, "constraints-seen")
			bin := filepath.Join(tmp, "uv")
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'uv 9.9.9 (0000000 2026-01-01 test)'; exit 0; fi\n"+
				"prev=''\nfor a in \"$@\"; do\n"+
				"  if [ \"$prev\" = \"venv\" ]; then mkdir -p \"$a\"; fi\n"+
				"  if [ \"$prev\" = \"--constraint\" ]; then cp \"$a\" %q; fi\n"+
				"  prev=\"$a\"\ndone\nexit 0\n", captured)
			if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv(uv.EnvBin, bin)
			client, err := uv.New(t.Context(), uv.Options{CacheDir: filepath.Join(tmp, "uvcache")})
			if err != nil {
				t.Fatal(err)
			}
			p := &uvProvisioner{client: client, cacheDir: filepath.Join(tmp, "venvs")}

			_, err = p.EnsureVenv(t.Context(), checks.VenvSpec{
				Airflow: "3.3", Python: ">=3.12", Reqs: []string{"apache-airflow==3.3.*"}, Constraints: tc.constraints,
			}, func(string) {})
			if err != nil {
				t.Fatalf("EnsureVenv: %v", err)
			}

			data, err := os.ReadFile(captured)
			if tc.constraints == nil {
				if err == nil {
					t.Errorf("uv was given a constraints file for a spec with none: %q", data)
				}
				return
			}
			if err != nil {
				t.Fatalf("uv was not given a constraints file: %v", err)
			}
			if got := strings.Fields(string(data)); !reflect.DeepEqual(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(tc.constraints))) {
				t.Errorf("constraints file = %q, want %q", got, tc.constraints)
			}
		})
	}
}
