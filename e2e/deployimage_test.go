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
// A generated build's context used to be a synthetic directory holding only
// requirements.txt and packages.txt, so the runtime base's ONBUILD `COPY . .`
// copied nothing of the project: plugins/ and include/ never reached a
// Deployment, though `astro local` mounts both and the project ran locally.
// 1.x built with the project as its context, keeping dags/ out only when the
// Deployment takes DAG uploads, and that is the rule asserted here, once per
// DAG mode.
//
// The Astro API is a fake that answers the two reads a deploy makes before it
// builds (the Deployment, and the runtimes on offer) and refuses the create
// that comes after, so the deploy stops with its image built and nothing
// pushed anywhere. The Deployment reports no runtime version, which the
// runtime checks read as an old Deployment with nothing to compare against.
func TestTheDeployImageCarriesTheProjectFiles(t *testing.T) {
	tier(t, 3)

	for _, tc := range []struct {
		name      string
		dagDeploy bool
		// want lists what the image's AIRFLOW_HOME holds under dags/,
		// plugins/ and include/.
		want []string
	}{
		{
			name:      "dag-deploy-on",
			dagDeploy: true,
			want:      []string{"include/queries/y.sql", "plugins/x.py"},
		},
		{
			name:      "dag-deploy-off",
			dagDeploy: false,
			want:      []string{"dags/exampledag.py", "dags/mine.py", "include/queries/y.sql", "plugins/x.py"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := deployImageProject(t, "carries-"+tc.name)
			needsDocker(t, p)
			srv := fakeAstroAPI(t, tc.dagDeploy)
			writeLoginTo(t, p, srv.URL)

			repo := "astro-deploy/" + filepath.Base(p.Dir) + "-*"
			// Before the build that creates it: a deploy that fails after
			// building leaves its image, and this case's failure is the run
			// where cleanup matters.
			t.Cleanup(func() { removeImagesNamed(t, repo) })

			r := p.runSlow("deploy", "dep-e2e", "--output", "json").requireFailure()
			if !strings.Contains(r.Stdout+r.Stderr, fakeCreateRefusal) {
				t.Fatalf("the deploy should have built its image and stopped at the fake create\n%s", r.output())
			}

			images, err := dockerLines(t.Context(), "images", "--format", "{{.Repository}}:{{.Tag}}", repo)
			if err != nil || len(images) != 1 {
				t.Fatalf("expected one deploy image under %s, found %v (%v)\n%s", repo, images, err, r.output())
			}
			got := projectFilesIn(t, images[0])
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("the deploy image holds %v, want %v", got, tc.want)
			}
		})
	}
}

// The packaged image carries the project's dags/, plugins/ and include/: it
// cannot know the DAG mode of whatever it is later deployed to with
// --image-name, and a Deployment that takes DAG uploads replaces the image's
// dags/ with the upload anyway.
func TestThePackagedImageCarriesTheProjectFiles(t *testing.T) {
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
	got := projectFilesIn(t, res.Image)
	want := []string{"dags/exampledag.py", "dags/mine.py", "include/queries/y.sql", "plugins/x.py"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the packaged image holds %v, want %v", got, want)
	}
}

// deployImageProject is a scaffolded project with a plugin, an include file and
// a second DAG, plus the per-machine files that must stay out of an image and
// one file the project's own .dockerignore leaves out.
func deployImageProject(t *testing.T, name string) *project {
	t.Helper()
	p := newNamedProject(t, name)
	stampKeyringUnavailable(t, p)
	p.run("init", "--name", name).requireSuccess()
	for path, body := range map[string]string{
		"plugins/x.py":                      "X = 1\n",
		"include/queries/y.sql":             "select 1;\n",
		"dags/mine.py":                      "# a second DAG file\n",
		"plugins/__pycache__/x.cpython.pyc": "",
		"include/.env":                      "SECRET=1\n",
		"include/private.txt":               "kept out by .dockerignore\n",
		"tests/test_dag.py":                 "# not shipped\n",
	} {
		full := filepath.Join(p.Dir, filepath.FromSlash(path))
		mkdir(t, filepath.Dir(full))
		write(t, full, body)
	}
	write(t, filepath.Join(p.Dir, ".dockerignore"), "include/private.txt\n")
	return p
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

// fakeAstroAPI answers the reads a deploy makes before it builds, and refuses
// the create after.
func fakeAstroAPI(t *testing.T, dagDeploy bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deployments/dep-e2e"):
			fmt.Fprintf(w, `{"id":"dep-e2e","name":"e2e","organizationId":"e2e-organization","workspaceId":"e2e-workspace","isDagDeployEnabled":%t,"isCicdEnforced":false}`, dagDeploy)
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

// projectFilesIn lists the regular files under dags/, plugins/ and include/ in
// the image's AIRFLOW_HOME, sorted. It copies the directory out of a created
// (never started) container, so no platform emulation is needed to read an
// amd64 image on an arm64 machine.
func projectFilesIn(t *testing.T, image string) []string {
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
	var files, all []string
	tr := tar.NewReader(stdout)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading AIRFLOW_HOME out of %s: %v", image, err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		_, rel, _ := strings.Cut(h.Name, "/")
		all = append(all, rel)
		for _, dir := range []string{"dags/", "plugins/", "include/"} {
			if strings.HasPrefix(rel, dir) {
				files = append(files, rel)
			}
		}
	}
	if err := cp.Wait(); err != nil {
		t.Fatalf("copying AIRFLOW_HOME out of %s: %v", image, err)
	}
	sort.Strings(files)
	sort.Strings(all)
	t.Logf("%s holds in AIRFLOW_HOME: %v", image, all)
	return files
}
