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
	return convertWithConfigOpts(t, body, &Options{})
}

func convertWithConfigOpts(t *testing.T, body string, opts *Options) (res *Result, about []string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)), []byte(body), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	res, err := Run(dir, *opts)
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
		assert.Contains(t, about[0], "--deployment cm1orders --workspace cm1ws",
			"both halves of a link are here, so the command carries both")
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
// An earlier wording described an entry to write by hand, and "add it under
// [tool.astro.deployments]" written out literally is a bare key of that table,
// which Parse rejects with CodeExpectedTable. The note now names the commands
// instead, leading with what was not linked; what they write (SaveLink, then
// SetDefaultLink, as astro link add and astro link default do) parses as the
// link the note names.
func TestTheDeployTargetNoteNamesCommandsThatLinkIt(t *testing.T) {
	note := deployTargetNote("cm1orders", "cm1ws", "")
	assert.Equal(t, "saved deploy target cm1orders was not linked. If it is an Astro Deployment, link it with: "+
		"astro link add prod --deployment cm1orders --workspace cm1ws, then: astro link default prod", note)
	assert.NotContains(t, note, "[tool.astro")

	res, _ := convertWithConfig(t, "project:\n  name: orders\n")
	require.NoError(t, SaveLink(res.Dir, nil, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "cm1orders", Workspace: "cm1ws"}))
	require.NoError(t, SetDefaultLink(res.Dir, nil, "prod"))
	m, err := manifest.Load(filepath.Join(res.Dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "cm1orders", m.Astro.Deployments["prod"].Deployment)
	assert.True(t, m.Astro.Deployments["prod"].Default)
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
	// One note per link, so each command is a line to copy and a json
	// consumer still groups every one under the file.
	assert.Equal(t, []string{
		".astro/config.yaml: link the Deployment it lists with: astro link add prod --deployment cm1prod --workspace <workspace-id>",
		".astro/config.yaml: link the Deployment it lists with: astro link add dev --deployment cm1dev --workspace <workspace-id>",
		".astro/config.yaml: link the Deployment it lists with: astro link add <name> --deployment cm1odd --workspace <workspace-id>",
		".astro/config.yaml: link the Deployment it lists with: astro link add orders --target mwaa",
	}, about, "notes: %v", res.Notes)
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
		"    name: example-dev\n    source: astro\n    url: https://cexampledeployment0000001.astronomer.run/t0000001\n"+
		"  - auth:\n      kind: astro_pat\n    name: example\n    source: astro\n"+
		"  - auth:\n      deployment_id: cm1auth\n    deployment_id: cm1top\n    name: both\n    source: astro\n")
	assert.Equal(t, []string{
		".astro/config.yaml: link the Deployment it lists with: astro link add example-dev --deployment cexampledeployment0000001 --workspace <workspace-id>",
		".astro/config.yaml: link the Deployment it lists with: astro link add example --deployment <id> --workspace <workspace-id>",
		".astro/config.yaml: link the Deployment it lists with: astro link add both --deployment cm1top --workspace <workspace-id>",
	}, about, "notes: %v", res.Notes)
}

// With InstanceWorkspace, each Astro entry of the instances list with a plain
// name and a Deployment id whose workspace it tells becomes a link, as astro
// link add writes it; the rest stay notes with their commands.
func TestConfigInstancesAreLinkedWhenAsked(t *testing.T) {
	res, about := convertWithConfigOpts(t, "project:\n  name: orders\n"+
		"instances:\n"+
		"  - name: prod\n    source: astro\n    deployment_id: cmbjjd11x0bre01k5sy1rw0wl\n"+
		"  - name: dev\n    deployment_id: cmqzyvtpc0tpn01o1e67v6dcb\n"+
		"  - name: not a plain name\n    source: astro\n    deployment_id: cm0000000000000000000odd1\n"+
		"  - name: short\n    source: astro\n    deployment_id: cm1short\n"+
		"  - name: orders\n    source: mwaa\n",
		&Options{InstanceWorkspace: func(id string) (string, string) {
			if id == "cm1short" {
				t.Errorf("asked about an id that is not a Deployment id: %s", id)
			}
			return map[string]string{
				"cmbjjd11x0bre01k5sy1rw0wl": "cmws00000000000000000prod",
				"cmqzyvtpc0tpn01o1e67v6dcb": "cmws000000000000000000dev",
			}[id], ""
		}})
	const badID = "  # its name or Deployment id is not one a link can take"
	assert.Equal(t, []string{
		".astro/config.yaml: link the Deployment it lists with: astro link add <name> --deployment cm0000000000000000000odd1 --workspace <workspace-id>" + badID,
		".astro/config.yaml: link the Deployment it lists with: astro link add short --deployment cm1short --workspace <workspace-id>" + badID,
		".astro/config.yaml: link the Deployment it lists with: astro link add orders --target mwaa  # not an Astro Deployment",
	}, about, "notes: %v", res.Notes)
	assert.Contains(t, res.Advisories, "pyproject.toml: linked Deployments dev and prod (from .astro/config.yaml)")
	require.Len(t, res.UnlinkedDeployments, 3)
	assert.Equal(t, UnlinkedDeployment{Name: "orders", Command: "astro link add orders --target mwaa", Reason: "not an Astro Deployment"},
		res.UnlinkedDeployments[2])
	m, err := manifest.Load(filepath.Join(res.Dir, manifest.Marker))
	require.NoError(t, err)
	require.Len(t, m.Astro.Deployments, 2)
	assert.Equal(t, "cmbjjd11x0bre01k5sy1rw0wl", m.Astro.Deployments["prod"].Deployment)
	assert.Equal(t, "cmqzyvtpc0tpn01o1e67v6dcb", m.Astro.Deployments["dev"].Deployment)
	assert.Equal(t, manifest.KindAstro, m.Astro.Deployments["dev"].Kind())
	assert.Equal(t, "cmws000000000000000000dev", m.Astro.Deployments["dev"].Workspace)
	// As two runs of astro link add leave them: the first link was the only
	// one, so it is the default.
	assert.True(t, m.Astro.Deployments["prod"].Default)
	assert.False(t, m.Astro.Deployments["dev"].Default)
}

// Under an Astro Private Cloud context the entries stay notes: an apc link
// needs a workspace the list does not hold.
func TestConfigInstancesStayNotesUnderAPC(t *testing.T) {
	res, about := convertWithConfigOpts(t, "project:\n  name: orders\n"+
		"instances:\n  - name: prod\n    source: astro\n    deployment_id: cmbjjd11x0bre01k5sy1rw0wl\n",
		&Options{InstanceWorkspace: func(string) (string, string) { return "cmws00000000000000000prod", "" }, APCDomain: "apc.example.com"})
	assert.Len(t, about, 1, "notes: %v", res.Notes)
	assert.Contains(t, about[0], "  # your current context is Astro Private Cloud")
	m, err := manifest.Load(filepath.Join(res.Dir, manifest.Marker))
	require.NoError(t, err)
	assert.Empty(t, m.Astro.Deployments)
}

// A Deployment the lookup cannot place stays a note, and the rest are linked.
func TestConfigInstanceTheLookupCannotPlaceStaysANote(t *testing.T) {
	res, about := convertWithConfigOpts(t, "project:\n  name: orders\n"+
		"instances:\n"+
		"  - name: prod\n    source: astro\n    deployment_id: cmbjjd11x0bre01k5sy1rw0wl\n"+
		"  - name: gone\n    source: astro\n    deployment_id: cmqzyvtpc0tpn01o1e67v6dcb\n",
		&Options{InstanceWorkspace: func(id string) (string, string) {
			if id == "cmbjjd11x0bre01k5sy1rw0wl" {
				return "cmws00000000000000000prod", ""
			}
			return "", "not found in your organization"
		}})
	assert.Equal(t, []string{
		".astro/config.yaml: link the Deployment it lists with: astro link add gone --deployment cmqzyvtpc0tpn01o1e67v6dcb --workspace <workspace-id>  # not found in your organization",
	}, about, "notes: %v", res.Notes)
	assert.Contains(t, res.Advisories, "pyproject.toml: linked Deployment prod (from .astro/config.yaml)")
	want := UnlinkedDeployment{
		Name:       "gone",
		Deployment: "cmqzyvtpc0tpn01o1e67v6dcb",
		Command:    "astro link add gone --deployment cmqzyvtpc0tpn01o1e67v6dcb --workspace <workspace-id>",
		Reason:     "not found in your organization",
	}
	assert.Equal(t, []UnlinkedDeployment{want}, res.UnlinkedDeployments)
}

// A name another link already takes stays a note that says so.
func TestConfigInstanceWhoseNameIsTakenSaysSo(t *testing.T) {
	res, about := convertWithConfigOpts(t, "project:\n  name: orders\n"+
		"instances:\n"+
		"  - name: prod\n    source: astro\n    deployment_id: cmbjjd11x0bre01k5sy1rw0wl\n"+
		"  - name: prod\n    source: astro\n    deployment_id: cmqzyvtpc0tpn01o1e67v6dcb\n",
		&Options{InstanceWorkspace: func(string) (string, string) { return "cmws00000000000000000prod", "" }})
	assert.Equal(t, []string{
		".astro/config.yaml: link the Deployment it lists with: astro link add prod --deployment cmqzyvtpc0tpn01o1e67v6dcb --workspace <workspace-id>  # a link named prod already exists",
	}, about, "notes: %v", res.Notes)
}
