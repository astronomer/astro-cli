package shipcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
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

// check surveys the project and checks it, as Survey does.
func check(t *testing.T, dir, dockerfile string) Report {
	t.Helper()
	s, err := imagebuild.SurveyProject(dir, dockerfile)
	require.NoError(t, err)
	return Check(dir, dockerfile, &s)
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
	r := check(t, dir, "")
	assert.Empty(t, r.Gitignored, "not a git repository: nothing to ask")
	assert.Empty(t, r.Warnings())
	require.NoError(t, r.Err())

	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "add", ".gitignore", ".dockerignore", "dags/a.py")
	r = check(t, dir, "")
	assert.ElementsMatch(t, []string{"target/manifest.json", "keys/service-account.json"}, r.Gitignored,
		".env and a .dockerignored file never ship, so git is not asked about them")
	assert.Equal(t, []string{"keys/service-account.json"}, r.Secrets)
	assert.Empty(t, r.Unanswered)
	w := r.Warnings()
	require.Len(t, w, 1)
	assert.Contains(t, w[0], "2 file(s) that .gitignore ignores")
	assert.Contains(t, w[0], "add them to .dockerignore")
	err := r.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keys/service-account.json look like credentials, are not committed to git")
	assert.Contains(t, err.Error(), "to a registry")

	// A .dockerignore line is the way out, and the warning goes with it.
	writeFiles(t, dir, map[string]string{".dockerignore": "scratch.txt\nkeys/\ntarget/\n"})
	r = check(t, dir, "")
	assert.Empty(t, r.Gitignored)
	require.NoError(t, r.Err())
}

// A file that looks like a credential and was never added is refused as a
// gitignored one is; a committed one is the project's choice.
func TestCheckRefusesUntrackedCredentialsAndLeavesCommittedOnesAlone(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "*.pem\n", "include/ca.pem": "public cert", "include/new-key.json": "k", "include/token.json": "t"})
	gitRun(t, dir, "add", "-f", "include/ca.pem")
	gitRun(t, dir, "add", "include/new-key.json")
	r := check(t, dir, "")
	assert.Empty(t, r.Gitignored)
	assert.Equal(t, []string{"include/token.json"}, r.Secrets, "staged counts as tracked; never added does not")
	assert.ErrorContains(t, r.Err(), "include/token.json")
}

// A declared Dockerfile's context is surveyed under its own ignore file, with
// none of a generated build's rules. A credential there stops the build only
// when the Dockerfile copies the whole context; otherwise it is warned about.
func TestCheckDeclaredDockerfile(t *testing.T) {
	needsGit(t)
	for name, tc := range map[string]struct {
		dockerfile string
		refused    bool
	}{
		"COPY . .":              {"FROM python:3.12\nCOPY . /app\n", true},
		"an Astro Runtime base": {"FROM astrocrpublic.azurecr.io/runtime:3.1-2\n", true},
		"only named files":      {"FROM python:3.12\nCOPY requirements.txt /app/\nCOPY --from=build . /app\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			gitRun(t, dir, "init", "-q")
			writeFiles(t, dir, map[string]string{
				"Dockerfile":     tc.dockerfile,
				".gitignore":     ".env\ncerts/\n",
				".env":           "SECRET=x\n",
				"certs/prod.pem": "k",
			})
			r := check(t, dir, "Dockerfile")
			assert.ElementsMatch(t, []string{".env", "certs/prod.pem"}, r.Secrets)
			if tc.refused {
				require.Error(t, r.Err())
				return
			}
			require.NoError(t, r.Err())
			assert.Contains(t, r.Warnings()[len(r.Warnings())-1], "does not copy the whole context")
		})
	}

	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "certs/\n", ".env": "x", "certs/prod.pem": "k"})
	r := check(t, dir, "")
	assert.Equal(t, []string{"certs/prod.pem"}, r.Secrets, "a generated build never carries .env")
}

// Where git cannot answer, the check says why and names the files that look
// like credentials, and the build goes ahead: a committed file there is the
// common case, and a CI deploy must not break on it.
func TestCheckWarnsWhenGitCannotAnswer(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".git/HEAD": "ref: refs/heads/main\n", "certs/prod.pem": "k", "dags/a.py": "", "README.md": ""})
	t.Setenv("PATH", "")
	r := check(t, dir, "")
	require.Len(t, r.Unanswered, 1)
	assert.Contains(t, r.Unanswered[0], "the project's repository")
	assert.Equal(t, []string{"certs/prod.pem"}, r.Unchecked)
	assert.Empty(t, r.Secrets)
	require.NoError(t, r.Err())
	w := r.Warnings()
	require.Len(t, w, 2)
	assert.Contains(t, w[0], "could not ask git")
	assert.Contains(t, w[1], "certs/prod.pem")
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

	r := check(t, dir, "")
	assert.Empty(t, r.Unanswered, "every repository answered")
	assert.ElementsMatch(t, []string{"vendor/lib/local.key", "scratch/notes.txt"}, r.Gitignored)
	assert.Equal(t, []string{"vendor/lib/local.key"}, r.Secrets)
}

// A generated build carries a linked directory's files, and git refuses a
// path beyond a symbolic link. One link must not fail the repository's query:
// a file is asked about where the link leads when that is in the work tree,
// and otherwise judged by the link itself.
func TestCheckAsksAboutFilesBehindALink(t *testing.T) {
	needsGit(t)
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	writeFiles(t, repo, map[string]string{
		".gitignore":          "*.pem\n",
		"shared-lib/util.py":  "",
		"shared-lib/prod.pem": "k",
		"project/dags/a.py":   "",
	})
	gitRun(t, repo, "add", ".gitignore", "shared-lib/util.py", "project/dags/a.py")
	project := filepath.Join(repo, "project")
	if err := os.Symlink(filepath.Join("..", "..", "shared-lib"), filepath.Join(project, "include-shared")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(project, "include"), 0o755))
	require.NoError(t, os.Rename(filepath.Join(project, "include-shared"), filepath.Join(project, "include", "shared")))
	outside := t.TempDir()
	writeFiles(t, outside, map[string]string{"sa-prod.json": "{}", "helper.py": ""})
	require.NoError(t, os.Symlink(outside, filepath.Join(project, "include", "ext")))

	r := check(t, project, "")
	assert.Empty(t, r.Unanswered, "the link did not fail the query")
	assert.Equal(t, []string{"include/shared/prod.pem"}, r.Gitignored)
	assert.ElementsMatch(t, []string{"include/shared/prod.pem", "include/ext/sa-prod.json"}, r.Secrets,
		"behind a link out of the work tree, an untracked link stands for its files")
}

// A declared Dockerfile's context carries .git unless its ignore file drops
// it. That is warned about; a config holding a credential stops a build that
// copies the context.
func TestCheckDeclaredGitMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"Dockerfile":  "FROM python:3.12\nCOPY . /app\n",
		".git/config": "[remote \"origin\"]\n\turl = https://github.com/acme/repo.git\n",
	})
	r := check(t, dir, "Dockerfile")
	assert.Equal(t, []string{".git"}, r.VCS)
	require.NoError(t, r.Err())
	assert.Empty(t, r.VCSSecrets)

	for _, config := range []string{
		"[http \"https://github.com/\"]\n\textraheader = AUTHORIZATION: basic eDp0b2tlbg==\n",
		"[remote \"origin\"]\n\turl = https://bot:ghp_secret@github.com/acme/repo.git\n",
	} {
		writeFiles(t, dir, map[string]string{".git/config": config})
		r = check(t, dir, "Dockerfile")
		assert.ErrorContains(t, r.Err(), "holds a credential", config)
	}

	writeFiles(t, dir, map[string]string{".dockerignore": "**/.git\n"})
	r = check(t, dir, "Dockerfile")
	assert.Empty(t, r.VCS)
	assert.NoError(t, r.Err())
}

func TestCopiesContext(t *testing.T) {
	for dockerfile, want := range map[string]bool{
		"FROM python:3.12\nCOPY . .\n":                                         true,
		"FROM python:3.12\nCOPY ./ /app/\n":                                    true,
		"FROM python:3.12\nADD --chown=1:1 . /app\n":                           true,
		"FROM python:3.12\nCOPY [\".\", \"/app\"]\n":                           true,
		"FROM python:3.12\nCOPY requirements.txt \\\n  . /app/\n":              true,
		"FROM quay.io/astronomer/astro-runtime:12.1.0\n":                       true,
		"FROM --platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-2\n": true,
		"FROM python:3.12\nCOPY dags/ /app/dags/\n":                            false,
		"FROM python:3.12 AS b\nFROM python:3.12\nCOPY --from=b . /app\n":      false,
		"FROM python:3.12\n# COPY . .\nRUN true\n":                             false,
	} {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		require.NoError(t, os.WriteFile(path, []byte(dockerfile), 0o600))
		assert.Equal(t, want, copiesContext(path), dockerfile)
	}
	assert.True(t, copiesContext(filepath.Join(t.TempDir(), "missing")), "an unreadable file is held to the stricter rule")
}

func TestSurvey(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "target/\n", "target/out.json": "{}", "dags/a.py": ""})
	var warned []string
	survey, err := Survey(dir, "", func(w string) { warned = append(warned, w) })
	require.NoError(t, err)
	assert.Equal(t, 1, survey.DagsCarried)
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "target/out.json")

	writeFiles(t, dir, map[string]string{".gitignore": "target/\nid_rsa\n", "id_rsa": "k"})
	_, err = Survey(dir, "", func(string) {})
	assert.ErrorContains(t, err, "id_rsa look like credentials")

	// A declared Dockerfile's local files are warned about from the same survey.
	writeFiles(t, dir, map[string]string{"Dockerfile": "FROM python:3.12\n", ".astro/standalone/airflow.db": ""})
	warned = nil
	_, err = Survey(dir, "Dockerfile", func(w string) { warned = append(warned, w) })
	require.NoError(t, err, "the Dockerfile copies nothing, so the key is warned about")
	assert.Contains(t, fmt.Sprint(warned), ".astro/standalone")
	assert.Contains(t, fmt.Sprint(warned), "local or per-machine files: .git, ")
}

func TestLooksSecret(t *testing.T) {
	for _, p := range []string{
		"certs/server.pem", "tls.KEY", "client.p12", "client.pfx", "id_rsa", "id_rsa.pub",
		"config/aws_credentials", "credentials.json", "gcp-key.json", "gcp_key.json",
		"service-account-prod.json", "sa-prod.json", ".aws/config", "home/.ssh/known_hosts",
		".netrc", ".npmrc", ".pypirc", "prod.kubeconfig", "kubeconfig", ".env", ".env.local",
		"id_ed25519", "id_ed25519.pub", "id_ecdsa", "id_dsa", "AuthKey.p8", "release.jks", "app.keystore",
		"putty.ppk", "prod.tfvars", "terraform.tfstate", "prod.env", "local.env", "app.env.prod",
		".pgpass", "include/.pgpass", ".docker/config.json", "home/.docker/config.json", "token.json",
		"include/token.json", "client_secret.json", "client_secret_123.apps.googleusercontent.com.json",
	} {
		assert.True(t, LooksSecret(p), p)
	}
	for _, p := range []string{
		"target/manifest.json", "dags/keys.py", "include/key.txt", "aws/config.yaml",
		"service.json", "sa.json", "env.py", "include/monkey.json", "environment.py", "envoy.yaml",
		"config.json", "docker/config.json", "secrets.py", "include/secret_santa.csv", "tokens.json",
		"client_secret.txt",
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
