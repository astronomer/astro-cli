//go:build e2e && !windows

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// unreachableDockerHost is a daemon address nothing answers. A compose call
// that took it for its own would fail to connect, so a start that succeeds
// with it set is the proof that compose kept talking to the real engine.
const unreachableDockerHost = "tcp://127.0.0.1:1"

// A project's DOCKER_HOST is for its Dags, a DockerOperator's remote daemon,
// never for the compose call that starts local Airflow. Both places a project
// can set it reach the containers with the project's value, and neither moves
// the start off the engine the CLI uses:
//
//   - .env, which docker mode writes into the generated compose file;
//   - the vault, whose values travel to compose only under an alias the file
//     references. Before that, a vault DOCKER_HOST became compose's own, and
//     this start failed connecting to it. Set here as a plain global, which
//     the vault stores without the keyring (e2e/doc.go: no case may reach the
//     real Keychain or Secret Service) and injects like any other vault value.
//
// The vault phase asks twice: of the running Airflow, which got its
// environment at the up, and of a stopped project, which `astro local run`
// serves from the image through RunInImage's own compose call.
func TestDockerModeHandsAProjectsDockerHostToItsContainersNotToCompose(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dockerhost")
	needsDocker(t, p)

	containerDockerHost := func(when string) {
		t.Helper()
		got := p.runSlow("local", "run", "--", "sh", "-c", `echo "DOCKER_HOST=[$DOCKER_HOST]"`).requireSuccess()
		if !strings.Contains(got.Stdout, "DOCKER_HOST=["+unreachableDockerHost+"]") {
			t.Errorf("%s: the container did not get the project's DOCKER_HOST\n%s", when, got.output())
		}
	}

	envFile := filepath.Join(p.Dir, ".env")
	write(t, envFile, "DOCKER_HOST="+unreachableDockerHost+"\n")
	p.runSlow("local", "start", "--docker").requireSuccess()
	if st := p.status(); st.State != "running" {
		t.Fatalf(".env: state = %q, want running", st.State)
	}
	containerDockerHost(".env, running")
	p.runSlow("local", "stop").requireSuccess()

	write(t, envFile, "")
	p.run("local", "env", "variable", "set", "DOCKER_HOST",
		"--global", "--plain", "--auto-link", "--value", unreachableDockerHost).requireSuccess()
	p.runSlow("local", "start", "--docker").requireSuccess()
	if st := p.status(); st.State != "running" {
		t.Fatalf("vault: state = %q, want running", st.State)
	}
	containerDockerHost("vault, running")
	p.runSlow("local", "stop").requireSuccess()
	containerDockerHost("vault, stopped")
}
