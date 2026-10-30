//go:build e2e && !windows

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A converted 1.x Dockerfile builds the same image with `docker build`, from
// what `astro package --context-dir` writes, as with `astro package astro`.
//
// The point of moving a converted Dockerfile onto its tag's -base image: the
// install is written in the file, reading the lists the CLI resolves from
// pyproject.toml and uv.lock through the astro-install build context, so the
// 1.x lists are gone and what both images carry has to have come from
// pyproject.toml, and the project's own RUN still runs. A plain `docker build
// .` has no such context, and fails.
//
// Both builds are linux/amd64, which `astro package astro` always builds and
// the printed command carries: the platform is one of the three differences
// the manifest reference names, and the other two (labels, build secrets) do
// not change what is installed.
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

	// A plain docker build of the project has no astro-install context.
	plain := exec.CommandContext(t.Context(), "docker", "build", "--platform", "linux/amd64", "-t", "astro-package/convbuild-docker:plain", ".")
	plain.Dir = p.Dir
	if out, err := plain.CombinedOutput(); err == nil {
		t.Errorf("docker build . of the converted project succeeded without the astro-install context:\n%s", out)
	}

	// Path one: docker alone, from the inputs --context-dir writes, as CI
	// without the CLI would, with the command it prints.
	const dockerTag = "astro-package/convbuild-docker:e2e"
	var written struct {
		ContextDir string   `json:"context_dir"`
		NextSteps  []string `json:"next_steps"`
	}
	dir := filepath.Join(t.TempDir(), "build")
	p.run("package", "astro", "--context-dir", dir, "--tag", dockerTag, "--output", "json").requireSuccess().requireLastJSON(&written)
	if len(written.NextSteps) == 0 || !strings.HasPrefix(written.NextSteps[0], "docker build ") {
		t.Fatalf("--context-dir printed no docker command: %q", written.NextSteps)
	}
	if !strings.Contains(written.NextSteps[0], " --build-context astro-install="+filepath.Join(dir, "astro-install")+" ") {
		t.Errorf("the docker command does not pass the lists: %q", written.NextSteps[0])
	}
	args := strings.Fields(written.NextSteps[0])
	if out, err := exec.CommandContext(t.Context(), args[0], args[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("%s failed on the converted project: %v\n%s", written.NextSteps[0], err, out)
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
			t.Errorf("the --context-dir build lacks %q, which the conversion carried:\n%s", want, fromDocker)
		}
	}
	if fromDocker != fromPackage {
		t.Errorf("the --context-dir build and astro package astro installed different things:\ndocker build:\n%s\nastro package:\n%s", fromDocker, fromPackage)
	}
}
