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

// apcNote is the one note that says what was kept for Astro Private Cloud, or
// "" when the run wrote none.
func apcNote(notes []string) string {
	for _, n := range notes {
		if strings.Contains(n, "kept for Astro Private Cloud") {
			return n
		}
	}
	return ""
}

// APC's `astro deploy` builds a 1.x project only: it requires
// .astro/config.yaml, which a conversion keeps, and builds the Dockerfile as it
// stands, which a conversion retires when it only pins a runtime. So a project
// converted for APC keeps what that build reads, and nothing else changes: the
// stock airflow_settings.yaml, which the build does not read, still goes.
func TestRetireKeepsTheAPCBuild(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{
		fileDockerfile:   pinOnlyDockerfile,
		fileRequirements: "pandas==2.1.0\n",
		filePackages:     "libpq-dev\n",
		SettingsRelPath:  stock1xSettings,
	})
	cs := runIn(t, dir, Options{DeploysToAPC: true})

	for _, name := range []string{fileDockerfile, fileRequirements, filePackages} {
		assert.FileExists(t, filepath.Join(dir, name), "APC's deploy builds from %s", name)
	}
	assert.Equal(t, []string{SettingsRelPath + " (nothing to carry, removed)"}, cs.Deleted)
	assert.Contains(t, cs.Notes, "Dockerfile, packages.txt and requirements.txt: kept for Astro Private Cloud "+
		"(the current context), whose `astro deploy` builds the project from its Dockerfile as it stands; its runtime "+
		"base image installs packages.txt and requirements.txt during that build. pyproject.toml carries the same "+
		"Airflow version, dependencies and OS packages for `astro local` and Astro, so change both together while the "+
		"project deploys to Astro Private Cloud, and delete them if it deploys to Astro instead")

	manifestText, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.NotContains(t, string(manifestText), "dockerfile =",
		"kept for APC, not declared: Astro and `astro local` still generate the build from the manifest")
	assert.Contains(t, string(manifestText), "'pandas==2.1.0'", "the manifest still carries the lists")

	ignore, err := os.ReadFile(filepath.Join(dir, fileDockerignore))
	require.NoError(t, err, "APC builds with the whole project as context, so per-machine files are kept out")
	assert.Contains(t, string(ignore), ".astro/standalone/")
}

// The note names what it kept and claims only what is true of it.
func TestTheAPCNoteSaysWhatEachFileWasKeptFor(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		// want are fragments the note must carry, lacks ones it must not.
		want, lacks []string
	}{
		{
			name:  "a pin-only Dockerfile alone",
			files: map[string]string{fileDockerfile: pinOnlyDockerfile},
			want:  []string{"Dockerfile: kept for Astro Private Cloud", "carries the same Airflow version for", "delete it if"},
			lacks: []string{"installs", "keeps as well"},
		},
		{
			// A -base runtime runs no ONBUILD steps, so nothing installs the
			// lists, and the note must not say anything does.
			name: "a -base runtime",
			files: map[string]string{
				fileDockerfile:   "FROM astrocrpublic.azurecr.io/runtime:3.1-12-base\n",
				fileRequirements: "pandas==2.1.0\n",
			},
			want: []string{
				"Dockerfile and requirements.txt: kept",
				"its -base runtime image runs no ONBUILD steps, so that build installs requirements.txt only if the Dockerfile does",
				"carries the same Airflow version and dependencies",
			},
			lacks: []string{"base image installs"},
		},
		{
			// The Dockerfile survives on its own note (no Astro Runtime base),
			// so the APC note names only the lists, says the Dockerfile stays
			// too, and claims no pin agreement it has not checked.
			name: "a Dockerfile kept for another reason",
			files: map[string]string{
				fileDockerfile:   "FROM python:3.12-slim\n",
				fileRequirements: "pandas==2.1.0\n",
				filePackages:     "libpq-dev\n",
			},
			want: []string{
				"packages.txt and requirements.txt: kept for Astro Private Cloud",
				"from its Dockerfile as it stands, which this run keeps as well",
				"that build installs packages.txt and requirements.txt only if the Dockerfile or its base image does",
				"carries the same dependencies and OS packages for",
			},
			lacks: []string{"Airflow version", "Dockerfile, "},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, tc.files)
			cs := runIn(t, dir, Options{DeploysToAPC: true})
			assert.Empty(t, cs.Deleted)
			note := apcNote(cs.Notes)
			require.NotEmpty(t, note, "notes: %q", cs.Notes)
			for _, w := range tc.want {
				assert.Contains(t, note, w)
			}
			for _, l := range tc.lacks {
				assert.NotContains(t, note, l)
			}
		})
	}
}

// The note names only what APC kept that would otherwise have gone. A declared
// Dockerfile keeps all three already and says so in notes of its own.
func TestRetireAPCNoteNamesOnlyWhatItKept(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{
		fileDockerfile:   pinOnlyDockerfile + "ENV FOO=bar\n",
		fileRequirements: "pandas==2.1.0\n",
	})
	cs := runIn(t, dir, Options{DeploysToAPC: true})

	assert.Empty(t, cs.Deleted)
	assert.Empty(t, apcNote(cs.Notes), "nothing was kept for APC alone")
}

// Without a Dockerfile there is no APC build to keep, so the 1.x lists retire
// as they would for Astro.
func TestRetireAPCWithoutADockerfileRetiresAsUsual(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileRequirements: "pandas==2.1.0\n"})
	cs := runIn(t, dir, Options{DeploysToAPC: true})

	assert.NoFileExists(t, filepath.Join(dir, fileRequirements))
	assert.NoFileExists(t, filepath.Join(dir, fileDockerignore))
	assert.Len(t, cs.Deleted, 1)
}

// The project's own answer outranks the context's. A saved Software release
// name (the 0.x CLI saved one there) is no Astro Deployment, so a project that
// has one keeps its APC build under an Astro context, and the note says the
// project, not the context, decided.
func TestASavedSoftwareTargetKeepsTheAPCBuildUnderAstro(t *testing.T) {
	dir := project1xWithConfig(t, "project:\n  name: orders\n  deployment: celestial-gravity-1234\n",
		map[string]string{fileRequirements: "pandas==2.1.0\n"})
	cs := runIn(t, dir, Options{DeploysToAPC: false})

	assert.FileExists(t, filepath.Join(dir, fileDockerfile))
	assert.FileExists(t, filepath.Join(dir, fileRequirements))
	assert.Contains(t, apcNote(cs.Notes), "kept for Astro Private Cloud (.astro/config.yaml saves celestial-gravity-1234 "+
		"as its deploy target, an Astro Private Cloud release name)")

	// Another value that is no Astro id is not read as APC's: it says nothing,
	// so the context decides (TestADeployTargetNamedLikeAFileStillRetiresIt).
	dir = project1xWithConfig(t, "project:\n  name: orders\n  deployment: cm1orders\n", nil)
	runIn(t, dir, Options{})
	assert.NoFileExists(t, filepath.Join(dir, fileDockerfile))
}

// A saved cuid pair is what both platforms save, so the context decides it, and
// the link and the retirement follow the same answer: under Astro the pair is a
// link and the 1.x build goes; under APC it stays a note beside the kept build,
// rather than an Astro link in a project whose files were kept for APC.
func TestTheDeployLinkAndTheAPCBuildAgree(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: " + workspaceID1x + "\n"
	for _, apc := range []bool{false, true} {
		dir := project1xWithConfig(t, config, nil)
		cs := runIn(t, dir, Options{DeploysToAPC: apc})
		m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
		require.NoError(t, err)

		_, linked := m.Astro.Deployments[Link1xName]
		_, statErr := os.Stat(filepath.Join(dir, fileDockerfile))
		kept := statErr == nil
		assert.Equal(t, !apc, linked, "APC %v: an Astro link", apc)
		assert.Equal(t, apc, kept, "APC %v: the Dockerfile kept", apc)
		assert.Equal(t, apc, apcNote(cs.Notes) != "", "APC %v: the APC note", apc)
		assert.Equal(t, apc, strings.Contains(strings.Join(cs.Notes, "\n"), "saved deploy target"),
			"APC %v: the unlinked target is left to do", apc)
	}
}

// A Dockerfile kept for APC is the build that deploys, so a pin that disagrees
// with its FROM is refused as it is for a declared one: the project would deploy
// one Airflow and run another, and the note's "carries the same Airflow
// version" would be false. Under Astro the same run converts, keeping the
// Dockerfile only as the record of the version the pin did not take.
func TestAPCRefusesAPinTheKeptDockerfileDisagreesWith(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		files        map[string]string
		opts         Options
	}{
		{
			name:   "--airflow-version",
			source: "--airflow-version 2.10",
			files:  map[string]string{fileDockerfile: pinOnlyDockerfile},
			opts:   Options{AirflowVersion: "2.10"},
		},
		{
			name:   "an adopted manifest's pin",
			source: "apache-airflow==3.3.* already in pyproject.toml",
			files: map[string]string{
				fileDockerfile:  pinOnlyDockerfile,
				manifest.Marker: "[project]\nname = 'theirs'\nversion = '1.0.0'\ndependencies = ['apache-airflow==3.3.*']\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, tc.files)
			apc := tc.opts
			apc.DeploysToAPC = true
			_, err := Plan(dir, apc)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.source+" disagrees with the Dockerfile this conversion keeps for Astro Private Cloud")
			assert.Contains(t, err.Error(), "runtime:3.1-12, which is Airflow 3.1")
			assert.Contains(t, err.Error(), "--airflow-version 3.1")

			_, err = Plan(dir, tc.opts)
			require.NoError(t, err, "under Astro nothing builds the kept Dockerfile")
		})
	}

	// The FROM's own series converts.
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: pinOnlyDockerfile})
	_, err := Plan(dir, Options{DeploysToAPC: true, AirflowVersion: "3.1"})
	require.NoError(t, err)
}
