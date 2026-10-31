package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// privateInstallDockerfile is the shape a 1.x project installs a private
// package from git in: two pip-only steps behind the netrc secret, run as
// root, with the comments that explain them.
const privateInstallDockerfile = "FROM astrocrpublic.azurecr.io/runtime:3.3-8-python-3.13\n" +
	"\n" +
	"# corona needs private-repo auth; mount the netrc build secret for the pip install step.\n" +
	"USER root\n" +
	"RUN --mount=type=secret,id=netrc,target=/root/.netrc \\\n" +
	"    pip install --no-cache-dir \"corona @ git+https://github.com/example-org/corona.git@4bfeaf8\"  # 26.9.24\n" +
	"\n" +
	"# The state client, from a subdirectory.\n" +
	"RUN --mount=type=secret,id=netrc,target=/root/.netrc \\\n" +
	"    pip install --no-cache-dir \"state-client @ git+https://github.com/example-org/state.git@f64d7b9#subdirectory=clients/python\"  # v0.7.0\n" +
	"\n" +
	"USER astro\n"

func movePlan(t *testing.T, files map[string]string) (string, *Result) {
	t.Helper()
	dir := t.TempDir()
	writeAll(t, dir, files)
	res, err := Run(dir, Options{MovePipSteps: true})
	require.NoError(t, err)
	return dir, res
}

func TestPipOnlyStepsMoveAndTheDockerfileGoes(t *testing.T) {
	dir, res := movePlan(t, map[string]string{
		"Dockerfile":       privateInstallDockerfile,
		"requirements.txt": "pandas==2.2.3\n",
	})
	corona := "corona @ git+https://github.com/example-org/corona.git@4bfeaf8"
	state := "state-client @ git+https://github.com/example-org/state.git@f64d7b9#subdirectory=clients/python"
	assert.Equal(t, []MovedPackage{{Spec: corona, Line: 5}, {Spec: state, Line: 9}}, res.MovedPackages)

	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow==3.3.*", "pandas==2.2.3", corona, state}, m.Project.Dependencies)
	assert.Empty(t, m.Astro.Dockerfile, "what was left of the Dockerfile builds what a generated build does")
	assert.Equal(t, "3.3-8", m.Astro.Runtime, "the runtime the FROM named stays recorded for the generated build")
	assert.Equal(t, "==3.13.*", m.Project.RequiresPython, "and so does its Python")
	assert.NoFileExists(t, filepath.Join(dir, "Dockerfile"))
	assert.Contains(t, res.Deleted, "Dockerfile (its pip steps moved into pyproject.toml, and the rest is what a project without one builds, removed)")

	assert.Contains(t, res.Advisories, "Dockerfile: moved corona and state-client (line 5, line 9) into [project] dependencies, "+
		"so standalone mode, astro local check and uv.lock have them too. What was left of the Dockerfile builds what a project "+
		"without one does, so it was removed. [tool.astro] build-secrets now names netrc, the secret the steps installed with: "+
		"a build reads ~/.netrc, or else asks git for the credentials of the hosts the packages come from")
	assert.Equal(t, []string{"id=netrc"}, m.Astro.BuildSecrets, "the netrc the steps mounted, for the build to supply")
	assert.Empty(t, res.Notes, "nothing about the secret is left to do")
	for _, n := range res.Notes {
		assert.False(t, strings.HasPrefix(n, "Dockerfile:"), "a removed Dockerfile has nothing left to do: %q", n)
	}
}

func TestNoSecretNoteForStepsThatMountedNone(t *testing.T) {
	_, res := movePlan(t, map[string]string{
		"Dockerfile": "FROM astrocrpublic.azurecr.io/runtime:3.3-11\nRUN pip install tabulate==0.9.0\n",
	})
	require.Len(t, res.MovedPackages, 1)
	assert.Empty(t, res.Notes)
	for _, u := range res.Updated {
		assert.NotContains(t, u, "declared Dockerfile", "a removed Dockerfile is not declared")
	}
}

func TestStepsAfterAnotherStepStay(t *testing.T) {
	// The install base installs before every step the Dockerfile has, so a pip
	// install after an apt-get it may need stays where it is.
	dir, res := movePlan(t, map[string]string{
		"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0\n" +
			"RUN pip install --no-cache-dir humanize==4.11.0\n" +
			"USER root\n" +
			"RUN apt-get update && apt-get install -y libpq-dev\n" +
			"USER astro\n" +
			"RUN pip install psycopg2==2.9.10\n",
	})
	assert.Equal(t, []MovedPackage{{Spec: "humanize==4.11.0", Line: 2}}, res.MovedPackages)
	body, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "humanize")
	assert.Contains(t, string(body), "RUN apt-get update && apt-get install -y libpq-dev\nUSER astro\nRUN pip install psycopg2==2.9.10\n")
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "Dockerfile", m.Astro.Dockerfile)
	assert.Contains(t, m.Project.Dependencies, "humanize==4.11.0")
	assert.NotContains(t, m.Project.Dependencies, "psycopg2==2.9.10")
	assert.Contains(t, res.Notes, "Dockerfile: its RUN steps install psycopg2, which standalone mode and astro local check do not have. "+
		"Add it to pyproject.toml with: uv add psycopg2==2.9.10")
}

func TestOnlyPlainPipInstallsMove(t *testing.T) {
	for name, step := range map[string]string{
		"chained":         "RUN pip install humanize && echo done",
		"requirements":    "RUN pip install -r extra.txt",
		"an index":        "RUN pip install --index-url https://pypi.example.com/simple humanize",
		"no deps":         "RUN pip install --no-deps humanize",
		"upgrade":         "RUN pip install --upgrade humanize",
		"a variable":      "RUN pip install humanize==${VERSION}",
		"another secret":  "RUN --mount=type=secret,id=pip,target=/etc/pip.conf pip install humanize",
		"another mount":   "RUN --mount=type=cache,target=/root/.cache pip install humanize",
		"exec form":       `RUN ["pip", "install", "humanize"]`,
		"editable":        "RUN pip install -e ./lib",
		"airflow":         "RUN pip install apache-airflow-providers-snowflake apache-airflow==3.3.1",
		"a local path":    "RUN pip install ./wheels/humanize.whl",
		"not an install":  "RUN pip download humanize",
		"python packages": "RUN python -c 'import humanize'",
	} {
		t.Run(name, func(t *testing.T) {
			dir, res := movePlan(t, map[string]string{
				"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0\n" + step + "\n",
			})
			assert.Empty(t, res.MovedPackages)
			body, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
			require.NoError(t, err)
			assert.Contains(t, string(body), step)
		})
	}
}

func TestPipInstallFormsThatMove(t *testing.T) {
	for name, step := range map[string]string{
		"pip3":             "RUN pip3 install -q humanize==4.11.0",
		"python -m pip":    "RUN python -m pip install --root-user-action=ignore humanize==4.11.0",
		"uv pip":           "RUN uv pip install --system humanize==4.11.0",
		"root user action": "RUN pip install --root-user-action ignore --disable-pip-version-check humanize==4.11.0",
	} {
		t.Run(name, func(t *testing.T) {
			_, res := movePlan(t, map[string]string{
				"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0\n" + step + "\n",
			})
			assert.Equal(t, []MovedPackage{{Spec: "humanize==4.11.0", Line: 2}}, res.MovedPackages)
		})
	}
}

func TestAStepNamingAListedPackageStays(t *testing.T) {
	// requirements.txt's pin would win, and the step's would go with the step.
	dir, res := movePlan(t, map[string]string{
		"Dockerfile":       "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN pip install humanize==4.11.0\n",
		"requirements.txt": "humanize==4.9.0\n",
	})
	assert.Empty(t, res.MovedPackages)
	body, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "RUN pip install humanize==4.11.0")
}

func TestStepsMoveOnlyWhenTheCallerAsks(t *testing.T) {
	// Astro Desktop converts without locking, so it cannot put a move back.
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{"Dockerfile": privateInstallDockerfile})
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Empty(t, res.MovedPackages)
	assert.FileExists(t, filepath.Join(dir, "Dockerfile"))
	assert.Contains(t, res.Notes, "Dockerfile: its RUN steps install corona, which standalone mode and astro local check do not have. "+
		"Add it to pyproject.toml with: uv add 'corona @ git+https://github.com/example-org/corona.git@4bfeaf8'")
}

func TestRevertPutsTheStepsBack(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"removed Dockerfile": {"Dockerfile": privateInstallDockerfile},
		"kept Dockerfile":    {"Dockerfile": privateInstallDockerfile + "ENV STAGE=prod\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, files)
			unmoved, err := Plan(dir, Options{})
			require.NoError(t, err)

			_, res := func() (string, *Result) {
				res, err := Run(dir, Options{MovePipSteps: true})
				require.NoError(t, err)
				return dir, res
			}()
			require.Len(t, res.MovedPackages, 2)
			require.NoError(t, res.RevertMovedPackages("uv lock: git operation failed"))

			var want []byte
			for _, c := range unmoved.Changes {
				if c.Path == "Dockerfile" {
					want = c.Content
				}
			}
			got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got), "the Dockerfile is the one a run without the move writes")
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			assert.Equal(t, []string{"apache-airflow==3.3.*"}, m.Project.Dependencies)
			assert.Equal(t, "Dockerfile", m.Astro.Dockerfile)
			assert.Empty(t, m.Astro.BuildSecrets, "the netrc entry the move added goes with it")
			assert.Empty(t, res.MovedPackages)
			assert.Contains(t, res.Notes, "Dockerfile: its pip steps stay, because uv lock could not resolve them here (uv lock: git operation failed)")
			assert.Contains(t, res.Notes, "Dockerfile: its RUN steps install corona, which standalone mode and astro local check do not have. "+
				"Add it to pyproject.toml with: uv add 'corona @ git+https://github.com/example-org/corona.git@4bfeaf8'")
			for _, a := range res.Advisories {
				assert.NotContains(t, a, "moved corona")
			}
			for _, d := range res.Deleted {
				assert.NotContains(t, d, "Dockerfile")
			}
		})
	}
}

func TestMovablePipStepsKeepsWhatItDoesNotMove(t *testing.T) {
	own := strings.Split("\n# about the apt step\nUSER root\nRUN pip install humanize\nRUN apt-get install -y jq\nUSER astro\n", "\n")
	rest, moved, netrc := movablePipSteps(own, 1)
	assert.Equal(t, []MovedPackage{{Spec: "humanize", Line: 5}}, moved)
	assert.False(t, netrc)
	assert.Equal(t, "# about the apt step\nUSER root\nRUN apt-get install -y jq\nUSER astro\n", strings.Join(rest, "\n"),
		"a USER pair that still wraps a step stays, and so does the comment above it")
}

func TestAnAirflow2DockerfileStaysAfterItsStepsMove(t *testing.T) {
	// On Astro an Airflow 2 project deploys only from a Dockerfile of its own,
	// since a generated build is Airflow 3, so the file stays even with
	// nothing of its own left.
	dir, res := movePlan(t, map[string]string{
		"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN pip install humanize==4.11.0\n",
	})
	assert.Equal(t, []MovedPackage{{Spec: "humanize==4.11.0", Line: 2}}, res.MovedPackages)
	body, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "humanize")
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "Dockerfile", m.Astro.Dockerfile)
}

func TestRevertAddsTheIgnoreRulesTheKeptDockerfileNeeds(t *testing.T) {
	// A build of the project's own Dockerfile has the project as its context,
	// so once the file is back its .dockerignore needs the rules a run that
	// never moved the steps adds.
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{"Dockerfile": privateInstallDockerfile, ".dockerignore": ".git\n"})
	res, err := Run(dir, Options{MovePipSteps: true})
	require.NoError(t, err)
	ignore, err := os.ReadFile(filepath.Join(dir, ".dockerignore"))
	require.NoError(t, err)
	assert.Equal(t, ".git\n", string(ignore), "no Dockerfile builds from the project, so its ignore file is left alone")

	require.NoError(t, res.RevertMovedPackages("uv lock failed"))
	ignore, err = os.ReadFile(filepath.Join(dir, ".dockerignore"))
	require.NoError(t, err)
	assert.Contains(t, string(ignore), "**/.env")
	assert.Contains(t, res.Updated, ".dockerignore (added the per-machine rules)")
}
