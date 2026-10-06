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

// convertWithConfig writes body as .astro/config.yaml beside a pin-only
// Dockerfile, converts, and returns every note naming that file.
//
// Every note, not the last one that matched. A malformed config emits its own
// note about the same file, so a helper that overwrites would let that one
// stand in for the deploy-target note and report a pass for the wrong reason.
func convertWithConfig(t *testing.T, body string) (res *Result, about []string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)), []byte(body), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(config1xRelPath)))
	require.NoError(t, statErr, "the config is never retired, whatever it names")

	for _, n := range res.Notes {
		if strings.Contains(n, "config.yaml") {
			about = append(about, n)
		}
	}
	return res, about
}

// The .astro/config.yaml note is about project.deployment, so it is reported on
// that key and not on the file.
//
// The file is in every 1.x project, because `astro dev init` writes it, while
// project.deployment is written only by `astro deploy --save` and that flag
// defaults to false. Keying the note on the file therefore told nearly every
// conversion to go and move a Deployment its config did not name.
func TestConfigNoteFollowsTheDeploymentNotTheFile(t *testing.T) {
	t.Run("no deploy target, no note", func(t *testing.T) {
		res, about := convertWithConfig(t, "project:\n  name: orders\n")
		assert.Empty(t, about,
			"a config that names no deploy target has nothing to hand over, got: %v", res.Notes)
	})

	t.Run("a saved deploy target is reported, and named", func(t *testing.T) {
		_, about := convertWithConfig(t, "project:\n  name: orders\n  deployment: cm1orders\n")
		require.Len(t, about, 1)
		assert.Contains(t, about[0], "cm1orders",
			"the note names the target, so nobody has to reopen the file to find it")
	})

	t.Run("a workspace beside it is carried into the note", func(t *testing.T) {
		_, about := convertWithConfig(t,
			"project:\n  name: orders\n  deployment: cm1orders\n  workspace: cm1ws\n")
		require.Len(t, about, 1)
		assert.Contains(t, about[0], "workspace = 'cm1ws'",
			"both halves of a link are here, so the note states the whole entry")
	})

	t.Run("an empty key is not a deploy target", func(t *testing.T) {
		res, about := convertWithConfig(t, "project:\n  name: orders\n  deployment: \"\"\n")
		assert.Empty(t, about, "the key is present and says nothing, got: %v", res.Notes)
	})
}

// A deploy target this cannot print does not cost the project its name.
//
// The decode reads project.deployment loosely for exactly this: typing the
// field as a string makes a mapping under that key fail the whole file, and the
// error path drops the name yaml had already decoded. The project would be
// renamed after its directory over a key the conversion does not need.
func TestAMalformedDeployTargetDoesNotCostTheName(t *testing.T) {
	res, about := convertWithConfig(t, "project:\n  name: orders\n  deployment:\n    id: cm1\n")
	assert.Equal(t, "orders", res.Name, "the name decoded fine and must survive")
	assert.Empty(t, about, "a mapping is not a deploy target, got: %v", res.Notes)
}

// A value that is not a plain id is still reported, without being printed.
//
// renderLeftToDo prints one note per line and the desktop groups the json notes
// array by the file each note leads with, so a value carrying a newline would
// put an unindented orphan line under "Left to do:" and a multi-line string
// into that array.
func TestAnUnprintableDeployTargetIsReportedWithoutItsValue(t *testing.T) {
	_, about := convertWithConfig(t, "project:\n  name: orders\n  deployment: \"cm1\\nsecond line\"\n")
	require.Len(t, about, 1)
	assert.NotContains(t, about[0], "second line", "the value is not printed")
	assert.NotContains(t, about[0], "\n", "a note is one line")
	assert.Contains(t, about[0], "project.deployment", "and the key is still named, so it can be found")
}

// A saved deploy target must not decide what gets deleted.
//
// planRetirements skips a file any note mentions, by substring. The deployment
// note is the only entry in this list built from a value out of a user's file,
// so it is withheld from that decision: otherwise a target called "Dockerfile"
// keeps this project's Dockerfile after the manifest has taken its pin, leaving
// a project carrying both and a rerun that refuses.
func TestADeployTargetNamedLikeAFileStillRetiresIt(t *testing.T) {
	res, about := convertWithConfig(t, "project:\n  name: orders\n  deployment: Dockerfile\n")
	require.Len(t, about, 1)
	var retired bool
	for _, d := range res.Deleted {
		if strings.HasPrefix(d, "Dockerfile (") {
			retired = true
		}
	}
	assert.True(t, retired,
		"the pin reached the manifest, so the Dockerfile goes: deleted=%v notes=%v", res.Deleted, res.Notes)
}

// Following the note produces a manifest that parses.
//
// The wording this replaced did not. "Add it under [tool.astro.deployments]"
// written out literally is a bare key of that table, and every entry there is a
// named sub-table, so Parse rejects it with CodeExpectedTable. A hand-off that
// produces an unparseable manifest when followed is worse than no hand-off.
func TestTheDeployTargetNoteDescribesAManifestThatParses(t *testing.T) {
	const head = "[project]\nname = 'orders'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n\n"
	named := head + "[tool.astro.deployments.prod]\ndeployment = 'cm1orders'\nworkspace = 'cm1ws'\n"
	bare := head + "[tool.astro.deployments]\ndeployment = 'cm1orders'\nworkspace = 'cm1ws'\n"

	m, err := manifest.Parse([]byte(named))
	require.NoError(t, err, "the shape the note describes")
	assert.Contains(t, m.Astro.Deployments, "prod")

	_, err = manifest.Parse([]byte(bare))
	require.Error(t, err, "the shape the old wording described")

	note := deployTargetNote("cm1orders", "cm1ws")
	assert.Contains(t, note, "[tool.astro.deployments.prod]",
		"the note has to name the table, not just the section it sits in")
	assert.Contains(t, note, "deployment = 'cm1orders'")
}

// The instances list an early v2 build kept in .astro/config.yaml is named,
// with the command that links each entry, and not converted.
func TestConfigInstancesAreReported(t *testing.T) {
	res, about := convertWithConfig(t, "project:\n  name: orders\n"+
		"instances:\n"+
		"  - name: prod\n    source: astro\n    deployment_id: cm1prod\n    url: https://x.astronomer.run/abc\n"+
		"    auth:\n      context: astronomer.io\n      kind: astro\n"+
		"  - name: dev\n    source: astro\n    deployment_id: cm1dev\n"+
		"  - name: not a plain name\n    source: astro\n    deployment_id: cm1odd\n"+
		"  - name: orders\n    source: mwaa\n")
	require.Len(t, about, 1, "notes: %v", res.Notes)
	assert.Equal(t, ".astro/config.yaml: its `instances` list names deployment links this run did not carry into pyproject.toml. "+
		"Link each one with `astro link add prod --deployment cm1prod`, `astro link add dev --deployment cm1dev`, "+
		"`astro link add <name> --deployment cm1odd`, `astro link add orders --target mwaa`", about[0])
	m, err := manifest.Load(filepath.Join(res.Dir, manifest.Marker))
	require.NoError(t, err)
	assert.Empty(t, m.Astro.Deployments, "the links are reported, not converted")
}

// The build that wrote `instances:` kept each deployment id under `auth:`, and
// the command has to print that id rather than a placeholder. An entry with no
// id anywhere keeps the placeholder, and a top-level id wins over one in auth.
func TestConfigInstancesReadTheIDUnderAuth(t *testing.T) {
	res, about := convertWithConfig(t, "project:\n  name: example-project\n"+
		"instances:\n"+
		"  - auth:\n      context: astronomer.io\n      deployment_id: cexampledeployment0000001\n      kind: astro_pat\n"+
		"    name: example-dev\n    source: astro\n    url: https://cexampledeployment0000001.wl.astronomer.run/dy1rw0wl\n"+
		"  - auth:\n      kind: astro_pat\n    name: example\n    source: astro\n"+
		"  - auth:\n      deployment_id: cm1auth\n    deployment_id: cm1top\n    name: both\n    source: astro\n")
	require.Len(t, about, 1, "notes: %v", res.Notes)
	assert.Equal(t, ".astro/config.yaml: its `instances` list names deployment links this run did not carry into pyproject.toml. "+
		"Link each one with `astro link add example-dev --deployment cexampledeployment0000001`, "+
		"`astro link add example --deployment <id>`, `astro link add both --deployment cm1top`", about[0])
}
