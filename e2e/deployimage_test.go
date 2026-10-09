//go:build e2e && !windows

package e2e

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// What a deploy's image carries from the project, asked of the image a real
// `astro deploy` built.
//
// A generated build used to copy nothing of the project into the image:
// plugins/ and include/ never reached a Deployment, though `astro local`
// mounts both. 1.x built with the project as its context, keeping dags/ out
// only when the Deployment takes DAG uploads or runs remote execution, and
// that is the rule asserted here, once per Deployment shape.
//
// The Astro API is a fake that answers the two reads a deploy makes before it
// builds (the Deployment, and the runtimes on offer) and refuses the create
// that comes after, so the deploy stops with its image built and nothing
// pushed anywhere. The Deployment reports no runtime version, which the
// runtime checks read as an old Deployment with nothing to compare against.
func TestTheDeployImageCarriesTheProject(t *testing.T) {
	tier(t, 3)

	for _, tc := range []struct {
		name            string
		dagDeploy       bool
		remoteExecution bool
		dagsInImage     bool
	}{
		{name: "dag-deploy-on", dagDeploy: true},
		{name: "dag-deploy-off", dagsInImage: true},
		{name: "remote-dag-deploy-on", dagDeploy: true, remoteExecution: true},
		{name: "remote-dag-deploy-off", remoteExecution: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := deployImageProject(t, "carries-"+tc.name)
			needsDocker(t, p)
			image := deployUntilTheCreate(t, p, fakeDeployment{dagDeploy: tc.dagDeploy, remoteExecution: tc.remoteExecution})

			got := imageTree(t, image)
			assertShipsTheProject(t, got)
			for _, dag := range []string{"dags/exampledag.py", "dags/mine.py"} {
				if _, ok := got[dag]; ok != tc.dagsInImage {
					t.Errorf("%s in the image: %t, want %t", dag, ok, tc.dagsInImage)
				}
			}
		})
	}
}

// Building the DAGs into the image of a Deployment without DAG deploys, under
// ignore rules that leave every DAG file out, would ship no DAGs to a
// Deployment that runs only the image's. The deploy refuses before it builds,
// and leaves the project's file alone.
func TestTheDeployRefusesDagsTheIgnoreFileLeavesOut(t *testing.T) {
	tier(t, 3)
	for _, rule := range []string{"dags/", "**/*.py"} {
		t.Run(strings.NewReplacer("/", "-", "*", "x").Replace(rule), func(t *testing.T) {
			p := deployImageProject(t, "dagsignored")
			needsDocker(t, p)
			ignore := filepath.Join(p.Dir, ".dockerignore")
			write(t, ignore, read(t, ignore)+rule+"\n")
			writeLoginTo(t, p, fakeAstroAPI(t, fakeDeployment{}).URL)
			repo := "astro-deploy/" + filepath.Base(p.Dir) + "-*"
			t.Cleanup(func() { removeImagesNamed(t, repo) })

			r := p.runSlow("deploy", "dep-e2e", "--output", "json").requireFailure()
			if !strings.Contains(r.Stdout, "leaves out every DAG file in dags/") {
				t.Errorf("the refusal should say the ignore rules leave the DAGs out\n%s", r.output())
			}
			if images, err := dockerLines(t.Context(), "images", "--format", "{{.Repository}}:{{.Tag}}", repo); err != nil || len(images) != 0 {
				t.Errorf("refused before the build, yet found %v (%v)", images, err)
			}
			if !strings.HasSuffix(read(t, ignore), rule+"\n") {
				t.Error("the project's .dockerignore was edited")
			}
		})
	}
}

// A prebuilt image deployed to a Deployment without DAG deploys replaces the
// DAGs it runs with the image's, which the CLI did not put there. The deploy
// says so on stderr before it goes ahead.
func TestTheDeployOfAPrebuiltImageWarnsWhatDagsWillRun(t *testing.T) {
	tier(t, 3)
	p := deployImageProject(t, "prebuiltwarn")
	needsDocker(t, p)
	t.Cleanup(func() { removeImagesNamed(t, "astro-package/prebuiltwarn") })
	var res struct {
		Image string `json:"image"`
	}
	p.runSlow("package", "astro", "--output", "json").requireSuccess().requireLastJSON(&res)
	writeLoginTo(t, p, fakeAstroAPI(t, fakeDeployment{}).URL)

	r := p.runSlow("deploy", "dep-e2e", "--image-name", res.Image, "--output", "json").requireFailure()
	if !strings.Contains(r.Stdout+r.Stderr, fakeCreateRefusal) {
		t.Fatalf("the deploy should have reached the fake create\n%s", r.output())
	}
	if !strings.Contains(r.Stderr, "runs only the DAGs inside "+res.Image) {
		t.Errorf("the deploy should warn that only the image's DAGs will run\n%s", r.output())
	}
}

// A symlink in the project is copied as a link, as docker and the 1.x build
// copy it, and what it points at outside the project is not copied at all.
func TestTheDeployImageKeepsALinkOutOfTheProjectAsALink(t *testing.T) {
	tier(t, 3)
	p := deployImageProject(t, "linked")
	needsDocker(t, p)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "outside-secret.txt"), "not the project's\n")
	if err := os.RemoveAll(filepath.Join(p.Dir, "include")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(p.Dir, "include")); err != nil {
		t.Fatal(err)
	}
	image := deployUntilTheCreate(t, p, fakeDeployment{dagDeploy: true})

	got := imageTree(t, image)
	if got["include"] != "link" {
		t.Errorf("include should be in the image as a link, found %q", got["include"])
	}
	for path := range got {
		if strings.Contains(path, "outside-secret") {
			t.Errorf("%s, from outside the project, is in the image", path)
		}
	}
}

// The packaged image carries the project, dags/ included: it cannot know the
// DAG mode of whatever it is later deployed to with --image-name, and a
// Deployment that takes DAG uploads replaces the image's DAGs with the upload
// anyway.
func TestThePackagedImageCarriesTheProject(t *testing.T) {
	tier(t, 3)
	p := deployImageProject(t, "pkgcarries")
	needsDocker(t, p)
	t.Cleanup(func() { removeImagesNamed(t, "astro-package/pkgcarries") })

	var res struct {
		Image string `json:"image"`
	}
	p.runSlow("package", "astro", "--output", "json").requireSuccess().requireLastJSON(&res)
	if res.Image == "" {
		t.Fatal("`astro package astro` named no image")
	}
	got := imageTree(t, res.Image)
	assertShipsTheProject(t, got)
	for _, dag := range []string{"dags/exampledag.py", "dags/mine.py"} {
		if _, ok := got[dag]; !ok {
			t.Errorf("%s is not in the packaged image", dag)
		}
	}
}

// deployImageProject is a scaffolded project with a plugin, an include file, a
// second DAG, a top-level package and tests, plus the per-machine files that
// must stay out of an image and one file the project's own .dockerignore
// leaves out.
func deployImageProject(t *testing.T, name string) *project {
	t.Helper()
	p := newNamedProject(t, name)
	stampKeyringUnavailable(t, p)
	p.run("init", "--name", name).requireSuccess()
	for path, body := range map[string]string{
		"plugins/x.py":                      "X = 1\n",
		"include/queries/y.sql":             "select 1;\n",
		"dags/mine.py":                      "# a second DAG file\n",
		"utils/u.py":                        "U = 1\n",
		"tests/test_dag.py":                 "# shipped, as 1.x shipped it\n",
		"plugins/__pycache__/x.cpython.pyc": "",
		"include/.env":                      "SECRET=1\n",
		".venv/bin/python":                  "a virtualenv\n",
		".astro/standalone/airflow.db":      "a local database\n",
		"include/private.txt":               "kept out by .dockerignore\n",
		// What 1.x's default .dockerignore kept out, and .env variants:
		// secrets and local state that must never reach a registry.
		"airflow_settings.yaml": "connections:\n  - conn_password: hunter2\n",
		"logs/scheduler.log":    "a log\n",
		"airflow.db":            "a database\n",
		"airflow.cfg":           "[core]\n",
		".env.local":            "SECRET=1\n",
		"include/.envrc":        "export SECRET=1\n",
		"astro/legacy.txt":      "1.x's own directory\n",
	} {
		full := filepath.Join(p.Dir, filepath.FromSlash(path))
		mkdir(t, filepath.Dir(full))
		write(t, full, body)
	}
	write(t, filepath.Join(p.Dir, ".dockerignore"), "include/private.txt\n")
	return p
}

// assertShipsTheProject checks what every generated image of
// deployImageProject carries, and what none does.
func assertShipsTheProject(t *testing.T, got map[string]string) {
	t.Helper()
	for _, want := range []string{"plugins/x.py", "utils/u.py", "tests/test_dag.py", "pyproject.toml"} {
		if _, ok := got[want]; !ok {
			t.Errorf("%s is not in the image", want)
		}
	}
	if _, ok := got["include"]; !ok {
		if _, ok := got["include/queries/y.sql"]; !ok {
			t.Error("include/queries/y.sql is not in the image")
		}
	}
	for path := range got {
		for _, never := range []string{
			"__pycache__", ".env", ".venv/", ".astro/", "include/private.txt",
			"airflow_settings.yaml", "logs/", "airflow.db", "airflow.cfg", "astro/legacy.txt",
		} {
			if strings.Contains(path, never) {
				t.Errorf("%s is in the image", path)
			}
		}
	}
}

// deployUntilTheCreate runs `astro deploy` against a fake API shaped like dep,
// which stops it at the create call, and returns the image it built. The image
// is removed when the test ends.
func deployUntilTheCreate(t *testing.T, p *project, dep fakeDeployment) string {
	t.Helper()
	writeLoginTo(t, p, fakeAstroAPI(t, dep).URL)
	repo := "astro-deploy/" + filepath.Base(p.Dir) + "-*"
	// Before the build that creates it: a deploy that fails after building
	// leaves its image, and a failing case is the run where cleanup matters.
	t.Cleanup(func() { removeImagesNamed(t, repo) })

	r := p.runSlow("deploy", "dep-e2e", "--output", "json").requireFailure()
	if !strings.Contains(r.Stdout+r.Stderr, fakeCreateRefusal) {
		t.Fatalf("the deploy should have built its image and stopped at the fake create\n%s", r.output())
	}
	images, err := dockerLines(t.Context(), "images", "--format", "{{.Repository}}:{{.Tag}}", repo)
	if err != nil || len(images) != 1 {
		t.Fatalf("expected one deploy image under %s, found %v (%v)\n%s", repo, images, err, r.output())
	}
	return images[0]
}

// stampKeyringUnavailable keeps a login in the config rather than moving it to
// the OS keyring, which no isolation lever relocates: see newStateProject.
func stampKeyringUnavailable(t *testing.T, p *project) {
	t.Helper()
	dir := mkdir(t, p.home, ".astro", "secrets")
	if err := os.WriteFile(filepath.Join(dir, "login-keyring-unavailable"), []byte("timeout"), 0o600); err != nil {
		t.Fatalf("stamping the keyring unavailable: %v", err)
	}
}

// writeLoginTo is writeLogin with the Astro API at core.
func writeLoginTo(t *testing.T, p *project, core string) {
	t.Helper()
	cfg := fmt.Sprintf(`context: localhost
local:
  core: %s
  platform: cloud
contexts:
  localhost:
    domain: localhost
    token: Bearer e2e-not-a-token
    expiresin: 2100-01-01T00:00:00Z
    organization: e2e-organization
    organization_product: HOSTED
    workspace: e2e-workspace
    last_used_workspace: e2e-workspace
    user_email: e2e@example.invalid
`, core)
	dir := mkdir(t, p.home, ".astro")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("writing the home config: %v", err)
	}
}

// fakeCreateRefusal is what the fake API answers the create-deploy call with,
// so the deploy stops after its build.
const fakeCreateRefusal = "e2e stops the deploy here"

// fakeDeployment is the shape of the Deployment the fake API reports.
type fakeDeployment struct {
	dagDeploy       bool
	remoteExecution bool
}

// fakeAstroAPI answers the reads a deploy makes before it builds, and refuses
// the create after.
func fakeAstroAPI(t *testing.T, dep fakeDeployment) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deployments/dep-e2e"):
			fmt.Fprintf(w, `{"id":"dep-e2e","name":"e2e","organizationId":"e2e-organization","workspaceId":"e2e-workspace","isDagDeployEnabled":%t,"isCicdEnforced":false,"remoteExecution":{"enabled":%t,"allowedIpAddressRanges":[],"remoteApiUrl":""}}`,
				dep.dagDeploy, dep.remoteExecution)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deployment-options"):
			fmt.Fprint(w, `{"runtimeReleases":[]}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"message":%q,"statusCode":500}`, fakeCreateRefusal)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// imageTree maps every file and symlink under the image's AIRFLOW_HOME,
// relative to it, to "file" or "link". It copies the directory out of a
// created (never started) container, so no platform emulation is needed to
// read an amd64 image on an arm64 machine.
func imageTree(t *testing.T, image string) map[string]string {
	t.Helper()
	ctx := context.Background()
	out, err := exec.CommandContext(ctx, "docker", "create", image).Output()
	if err != nil {
		t.Fatalf("creating a container from %s: %v", image, err)
	}
	id := strings.TrimSpace(string(out))
	defer exec.Command("docker", "rm", "-f", id).Run()

	cp := exec.CommandContext(ctx, "docker", "cp", id+":/usr/local/airflow", "-")
	stdout, err := cp.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.Start(); err != nil {
		t.Fatalf("copying AIRFLOW_HOME out of %s: %v", image, err)
	}
	got := map[string]string{}
	tr := tar.NewReader(stdout)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading AIRFLOW_HOME out of %s: %v", image, err)
		}
		_, rel, _ := strings.Cut(h.Name, "/")
		switch h.Typeflag {
		case tar.TypeReg:
			got[rel] = "file"
		case tar.TypeSymlink:
			got[rel] = "link"
		}
	}
	if err := cp.Wait(); err != nil {
		t.Fatalf("copying AIRFLOW_HOME out of %s: %v", image, err)
	}
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	t.Logf("%s holds in AIRFLOW_HOME: %v", image, names)
	return got
}
