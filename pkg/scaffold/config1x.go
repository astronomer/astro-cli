package scaffold

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// A 1.x project's .astro/config.yaml holds two kinds of thing: facts about the
// project (project.name, project.workspace, the saved deploy target), which a
// conversion carries into pyproject.toml, and CLI settings that
// `astro config set` without --global wrote there, which Astro CLI v2 keeps per
// machine and never reads from a project. A conversion carries the first,
// names each of the second with what replaces it, and then deletes the file,
// so a converted project is described by its pyproject.toml alone.

// SettingReplacement is what replaces a CLI setting a 1.x project's
// .astro/config.yaml set, as Options.Setting1x reports it.
type SettingReplacement struct {
	// Global is the setting's name in Astro CLI v2, which `astro config set --global`
	// sets for every project on the machine. Empty for a setting v2 removed.
	Global string
	// Removed is what replaces a setting v2 removed, in the words
	// `astro config` refuses it with. Read only when Global is empty.
	Removed string
}

// config1xProjectTable is the top-level key of the facts a conversion carries.
const config1xProjectTable = "project"

// setting1x is one setting .astro/config.yaml sets: its dotted key, as the
// file spells it, and its value as text. value is "" for one that is not a
// single scalar (a list, a null), which is never printed.
type setting1x struct {
	key, value string
}

// yamlSettings is every leaf the file sets outside project: and instances:,
// which the conversion reads as facts. Sorted by key, so the notes come out in
// the same order on every run.
func yamlSettings(doc map[string]any) []setting1x {
	var out []setting1x
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		if m, ok := v.(map[string]any); ok {
			for k, child := range m {
				walk(prefix+"."+k, child)
			}
			return
		}
		s := setting1x{key: prefix}
		switch v.(type) {
		case string, bool, int, int64, uint64, float64:
			s.value = fmt.Sprint(v)
		}
		out = append(out, s)
	}
	for k, v := range doc {
		switch k {
		case config1xProjectTable, "instances":
			continue
		}
		walk(k, v)
	}
	slices.SortFunc(out, func(a, b setting1x) int { return strings.Compare(a.key, b.key) })
	return out
}

// settingNotes are a note per CLI setting .astro/config.yaml set, each naming
// what replaces it. The file is deleted, so the note is what is left of the
// setting: one in Global's form ends with the command that keeps it on this
// machine.
//
// A key or value is printed only when plain, for the reason deployTargetNote
// gives: it comes out of a user's file and lands in a json array a consumer
// parses line by line. A value is never printed for a key that names a
// credential.
func (from1x *project1x) settingNotes() []string {
	_, handled := from1x.carriedDeploySettings()
	var notes []string
	for _, s := range from1x.cliSettings {
		if !plainSettingKey(s.key) || handled[s.key] {
			continue
		}
		var fate SettingReplacement
		if from1x.setting1x != nil {
			var ok bool
			if fate, ok = from1x.setting1x(s.key, s.value); !ok {
				continue
			}
		}
		head := config1xRelPath + ": " + s.key
		switch {
		case fate.Global != "":
			value := "<value>"
			if plainSettingValue(s.value) && !credentialKey(s.key) {
				value = ShellQuote(s.value)
			}
			notes = append(notes, head+" is a CLI setting; Astro CLI v2 keeps it per machine, not per project. "+
				"To keep it on this machine, run: astro config set --global "+fate.Global+" "+value)
		case fate.Removed != "":
			notes = append(notes, head+" is an Astro CLI 1.x setting v2 does not have. "+fate.Removed)
		default:
			notes = append(notes, head+" was not carried: Astro CLI v2 reads no CLI setting from a project")
		}
	}
	return notes
}

// config1xRetirable reports whether .astro/config.yaml may be deleted: it is
// there, it parsed, and everything in it that is not carried is printed in
// full by a note. That leaves out a saved deploy target, or an instances:
// entry, whose note has to stand a placeholder in for a value that is not
// plain: the file is then still the only record of it.
//
// The settings never keep it. Each is named, with its value when plain, and a
// project's CLI settings are deliberately not carried: they belong to the
// machine.
func (from1x *project1x) config1xRetirable() bool {
	if !from1x.config1xPresent || from1x.config1xUnreadable {
		return false
	}
	if _, linked := from1x.deployLink(); from1x.deployment != "" && !linked {
		if !plainDeployID(from1x.deployment) || (from1x.workspace != "" && !plainDeployID(from1x.workspace)) {
			return false
		}
	}
	for _, inst := range from1x.instances {
		if !plainDeployID(inst.name) || !plainDeployID(inst.deploymentID) || (inst.source != "" && !plainDeployID(inst.source)) {
			return false
		}
	}
	return true
}

// config1xDeletedAdvisory says what deleting .astro/config.yaml means for
// Astro CLI 1.x, which takes the file as the mark of a project.
const config1xDeletedAdvisory = config1xRelPath + ": deleted, once pyproject.toml held what it said. " +
	"Astro CLI 1.x finds a project by this file, so it no longer sees one here"

// plainSettingKey is a dotted setting name: letters, digits, _, - and dots.
func plainSettingKey(s string) bool {
	if s == "" || len(s) > 96 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// plainSettingValue is a value that prints on one line as itself: no control
// characters, no newline, and short. A registry endpoint or a URL passes; a
// multi-line string does not.
func plainSettingValue(s string) bool {
	if s == "" || len(s) > 200 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// credentialKey reports a key whose value is a credential, which a note
// never prints.
func credentialKey(key string) bool {
	k := strings.ToLower(key)
	for _, w := range []string{"token", "password", "secret", "key"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// deployAgentRegistry is [tool.astro.deploy] agent-registry, the one deploy
// setting that is a string rather than a boolean.
const deployAgentRegistry = "agent-registry"

// deploySettings1x maps each 1.x CLI setting a project decides to the
// [tool.astro.deploy] key that holds it now, under every name 1.x wrote it.
// These are carried rather than named: every checkout of the project, a
// teammate's or a CI runner's, read them from the committed file, and the
// manifest is where they are committed now.
var deploySettings1x = map[string]string{
	"deploy.git_metadata":                     "git-metadata",
	"cosmos_boost.pre_deploy":                 "cosmos-boost",
	"deploy.cosmos_boost":                     "cosmos-boost",
	"sha_as_tag":                              "sha-as-tag",
	"apc.sha_as_tag":                          "sha-as-tag",
	"remote.client_registry":                  deployAgentRegistry,
	"deploy.remote_execution.client_registry": deployAgentRegistry,
}

// carried1x is one 1.x deploy setting the conversion writes into
// [tool.astro.deploy].
type carried1x struct {
	from, key string
	value     any
}

// deploySettingsDecided is carriedDeploySettings' answer, worked out once per
// conversion: the notes, the manifest edit and the advisories all read it, so
// they cannot disagree about what was carried.
type deploySettingsDecided struct {
	carried []carried1x
	handled map[string]bool
}

// carriedDeploySettings are the deploy settings .astro/config.yaml set to
// something other than the default, as the [tool.astro.deploy] values that
// keep them. handled holds every key, by the name the file spells it, that
// needs no note: carried, at the default (where the manifest needs no line
// either), or shadowed by the spelling 1.x read instead. A value that is not
// a boolean, or a registry that does not print on one line, is none of these,
// and keeps its note.
//
// Where a file holds a setting under both its 1.x name and its v2 one, the
// 1.x name decides, as it did for Astro CLI 1.x, which read only that name.
func (from1x *project1x) carriedDeploySettings() (carried []carried1x, handled map[string]bool) {
	if d := from1x.deployDecided; d != nil {
		return d.carried, d.handled
	}
	handled = map[string]bool{}
	byKey := map[string]setting1x{}
	for _, s := range from1x.cliSettings {
		name := strings.ToLower(s.key)
		key, ok := deploySettings1x[name]
		if !ok {
			continue
		}
		if prev, seen := byKey[key]; seen {
			// Keep the spelling 1.x read; the other is shadowed.
			if deploySettingIs1xName(strings.ToLower(prev.key)) || !deploySettingIs1xName(name) {
				handled[s.key] = true
				continue
			}
			handled[prev.key] = true
		}
		byKey[key] = s
	}
	for _, s := range from1x.cliSettings {
		key := deploySettings1x[strings.ToLower(s.key)]
		if key == "" || byKey[key].key != s.key {
			continue
		}
		var value any
		switch key {
		case deployAgentRegistry:
			if !plainSettingValue(s.value) {
				continue
			}
			value = strings.TrimSpace(s.value)
		default:
			b, err := strconv.ParseBool(s.value)
			if err != nil {
				continue
			}
			if b == (key == "git-metadata") {
				handled[s.key] = true
				continue // the default
			}
			value = b
		}
		handled[s.key] = true
		carried = append(carried, carried1x{from: s.key, key: key, value: value})
	}
	from1x.deployDecided = &deploySettingsDecided{carried: carried, handled: handled}
	return carried, handled
}

// deploySettingIs1xName reports a spelling Astro CLI 1.x read: every name in
// deploySettings1x but the v2 ones, which 1.x never wrote.
func deploySettingIs1xName(name string) bool {
	switch name {
	case "deploy.cosmos_boost", "apc.sha_as_tag", "deploy.remote_execution.client_registry":
		return false
	}
	_, ok := deploySettings1x[name]
	return ok
}

// setDeploySettings writes the carried deploy settings into
// [tool.astro.deploy].
func setDeploySettings(ed tomledit.Editor, from1x *project1x) error {
	carried, _ := from1x.carriedDeploySettings()
	for _, c := range carried {
		if err := ed.Set([]string{"tool", "astro", "deploy", c.key}, c.value); err != nil {
			return err
		}
	}
	return nil
}

// deploySettingsAdvisories say what setDeploySettings carried, one line each.
func (from1x *project1x) deploySettingsAdvisories() []string {
	carried, _ := from1x.carriedDeploySettings()
	out := make([]string, 0, len(carried))
	for _, c := range carried {
		out = append(out, config1xRelPath+": carried "+c.from+" into [tool.astro.deploy] "+c.key+" in "+manifest.Marker)
	}
	return out
}
