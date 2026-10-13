package shipcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func needsGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestCheckFindsGitignoredFilesAndSecretsAmongThem(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".gitignore":                "target/\nkeys/\n.env\nscratch.txt\n",
		"target/manifest.json":      "{}",
		"keys/service-account.json": "{}",
		"dags/a.py":                 "",
		".env":                      "SECRET=x\n",
		".dockerignore":             "scratch.txt\n",
		"scratch.txt":               "",
	})
	r, err := Check(dir, "")
	require.NoError(t, err)
	assert.Empty(t, r.Gitignored, "not a git repository: nothing to ask")
	assert.Empty(t, r.Warnings())
	require.NoError(t, r.Err())

	gitRun(t, dir, "init", "-q")
	r, err = Check(dir, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"target/manifest.json", "keys/service-account.json"}, r.Gitignored,
		".env and a .dockerignored file never ship, so git is not asked about them")
	assert.Equal(t, []string{"keys/service-account.json"}, r.Secrets)
	assert.Empty(t, r.Unanswered)
	w := r.Warnings()
	require.Len(t, w, 1)
	assert.Contains(t, w[0], "2 file(s) that .gitignore ignores")
	assert.Contains(t, w[0], "add them to .dockerignore")
	err = r.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keys/service-account.json look like credentials, git ignores them")
	assert.Contains(t, err.Error(), "to a registry")
	assert.Contains(t, err.Error(), ".dockerignore")

	// A .dockerignore line is the way out, and the warning goes with it.
	writeFiles(t, dir, map[string]string{".dockerignore": "scratch.txt\nkeys/\ntarget/\n"})
	r, err = Check(dir, "")
	require.NoError(t, err)
	assert.Empty(t, r.Gitignored)
	require.NoError(t, r.Err())
}

// A tracked file that looks like a credential is the project's choice: git
// vouches for it.
func TestCheckLeavesTrackedCredentialsAlone(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "*.pem\n", "include/ca.pem": "public cert"})
	gitRun(t, dir, "add", "-f", "include/ca.pem")
	r, err := Check(dir, "")
	require.NoError(t, err)
	assert.Empty(t, r.Gitignored)
	assert.NoError(t, r.Err())
}

// A declared Dockerfile's context is surveyed under its own ignore file, with
// none of a generated build's rules: a gitignored .env there ships.
func TestCheckDeclaredDockerfile(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		"Dockerfile":     "FROM x\n",
		".gitignore":     ".env\ncerts/\n",
		".env":           "SECRET=x\n",
		"certs/prod.pem": "k",
	})
	r, err := Check(dir, "Dockerfile")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".env", "certs/prod.pem"}, r.Secrets)

	r, err = Check(dir, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"certs/prod.pem"}, r.Secrets, "a generated build never carries .env")
}

// Where git cannot answer, the check says so, and every carried file that
// looks like a credential is refused instead, tracked or not.
func TestCheckFailsClosedWhenGitCannotAnswer(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".git/HEAD": "ref: refs/heads/main\n", "certs/prod.pem": "k", "dags/a.py": "", "README.md": ""})
	t.Setenv("PATH", "")
	r, err := Check(dir, "")
	require.NoError(t, err)
	require.Len(t, r.Unanswered, 1)
	assert.Contains(t, r.Unanswered[0], "the project's repository")
	assert.Equal(t, []string{"certs/prod.pem"}, r.Secrets)
	assert.Empty(t, r.Gitignored)
	w := r.Warnings()
	require.Len(t, w, 1)
	assert.Contains(t, w[0], "could not ask git")
	assert.Contains(t, r.Err().Error(), "could not say whether they are committed")
}

// A submodule's files are asked about from inside it, since git refuses to
// answer for them from the superproject; and a nested repository the
// enclosing one ignores as a whole has all its files counted as ignored.
func TestCheckAsksEachRepositoryAboutItsOwnFiles(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	sub := filepath.Join(dir, "vendor", "lib")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	gitRun(t, sub, "init", "-q")
	writeFiles(t, sub, map[string]string{".gitignore": "local.key\n", "lib.py": "", "local.key": "k"})
	gitRun(t, sub, "add", "lib.py", ".gitignore")
	gitRun(t, sub, "commit", "-q", "-m", "lib")
	gitRun(t, dir, "add", "vendor/lib")
	ignoredRepo := filepath.Join(dir, "scratch")
	require.NoError(t, os.MkdirAll(ignoredRepo, 0o755))
	gitRun(t, ignoredRepo, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "scratch/\n", "scratch/notes.txt": "n", "dags/a.py": ""})

	r, err := Check(dir, "")
	require.NoError(t, err)
	assert.Empty(t, r.Unanswered, "every repository answered")
	assert.ElementsMatch(t, []string{"vendor/lib/local.key", "scratch/notes.txt"}, r.Gitignored)
	assert.Equal(t, []string{"vendor/lib/local.key"}, r.Secrets)
}

func TestRun(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "target/\n", "target/out.json": "{}"})
	var warned []string
	require.NoError(t, Run(dir, "", func(w string) { warned = append(warned, w) }))
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "target/out.json")

	writeFiles(t, dir, map[string]string{".gitignore": "target/\nid_rsa\n", "id_rsa": "k"})
	assert.ErrorContains(t, Run(dir, "", func(string) {}), "id_rsa look like credentials")
}

func TestLooksSecret(t *testing.T) {
	for _, p := range []string{
		"certs/server.pem", "tls.KEY", "client.p12", "client.pfx", "id_rsa", "id_rsa.pub",
		"config/aws_credentials", "credentials.json", "gcp-key.json", "gcp_key.json",
		"service-account-prod.json", "sa-prod.json", ".aws/config", "home/.ssh/known_hosts",
		".netrc", ".npmrc", ".pypirc", "prod.kubeconfig", "kubeconfig", ".env", ".env.local",
		"id_ed25519", "id_ed25519.pub", "id_ecdsa", "id_dsa", "AuthKey.p8", "release.jks", "app.keystore",
		"putty.ppk", "prod.tfvars", "terraform.tfstate", "prod.env", "local.env", "app.env.prod",
	} {
		assert.True(t, LooksSecret(p), p)
	}
	for _, p := range []string{
		"target/manifest.json", "dags/keys.py", "include/key.txt", "aws/config.yaml",
		"service.json", "sa.json", "env.py", "include/monkey.json", "environment.py", "envoy.yaml",
	} {
		assert.False(t, LooksSecret(p), p)
	}
}

func TestCappedList(t *testing.T) {
	many := make([]string, 12)
	for i := range many {
		many[i] = fmt.Sprintf("k%02d.pem", i)
	}
	got := cappedList(many)
	assert.Contains(t, got, "k09.pem and 2 more")
	assert.NotContains(t, got, "k10.pem")
	assert.Equal(t, "a, b", cappedList([]string{"a", "b"}))
}
