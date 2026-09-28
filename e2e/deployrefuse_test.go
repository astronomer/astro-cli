//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// `astro deploy` in a v2 project refuses the flags it would otherwise drop.
//
// The v1 deploy accepted these and did something with them; the v2 path does
// not, and the failure worth preventing is the silent one — a CI job passing
// --pytest, watching the deploy succeed, and believing tests ran. Each refusal
// therefore names the flag and says what to do instead, and the text is the
// feature here rather than the exit code.
//
// Tier 0: nothing on this path is contacted. One scaffold serves every case,
// because refuseFlagsV2DeployIgnores runs before the manifest is read and
// mutates nothing — each subtest takes a view bound to its own T so a failure
// stays in its own case.
func TestDeployRefusesTheFlagsAV2ProjectIgnores(t *testing.T) {
	tier(t, 0)

	parent := v2ProjectForDeploy(t)

	// The eight, written out rather than imported. Importing the table from
	// cmd/astro would make this a test that the CLI agrees with itself; the
	// point is that somebody decided these are the eight and said why.
	for _, tc := range []struct {
		args []string
		// says is a fragment of the guidance, not the whole sentence: the
		// wording will be reworded, the advice should not silently vanish.
		says string
	}{
		{[]string{"--pytest"}, "run your tests before deploying"},
		{[]string{"--parse"}, "check your DAGs before deploying"},
		{[]string{"--dags-path", "somewhere"}, "deploy from the project directory"},
		{[]string{"--dag-bundle-name", "bundle"}, "named DAG bundles are not supported"},
		{[]string{"--test", "tests/"}, "run your tests before deploying"},
		{[]string{"--env", ".env.test"}, "a v2 deploy runs no tests"},
		{[]string{"--save"}, "a v2 deploy always asks"},
		{[]string{"--deployment-name", "prod"}, "name the target with --deployment"},
	} {
		flag := strings.TrimPrefix(tc.args[0], "--")
		t.Run(flag, func(t *testing.T) {
			p := parent.forT(t)
			r := p.run(append([]string{"deploy"}, tc.args...)...).requireFailure()

			// Stderr, not the combined output: guidance a script is meant to
			// notice must not be on stdout, where --output json consumers
			// parse. Reading the whole output would also pass on the usage
			// block cobra prints underneath, which quotes several of these
			// strings back in its own flag help.
			if !strings.Contains(r.Stderr, "--"+flag+" has no effect") {
				t.Errorf("the refusal should name the flag that was ignored\n%s", r.output())
			}
			if !strings.Contains(r.Stderr, tc.says) {
				t.Errorf("the refusal should say what to do instead (%q)\n%s", tc.says, r.output())
			}
			if !strings.Contains(r.Stderr, "v2 project") {
				t.Errorf("the refusal should say a v2 project is why\n%s", r.output())
			}
		})
	}

	// And in json mode the refusal is an object, like every other failure on
	// this path. It was not: refuseFlagsV2DeployIgnores ran before the format
	// was parsed, so `--output json` exited 1 with an empty stdout while the
	// build-secret refusal below published {"error":...}. A script reading
	// stdout to learn what went wrong got nothing — the same silence these
	// refusals exist to break, one layer up.
	t.Run("json mode publishes the refusal as an object", func(t *testing.T) {
		p := parent.forT(t)
		var payload struct {
			Error string `json:"error"`
			Code  int    `json:"code"`
		}
		p.run("deploy", "--pytest", "--output", "json").requireFailure().requireJSON(&payload)

		if !strings.Contains(payload.Error, "--pytest has no effect") {
			t.Errorf("the object should carry the refusal, got %q", payload.Error)
		}
		if payload.Code == 0 {
			t.Error("a refusal should not publish code 0")
		}
	})
}

// `--build-secret` is the one that depends on the project, not just the flag.
//
// Without a declared Dockerfile only the netrc secret is accepted, because
// the runtime image's install step is the one build step and it mounts only
// that. With one declared, any secret is accepted, since a RUN step of the
// user's mounts it.
func TestDeployBuildSecretDependsOnADeclaredDockerfile(t *testing.T) {
	tier(t, 0)

	const needsOne = "reads only the netrc build secret"
	// A fragment unique to the message. "[tool.astro]" alone would pass on
	// the usage block cobra prints underneath, whose --deployment help quotes
	// it back.
	const whereToPutIt = "under [tool.astro] in pyproject.toml"

	t.Run("refused without a declared dockerfile", func(t *testing.T) {
		p := v2ProjectForDeploy(t).forT(t)
		r := p.run("deploy", "--build-secret", "id=x,src=/dev/null").requireFailure()

		if !strings.Contains(r.Stderr, needsOne) {
			t.Errorf("a project with no dockerfile should be told why the secret cannot reach a build\n%s", r.output())
		}
		if !strings.Contains(r.Stderr, whereToPutIt) {
			t.Errorf("the refusal should say where to declare one\n%s", r.output())
		}
	})

	// The deprecated alias shares one pflag Value with --build-secret, so
	// dropping it from the gate would leave the secrets populated and skip
	// the Dockerfile check entirely — the deploy would proceed with them
	// silently dropped, which is the failure this file is about.
	t.Run("the deprecated --build-secrets alias is refused too", func(t *testing.T) {
		p := v2ProjectForDeploy(t).forT(t)
		r := p.run("deploy", "--build-secrets", "id=x,src=/dev/null").requireFailure()

		if !strings.Contains(r.Stderr, needsOne) {
			t.Errorf("the alias reaches the same gate and should be refused the same way\n%s", r.output())
		}
	})

	// With one declared the gate must let it through. The deploy still fails,
	// because there is no account here — so rather than assert the absence of
	// the refusal, which would also hold if the command died earlier for some
	// unrelated reason, this asserts it got PAST the gate: the next thing it
	// complains about is the workspace, which is checked further down.
	t.Run("not refused when the project declares one", func(t *testing.T) {
		p := v2ProjectForDeploy(t).forT(t)
		write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM scratch\n")
		declareDockerfile(t, p)

		r := p.run("deploy", "--build-secret", "id=x,src=/dev/null")
		if strings.Contains(r.output(), needsOne) {
			t.Fatalf("a declared dockerfile is what --build-secret needs; it should not be refused for the want of one\n%s", r.output())
		}
		if !strings.Contains(r.output(), "workspace is required") {
			t.Errorf("expected it to get past the build-secret gate and stop at the workspace;\n"+
				"it stopped somewhere else, so the check above proved nothing\n%s", r.output())
		}
	})

	// The two flag combinations that make a build secret meaningless. They
	// are tested BEFORE the Dockerfile arm of the same switch, so both run
	// with and without one declared — otherwise the declared setup would
	// change nothing, which is how a case comes to claim coverage it has not
	// got.
	for _, tc := range []struct {
		name, with, says string
	}{
		{"with --dags", "--dags", "a dags-only deploy builds no image"},
		{"with --image-name", "--image-name", "the image is already built"},
	} {
		for _, declared := range []bool{false, true} {
			label := tc.name
			if declared {
				label += " and a declared dockerfile"
			}
			t.Run("refused "+label, func(t *testing.T) {
				p := v2ProjectForDeploy(t).forT(t)
				if declared {
					write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM scratch\n")
					declareDockerfile(t, p)
				}

				args := []string{"deploy", "--build-secret", "id=x,src=/dev/null", tc.with}
				if tc.with == "--image-name" {
					args = append(args, "an-image")
				}
				r := p.run(args...).requireFailure()
				if !strings.Contains(r.Stderr, tc.says) {
					t.Errorf("expected the refusal to say %q\n%s", tc.says, r.output())
				}
			})
		}
	}
}

// v2ProjectForDeploy is a scaffolded project, which is all `astro deploy`
// needs to take the v2 path: project.IsV2 looks for a pyproject.toml carrying
// [tool.astro].
func v2ProjectForDeploy(t *testing.T) *project {
	t.Helper()
	p := newProject(t)
	p.run("init", "--name", "deployable").requireSuccess()
	return p
}

// declareDockerfile adds `dockerfile = "Dockerfile"` under [tool.astro].
func declareDockerfile(t *testing.T, p *project) {
	t.Helper()
	addAstroKey(t, p, `dockerfile = "Dockerfile"`)
}

// addAstroKey inserts one line under the manifest's [tool.astro] table.
//
// One copy of the anchor. It was written out twice — once here and once for the
// packages case — which states the same assumption about the scaffold's layout
// in two places: when that layout changes, a comment landing above the table or
// tomledit emitting it differently, one copy gets fixed and the other keeps
// inserting somewhere harmless-looking and wrong.
func addAstroKey(t *testing.T, p *project, line string) {
	t.Helper()
	path := filepath.Join(p.Dir, "pyproject.toml")
	raw := read(t, path)

	const anchor = "[tool.astro]\n"
	if !strings.Contains(raw, anchor) {
		t.Fatalf("no [tool.astro] in the scaffolded manifest:\n%s", raw)
	}
	write(t, path, strings.Replace(raw, anchor, anchor+line+"\n", 1))
}

// The runtime image's install step mounts a netrc secret, so a generated
// build takes one without a declared Dockerfile.
func TestDeployTakesANetrcBuildSecretWithoutADockerfile(t *testing.T) {
	tier(t, 0)

	p := v2ProjectForDeploy(t).forT(t)
	r := p.run("deploy", "--build-secret", "id=netrc,src=/dev/null")
	if strings.Contains(r.output(), "reads only the netrc build secret") {
		t.Fatalf("the runtime image mounts netrc, so a generated build takes it\n%s", r.output())
	}
	if !strings.Contains(r.output(), "workspace is required") {
		t.Errorf("expected it to get past the build-secret gate and stop at the workspace\n%s", r.output())
	}
}
