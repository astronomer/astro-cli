package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSettings stands in for the CLI's answer: container.binary is a setting,
// remote.client_registry its 1.x name for deploy.remote_execution.client_registry,
// webserver.port a setting v2 removed, and a value equal to "default" one the
// setting shipped with.
func fakeSettings(key, value string) (SettingReplacement, bool) {
	if value == "default" {
		return SettingReplacement{}, false
	}
	switch key {
	case "container.binary", "cloud.api.token":
		return SettingReplacement{Global: key}, true
	case "remote.client_registry":
		return SettingReplacement{Global: "deploy.remote_execution.client_registry"}, true
	case "webserver.port":
		return SettingReplacement{Removed: "Pick the local Airflow port with astro local start --port <port>."}, true
	}
	return SettingReplacement{}, false
}

// convert1x converts a 1.x project whose .astro/config.yaml is body, beside a
// pin-only Dockerfile, with setting1x as Options.Setting1x.
func convert1x(t *testing.T, body string, setting1x func(key, value string) (SettingReplacement, bool)) *Result {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(config1xRelPath)), []byte(body), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})
	res, err := Run(dir, Options{Setting1x: setting1x})
	require.NoError(t, err)
	return res
}

func notesAbout(res *Result, key string) []string {
	var out []string
	for _, n := range res.Notes {
		if strings.HasPrefix(n, config1xRelPath+": "+key+" ") {
			out = append(out, n)
		}
	}
	return out
}

// Every CLI setting the file set is named, with what replaces it, and the
// file is deleted: the settings belong to the machine, and the notes are what
// is left of them.
func TestEachSettingIn1xConfigIsNamedAndTheFileGoes(t *testing.T) {
	res := convert1x(t, "project:\n  name: orders\n"+
		"container:\n  binary: podman\n"+
		"remote:\n  client_registry: quay.io/acme/client\n"+
		"webserver:\n  port: 8099\n"+
		"show_warnings: default\n"+
		"unknown:\n  thing: 1\n", fakeSettings)

	assert.Equal(t, []string{config1xRelPath + ": container.binary is a CLI setting, which Astro CLI v2 keeps per machine rather than per project. " +
		"To keep it on this machine, run: astro config set -g container.binary podman"}, notesAbout(res, "container.binary"))
	assert.Equal(t, []string{config1xRelPath + ": remote.client_registry is a CLI setting, which Astro CLI v2 keeps per machine rather than per project. " +
		"To keep it on this machine, run: astro config set -g deploy.remote_execution.client_registry quay.io/acme/client"},
		notesAbout(res, "remote.client_registry"), "named as the file spells it, set under its v2 name")
	assert.Equal(t, []string{config1xRelPath + ": webserver.port is an Astro CLI 1.x setting v2 does not have. " +
		"Pick the local Airflow port with astro local start --port <port>."}, notesAbout(res, "webserver.port"))
	assert.Empty(t, notesAbout(res, "show_warnings"), "a shipped default needs no word")
	assert.Empty(t, notesAbout(res, "unknown.thing"), "nor a key the CLI does not know")
	assert.Empty(t, notesAbout(res, "project.name"), "the project's name is carried, not a setting")

	_, err := os.Stat(filepath.Join(res.Dir, filepath.FromSlash(config1xRelPath)))
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, res.Deleted, config1xRelPath+" (what it said is in pyproject.toml or in the notes)")
	assert.Contains(t, res.Advisories, config1xDeletedAdvisory)
}

// Without a caller to say what replaces each, every key is still named.
func TestWithoutAnAnswerEverySettingIsNamed(t *testing.T) {
	res := convert1x(t, "project:\n  name: orders\ncontainer:\n  binary: podman\n", nil)
	assert.Equal(t, []string{config1xRelPath + ": container.binary was not carried: Astro CLI v2 reads no CLI setting from a project"},
		notesAbout(res, "container.binary"))
}

// A value is printed only when it is plain and not a credential: the note
// lands in a json array and in a terminal, and a token in either is a leak.
func TestASettingNoteDoesNotPrintWhatItMustNot(t *testing.T) {
	res := convert1x(t, "project:\n  name: orders\n"+
		"container:\n  binary: \"pod\\nman\"\n"+
		"cloud:\n  api:\n    token: s3cr3t-t0ken\n", fakeSettings)

	binary := notesAbout(res, "container.binary")
	require.Len(t, binary, 1)
	assert.True(t, strings.HasSuffix(binary[0], "astro config set -g container.binary <value>"), binary[0])
	token := notesAbout(res, "cloud.api.token")
	require.Len(t, token, 1)
	assert.NotContains(t, token[0], "s3cr3t-t0ken")
	for _, n := range res.Notes {
		assert.NotContains(t, n, "\n", "a note is one line")
	}
}

// A setting's value is the user's and must not decide what gets deleted, as a
// saved deploy target's must not (TestADeployTargetNamedLikeAFileStillRetiresIt).
func TestASettingNamedLikeAFileStillRetiresIt(t *testing.T) {
	res := convert1x(t, "project:\n  name: orders\ncontainer:\n  binary: Dockerfile\n", fakeSettings)
	require.Len(t, notesAbout(res, "container.binary"), 1)
	retired := false
	for _, d := range res.Deleted {
		retired = retired || strings.HasPrefix(d, "Dockerfile (")
	}
	assert.True(t, retired, "the pin reached the manifest, so the Dockerfile goes: deleted=%v", res.Deleted)
}

// A file that will not parse is kept: nothing in it was read, so nothing in it
// was carried or named.
func TestAnUnreadable1xConfigIsKept(t *testing.T) {
	res := convert1x(t, "project:\n  name: [not a string\n", fakeSettings)
	_, err := os.Stat(filepath.Join(res.Dir, filepath.FromSlash(config1xRelPath)))
	assert.NoError(t, err)
	assert.NotContains(t, res.Advisories, config1xDeletedAdvisory)
}

// A saved Deployment the note can print in full does not keep the file; one in
// an instances: entry it cannot print does.
func TestALinkTheNoteCannotPrintKeepsTheFile(t *testing.T) {
	res := convert1x(t, "project:\n  name: orders\ninstances:\n  - name: prod\n    deployment_id: cm1prod\n", nil)
	_, err := os.Stat(filepath.Join(res.Dir, filepath.FromSlash(config1xRelPath)))
	assert.ErrorIs(t, err, os.ErrNotExist, "the note carries the whole command")

	res = convert1x(t, "project:\n  name: orders\ninstances:\n  - name: \"prod west\"\n    deployment_id: cm1prod\n", nil)
	_, err = os.Stat(filepath.Join(res.Dir, filepath.FromSlash(config1xRelPath)))
	assert.NoError(t, err, "the note stands <name> in for the name, so the file is its only record")
}
