//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `astro deploy` no longer has the flags only a 1.x project's deploy read, and
// a run passing one is told what replaced it.
//
// The failure worth preventing is the silent one: a CI job passing --pytest,
// watching the deploy succeed, and believing tests ran. Each refusal therefore
// names the flag and says what to do instead, and the text is the feature
// here rather than the exit code.
//
// Tier 0: the refusal happens while flags parse, before anything is read or
// contacted. One scaffold serves every case, and each subtest takes a view
// bound to its own T so a failure stays in its own case.
func TestDeployRefusesThe1xOnlyFlags(t *testing.T) {
	tier(t, 0)

	parent := manifestProjectForDeploy(t)

	// The eight, written out rather than imported. Importing the messages from
	// cmd would make this a test that the CLI agrees with itself; the point
	// is that somebody decided these are the eight and said why.
	for _, tc := range []struct {
		args []string
		// says is a fragment of the guidance, not the whole sentence: the
		// wording will be reworded, the advice should not silently vanish.
		says string
	}{
		{[]string{"--pytest"}, "run your tests before deploying"},
		{[]string{"--parse"}, "check your DAGs before deploying"},
		{[]string{"--dags-path", "somewhere"}, "ships the project's dags directory"},
		{[]string{"--dag-bundle-name", "bundle"}, "named DAG bundle is not supported yet"},
		{[]string{"--test", "tests/"}, "run your tests before deploying"},
		{[]string{"--env", ".env.test"}, "runs no tests"},
		{[]string{"--save"}, "asks which Deployment to deploy to"},
		{[]string{"--deployment-name", "prod"}, "with --deployment"},
	} {
		flag := strings.TrimPrefix(tc.args[0], "--")
		t.Run(flag, func(t *testing.T) {
			p := parent.forT(t)
			r := p.run(append([]string{"deploy"}, tc.args...)...).requireFailure()

			// Stderr, not the combined output: guidance a script is meant to
			// notice must not be on stdout, where --output json consumers
			// parse.
			if !strings.Contains(r.Stderr, "--"+flag+" was removed in Astro CLI v2") {
				t.Errorf("the refusal should name the flag that was removed\n%s", r.output())
			}
			if !strings.Contains(r.Stderr, tc.says) {
				t.Errorf("the refusal should say what to do instead (%q)\n%s", tc.says, r.output())
			}
		})
	}

	// And in json mode the refusal is the error object, a usage error.
	t.Run("json mode publishes the refusal as an object", func(t *testing.T) {
		p := parent.forT(t)
		var payload struct {
			Error string `json:"error"`
			Code  int    `json:"code"`
			Kind  string `json:"kind"`
		}
		p.run("deploy", "--pytest", "--output", "json").requireFailure().requireJSON(&payload)

		if !strings.Contains(payload.Error, "--pytest was removed in Astro CLI v2") {
			t.Errorf("the object should carry the refusal, got %q", payload.Error)
		}
		if payload.Code != 2 || payload.Kind != "usage" {
			t.Errorf("want a usage error (code 2), got code %d kind %q", payload.Code, payload.Kind)
		}
	})
}

// make1xProject lays out a project the way Astro CLI 1.x made one: a
// Dockerfile, a .astro/config.yaml and dags/, and no pyproject.toml.
func make1xProject(t *testing.T, p *project) {
	t.Helper()
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM quay.io/astronomer/astro-runtime:12.0.0\n")
	mkdir(t, p.Dir, ".astro")
	write(t, filepath.Join(p.Dir, ".astro", "config.yaml"), "project:\n  name: legacy\n")
	mkdir(t, p.Dir, "dags")
}

// v2 deploys only pyproject.toml projects. A project in the 1.x layout is
// refused whatever the deploy asked for, with the two ways forward: convert
// it with astro init, or deploy it with Astro CLI 1.x. Under --output json it
// is the error object, of kind no_project. Nothing is contacted, and the
// project is left as it was: no file the deploy would have written (a
// .dockerignore edit, a saved Deployment) appears.
func TestDeployRefusesA1xProject(t *testing.T) {
	tier(t, 0)

	parent := newProject(t)
	make1xProject(t, parent)
	before := listTree(t, parent.Dir)

	for _, args := range [][]string{
		{"deploy", "dep-id"},
		{"deploy", "dep-id", "--dags"},
		{"deploy", "dep-id", "--image"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			p := parent.forT(t)
			r := p.run(args...).requireFailure()
			for _, want := range []string{"Astro CLI 1.x layout", "astro init", "deploy it with Astro CLI 1.x"} {
				if !strings.Contains(r.Stderr, want) {
					t.Errorf("the refusal should say %q\n%s", want, r.output())
				}
			}
			if strings.Contains(r.Stderr, "Usage:") {
				t.Errorf("the refusal is not a usage mistake, so no usage block\n%s", r.output())
			}

			var payload struct {
				Error string `json:"error"`
				Kind  string `json:"kind"`
			}
			p.run(append(args, "--output", "json")...).requireFailure().requireJSON(&payload)
			if payload.Kind != "no_project" || !strings.Contains(payload.Error, "Astro CLI 1.x layout") {
				t.Errorf("want the 1.x refusal of kind no_project, got %+v", payload)
			}
		})
	}

	if after := listTree(t, parent.Dir); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Errorf("a refused deploy changed the project\nbefore: %v\nafter:  %v", before, after)
	}
}

// Outside any project the deploy gives the no-project advice, unchanged by
// the 1.x refusal: it says to run astro init, and says nothing of 1.x.
func TestDeployOutsideAProject(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	r := p.run("deploy", "dep-id").requireFailure()
	if !strings.Contains(r.Stderr, "not an Astro project directory") || !strings.Contains(r.Stderr, "astro init") {
		t.Errorf("want the no-project advice\n%s", r.output())
	}
	if strings.Contains(r.Stderr, "1.x") {
		t.Errorf("an empty directory is not a 1.x project\n%s", r.output())
	}
}

// --image-name deploys an image already built, which reads nothing from the
// project, so neither an empty directory nor a 1.x project stops it: it gets
// past the project check, to the container engine an image push needs. With
// Docker pointed at nothing (offline), that is where it stops, so nothing
// leaves the machine.
func TestDeployImageNameNeedsNoProject(t *testing.T) {
	tier(t, 0)

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, p *project)
	}{
		{"outside any project", func(*testing.T, *project) {}},
		{"in a 1.x project", make1xProject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			tc.setup(t, p)
			r := runIn(t, p, "deploy", "dep-id", "--image-name", "img:1").requireFailure()
			if !strings.Contains(r.Stderr, "an image deploy needs") {
				t.Errorf("want it past the project check, stopped at the container engine\n%s", r.output())
			}
			if strings.Contains(r.Stderr, "not an Astro project directory") || strings.Contains(r.Stderr, "1.x layout") {
				t.Errorf("--image-name reads no project, so none should be asked for\n%s", r.output())
			}
		})
	}
}

// listTree is every path under dir, relative to it, in walk order.
func listTree(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		paths = append(paths, rel)
		return err
	})
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	return paths
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
		p := manifestProjectForDeploy(t).forT(t)
		r := p.run("deploy", "--build-secret", "id=x,src=/dev/null").requireFailure()

		if !strings.Contains(r.Stderr, needsOne) {
			t.Errorf("a project with no dockerfile should be told why the secret cannot reach a build\n%s", r.output())
		}
		if !strings.Contains(r.Stderr, whereToPutIt) {
			t.Errorf("the refusal should say where to declare one\n%s", r.output())
		}
	})

	// With one declared the gate must let it through. The deploy still fails,
	// because there is no account here — so rather than assert the absence of
	// the refusal, which would also hold if the command died earlier for some
	// unrelated reason, this asserts it got PAST the gate: the next thing it
	// complains about is the workspace, which is checked further down.
	t.Run("not refused when the project declares one", func(t *testing.T) {
		p := manifestProjectForDeploy(t).forT(t)
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
				p := manifestProjectForDeploy(t).forT(t)
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

// manifestProjectForDeploy is a scaffolded project, which is all `astro deploy`
// needs to take the manifest path: project.HasManifest looks for a pyproject.toml carrying
// [tool.astro].
func manifestProjectForDeploy(t *testing.T) *project {
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

	p := manifestProjectForDeploy(t).forT(t)
	r := p.run("deploy", "--build-secret", "id=netrc,src=/dev/null")
	if strings.Contains(r.output(), "reads only the netrc build secret") {
		t.Fatalf("the runtime image mounts netrc, so a generated build takes it\n%s", r.output())
	}
	if !strings.Contains(r.output(), "workspace is required") {
		t.Errorf("expected it to get past the build-secret gate and stop at the workspace\n%s", r.output())
	}
}

// On Astro Private Cloud v2 builds no project yet: a deploy that would build
// one is refused, saying to use Astro CLI 1.x, before Houston is asked
// anything, and a project in the 1.x layout makes no deploy at all. The
// login is to a Houston that refuses every connection, so a refusal that
// came after a request would read as a connection error instead.
func TestAPCDeployRefusesToBuild(t *testing.T) {
	tier(t, 0)

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, p *project)
		args  []string
		says  []string
		kind  string
	}{
		{"a 1.x project", make1xProject, []string{"deploy", "dep-id"}, []string{"Astro CLI 1.x layout", "Deploy it with Astro CLI 1.x"}, "no_project"},
		{"--image-name from a 1.x project", make1xProject, []string{"deploy", "dep-id", "--image-name", "img:1"}, []string{"Astro CLI 1.x layout"}, "no_project"},
		{"--dags from a 1.x project", make1xProject, []string{"deploy", "dep-id", "--dags"}, []string{"Astro CLI 1.x layout", "does not deploy to Astro Private Cloud"}, "no_project"},
		{"a pyproject.toml project", func(t *testing.T, p *project) {
			p.forT(t).run("init", "--name", "deployable").requireSuccess()
		}, []string{"deploy", "dep-id"}, []string{"cannot build and deploy projects to Astro Private Cloud yet", "use Astro CLI 1.x", "pyproject.toml projects on Astro Private Cloud is coming"}, "usage"},
		{"outside any project", func(*testing.T, *project) {}, []string{"deploy", "dep-id"}, []string{"cannot build and deploy projects to Astro Private Cloud yet", "--image-name"}, "no_project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// With the keyring stamped unavailable, as the json walk's
			// states are, so the login is read without asking the OS for it.
			p := newStateProject(t, jsonState{}, "")
			tc.setup(t, p)
			writeLogin(t, p, "software")

			r := runIn(t, p, tc.args...).requireFailure()
			for _, want := range tc.says {
				if !strings.Contains(r.Stderr, want) {
					t.Errorf("the refusal should say %q\n%s", want, r.output())
				}
			}
			if strings.Contains(r.output(), "connection refused") {
				t.Errorf("the refusal should come before any request\n%s", r.output())
			}

			var payload struct {
				Error string `json:"error"`
				Kind  string `json:"kind"`
			}
			runIn(t, p, append(tc.args, "--output", "json")...).requireFailure().requireJSON(&payload)
			if payload.Kind != tc.kind || !strings.Contains(payload.Error, tc.says[0]) {
				t.Errorf("want %q of kind %s, got %+v", tc.says[0], tc.kind, payload)
			}
		})
	}
}
