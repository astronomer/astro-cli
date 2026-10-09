package shipcontext

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
}

func needsSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege Windows does not grant by default")
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(target, link))
}

func TestTakeListsWhatShipsAndCountsDags(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"dags/a.py": "", "dags/sub/b.py": "", "dags/readme.md": "", "plugins/p.py": "", ".venv/x.py": ""})
	s, err := Take(dir, Options{Ignore: ".venv\n"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dags/a.py", "dags/readme.md", "dags/sub/b.py", "plugins/p.py"}, s.Files)
	assert.Equal(t, 2, s.DagsOnDisk)
	assert.Equal(t, 2, s.DagsShipped)

	for ignore, want := range map[string]int{"dags/\n": 0, "dags/**\n": 0, "**/*.py\n": 0, "*\n": 0, "dags/sub\n": 1, "*\n!dags/a.py\n": 1} {
		got, err := Take(dir, Options{Ignore: ignore})
		require.NoError(t, err)
		assert.Equal(t, 2, got.DagsOnDisk, ignore)
		assert.Equal(t, want, got.DagsShipped, ignore)
	}

	s, err = Take(t.TempDir(), Options{})
	require.NoError(t, err)
	assert.Zero(t, s.DagsOnDisk+s.DagsShipped, "no dags/ has none")
}

// Docker copies a link as its text, so a DAG behind a link reaches the image
// only when the text is relative, stays inside the project read from the
// link's directory, and leads to a path that ships.
func TestTakeCountsDagsThroughLinksAsDockerCopiesThem(t *testing.T) {
	needsSymlinks(t)
	outside := t.TempDir()
	writeFiles(t, outside, map[string]string{"x.py": ""})

	for _, tc := range []struct {
		name    string
		setup   func(dir string)
		ignore  string
		shipped int
	}{
		{"dags/ linked inside", func(dir string) {
			writeFiles(t, dir, map[string]string{"src/airflow_dags/a.py": "", "src/airflow_dags/b.py": ""})
			symlink(t, "src/airflow_dags", filepath.Join(dir, "dags"))
		}, "", 2},
		{"dags/ linked inside, target left out", func(dir string) {
			writeFiles(t, dir, map[string]string{"src/airflow_dags/a.py": ""})
			symlink(t, "src/airflow_dags", filepath.Join(dir, "dags"))
		}, "src\n", 0},
		{"dags/ linked inside, link left out", func(dir string) {
			writeFiles(t, dir, map[string]string{"src/airflow_dags/a.py": ""})
			symlink(t, "src/airflow_dags", filepath.Join(dir, "dags"))
		}, "dags\n", 0},
		{"dags/ linked by absolute path inside", func(dir string) {
			writeFiles(t, dir, map[string]string{"src/airflow_dags/a.py": ""})
			symlink(t, filepath.Join(dir, "src", "airflow_dags"), filepath.Join(dir, "dags"))
		}, "", 0},
		{"dags/ linked outside", func(dir string) {
			symlink(t, outside, filepath.Join(dir, "dags"))
		}, "", 0},
		{"a DAG file linked inside", func(dir string) {
			writeFiles(t, dir, map[string]string{"lib/job.py": ""})
			symlink(t, "../lib/job.py", filepath.Join(dir, "dags", "job.py"))
		}, "", 1},
		{"a DAG file linked climbing out", func(dir string) {
			rel, err := filepath.Rel(filepath.Join(dir, "dags"), filepath.Join(outside, "x.py"))
			require.NoError(t, err)
			symlink(t, rel, filepath.Join(dir, "dags", "x.py"))
		}, "", 0},
		{"a DAG file linked to one under dags/ counts once", func(dir string) {
			writeFiles(t, dir, map[string]string{"dags/a.py": ""})
			symlink(t, "a.py", filepath.Join(dir, "dags", "alias.py"))
		}, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)
			s, err := Take(dir, Options{Ignore: tc.ignore})
			require.NoError(t, err)
			assert.Equal(t, tc.shipped, s.DagsShipped)
			assert.Positive(t, s.DagsOnDisk, "the DAGs are on disk")
		})
	}
}

func TestTakeThroughAProjectReachedByALink(t *testing.T) {
	needsSymlinks(t)
	target := t.TempDir()
	writeFiles(t, target, map[string]string{"dags/a.py": ""})
	link := filepath.Join(t.TempDir(), "project")
	symlink(t, target, link)
	s, err := Take(link, Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{"dags/a.py"}, s.Files)
	assert.Equal(t, 1, s.DagsShipped)
}

func TestTakeDigestFollowsWhatShips(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"plugins/x.py": "x", ".venv/y": "y"})
	digest := func() string {
		t.Helper()
		h := sha256.New()
		_, err := Take(dir, Options{Ignore: ".venv\n", Digest: h})
		require.NoError(t, err)
		return fmt.Sprintf("%x", h.Sum(nil))
	}
	first := digest()
	writeFiles(t, dir, map[string]string{".venv/y": "changed"})
	assert.Equal(t, first, digest())
	writeFiles(t, dir, map[string]string{"plugins/x.py": "changed"})
	assert.NotEqual(t, first, digest())
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	require.NoError(t, cmd.Run())
}

func TestTakeFindsGitignoredFilesAndSecretsAmongThem(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".gitignore":                "target/\nkeys/\n",
		"target/manifest.json":      "{}",
		"keys/service-account.json": "{}",
		"dags/a.py":                 "",
	})
	s, err := Take(dir, Options{Git: true})
	require.NoError(t, err)
	assert.Empty(t, s.Gitignored, "not a git repository")

	initRepo(t, dir)
	s, err = Take(dir, Options{Git: true})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"target/manifest.json", "keys/service-account.json"}, s.Gitignored)
	assert.Equal(t, []string{"keys/service-account.json"}, s.Secrets)

	s, err = Take(dir, Options{})
	require.NoError(t, err)
	assert.Empty(t, s.Gitignored, "only when asked")
}

func TestLooksSecret(t *testing.T) {
	for _, p := range []string{
		"certs/server.pem", "tls.KEY", "client.p12", "client.pfx", "id_rsa", "id_rsa.pub",
		"config/aws_credentials", "credentials.json", "gcp-key.json", "gcp_key.json",
		"service-account-prod.json", "sa-prod.json", ".aws/config", "home/.ssh/known_hosts",
		".netrc", ".npmrc", ".pypirc", "prod.kubeconfig", "kubeconfig", ".env", ".env.local",
	} {
		assert.True(t, LooksSecret(p), p)
	}
	for _, p := range []string{
		"target/manifest.json", "dags/keys.py", "include/key.txt", "aws/config.yaml",
		"service.json", "sa.json", "env.py", "include/monkey.json",
	} {
		assert.False(t, LooksSecret(p), p)
	}
}

func TestSecretsError(t *testing.T) {
	assert.NoError(t, SecretsError(nil))
	err := SecretsError([]string{"a.pem"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a.pem")
	assert.Contains(t, err.Error(), "ignored by git")
	assert.Contains(t, err.Error(), "pushed to a registry")
	assert.Contains(t, err.Error(), ".dockerignore")

	many := make([]string, 12)
	for i := range many {
		many[i] = fmt.Sprintf("k%02d.pem", i)
	}
	err = SecretsError(many)
	assert.Contains(t, err.Error(), "k09.pem and 2 more")
	assert.NotContains(t, err.Error(), "k10.pem")
}
