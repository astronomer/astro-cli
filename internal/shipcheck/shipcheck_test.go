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

// check surveys the project for a docker build and checks it, as Survey
// does.
func check(t *testing.T, dir, dockerfile string) Report {
	t.Helper()
	s, err := imagebuild.SurveyProject(dir, dockerfile, false)
	require.NoError(t, err)
	return Check(dir, &s)
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
	assert.Contains(t, err.Error(), "keys/service-account.json look like credentials and are not tracked by git")
	assert.Contains(t, err.Error(), "to a registry")

	// A .dockerignore line is the way out, and the warning goes with it.
	writeFiles(t, dir, map[string]string{".dockerignore": "scratch.txt\nkeys/\ntarget/\n"})
	r = check(t, dir, "")
	assert.Empty(t, r.Gitignored)
	require.NoError(t, r.Err())
}

// A file that looks like a credential and was never added is refused as a
// gitignored one is; a committed or staged one is the project's choice.
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

// A declared Dockerfile is held to a generated build's rule: everything in its
// build context counts as shipped, whatever the Dockerfile copies. The way out
// it names is the ignore file the build reads.
func TestCheckDeclaredDockerfile(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		"Dockerfile":     "FROM python:3.12\nCOPY requirements.txt /app/\n",
		".gitignore":     ".env\ncerts/\n",
		".env":           "SECRET=x\n",
		"certs/prod.pem": "k",
	})
	r := check(t, dir, "Dockerfile")
	assert.ElementsMatch(t, []string{".env", "certs/prod.pem"}, r.Secrets)
	assert.ErrorContains(t, r.Err(), "Add them to .dockerignore")

	writeFiles(t, dir, map[string]string{".containerignore": "nothing\n"})
	s, err := imagebuild.SurveyProject(dir, "Dockerfile", true)
	require.NoError(t, err)
	podman := Check(dir, &s)
	assert.ErrorContains(t, podman.Err(), "Add them to .containerignore", "podman reads its own ignore file")

	r = check(t, dir, "")
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

// A link that stays inside the project is asked about at its real path, which
// git can answer for where it refuses a path beyond a link.
func TestCheckAsksAboutAnInProjectLinkAtItsRealPath(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		".gitignore":          "*.pem\n",
		"libs/shared/util.py": "",
		"libs/shared/ca.pem":  "committed",
		"libs/shared/new.pem": "k",
	})
	gitRun(t, dir, "add", ".gitignore", "libs/shared/util.py")
	gitRun(t, dir, "add", "-f", "libs/shared/ca.pem")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "include"), 0o755))
	if err := os.Symlink(filepath.Join("..", "libs", "shared"), filepath.Join(dir, "include", "shared")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	r := check(t, dir, "")
	assert.Empty(t, r.Unanswered, "the link did not fail the query")
	assert.ElementsMatch(t, []string{"libs/shared/new.pem", "include/shared/new.pem"}, r.Secrets,
		"judged by the real path: the committed ca.pem is not refused through the link either")
}

// A file a generated build reaches through a link out of the project is not
// asked of git: one that looks like a credential, by its path in the project
// or where the link leads, is refused. That includes a credential committed in
// a monorepo's shared library, by intent: the fix is one .dockerignore line.
func TestCheckRefusesCredentialsBehindALinkOutOfTheProject(t *testing.T) {
	needsGit(t)
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	writeFiles(t, repo, map[string]string{
		"shared-lib/util.py":      "",
		"shared-lib/sa-prod.json": "{}",
		"project/dags/a.py":       "",
	})
	gitRun(t, repo, "add", "shared-lib/util.py", "shared-lib/sa-prod.json", "project/dags/a.py")
	project := filepath.Join(repo, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "include"), 0o755))
	if err := os.Symlink(filepath.Join("..", "..", "shared-lib"), filepath.Join(project, "include", "shared")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	r := check(t, project, "")
	require.Len(t, r.Outside, 1)
	assert.Contains(t, r.Outside[0], "include/shared/sa-prod.json → ")
	assert.Contains(t, r.Outside[0], filepath.Join("shared-lib", "sa-prod.json"))
	assert.Empty(t, r.Secrets)
	err := r.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reached through a link out of the project")
	assert.NotContains(t, err.Error(), "not tracked by git")

	writeFiles(t, project, map[string]string{".dockerignore": "include/shared/sa-prod.json\n"})
	r = check(t, project, "")
	assert.NoError(t, r.Err(), "one .dockerignore line is the way out")
}

// Where the link leads counts as much as the name in the project, and the
// check runs whether or not the project is a git repository.
func TestCheckRefusesALinkToACredentialOutsideAnyRepository(t *testing.T) {
	project := t.TempDir()
	writeFiles(t, project, map[string]string{"dags/a.py": ""})
	home := t.TempDir()
	writeFiles(t, home, map[string]string{".docker/config.json": "{\"auths\":{}}"})
	if err := os.Symlink(filepath.Join(home, ".docker"), filepath.Join(project, "dockercfg")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	r := check(t, project, "")
	require.Len(t, r.Outside, 1, "dockercfg/config.json looks like nothing; ~/.docker/config.json does")
	assert.Contains(t, r.Outside[0], "dockercfg/config.json → ")
	assert.Error(t, r.Err())
}

// A linked file that cannot be resolved (gone since the survey) is asked
// about as its outermost link, never as a path beyond any link; a file under
// no link is asked about as listed, without resolving anything.
func TestLocate(t *testing.T) {
	root := t.TempDir()
	in, out := locate(root, []string{"gone/inner/x.py", "dags/a.py"}, []string{"gone", "gone/inner"})
	assert.Equal(t, map[string]string{"gone/inner/x.py": "gone", "dags/a.py": "dags/a.py"}, in)
	assert.Empty(t, out)
}

// A declared build carries a link as a link, not what it points at, so a
// symlink there is never refused, wherever it points and whatever git says.
func TestCheckNeverRefusesADeclaredBuildsLink(t *testing.T) {
	needsGit(t)
	project := t.TempDir()
	gitRun(t, project, "init", "-q")
	writeFiles(t, project, map[string]string{"Dockerfile": "FROM python:3.12\n", "real.pem": "k"})
	outside := t.TempDir()
	writeFiles(t, outside, map[string]string{"far.pem": "k"})
	if err := os.Symlink(filepath.Join(outside, "far.pem"), filepath.Join(project, "new.pem")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	r := check(t, project, "Dockerfile")
	assert.Equal(t, []string{"real.pem"}, r.Secrets, "the regular file is refused; the link is not")
	assert.Empty(t, r.Outside)
}

// A declared Dockerfile's context carries .git unless its ignore file drops
// it. A config holding a credential stops the build; a plain user does not.
func TestCheckDeclaredGitMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"Dockerfile":  "FROM python:3.12\n",
		".git/config": "[remote \"origin\"]\n\turl = https://github.com/acme/repo.git\n",
	})
	r := check(t, dir, "Dockerfile")
	require.NoError(t, r.Err())
	assert.Empty(t, r.VCSSecrets)

	for _, config := range []string{
		"[http \"https://github.com/\"]\n\textraheader = AUTHORIZATION: basic eDp0b2tlbg==\n",
		"[remote \"origin\"]\n\turl = https://bot:ghp_secret@github.com/acme/repo.git\n",
		"[remote \"origin\"]\n\turl = https://ghp_0123456789abcdef@github.com/acme/repo.git\n",
		"[url \"https://x-access-token:ghs_abc@github.com/\"]\n\tinsteadOf = https://github.com/\n",
		"[url \"https://oauth2:glpat-abcdef@gitlab.com/\"]\n\tinsteadOf = https://gitlab.com/\n",
		"[remote \"origin\"]\n\turl = https://glpat-abcdefghij@gitlab.com/acme/repo.git\n",
	} {
		writeFiles(t, dir, map[string]string{".git/config": config})
		r = check(t, dir, "Dockerfile")
		assert.ErrorContains(t, r.Err(), "holds a credential", config)
	}
	// A plain user is not a credential.
	for _, config := range []string{
		"[remote \"origin\"]\n\turl = ssh://git@github.com/acme/repo.git\n",
		"[remote \"origin\"]\n\turl = https://user@bitbucket.org/acme/repo.git\n",
		"[remote \"origin\"]\n\turl = git@github.com:acme/repo.git\n",
		// An opaque token of an unknown kind as a bare user is not detected:
		// it cannot be told from a user name.
		"[remote \"origin\"]\n\turl = https://0123456789abcdef0123456789abcdef@example.com/acme/repo.git\n",
	} {
		writeFiles(t, dir, map[string]string{".git/config": config})
		r = check(t, dir, "Dockerfile")
		assert.NoError(t, r.Err(), config)
	}

	writeFiles(t, dir, map[string]string{".dockerignore": "**/.git\n"})
	r = check(t, dir, "Dockerfile")
	assert.NoError(t, r.Err())
}

func TestSurvey(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{".gitignore": "target/\n", "target/out.json": "{}", "dags/a.py": ""})
	var warned []string
	survey, err := Survey(dir, "", false, func(w string) { warned = append(warned, w) })
	require.NoError(t, err)
	assert.Equal(t, 1, survey.DagsCarried)
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "target/out.json")

	writeFiles(t, dir, map[string]string{".gitignore": "target/\nid_rsa\n", "id_rsa": "k"})
	_, err = Survey(dir, "", false, func(string) {})
	assert.ErrorContains(t, err, "id_rsa look like credentials")

	// A declared Dockerfile's local files, .git among them, are warned about
	// from the same survey.
	writeFiles(t, dir, map[string]string{"Dockerfile": "FROM python:3.12\n", ".astro/standalone/airflow.db": "", ".dockerignore": "id_rsa\n"})
	warned = nil
	_, err = Survey(dir, "Dockerfile", false, func(w string) { warned = append(warned, w) })
	require.NoError(t, err)
	assert.Contains(t, fmt.Sprint(warned), "local or per-machine files: .git, .astro/standalone")
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
