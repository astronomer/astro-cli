//go:build e2e && !windows

package e2e

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The two paths that build a project's own Dockerfile agree about it.
//
// T7.3's last claim, and the one with history. `astro local start --docker` and
// `astro package astro` are separate callers of pkg/imagebuild, and a declared
// Dockerfile had already been broken once on exactly that seam: the composition
// root's adapter dropped Dockerfile and Context on the way to the builder, so
// docker mode built nothing at all until that was fixed. One caller getting the file and
// the other not is the failure this is for — "works locally, wrong in the
// artifact", which nobody sees until the artifact is deployed.
//
// So the assertion is not that each path builds, which their own cases already
// cover. It is that a RUN line in the project's file reaches BOTH images, asked
// the same way of each.
func TestTheLocalBuildAndThePackagedImageAgreeOnADockerfile(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "agrees")
	needsDocker(t, p)

	// FROM the runtime image, whose ONBUILD triggers want these two; a project
	// that omits them fails on a missing requirements.txt rather than on
	// anything this case is about.
	write(t, filepath.Join(p.Dir, "requirements.txt"), "")
	write(t, filepath.Join(p.Dir, "packages.txt"), "")
	write(t, filepath.Join(p.Dir, "Dockerfile"),
		"FROM "+runtimeImageFor(t, p)+"\n"+
			"RUN echo from-my-dockerfile > /tmp/astro-agrees\n")
	declareDockerfile(t, p)

	// Path one: the local build, asked through the running container.
	p.runSlow("local", "start", "--docker").requireSuccess()
	got := p.runSlow("local", "run", "--", "cat", "/tmp/astro-agrees").requireSuccess()
	if !strings.Contains(got.Stdout, "from-my-dockerfile") {
		t.Fatalf("the local docker build did not apply the project's Dockerfile\n%s", got.output())
	}

	// Registered BEFORE the build that creates them, and by tag prefix rather
	// than by the name the result reports. dockerProject's cleanup only knows
	// the tags the local build makes; astro-package/<name> is this case's to
	// take away. Registering after the result is parsed leaks the image
	// whenever that parse fails, which is the run where cleanup matters most.
	t.Cleanup(func() { removeImagesNamed(t, "astro-package/agrees") })

	// Path two: the packaged image, asked directly of the image it names.
	var res struct {
		Target string `json:"target"`
		Kind   string `json:"kind"`
		Image  string `json:"image"`
	}
	// The last line: `astro package` streams its build log as NDJSON and the
	// result is the object after it.
	p.runSlow("package", "astro", "--output", "json").requireSuccess().requireLastJSON(&res)
	if res.Image == "" {
		t.Fatalf("`astro package astro` named no image: %+v", res)
	}

	// Through a shell that succeeds either way. A bare `cat` exits 1 on a
	// missing file, so the case that matters — an image built without the
	// project's Dockerfile — surfaced as "exit status 1" from the docker run
	// rather than as the two paths disagreeing. Measured: that is what it
	// reported before this line existed.
	out, err := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--entrypoint", "sh",
		res.Image, "-c", "cat /tmp/astro-agrees 2>/dev/null || echo MARKER-ABSENT").CombinedOutput()
	if err != nil {
		t.Fatalf("could not run anything in the packaged image %s: %v\n%s", res.Image, err, out)
	}
	if !strings.Contains(string(out), "from-my-dockerfile") {
		t.Errorf("the packaged image does not carry what the project's Dockerfile installs, "+
			"though the local build does — the two paths disagree about the same file:\n%s", out)
	}
}

// removeImagesNamed deletes every tag under a repository.
//
// By repository rather than by one tag: the default naming scheme writes both
// `<repo>:<runtime>-<hash>` and a moving `<repo>:latest`, and a case that
// removed only the tag its result named left the other behind pinning the
// layers.
//
// context.Background rather than t.Context: this runs from t.Cleanup, and the
// test's context is already canceled by then. dockerLines bounds it.
func removeImagesNamed(t *testing.T, repo string) {
	t.Helper()
	tags, err := dockerLines(context.Background(), "images", "--format", "{{.Repository}}:{{.Tag}}", repo)
	if err != nil {
		t.Logf("listing images for %s: %v", repo, err)
		return
	}
	for _, tag := range tags {
		if err := exec.Command("docker", "image", "rm", "-f", tag).Run(); err != nil {
			t.Logf("removing %s: %v", tag, err)
		}
	}
}
