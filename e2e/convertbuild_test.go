//go:build e2e && !windows

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A converted 1.x Dockerfile builds the same image with plain `docker build .`
// as with `astro package astro`.
//
// The point of moving a converted Dockerfile onto its tag's -base image: the
// install is written in the file, reading pyproject.toml through the helper
// the conversion wrote beside it, so nothing the CLI adds at build time
// decides what goes in. The 1.x lists are gone, so what both images carry has
// to have come from pyproject.toml, and the project's own RUN still runs.
//
// Both builds are linux/amd64, which `astro package astro` always builds and a
// plain `docker build` does only when asked: the platform is one of the three
// differences the manifest reference names, and the other two (labels, build
// secrets) do not change what is installed.
func TestAConvertedDockerfileBuildsTheSameWithDockerAsWithPackage(t *testing.T) {
	tier(t, 3)
	p := newNamedProject(t, "convbuild")
	needsDocker(t, p)

	mkdir(t, filepath.Join(p.Dir, ".astro"))
	write(t, filepath.Join(p.Dir, ".astro", "config.yaml"), "project:\n  name: convbuild\n")
	write(t, filepath.Join(p.Dir, "Dockerfile"),
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12\nRUN echo from-1x > /tmp/astro-converted\n")
	write(t, filepath.Join(p.Dir, "requirements.txt"), "six==1.16.0\n")
	write(t, filepath.Join(p.Dir, "packages.txt"), "jq\n")
	mkdir(t, filepath.Join(p.Dir, "dags"))

	var res initResult
	p.run("init", "--output", "json").requireSuccess().requireJSON(&res)
	if !reports(res.Updated, "Dockerfile (now builds FROM astrocrpublic.azurecr.io/runtime:3.1-12-base") {
		t.Fatalf("the conversion did not move the Dockerfile onto -base: updated %q, notes %q", res.Updated, res.Notes)
	}
	for _, gone := range []string{"requirements.txt", "packages.txt"} {
		if !reports(res.Deleted, gone) {
			t.Errorf("%s is not retired: deleted %q", gone, res.Deleted)
		}
	}

	t.Cleanup(func() { removeImagesNamed(t, "astro-package/convbuild") })
	t.Cleanup(func() { removeImagesNamed(t, "astro-package/convbuild-docker") })

	// Path one: docker alone, in the project, as CI without the CLI would.
	const dockerTag = "astro-package/convbuild-docker:e2e"
	build := exec.CommandContext(t.Context(), "docker", "build", "--platform", "linux/amd64", "-t", dockerTag, ".")
	build.Dir = p.Dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build . failed on the converted project: %v\n%s", err, out)
	}

	// Path two: the CLI's own build of the same project.
	var pkg struct {
		Image string `json:"image"`
	}
	p.runSlow("package", "astro", "--output", "json").requireSuccess().requireLastJSON(&pkg)
	if pkg.Image == "" {
		t.Fatal("`astro package astro` named no image")
	}

	installed := func(image string) string {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--platform", "linux/amd64", "--entrypoint", "sh", image, "-c",
			"pip freeze | sort; cat /tmp/astro-converted 2>/dev/null || echo NO-RUN-STEP; command -v jq || echo NO-JQ").CombinedOutput()
		if err != nil {
			t.Fatalf("could not run anything in %s: %v\n%s", image, err, out)
		}
		return string(out)
	}
	fromDocker, fromPackage := installed(dockerTag), installed(pkg.Image)
	for _, want := range []string{"six==1.16.0", "from-1x", "/jq"} {
		if !strings.Contains(fromDocker, want) {
			t.Errorf("docker build . lacks %q, which the conversion carried:\n%s", want, fromDocker)
		}
	}
	if fromDocker != fromPackage {
		t.Errorf("docker build . and astro package astro installed different things:\ndocker build:\n%s\nastro package:\n%s", fromDocker, fromPackage)
	}
}
