package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// apcByContext and astroByContext are the bases `astro init` hands over when
// the current context decided.
var (
	apcByContext = DeployTargetBasis{
		Why:     "the current context is Astro Private Cloud (apc.example.com)",
		Instead: "pass --deploy-target astro",
	}
	astroByContext = DeployTargetBasis{
		Why:     "the current context is Astro (astronomer.io)",
		Instead: "pass --deploy-target apc",
	}
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
	cs := runIn(t, dir, Options{DeploysToAPC: true, DeployTargetBasis: apcByContext})

	for _, name := range []string{fileDockerfile, fileRequirements, filePackages} {
		assert.FileExists(t, filepath.Join(dir, name), "APC's deploy builds from %s", name)
	}
	assert.Equal(t, []string{SettingsRelPath + " (nothing to carry, removed)"}, cs.Deleted)
	assert.Contains(t, cs.Notes, "Dockerfile, packages.txt and requirements.txt: kept for Astro Private Cloud, "+
		"whose `astro deploy` builds the project from its Dockerfile as it stands; its runtime "+
		"base image installs packages.txt and requirements.txt during that build. pyproject.toml carries the same "+
		"Airflow version, dependencies and OS packages for `astro local` and Astro, so change both together while the "+
		"project deploys to Astro Private Cloud, and delete them if it deploys to Astro instead. This run converted "+
		"the project for Astro Private Cloud because the current context is Astro Private Cloud (apc.example.com); "+
		"to convert for Astro instead, first restore airflow_settings.yaml from version control and delete the "+
		"pyproject.toml this run created (`git checkout -- airflow_settings.yaml && rm pyproject.toml`), then convert "+
		"again and pass --deploy-target astro")

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
			// and is still APC's build, so the note names it, claims no pin
			// agreement it has not checked, and offers to delete only the
			// lists it kept.
			name: "a Dockerfile kept for another reason",
			files: map[string]string{
				fileDockerfile:   "FROM python:3.12-slim\n",
				fileRequirements: "pandas==2.1.0\n",
				filePackages:     "libpq-dev\n",
			},
			want: []string{
				"Dockerfile, packages.txt and requirements.txt: kept for Astro Private Cloud",
				"that build installs packages.txt and requirements.txt only if the Dockerfile or its base image does",
				"carries the same dependencies and OS packages for",
				"delete packages.txt and requirements.txt if",
			},
			lacks: []string{"Airflow version"},
		},
		{
			// Kept for its own note alone, it is still named.
			name:  "only a Dockerfile kept for another reason",
			files: map[string]string{fileDockerfile: "FROM python:3.12-slim\n"},
			want:  []string{"Dockerfile: kept for Astro Private Cloud"},
			lacks: []string{"carries the same", "delete"},
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

// The caller decides, and nothing in the project overrides it. A saved target
// shaped like a Software release name decided APC once, but an Astro
// Deployment's namespace has the same shape and Houston takes custom release
// names, so the shape misled both ways. Under Astro it is now converted as any
// other target that is no Astro id; under APC it is APC's. Either way the note
// on it agrees with the platform, and says what decided it and how to choose
// the other.
func TestASavedReleaseNameDoesNotDecide(t *testing.T) {
	const config = "project:\n  name: orders\n  deployment: celestial-gravity-1234\n"
	extra := map[string]string{fileRequirements: "pandas==2.1.0\n"}

	dir := project1xWithConfig(t, config, extra)
	cs := runIn(t, dir, Options{DeploysToAPC: false, DeployTargetBasis: astroByContext})
	assert.NoFileExists(t, filepath.Join(dir, fileDockerfile), "Astro builds from the manifest")
	assert.NoFileExists(t, filepath.Join(dir, fileRequirements))
	assert.Empty(t, apcNote(cs.Notes))
	// TestTheOtherPlatformAdviceSaysWhatToRestore has the rest of the sentence.
	assert.Contains(t, strings.Join(cs.Notes, "\n"), ".astro/config.yaml: celestial-gravity-1234 is this project's "+
		"saved deploy target. If that is an Astro Deployment, give it a name under [tool.astro.deployments], say "+
		"[tool.astro.deployments.prod], with deployment = 'celestial-gravity-1234' and the workspace it lives in. "+
		"This run converted the project for Astro because the current context is Astro (astronomer.io); "+
		"to convert for Astro Private Cloud instead, first restore")

	dir = project1xWithConfig(t, config, extra)
	cs = runIn(t, dir, Options{DeploysToAPC: true, DeployTargetBasis: apcByContext})
	assert.FileExists(t, filepath.Join(dir, fileDockerfile))
	assert.FileExists(t, filepath.Join(dir, fileRequirements))
	assert.NotEmpty(t, apcNote(cs.Notes))
	joined := strings.Join(cs.Notes, "\n")
	assert.NotContains(t, joined, "give it a name under [tool.astro.deployments]",
		"no advice to make an Astro link beside a build kept for APC")
	assert.Contains(t, cs.Notes, ".astro/config.yaml: celestial-gravity-1234 is this project's saved deploy target, "+
		"which Astro Private Cloud's `astro deploy` reads from this file, so it stays here rather than becoming a "+
		"[tool.astro.deployments] link. This run converted the project for Astro Private Cloud because the current "+
		"context is Astro Private Cloud (apc.example.com); to convert for Astro instead, first delete the "+
		"pyproject.toml this run created (`rm pyproject.toml`), then convert again and pass --deploy-target astro")
}

// One fact says a Dockerfile will be built, and every decision that turns on
// it asks the same helper: the lists it keeps, the .dockerignore for its
// context, and whether the pin is held to its FROM.
func TestBuildsDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dockerfile string // "" is none, unless empty
		empty      bool
		astro, apc bool
	}{
		{name: "no Dockerfile"},
		{name: "an empty Dockerfile", empty: true},
		{name: "a pin-only Dockerfile", dockerfile: pinOnlyDockerfile, apc: true},
		{name: "a declared Dockerfile", dockerfile: pinOnlyDockerfile + "ENV FOO=bar\n", astro: true, apc: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.dockerfile != "" || tc.empty {
				writeAll(t, dir, map[string]string{fileDockerfile: tc.dockerfile})
			}
			from1x, err := read1xProject(dir)
			require.NoError(t, err)
			assert.Equal(t, tc.astro, buildsDockerfile(from1x, false), "under Astro")
			assert.Equal(t, tc.apc, buildsDockerfile(from1x, true), "under APC")

			// The decisions agree with it.
			for _, apc := range []bool{false, true} {
				run := t.TempDir()
				files := map[string]string{fileRequirements: "pandas==2.1.0\n"}
				if tc.dockerfile != "" || tc.empty {
					files[fileDockerfile] = tc.dockerfile
				}
				writeAll(t, run, files)
				runIn(t, run, Options{DeploysToAPC: apc})
				_, keptErr := os.Stat(filepath.Join(run, fileRequirements))
				_, ignoreErr := os.Stat(filepath.Join(run, fileDockerignore))
				want := buildsDockerfile(from1x, apc)
				// A list can be kept for a reason of its own (a note naming it),
				// so only a build is required to keep it.
				if want {
					assert.NoError(t, keptErr, "APC %v: requirements.txt kept", apc)
				}
				assert.Equal(t, want, ignoreErr == nil, "APC %v: .dockerignore written", apc)
			}
		})
	}
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
			apc.DeployTargetBasis = DeployTargetBasis{Why: "of --deploy-target apc", Instead: "pass --deploy-target astro"}
			_, err := Plan(dir, apc)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.source+" disagrees with the Dockerfile this conversion keeps for Astro Private Cloud")
			assert.Contains(t, err.Error(), "runtime:3.1-12, which is Airflow 3.1")
			assert.Contains(t, err.Error(), "--airflow-version 3.1")
			assert.True(t, strings.HasSuffix(err.Error(), ". This run converted the project for Astro Private Cloud "+
				"because of --deploy-target apc; to convert for Astro instead, pass --deploy-target astro"),
				"names what decided and how to choose otherwise: %v", err)

			_, err = Plan(dir, tc.opts)
			require.NoError(t, err, "under Astro nothing builds the kept Dockerfile")
		})
	}

	// The FROM's own series converts, as does one release of it.
	for _, v := range []string{"3.1", "3.1.2"} {
		dir := t.TempDir()
		writeAll(t, dir, map[string]string{fileDockerfile: pinOnlyDockerfile})
		_, err := Plan(dir, Options{DeploysToAPC: true, AirflowVersion: v})
		require.NoError(t, err, v)
	}
}

// APC deploys the one series its FROM carries, so a pin naming only the
// generation is broader than the build: "3" resolves to the newest 3.x under
// `astro local` while APC deploys 3.1. It is refused under APC, from the flag
// and from an adopted manifest alike, with the exact series as the fix. Under
// Astro the Dockerfile is not what deploys, and the same run converts.
func TestAPCRefusesAGenerationPinBroaderThanTheBuild(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		files        map[string]string
		opts         Options
	}{
		{
			name:   "--airflow-version",
			source: "--airflow-version 3",
			files:  map[string]string{fileDockerfile: pinOnlyDockerfile},
			opts:   Options{AirflowVersion: "3"},
		},
		{
			name:   "an adopted manifest's pin",
			source: "apache-airflow==3.* already in pyproject.toml",
			files: map[string]string{
				fileDockerfile:  pinOnlyDockerfile,
				manifest.Marker: "[project]\nname = 'theirs'\nversion = '1.0.0'\ndependencies = ['apache-airflow==3.*']\n",
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
			assert.Contains(t, err.Error(), "Convert with --airflow-version 3.1,")

			_, err = Plan(dir, tc.opts)
			require.NoError(t, err)
		})
	}
}

// airflow2Catalog says runtime 12.1.0 carries Airflow 2.10.3.
func airflow2Catalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersions": {"12.1.0": {"metadata": {"airflowVersion": "2.10.3", "channel": "stable"}}}}`))
	require.NoError(t, err)
	return c
}

const airflow2Dockerfile = "FROM quay.io/astronomer/astro-runtime:12.1.0\n"

// An Airflow 2 FROM names a runtime version, not an Airflow one, so on its own
// it says only "2", which `astro local` resolves to the newest Airflow 2. APC
// deploys the one that runtime carries, so under APC the catalog the caller
// read says which, and its series is the pin a conversion reads and the fix a
// refusal names. Under Astro nothing deploys that Dockerfile, and the pin
// stays "2" whatever catalog is given.
func TestAPCPinsTheSeriesAnAirflow2BuildCarries(t *testing.T) {
	catalog := airflow2Catalog(t)

	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: airflow2Dockerfile})
	cs := runIn(t, dir, Options{DeploysToAPC: true, RuntimeCatalog: catalog})
	assert.Equal(t, "2.10", cs.AirflowVersion)
	assert.FileExists(t, filepath.Join(dir, fileDockerfile), "spent at the series it carries, and kept for APC")
	for _, n := range cs.Notes {
		assert.NotContains(t, n, "the newest Airflow 2", "the series is known: %q", n)
	}

	for _, v := range []string{"2", "2.9", "3.1"} {
		dir = t.TempDir()
		writeAll(t, dir, map[string]string{fileDockerfile: airflow2Dockerfile})
		_, err := Plan(dir, Options{DeploysToAPC: true, RuntimeCatalog: catalog, AirflowVersion: v})
		require.Error(t, err, v)
		assert.Contains(t, err.Error(), "runtime:12.1.0, which is Airflow 2.10,", v)
		assert.Contains(t, err.Error(), "Convert with --airflow-version 2.10,", v)
	}

	dir = t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: airflow2Dockerfile})
	cs = runIn(t, dir, Options{RuntimeCatalog: catalog})
	assert.Equal(t, "2", cs.AirflowVersion, "under Astro")
}

// Without the series an Airflow 2 runtime carries, the conversion says so
// rather than name a pin it cannot vouch for, and says which reason it was:
// no catalog to ask, or a catalog that does not list the runtime. The note
// names no particular pin, since the pin may be a flag's or a manifest's
// rather than the "2" the tag reads as, and a refusal of another generation
// names no major-only fix.
func TestAPCSaysWhenAnAirflow2SeriesIsUnknown(t *testing.T) {
	unlisted, err := runtimeversions.Parse([]byte(`{"runtimeVersions": {"13.0.0": {"metadata": {"airflowVersion": "2.11.0", "channel": "stable"}}}}`))
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		catalog *runtimeversions.Catalog
		why     string
	}{
		{"no catalog", nil, "the runtime catalog, which says which, could not be read"},
		{"a catalog without that runtime", unlisted, "the runtime catalog does not list runtime 12.1.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, pin := range []string{"", "2.10.3"} {
				dir := t.TempDir()
				writeAll(t, dir, map[string]string{fileDockerfile: airflow2Dockerfile})
				cs := runIn(t, dir, Options{DeploysToAPC: true, RuntimeCatalog: tc.catalog, AirflowVersion: pin})
				joined := strings.Join(cs.Notes, "\n")
				assert.Contains(t, joined, "Dockerfile: runtime 12.1.0 is an Airflow 2 image whose tag does not name the "+
					"Airflow minor, and "+tc.why+", so the Airflow requirement in pyproject.toml was not checked against it")
				assert.NotContains(t, joined, `a pin of "2"`, "pin %q", pin)
				assert.NotContains(t, joined, "meaning the newest Airflow 2. Set it explicitly")
				assert.NotContains(t, apcNote(cs.Notes), "carries the same Airflow version",
					"a pin nobody could check is not claimed to agree, pin %q", pin)
			}

			dir := t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: airflow2Dockerfile})
			_, err := Plan(dir, Options{DeploysToAPC: true, RuntimeCatalog: tc.catalog, AirflowVersion: "3.1"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "which is an Airflow 2 runtime whose tag does not name the Airflow series")
			assert.Contains(t, err.Error(), "Convert with --airflow-version set to the Airflow 2 series runtime 12.1.0 "+
				"carries ("+tc.why+")")
			assert.NotContains(t, err.Error(), "--airflow-version 2,")
		})
	}
}

// The floating Airflow 3 tags name their series as the pinned ones do, so
// under APC they pin and refuse at that series, and the kept Dockerfile is in
// the note claiming the pin agrees where the tag names a series. Read as the old runtime-version format,
// "3.1" was an Airflow 2 image: kept beside a "2" pin, with a note saying the
// two agreed.
func TestAPCReadsFloatingAirflow3Tags(t *testing.T) {
	for _, tc := range []struct{ tag, series string }{
		{"3.1", "3.1"},
		{"3.1-python-3.12", "3.1"},
		{"3", "3"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			dockerfile := "FROM astrocrpublic.azurecr.io/runtime:" + tc.tag + "\n"
			dir := t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
			cs := runIn(t, dir, Options{DeploysToAPC: true})
			assert.Equal(t, tc.series, cs.AirflowVersion)
			assert.Contains(t, apcNote(cs.Notes), "Dockerfile: kept for Astro Private Cloud")
			if tc.series == "3" {
				// A pin naming only the generation, beside a tag naming
				// only the generation, is not the same Airflow: each
				// follows the newest 3.x on its own.
				assert.NotContains(t, apcNote(cs.Notes), "carries the same Airflow version")
				assert.Contains(t, apcNote(cs.Notes), "Delete Dockerfile if the project deploys to Astro instead")
			} else {
				assert.Contains(t, apcNote(cs.Notes), "carries the same Airflow version")
			}

			dir = t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
			_, err := Plan(dir, Options{DeploysToAPC: true, AirflowVersion: "2.10"})
			require.Error(t, err)
			if tc.series == "3" {
				assert.Contains(t, err.Error(), "which is a floating Airflow 3 tag that names no series")
				assert.Contains(t, err.Error(), "Convert with --airflow-version 3,")
			} else {
				assert.Contains(t, err.Error(), "which is Airflow "+tc.series)
				assert.Contains(t, err.Error(), "Convert with --airflow-version "+tc.series+",")
			}
		})
	}
}

// An astro-runtime tag names a runtime version whatever its number, so a
// legacy "3.0" there is Astro Runtime 3, an Airflow 2 image, under APC as
// anywhere: a 3.x pin against it is refused, not read as agreeing.
func TestAPCReadsALegacyAstroRuntime3TagAsAirflow2(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: "FROM quay.io/astronomer/astro-runtime:3.0\n"})
	_, err := Plan(dir, Options{DeploysToAPC: true, AirflowVersion: "3.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "which is a floating Airflow 2 tag")
}

// The repository decides that a FROM is a runtime image, on any registry, so a
// private mirror, common under APC, is checked like the image it mirrors.
func TestAPCChecksAMirroredRuntime(t *testing.T) {
	const dockerfile = "FROM artifactory.corp/astronomer/runtime:3.1-12\n"
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
	_, err := Plan(dir, Options{DeploysToAPC: true, AirflowVersion: "3.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifactory.corp/astronomer/runtime:3.1-12, which is Airflow 3.1")

	dir = t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
	cs := runIn(t, dir, Options{DeploysToAPC: true})
	assert.Equal(t, "3.1", cs.AirflowVersion)
	assert.Contains(t, apcNote(cs.Notes), "carries the same Airflow version")
}

// A floating Airflow 2 tag names whatever runtime the registry serves under
// it when APC builds, so the catalog is not asked and not blamed: the
// conversion says the tag floats and how to pin it.
func TestAPCSaysAFloatingAirflow2TagCannotBeVouchedFor(t *testing.T) {
	catalog := airflow2Catalog(t)
	for _, tag := range []string{"12", "12.1"} {
		t.Run(tag, func(t *testing.T) {
			dockerfile := "FROM quay.io/astronomer/astro-runtime:" + tag + "\n"
			dir := t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
			assert.False(t, NeedsRuntimeCatalog(dir))
			cs := runIn(t, dir, Options{DeploysToAPC: true, RuntimeCatalog: catalog})
			assert.Equal(t, "2", cs.AirflowVersion)
			joined := strings.Join(cs.Notes, "\n")
			assert.Contains(t, joined, "Dockerfile: runtime "+tag+" is an Airflow 2 image whose tag floats")
			assert.Contains(t, joined, "Pin the FROM line to a full runtime version")
			assert.NotContains(t, joined, "runtime catalog")
			assert.NotContains(t, apcNote(cs.Notes), "carries the same Airflow version")

			dir = t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: dockerfile})
			_, err := Plan(dir, Options{DeploysToAPC: true, RuntimeCatalog: catalog, AirflowVersion: "3.1"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "which is a floating Airflow 2 tag")
			assert.Contains(t, err.Error(), "Pin the FROM line to a full runtime version")
			assert.NotContains(t, err.Error(), "runtime catalog")
		})
	}
}

// NeedsRuntimeCatalog says, offline, whether a conversion for APC would ask
// the catalog: only for a pin-only Dockerfile naming one Airflow 2 runtime
// build.
func TestNeedsRuntimeCatalog(t *testing.T) {
	for _, tc := range []struct {
		name, dockerfile string
		want             bool
	}{
		{"no Dockerfile", "", false},
		{"an Airflow 3 tag", pinOnlyDockerfile, false},
		{"a floating Airflow 2 tag", "FROM quay.io/astronomer/astro-runtime:12\n", false},
		{"a pinned Airflow 2 tag", airflow2Dockerfile, true},
		{"a mirrored pinned Airflow 2 tag", "FROM registry.corp/astronomer/astro-runtime:12.1.0-python-3.11\n", true},
		{"a declared Dockerfile", airflow2Dockerfile + "RUN echo hi\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.dockerfile != "" {
				writeAll(t, dir, map[string]string{fileDockerfile: tc.dockerfile})
			}
			assert.Equal(t, tc.want, NeedsRuntimeCatalog(dir))
		})
	}
}

// A pin-only Dockerfile naming a Python carries it as requires-python when
// it retires, so the Python the project ran is not dropped.
func TestARetiredPinOnlyDockerfileCarriesItsPython(t *testing.T) {
	for _, tag := range []string{"3.1-python-3.12", "3.1-12-python-3.12"} {
		t.Run(tag, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, map[string]string{fileDockerfile: "FROM astrocrpublic.azurecr.io/runtime:" + tag + "\n"})
			runIn(t, dir, Options{})
			assert.NoFileExists(t, filepath.Join(dir, fileDockerfile))
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			assert.Equal(t, "==3.12.*", m.Project.RequiresPython)
		})
	}
}

// Every Dockerfile a conversion for APC keeps is named in the APC note, since
// it is the build that deploys: one the pin agrees with but did not take, a
// release of its series, included.
func TestAPCNamesAKeptDockerfileThePinDidNotTake(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{fileDockerfile: pinOnlyDockerfile})
	cs := runIn(t, dir, Options{DeploysToAPC: true, AirflowVersion: "3.1.2"})
	assert.FileExists(t, filepath.Join(dir, fileDockerfile))
	assert.Contains(t, apcNote(cs.Notes), "Dockerfile: kept for Astro Private Cloud")
}

// A conversion's advice on converting for the other platform is read after
// the run, when converting again is refused because pyproject.toml now
// carries [tool.astro]. So it says what to put back first: the files this run
// removed, and pyproject.toml, restored if it was there before and deleted if
// this run created it. A refusal writes nothing, so its advice needs neither.
func TestTheOtherPlatformAdviceSaysWhatToRestore(t *testing.T) {
	const config = "project:\n  name: orders\n  deployment: celestial-gravity-1234\n"
	extra := map[string]string{fileRequirements: "pandas==2.1.0\n"}

	dir := project1xWithConfig(t, config, extra)
	cs := runIn(t, dir, Options{DeployTargetBasis: astroByContext})
	assert.Contains(t, strings.Join(cs.Notes, "\n"), "to convert for Astro Private Cloud instead, first restore "+
		"Dockerfile and requirements.txt from version control and delete the pyproject.toml this run created "+
		"(`git checkout -- Dockerfile requirements.txt && rm pyproject.toml`), then convert again and pass --deploy-target apc")

	extra[manifest.Marker] = "[project]\nname = 'orders'\nversion = '1.0.0'\ndependencies = []\n"
	dir = project1xWithConfig(t, config, extra)
	cs = runIn(t, dir, Options{DeployTargetBasis: astroByContext})
	assert.Contains(t, strings.Join(cs.Notes, "\n"), "first restore Dockerfile, pyproject.toml and requirements.txt "+
		"from version control (`git checkout -- Dockerfile requirements.txt pyproject.toml`), then convert again")

	dir = project1xWithConfig(t, config, nil)
	cs = runIn(t, dir, Options{DeploysToAPC: true, DeployTargetBasis: apcByContext})
	assert.Contains(t, apcNote(cs.Notes), "to convert for Astro instead, first delete the pyproject.toml this run "+
		"created (`rm pyproject.toml`), then convert again and pass --deploy-target astro")
}
