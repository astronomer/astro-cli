//go:build e2e && !windows

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The image `astro package astro` generates carries the project and none of
// its local secrets, and a gitignored credential stops it before it builds.
//
// The image is the same one a deploy generates (imagebuild.ForManifest), and
// it goes to a registry either way, so what is asked here is what a deploy
// would push: include/ and the rest of the project in /usr/local/airflow,
// and no .env at any depth, no airflow_settings.yaml at any depth, and none of
// the root files an Airflow run with the project as its home leaves behind.
func TestThePackagedImageCarriesTheProjectAndNoSecrets(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "shipsafe")
	needsDocker(t, p)
	// Registered before the build that creates them, by repository, as
	// TestTheLocalBuildAndThePackagedImageAgreeOnADockerfile explains.
	t.Cleanup(func() { removeImagesNamed(t, "astro-package/shipsafe") })

	files := map[string]string{
		"include/helpers.py":            "VALUE = 1\n",
		"include/airflow.cfg":           "the project's own fixture\n",
		"include/logs/fixture.json":     "{}\n",
		"include/.env":                  "SECRET=nested\n",
		"include/airflow_settings.yaml": "airflow:\n  connections: []\n",
		"airflow_settings.yaml":         "airflow:\n  connections: []\n",
		".env.prod":                     "SECRET=prod\n",
		".envrc":                        "export SECRET=direnv\n",
		"airflow.cfg":                   "[core]\nfernet_key = local\n",
		"airflow.db":                    "sqlite\n",
		"logs/scheduler/latest.log":     "local log\n",
		"certs/prod.pem":                "-----BEGIN PRIVATE KEY-----\n",
		"target/manifest.json":          "{}\n",
	}
	for rel, body := range files {
		path := filepath.Join(p.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, path, body)
	}
	// A git repository that ignores the key and a dbt target, under the
	// suite's own HOME, so no developer's git config is read.
	gitInit := exec.CommandContext(t.Context(), "git", "-C", p.Dir, "init", "-q")
	gitInit.Env = p.env(map[string]string{"GIT_CONFIG_NOSYSTEM": "1"})
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Skipf("git is needed for the gitignored-credential half: %v\n%s", err, out)
	}
	write(t, filepath.Join(p.Dir, ".gitignore"), read(t, filepath.Join(p.Dir, ".gitignore"))+"certs/\ntarget/\n")
	// A linked directory: the image carries what it points at, and git, which
	// refuses a path beyond a link, must still answer for the rest.
	if err := os.MkdirAll(filepath.Join(p.Dir, "libs", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.Dir, "libs", "shared", "util.py"), "UTIL = 1\n")
	if err := os.Symlink(filepath.Join("..", "libs", "shared"), filepath.Join(p.Dir, "include", "shared")); err != nil {
		t.Fatal(err)
	}

	refused := p.runSlow("package", "astro").requireFailure()
	if !strings.Contains(refused.Stderr, "certs/prod.pem looks like credentials") {
		t.Fatalf("a gitignored key was not refused by name\n%s", refused.output())
	}
	if tags, _ := dockerLines(t.Context(), "images", "--format", "{{.Repository}}:{{.Tag}}", "astro-package/shipsafe"); len(tags) > 0 {
		t.Errorf("a refused package built an image anyway: %v", tags)
	}

	// The way out the message names: .dockerignore. The dbt target is still
	// gitignored and ships, which is a warning, not a refusal.
	write(t, filepath.Join(p.Dir, ".dockerignore"), "certs/\n")
	var res struct {
		Image string `json:"image"`
	}
	built := p.runSlow("package", "astro", "--output", "json").requireSuccess()
	built.requireLastJSON(&res)
	if !strings.Contains(built.Stdout+built.Stderr, "target/manifest.json") {
		t.Errorf("the gitignored target/manifest.json that ships was not warned about\n%s", built.output())
	}
	if res.Image == "" {
		t.Fatalf("`astro package astro` named no image\n%s", built.output())
	}

	out, err := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--entrypoint", "sh", res.Image,
		"-c", "cd /usr/local/airflow && find . -type f -o -type l").CombinedOutput()
	if err != nil {
		t.Fatalf("could not list the packaged image %s: %v\n%s", res.Image, err, out)
	}
	var carried []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		carried = append(carried, strings.TrimPrefix(line, "./"))
	}
	for _, want := range []string{"include/helpers.py", "include/shared/util.py", "include/airflow.cfg", "include/logs/fixture.json", "target/manifest.json", "pyproject.toml"} {
		if !slices.Contains(carried, want) {
			t.Errorf("the image lacks the project's %s\n%s", want, out)
		}
	}
	for _, never := range []string{
		"include/.env", "include/airflow_settings.yaml", "airflow_settings.yaml", ".env.prod", ".envrc",
		"airflow.cfg", "airflow.db", "logs/scheduler/latest.log", "certs/prod.pem",
	} {
		if slices.Contains(carried, never) {
			t.Errorf("the image carries %s, which no shipped image may\n%s", never, out)
		}
	}
}
