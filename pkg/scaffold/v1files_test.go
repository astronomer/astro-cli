package scaffold

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// requirements.txt is a pip input format, not a list of PEP 508 requirements.
// Everything that is a requirement is carried verbatim; everything that is a
// pip instruction is reported and left alone. Carrying a pip option would
// produce a manifest that either fails to parse or resolves differently than
// the project does today, and the second is the one nobody notices.
func TestParseRequirements(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        string
		wantDeps  []string
		wantNotes []string // substrings, one per expected note
	}{
		{
			name:     "plain requirements are carried verbatim",
			in:       "pandas==2.1.0\nrequests>=2.28\nboto3\n",
			wantDeps: []string{"pandas==2.1.0", "requests>=2.28", "boto3"},
		},
		{
			name:     "comments and blank lines are dropped",
			in:       "# pinned for the connector\npandas==2.1.0\n\n   \nrequests  # keep in sync\n",
			wantDeps: []string{"pandas==2.1.0", "requests"},
		},
		{
			name: "a fragment in a URL is not a comment",
			in:   "pkg @ git+https://example.com/p.git@v1#egg=pkg\n",
			// pip's rule, and the reason stripComment looks at what precedes
			// the '#': a fragment carries the egg name, and cutting there would
			// silently change the requirement.
			wantDeps: []string{"pkg @ git+https://example.com/p.git@v1#egg=pkg"},
		},
		{
			name:     "a continuation is one requirement",
			in:       "pandas[performance] \\\n  ==2.1.0\n",
			wantDeps: []string{"pandas[performance]   ==2.1.0"},
		},
		{
			name:     "extras and markers are PEP 508 and survive",
			in:       "celery[redis]==5.3.0\nbackports.zoneinfo;python_version<\"3.9\"\n",
			wantDeps: []string{"celery[redis]==5.3.0", "backports.zoneinfo;python_version<\"3.9\""},
		},
		{
			name:      "an include is not read",
			in:        "-r base.txt\npandas\n",
			wantDeps:  []string{"pandas"},
			wantNotes: []string{"includes another requirements file"},
		},
		{
			name:      "a constraints file cannot be expressed",
			in:        "-c constraints.txt\n",
			wantNotes: []string{"names a constraints file"},
		},
		{
			name:      "an editable install goes to tool.uv.sources",
			in:        "-e ./libs/shared\n",
			wantNotes: []string{"is an editable install"},
		},
		{
			name:      "an index is pip configuration, not a dependency",
			in:        "--extra-index-url https://pypi.example.com/simple\npandas\n",
			wantDeps:  []string{"pandas"},
			wantNotes: []string{"names a package index"},
		},
		{
			name:      "a hash belongs in the lock file",
			in:        "--hash=sha256:abc123\n",
			wantNotes: []string{"pins a hash"},
		},
		{
			name:      "an unrecognized option is reported rather than guessed at",
			in:        "--no-binary :all:\n",
			wantNotes: []string{"is a pip option, not a requirement"},
		},
		{
			name: "a VCS URL carrying a ref is still bare",
			in:   "git+https://github.com/org/repo.git@main\n",
			// The most common VCS form there is. An earlier isBareURL
			// short-circuited on any '@', reading it as the PEP 508 "name @ url"
			// separator — but here it is the git ref, and the line went into the
			// manifest as an unparseable requirement with no note.
			wantNotes: []string{"is a bare URL"},
		},
		{
			name:      "a local path cannot be a dependency either",
			in:        "./libs/shared\n",
			wantNotes: []string{"is a local path"},
		},
		{
			name:      "a bare dot is the same thing",
			in:        ".\n",
			wantNotes: []string{"is a local path"},
		},
		{
			name: "a dangling continuation is reported, not emitted",
			in:   "pandas \\  \n  ==2.1.0\n",
			// Trailing whitespace after the backslash means pip does not treat
			// it as a continuation. Stripping comments before joining turned
			// this into a dependency whose text ended in a backslash plus a
			// second, nameless "==2.1.0".
			wantDeps:  []string{"==2.1.0"},
			wantNotes: []string{"ends in a line continuation"},
		},
		{
			name: "a comment swallows a continuation, as pip's order implies",
			in:   "pandas  # note \\\n  ==2.1.0\n",
			// Surprising, and correct. Joining happens first, so the logical
			// line is "pandas  # note   ==2.1.0" and stripping the comment
			// takes the version with it. pip does exactly this. Pinned because
			// the other order silently produced a DIFFERENT wrong answer — a
			// dependency ending in a backslash plus a nameless "==2.1.0" — and
			// it would be easy to "fix" this back.
			wantDeps: []string{"pandas"},
		},
		{
			name: "a bare URL has no name to declare it under",
			in:   "git+https://example.com/p.git\n",
			// No '@' in the line, so there is no distribution name to use, and
			// inventing one would be a guess about what the package is called.
			wantNotes: []string{"is a bare URL"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, notes := parseRequirements([]byte(tc.in))
			assert.Equal(t, tc.wantDeps, deps)
			require.Len(t, notes, len(tc.wantNotes), "notes: %v", notes)
			for i, want := range tc.wantNotes {
				assert.Contains(t, notes[i], want)
			}
		})
	}
}

func TestParsePackages(t *testing.T) {
	got := parsePackages([]byte("# needed by psycopg2\nlibpq-dev\n\ngcc  # for the C extension\n"))
	assert.Equal(t, []string{"libpq-dev", "gcc"}, got)
	assert.Nil(t, parsePackages([]byte("# only a comment\n")), "an all-comment file declares no packages, not one empty one")
}

// TestRuntimeTagFormats pins the two tag formats this package recognizes.
//
// runtimeTagRe and oldRuntimeTagRe are a deliberate second copy of what
// airflow_versions decides, which cannot be imported across the sub-module
// boundary (see their doc comments). A copy that drifts from the original is
// worse than no copy: it would read a real project's Airflow version wrongly
// and pin the conversion to the wrong generation. This is the drift alarm.
//
// The cases are written as literals rather than derived from the regexes, so
// they can actually fail.
func TestRuntimeTagFormats(t *testing.T) {
	// New format, and a suffix after the build number is part of it:
	// "3.1-12.rc1" and "3.1-12-base" are real published shapes.
	for _, tag := range []string{"3.0-1", "3.1-12", "3.1-12.rc1", "3.1-12-base", "12.0-4"} {
		assert.True(t, runtimeTagRe.MatchString(tag), "%q is the Airflow 3 tag format", tag)
	}
	// Old format: one to three segments, optionally with a flavor suffix.
	// airflow_versions accepts anything semver accepts, and Go's semver takes
	// "v9" and "v9.1" as readily as "v9.1.0" and reads a suffix as a
	// prerelease. Every case this copy used to reject was a real Airflow 2
	// project silently defaulted to Airflow 3.
	for _, tag := range []string{"9", "9.1", "9.1.0", "12.1.0", "9.1.0-base", "9.1.0-python-3.10", "9.1.0-1"} {
		assert.True(t, oldRuntimeTagRe.MatchString(tag), "%q is the pre-Airflow-3 tag format", tag)
	}
	for _, tag := range []string{"latest", "", "slim-3.1-12"} {
		assert.False(t, runtimeTagRe.MatchString(tag), "%q is not the new format", tag)
		assert.False(t, oldRuntimeTagRe.MatchString(tag), "%q is not the old format either", tag)
	}

	// The two patterns OVERLAP, and the order of the check is the contract.
	//
	// An earlier version of this test asserted they were disjoint. They are not,
	// once the old format admits a suffix: "3.1-12" reads as new format, and
	// also as old format ("3.1" plus the suffix "-12"). Upstream has exactly the
	// same overlap and resolves it the same way, by testing isNewFormat first,
	// so airflowFromDockerfile's switch order mirrors airflow_versions rather
	// than being an accident of how these were written.
	assert.True(t, runtimeTagRe.MatchString("3.1-12"))
	assert.True(t, oldRuntimeTagRe.MatchString("3.1-12"), "overlapping is expected; precedence is what decides")
	got, _, _ := airflowFromDockerfile([]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"))
	assert.Equal(t, "3.1", got, "the new format must win the overlap")
}

func TestAirflowFromDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name        string
		in          string
		wantVersion string
		wantStated  bool
		wantNote    string
	}{
		{
			name:        "an Airflow 3 tag names the version outright",
			in:          "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantVersion: "3.1",
			wantStated:  true,
		},
		{
			name:        "a registry port is not the tag separator",
			in:          "FROM localhost:5000/astronomer/astro-runtime:3.0-1\n",
			wantVersion: "3.0",
			wantStated:  true,
		},
		{
			name:        "a lowercase from still parses",
			in:          "# syntax=docker/dockerfile:1\nfrom quay.io/astronomer/astro-runtime:3.1-12\n",
			wantVersion: "3.1",
			wantStated:  true,
		},
		{
			name: "an old tag names Airflow 2 but not which one",
			in:   "FROM quay.io/astronomer/astro-runtime:12.1.0\n",
			// "2" is a legal pin meaning the newest Airflow 2, which is exactly
			// what is known. Guessing a minor would be inventing a fact.
			wantVersion: "2",
			wantStated:  true,
			wantNote:    "does not name the Airflow minor",
		},
		{
			name:       "a tag that is not a runtime version is reported",
			in:         "FROM quay.io/astronomer/astro-runtime:latest\n",
			wantStated: true,
			wantNote:   "is not an Astro Runtime version",
		},
		{
			name: "an untagged runtime image states nothing",
			in:   "FROM astrocrpublic.azurecr.io/runtime\n",
			// Reached only because the image IS a runtime image; a tagless
			// python base is rejected earlier, by the image check.
			wantNote: "carries no tag",
		},
		{
			name:     "a digest pins no readable tag",
			in:       "FROM astrocrpublic.azurecr.io/runtime@sha256:abc123\n",
			wantNote: "carries no tag",
		},
		{
			name:     "a Dockerfile with no FROM states nothing",
			in:       "# a comment and nothing else\n",
			wantNote: "no FROM instruction",
		},
		{
			// The worst of the bugs this rewrite fixes. apache/airflow is the
			// ordinary OSS Airflow Dockerfile and exactly the repo shape adopt
			// exists for; reading the tag without checking the image made this
			// Airflow "2", pinning a 3.0.1 project back a whole generation.
			name:     "an Airflow image that is not Astro Runtime is not read",
			in:       "FROM apache/airflow:3.0.1\n",
			wantNote: "no stage builds on an Astro Runtime image",
		},
		{
			name:     "an unrelated base image is not read as Airflow 2",
			in:       "FROM ubuntu:22.04\n",
			wantNote: "no stage builds on an Astro Runtime image",
		},
		{
			// The standard Apple-silicon workaround. A \S+ capture took the
			// flag as the image, so the version went unread AND stated stayed
			// false, which suppressed the defaulted-pin warning as well.
			name:        "a platform flag is not the image",
			in:          "FROM --platform=linux/amd64 quay.io/astronomer/astro-runtime:9.1.0\n",
			wantVersion: "2",
			wantStated:  true,
			wantNote:    "does not name the Airflow minor",
		},
		{
			name:        "a flavored old tag is still an Airflow 2 image",
			in:          "FROM quay.io/astronomer/astro-runtime:9.1.0-base\n",
			wantVersion: "2",
			wantStated:  true,
			wantNote:    "does not name the Airflow minor",
		},
		{
			name:        "a v-prefixed tag is read",
			in:          "FROM quay.io/astronomer/astro-runtime:v9.1.0\n",
			wantVersion: "2",
			wantStated:  true,
			wantNote:    "does not name the Airflow minor",
		},
		{
			// Multi-stage ordinarily starts with a builder, so taking the first
			// FROM read the wrong stage. With a numeric builder tag it read a
			// confident wrong answer rather than none.
			name:        "the last runtime stage wins, not the first FROM",
			in:          "FROM python:3.11-slim AS builder\nRUN pip install x\n\nFROM astrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantVersion: "3.1",
			wantStated:  true,
		},
		{
			name:        "a numeric builder tag does not win either",
			in:          "FROM alpine:3.19 AS build\n\nFROM astrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantVersion: "3.1",
			wantStated:  true,
		},
		{
			name:        "a stage alias is not part of the image",
			in:          "FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS astro\n",
			wantVersion: "3.1",
			wantStated:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version, stated, notes := airflowFromDockerfile([]byte(tc.in))
			assert.Equal(t, tc.wantVersion, version)
			assert.Equal(t, tc.wantStated, stated)
			if tc.wantNote == "" {
				assert.Empty(t, notes)
				return
			}
			require.Len(t, notes, 1, "notes: %v", notes)
			assert.Contains(t, notes[0], tc.wantNote)
		})
	}
}

// A directory with none of the v1 files is the greenfield case and must read
// cleanly rather than as an error.
func TestReadV1ProjectWithNothingThere(t *testing.T) {
	v1, err := readV1Project(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, &v1Project{}, v1)
}

// A file that exists and cannot be read is an error, not an empty file.
//
// The failure mode this prevents is quiet: an unreadable requirements.txt read
// as empty converts the project to a manifest that declares no dependencies,
// which installs nothing and fails later at import time with a traceback that
// says nothing about the conversion.
func TestReadV1ProjectFailsOnAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root is not denied by permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "requirements.txt")
	require.NoError(t, os.WriteFile(path, []byte("pandas\n"), 0o600))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := readV1Project(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requirements.txt")
}

// The greenfield conversion: a v1 project with no pyproject.toml at all, which
// is what a real Astro v1 project looks like.
func TestPlanConvertsAV1ProjectWithNoManifest(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("Dockerfile", "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n")
	write("requirements.txt", "# our stack\npandas==2.1.0\nsnowflake-connector-python==3.6.0\n-r dev.txt\n")
	write("packages.txt", "libpq-dev\n")

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "3.1", m.Astro.AirflowVersion, "the runtime tag names Airflow 3.1")
	assert.Equal(t, []string{
		"apache-airflow==3.1.*",
		"pandas==2.1.0",
		"snowflake-connector-python==3.6.0",
	}, m.Project.Dependencies)
	assert.Equal(t, []string{"libpq-dev"}, m.Astro.Packages)

	// The include could not be followed, so it is reported rather than dropped.
	assert.Contains(t, strings.Join(cs.Notes, "\n"), "includes another requirements file")

	// And the v1 files are still there. A conversion the user has not reviewed
	// has to be ignorable, which it is not if the inputs are gone.
	for _, name := range []string{"Dockerfile", "requirements.txt", "packages.txt"} {
		assert.FileExists(t, filepath.Join(dir, name))
	}
}

// Adopting a manifest that already declares dependencies must not restate or
// re-pin them: the author wrote those specifiers, and requirements.txt is the
// file being retired.
func TestAdoptMergesRequirementsWithoutTouchingExistingPins(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(
		"[project]\nname = 'mine'\nversion = '0.1.0'\ndependencies = ['pandas==1.5.0', 'python-dateutil']\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(
		"pandas==2.1.0\nPython_DateUtil==2.8.2\nboto3\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, []string{
		// Untouched, and still pinned the way its author pinned it, even
		// though requirements.txt names a newer one.
		"pandas==1.5.0",
		// Untouched, and matched across a different spelling: PEP 503 folds
		// case and separators, so "Python_DateUtil" is the same distribution as
		// "python-dateutil" and is not declared a second time.
		"python-dateutil",
		// The one name the manifest did not have, appended.
		"boto3",
		// The Airflow requirement lands last here: the manifest had none, and
		// ensureAirflowDependency appends after the merge.
		"apache-airflow==3.1.*",
	}, m.Project.Dependencies)

	assert.Equal(t, distName("python-dateutil"), distName("Python_DateUtil==2.8.2"),
		"the normalization the dedup relies on")
	assert.Contains(t, strings.Join(res.Updated, "\n"), "migrated 1 from requirements.txt")
}

// packages.txt is carried into a manifest that has another tool's packages key,
// which is a different key entirely.
//
// This test used to be called TestAdoptLeavesAnExistingPackagesListAlone and
// asserted, under that name, that the list IS carried. The name described a path
// no input can reach: a [tool.astro].packages key needs [tool.astro], and a
// manifest carrying that table is refused with ErrAlreadyAstroProject before any
// of this runs. The guard it was named for was dead code and is gone; what is
// left worth pinning is that a neighboring tool's key is not mistaken for ours.
func TestAdoptCarriesPackagesAlongsideAnotherToolsKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(
		"[project]\nname = 'mine'\nversion = '0.1.0'\ndependencies = []\n\n[tool.other]\npackages = ['x']\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "packages.txt"), []byte("libpq-dev\n"), 0o600))

	_, err := Run(dir, Options{})
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"libpq-dev"}, m.Astro.Packages)

	// And a manifest that already declares [tool.astro] is refused outright,
	// which is why the reconciliation case cannot arise.
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "pyproject.toml"), []byte(
		"[project]\nname = 'mine'\nversion = '0.1.0'\ndependencies = []\n\n[tool.astro]\nairflow = '3.1'\npackages = ['x']\n"), 0o600))
	_, err = Run(dir2, Options{})
	assert.ErrorIs(t, err, ErrAlreadyAstroProject)
}

// The greenfield arm deduplicates by distribution name, the way adopt does.
//
// Only adopt guarded against this, and greenfield is the arm a real v1 project
// takes. A requirements.txt naming one distribution twice produced two entries
// for it; manifest.Parse accepts that, then uv intersects the specifiers and the
// environment is unsatisfiable at the first start.
func TestGreenfieldDeduplicatesRequirements(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(
		"pandas==1.5.0\npandas==2.1.0\nFlask\nflask\n"), 0o600))

	_, err := Run(dir, Options{})
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	// First spelling wins, and "Flask"/"flask" are one PEP 503 name.
	assert.Equal(t, []string{"apache-airflow==3.1.*", "pandas==1.5.0", "Flask"}, m.Project.Dependencies)
}

// Extras on a dropped apache-airflow requirement are real dependencies and are
// reported rather than lost.
//
// Both arms replace a v1 apache-airflow entry with the requirement the pin
// generates, on the grounds that they say the same thing. That holds only when
// the entry carries no extras: apache-airflow[celery,statsd] also names two
// installed distributions, and apache-airflow==2.9.* does not.
func TestAirflowExtrasAreReportedWhenDropped(t *testing.T) {
	for _, tc := range []struct{ name, manifest string }{
		{"greenfield", ""},
		{"adopt", "[project]\nname = 'mine'\nversion = '0.1.0'\ndependencies = []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"),
				[]byte("apache-airflow[celery,statsd]==2.9.1\n"), 0o600))
			if tc.manifest != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(tc.manifest), 0o600))
			}

			res, err := Run(dir, Options{})
			require.NoError(t, err)
			assert.Equal(t, "2.9.1", res.AirflowVersion, "the pin still comes from that requirement")
			assert.Contains(t, strings.Join(res.Notes, "\n"), "declares extras [celery,statsd]")
		})
	}
}

// A Dockerfile that does more than name a base image is still reported, even
// when its tag parses cleanly.
//
// Removing the unconditional "Dockerfile: not read" entry from leftovers —
// correct, since the tag IS read now — meant a Dockerfile with real build steps
// went unmentioned entirely, which is a worse answer than the one it replaced.
func TestDockerfileBuildStepsAreReported(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"+
			"RUN apt-get install -y unixodbc-dev\n"+
			"COPY certs/ /usr/local/share/ca-certificates/\n"+
			"ENV PYTHONPATH=/usr/local/airflow\n"), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, "3.1", res.AirflowVersion, "the tag is still read")
	joined := strings.Join(res.Notes, "\n")
	assert.Contains(t, joined, "RUN, COPY, ENV")
	// The instructions are named; their arguments are the user's own.
	assert.NotContains(t, joined, "unixodbc-dev")

	// A Dockerfile that only names a base image has nothing to report.
	plain := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(plain, "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"), 0o600))
	res, err = Run(plain, Options{})
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(res.Notes, "\n"), "instructions were not read")
}

// The greenfield conversion says what it absorbed.
//
// A real v1 project has no pyproject.toml, so it takes this arm, where Plan
// hardcodes the manifest's label to the filename: `astro init` printed
// "pyproject.toml" and never mentioned that the requirements and packages had
// just been moved into it. The rarer adopt arm did say so.
func TestGreenfieldReportsWhatItMigrated(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("pandas==2.1.0\nboto3\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "packages.txt"), []byte("libpq-dev\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	joined := strings.Join(res.Created, "\n")
	assert.Contains(t, joined, "migrated 2 from requirements.txt")
	assert.Contains(t, joined, "migrated packages.txt")
	assert.Contains(t, joined, "read airflow = 3.1 from the Dockerfile")
}

// The caller's version wins over the Dockerfile, which is what lets Plan stay
// offline: an Airflow 2 tag names no minor, so a caller that wants the exact
// one resolves it through the release index and passes it here.
func TestOptionsAirflowVersionBeatsTheDockerfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(
		"FROM quay.io/astronomer/astro-runtime:12.1.0\n"), 0o600))

	res, err := Run(dir, Options{AirflowVersion: "2.10.5"})
	require.NoError(t, err)
	assert.Equal(t, "2.10.5", res.AirflowVersion)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "2.10.5", m.Astro.AirflowVersion)
	assert.Equal(t, []string{"apache-airflow==2.10.5"}, m.Project.Dependencies)
}
