package scaffold

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// A 1.x project's .astro/config.yaml holds two kinds of thing: facts about the
// project (project.name, project.workspace, the saved deploy target), which a
// conversion carries into pyproject.toml, and CLI settings that
// `astro config set` without -g wrote there, which Astro CLI v2 keeps per
// machine and never reads from a project. A conversion carries the first,
// names each of the second with what replaces it, and then deletes the file,
// so a converted project is described by its pyproject.toml alone.

// SettingReplacement is what replaces a CLI setting a 1.x project's
// .astro/config.yaml set, as Options.Setting1x reports it.
type SettingReplacement struct {
	// Global is the setting's name in Astro CLI v2, which `astro config set -g`
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
	var notes []string
	for _, s := range from1x.cliSettings {
		if !plainSettingKey(s.key) {
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
			notes = append(notes, head+" is a CLI setting, which Astro CLI v2 keeps per machine rather than per project. "+
				"To keep it on this machine, run: astro config set -g "+fate.Global+" "+value)
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
