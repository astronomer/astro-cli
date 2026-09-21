//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// A declared Dockerfile on a base docker mode cannot run is refused, and the
// refusal costs nothing.
//
// Tier 0: what a Dockerfile builds on is a property of the file, so the answer
// is on disk before any engine is asked, and this case runs on a machine with
// no docker at all.
//
// It does NOT prove the refusal comes before the engine. On a developer machine
// with Docker Desktop running it would pass either way, so that guarantee is
// asserted where it can be —
// TestStartRefusesBeforeItTouchesTheEngine in pkg/localrt/internal/localdocker,
// which fails the engine seam if anything asks it to come up.
//
// The failure this replaces was a daemon error about a missing unix user,
// reaching the person as "starting project containers: exit status 1" after a
// base image pull. So the assertions are about what the message names: the
// file, and the base it found. A refusal that said only "unsupported" would
// pass a test that checked the exit code alone.
func TestDockerModeRefusesADockerfileNotOnARuntimeBase(t *testing.T) {
	tier(t, 0)

	p := newNamedProject(t, "oddbase")
	p.run("init", "--name", "oddbase").requireSuccess()
	// The ordinary OSS Airflow image: not an Astro Runtime, and the shape a v1
	// repo brings.
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM apache/airflow:2.9.3\n")
	declareDockerfile(t, p)

	var emitted map[string]any
	p.run("local", "start", "--docker", "--output", "json").
		requireFailure().
		requireLastJSON(&emitted)

	if got := emitted["kind"]; got != "unsupported_base" {
		t.Errorf("kind = %v, want %q\nemitted: %v", got, "unsupported_base", emitted)
	}
	msg, _ := emitted["error"].(string)
	for _, want := range []string{"Dockerfile", "apache/airflow", "astrocrpublic.azurecr.io/runtime"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not mention %q, which is what the reader has to act on:\n%s", want, msg)
		}
	}
}

// The other half of this — that a runtime base is NOT refused, without which a
// blanket refusal would satisfy the case above — is a unit test in
// pkg/localrt/internal/localdocker rather than a case here.
//
// It cannot be a tier-0 case: the only way to ask the binary is to run the
// start, and a start that gets past the refusal goes on to build. That build
// fails quickly on this machine, on a missing requirements.txt the runtime
// image's ONBUILD triggers expect — but only because buildkit noticed the
// missing context file before fetching anything. An engine that resolved the
// base first would pull 1.34 GB into a tier whose whole promise is temp
// directories and nothing else.
