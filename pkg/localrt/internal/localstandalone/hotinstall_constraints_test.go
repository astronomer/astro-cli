//go:build !windows

package localstandalone

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

const (
	manifestHead = "[project]\nname = 'demo'\nrequires-python = '>=3.12'\n" +
		"dependencies = [\"apache-airflow==3.3.*\"]\n[tool.astro]\n\n"
	constrainedManifest = manifestHead + "[tool.uv]\nconstraint-dependencies = ['pandas<3', 'numpy<2.4']\n"
)

// hotInstallSeen is what the fake uv recorded of the one `uv pip install` a
// hot install runs.
type hotInstallSeen struct {
	args []string
	// constraints is the constraints file's content as uv would have read it,
	// copied while uv was running because the engine removes it afterwards.
	// Nil when uv was given no --constraint.
	constraints []string
	lines       []string
}

// hotInstallWithFakeUV runs a hot install of project through the engine's real
// uv client, with a shell script standing in for the uv binary — the same fake
// `astro local check`'s constraints test uses — so what is asserted is the argv
// uv receives rather than what the engine meant to pass.
//
// NoConfig is on, because that is the case the constraints file exists for:
// Astro Desktop sets it, and --no-config is what makes uv pip skip the
// project's [tool.uv] table.
func hotInstallWithFakeUV(t *testing.T, manifest string) (hotInstallSeen, error) {
	t.Helper()
	project := t.TempDir()
	seedVenv(t, project)
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	tmp := t.TempDir()
	argsSeen := filepath.Join(tmp, "args-seen")
	constraintsSeen := filepath.Join(tmp, "constraints-seen")
	bin := filepath.Join(tmp, "uv")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'uv 9.9.9 (0000000 2026-01-01 test)'; exit 0; fi\n"+
		"prev=''\nfor a in \"$@\"; do\n"+
		"  echo \"$a\" >> %q\n"+
		"  if [ \"$prev\" = \"--constraint\" ]; then cp \"$a\" %q; fi\n"+
		"  prev=\"$a\"\ndone\nexit 0\n", argsSeen, constraintsSeen)
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(uv.EnvBin, bin)

	var seen hotInstallSeen
	e := New(t.TempDir(), nil, UVOptions{CacheDir: filepath.Join(tmp, "uvcache"), NoConfig: true})
	err := e.HotInstall(t.Context(), project, []string{"pandas"}, rt.Callbacks{
		OnLine: func(l rt.LogLine) { seen.lines = append(seen.lines, l.Text) },
	})

	data, rerr := os.ReadFile(argsSeen)
	if rerr != nil {
		t.Fatalf("uv was never run: %v", rerr)
	}
	seen.args = strings.Split(strings.TrimSpace(string(data)), "\n")
	if data, rerr := os.ReadFile(constraintsSeen); rerr == nil {
		seen.constraints = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return seen, err
}

// The fix itself: a hot install is held to the same constraints the start that
// built the environment was. Without the file, uv pip under --no-config
// resolves the new package as if the project declared none.
func TestHotInstallHoldsTheInstallToTheManifestsConstraints(t *testing.T) {
	seen, err := hotInstallWithFakeUV(t, constrainedManifest)
	if err != nil {
		t.Fatalf("HotInstall() = %v", err)
	}
	if want := []string{"pandas<3", "numpy<2.4"}; !slices.Equal(seen.constraints, want) {
		t.Errorf("uv's constraints file = %q, want the manifest's %q (argv %q)", seen.constraints, want, seen.args)
	}
	if !slices.Contains(seen.args, "--no-config") {
		t.Errorf("argv %q lacks --no-config, so this test is not exercising the case the file exists for", seen.args)
	}

	// The file is the engine's to clean up, not the temp dir's to accumulate.
	i := slices.Index(seen.args, "--constraint")
	if i < 0 || i+1 >= len(seen.args) {
		t.Fatalf("argv %q has no --constraint path", seen.args)
	}
	if _, err := os.Stat(seen.args[i+1]); !os.IsNotExist(err) {
		t.Errorf("constraints file %s is still there after the install (stat: %v)", seen.args[i+1], err)
	}
}

// A project that declares no constraints gets no --constraint: an empty file
// would be harmless to uv, but it would be a file written for nothing.
func TestHotInstallWithoutDeclaredConstraintsPassesNone(t *testing.T) {
	for name, manifest := range map[string]string{
		"no [tool.uv]":     manifestHead,
		"an empty list":    manifestHead + "[tool.uv]\nconstraint-dependencies = []\n",
		"other uv options": manifestHead + "[tool.uv]\nindex-url = 'https://example.invalid/simple'\n",
	} {
		t.Run(name, func(t *testing.T) {
			seen, err := hotInstallWithFakeUV(t, manifest)
			if err != nil {
				t.Fatalf("HotInstall() = %v", err)
			}
			if slices.Contains(seen.args, "--constraint") {
				t.Errorf("argv %q carries --constraint for a manifest that declares none", seen.args)
			}
			for _, l := range seen.lines {
				if strings.Contains(l, "constraint-dependencies") {
					t.Errorf("logged %q for a manifest that loaded fine", l)
				}
			}
		})
	}
}

// Best-effort: a manifest that does not load costs the constraints, not the
// install, and the log says which of the two happened.
func TestHotInstallWithAManifestThatDoesNotLoadInstallsUnconstrained(t *testing.T) {
	for name, manifest := range map[string]string{
		"not TOML":          "[project\nname = ",
		"no [tool.astro]":   "[project]\nname = 'demo'\n[tool.uv]\nconstraint-dependencies = ['pandas<3']\n",
		"no pyproject.toml": "",
	} {
		t.Run(name, func(t *testing.T) {
			seen, err := hotInstallWithFakeUV(t, manifest)
			if err != nil {
				t.Fatalf("HotInstall() = %v; an unreadable manifest must not fail the install", err)
			}
			if !slices.Contains(seen.args, "pandas") {
				t.Errorf("argv %q does not install the requested package", seen.args)
			}
			if slices.Contains(seen.args, "--constraint") {
				t.Errorf("argv %q carries --constraint from a manifest that did not load", seen.args)
			}
			said := false
			for _, l := range seen.lines {
				said = said || strings.Contains(l, "without the project's constraint-dependencies")
			}
			if !said {
				t.Errorf("logged %q; want a line saying the install went ahead without the constraints", seen.lines)
			}
		})
	}
}
