//go:build e2e

package e2e

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A v2 project whose declared Dockerfile is still on a runtime tag with
// ONBUILD steps is refused before anything builds, with the lines to use
// instead; astro init, run again in the project, moves it onto -base and
// leaves the rest of the project alone.
//
// Tier 0 because both halves come before any engine: astro package refuses
// while it reads the manifest, and init only writes files.
func TestAnONBUILDDockerfileIsRefusedAndInitMovesIt(t *testing.T) {
	tier(t, 0)
	p := newNamedProject(t, "onbuild")
	p.run("init", "--name", "onbuild").requireSuccess()
	// The project's own series, read here rather than through runtimeImageFor,
	// which lives in a file Windows does not build, and this case runs there.
	pin := regexp.MustCompile(`apache-airflow==(\d+\.\d+)\.\*`).FindStringSubmatch(read(t, filepath.Join(p.Dir, "pyproject.toml")))
	if pin == nil {
		t.Fatal("no apache-airflow series pin in the scaffolded manifest")
	}
	base := "astrocrpublic.azurecr.io/runtime:" + pin[1]
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM "+base+"\nRUN echo hi\n")
	declareDockerfile(t, p)
	manifest := read(t, filepath.Join(p.Dir, "pyproject.toml"))

	r := p.run("package", "astro").requireFailure()
	for _, want := range []string{
		"Dockerfile builds FROM " + base + ", whose ONBUILD steps",
		"FROM " + base + "-base AS astro-install",
		"Run astro init",
	} {
		if !strings.Contains(r.output(), want) {
			t.Errorf("the refusal lacks %q:\n%s", want, r.output())
		}
	}

	var res struct {
		Existing bool     `json:"existing"`
		Created  []string `json:"created"`
		Updated  []string `json:"updated"`
	}
	out := p.run("init", "--output", "json").requireSuccess()
	if err := json.Unmarshal([]byte(out.Stdout), &res); err != nil {
		t.Fatalf("init --output json: %v\n%s", err, out.output())
	}
	if !res.Existing || !reports(res.Updated, "Dockerfile (now builds FROM "+base+"-base") ||
		!reports(res.Created, ".astro/install-project.py") {
		t.Errorf("init in the project did not move the Dockerfile and write the helper: %+v", res)
	}
	if got := read(t, filepath.Join(p.Dir, "Dockerfile")); !strings.HasSuffix(got, "\nRUN echo hi\n") {
		t.Errorf("the Dockerfile's own step is not kept last:\n%s", got)
	}
	if got := read(t, filepath.Join(p.Dir, "pyproject.toml")); got != manifest {
		t.Errorf("init changed the manifest of an existing project:\n%s", got)
	}

	again := p.run("init").requireSuccess()
	if !strings.Contains(again.Stdout, "is up to date") {
		t.Errorf("a second init has something to do:\n%s", again.output())
	}
}
