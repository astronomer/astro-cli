//go:build e2e && !windows

package e2e

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A declared Dockerfile on a base docker mode cannot run is refused, with the
// reason, and leaves nothing behind.
//
// Tier 3: the base is judged by the runtime version label on the image the
// Dockerfile builds, so the answer needs an engine and a build. A name cannot
// give it — a mirror of a Runtime image under a private registry's name is a
// Runtime image, which TestDockerModeStartsOnAMirroredRuntime below starts.
//
// The failure this replaced was a daemon error about a missing unix user,
// reaching the person as "starting project containers: exit status 1" after a
// base image pull. So the assertions are about what the message names: the
// base it found and what that base lacks. A refusal that said only
// "unsupported" would pass a test that checked the exit code alone.
func TestDockerModeRefusesADockerfileNotOnARuntimeBase(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "oddbase")
	needsDocker(t, p)

	// A small base that is not Astro Runtime. Removed afterwards only if this
	// case is what pulled it: a developer's own copy is theirs.
	const base = "python:3.12-slim"
	removeIfPulledHere(t, base)
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM "+base+"\n")
	declareDockerfile(t, p)

	var emitted map[string]any
	r := p.runSlow("local", "start", "--docker", "--output", "json").requireFailure()
	r.requireLastJSON(&emitted)

	if got := emitted["kind"]; got != "unsupported_base" {
		t.Errorf("kind = %v, want %q\nemitted: %v", got, "unsupported_base", emitted)
	}
	msg, _ := emitted["error"].(string)
	const want = base + " is not an Astro Runtime image: it has no runtime version label (io.astronomer.docker.runtime.version)"
	if !strings.Contains(msg, want) {
		t.Errorf("the refusal should read\n\t%s\ngot:\n%s", want, msg)
	}

	// Refused before compose, so nothing was recorded as started, and the
	// image the build made, which nothing can run, went with the refusal
	// rather than waiting for a cleanup a person may never run.
	if st := p.status(); st.State != "stopped" {
		t.Errorf("state after a refused start = %q, want stopped", st.State)
	}
	imgs, err := dockerLines(t.Context(), "image", "ls", "--format", "{{.Repository}}", "--filter", "reference=astro-local/*")
	if err != nil {
		t.Fatalf("listing images: %v", err)
	}
	for _, img := range imgs {
		if strings.Contains(img, "oddbase") {
			t.Errorf("the refused build's image %s was left behind", img)
		}
	}
}

// A Runtime image under a name that is not Astronomer's starts.
//
// What a mirror is — an exact copy in a private registry — is simulated with a
// local tag: the Dockerfile's FROM names registry.corp.example, which serves
// nothing, so the build has to find the base locally, and the image it builds
// carries the label the copy kept. Before the base was judged by its label,
// this start was refused for the registry's name.
func TestDockerModeStartsOnAMirroredRuntime(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "mirrored")
	needsDocker(t, p)

	upstream := runtimeImageFor(t, p)
	_, tag, _ := strings.Cut(upstream, ":")
	// Unique to this run, so an overlapping run's cleanup cannot untag it.
	mirror := "registry.corp.example/astronomer/astro-runtime:" + tag + "-e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dockerRun(t, 20*time.Minute, "pull", upstream)
	dockerRun(t, time.Minute, "tag", upstream, mirror)
	// Untags only: the upstream tag still names the same image.
	t.Cleanup(func() { dockerRun(t, time.Minute, "image", "rm", mirror) })

	// The runtime image's ONBUILD triggers expect these two.
	write(t, filepath.Join(p.Dir, "requirements.txt"), "")
	write(t, filepath.Join(p.Dir, "packages.txt"), "")
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM "+mirror+"\n")
	declareDockerfile(t, p)

	r := p.runSlow("local", "start", "--docker").requireSuccess()
	if strings.Contains(r.output(), "not an Astro Runtime image") {
		t.Errorf("a mirrored runtime is accepted silently\n%s", r.output())
	}
	st := p.status()
	if st.State != "running" {
		t.Fatalf("state = %q, want running", st.State)
	}
	if !acceptsWithin(st.Port, 3*time.Minute) {
		t.Fatalf("port %d never answered", st.Port)
	}
}

// dockerRun runs one docker command for a case's own setup, failing the case
// when it fails.
func dockerRun(t *testing.T, timeout time.Duration, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), timeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// removeIfPulledHere removes image when the case ends, if it was not in the
// local store when the case began and is there now. BuildKit keeps a base it
// pulled in its own cache rather than tagging it in the image store, so the
// image is often not there to remove.
func removeIfPulledHere(t *testing.T, image string) {
	t.Helper()
	present := func(ctx context.Context) bool {
		return exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil
	}
	if present(t.Context()) {
		return
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if present(ctx) {
			dockerRun(t, time.Minute, "image", "rm", image)
		}
	})
}
