package config

import (
	"slices"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// 1.x coexistence.
//
// Astro CLI 1.x and v2 share ~/.astro/config.yaml, and v2 still reads a 1.x
// project's .astro/config.yaml. Everything in this file exists so the two can
// run side by side, and goes when that support ends:
//
//   - a renamed setting is read under its 1.x name, and written under both
//     names while the file holds the 1.x one (names, keyIn, set);
//   - the one-time cleanup of the defaults older CLIs wrote into the home
//     config (migrateHomeConfig), with the version stamp saveConfig puts on
//     a new file.
//
// What stays after that is the tombstones in removed.go: `astro config`
// refuses a 1.x name, removed or renamed, the way a removed 1.x flag is
// refused, naming what replaces it.

// names is every name c is set under, the 1.x ones first: while a file holds
// a 1.x name, that is the one both CLIs read.
func (c cfg) names() []string {
	return append(slices.Clone(c.Legacy), c.Path)
}

// keyIn is the name v sets c under: a 1.x name v sets, else its path, else
// "" when v does not set it (or there is no v).
//
// The 1.x name first, because 1.x reads nothing else: a value 1.x saved
// after v2 last wrote the setting is in the 1.x name only, and reading the
// current name first would ignore it for good.
func (c cfg) keyIn(v *viper.Viper) string {
	if v == nil {
		return ""
	}
	for _, name := range c.names() {
		if v.IsSet(name) {
			return name
		}
	}
	return ""
}

// projectKey is keyIn for the 1.x project config, "" when there is none.
func (c cfg) projectKey() string {
	if !configExists(viperProject) {
		return ""
	}
	return c.keyIn(viperProject)
}

// set stores value as c in v under its current name, and under each 1.x name
// v already holds, so that 1.x, which reads only those, sees the change too.
// A 1.x name the file does not hold is not added: nothing in it needs one.
func (c cfg) set(v *viper.Viper, value any) {
	for _, name := range c.Legacy {
		if v.IsSet(name) {
			v.Set(name, value)
		}
	}
	v.Set(c.Path, value)
}

const (
	// configVersionKey records which one-time cleanups the home config has
	// been through. Astro CLI 1.x reads and writes it too, and the two share
	// one sequence: 1 is 1.x's cleanup (astronomer/astro-cli#2255, in
	// v1.46.0), 2 is this one. Each runs only against a file below its own
	// number, so whichever line adds the next cleanup takes 3.
	configVersionKey     = "config_version"
	currentConfigVersion = 2
)

// migrateHomeConfig runs once against a home config written before either CLI
// stopped writing its defaults into the file — every 1.x before v1.46.0 and
// every v2 before this — which put some 45 of them there on the first save.
// Those entries cannot be told from a deliberate choice, so they freeze the
// default of the day the file was written: no later change to a default
// reaches the user, and in 1.x, a postgres.tag frozen at 12.6 kept new
// projects off Postgres 15.
//
// Removing an entry that still holds a shipped default changes nothing about
// what either CLI does today, since the same value comes back as the default,
// and lets the setting follow the default again. It covers 1.x's settings as
// well as v2's: once this stamps the file, 1.x's own cleanup (version 1)
// never runs against it. Anything set from then on is the user's for good.
//
// It works on the file as it is on disk, under the lock, rather than on what
// this process read at startup, so a save another process made in between is
// kept. A cleanup that cannot be written is not worth failing the command
// for: it is tried again next time.
func migrateHomeConfig() {
	if viperHome.GetInt(configVersionKey) >= currentConfigVersion {
		return
	}
	unlock, err := lockConfig(HomeConfigFile)
	if err != nil {
		return
	}
	defer unlock()

	disk, ok := readHomeFromDisk()
	if !ok || disk.GetInt(configVersionKey) >= currentConfigVersion {
		return
	}
	settings := disk.AllSettings()
	for path, values := range defaultsShipped() {
		if disk.IsSet(path) && slices.Contains(values, disk.GetString(path)) {
			deleteNested(settings, path)
		}
	}
	settings[configVersionKey] = currentConfigVersion
	data, err := yaml.Marshal(settings)
	if err != nil {
		return
	}
	if err := writeConfigFile(configFs, HomeConfigFile, data); err != nil {
		return
	}
	// Hold what is on disk now, as a process starting after the cleanup would.
	_ = readConfigFile(viperHome, configFs) //nolint:errcheck // the file was just written from a parse of itself
}

// defaultsShipped is every value an older CLI wrote into the home config
// without being asked, by setting name: each removed setting's defaults, and
// each registered setting's Default under its current name and every legacy
// one, as an older CLI wrote it under the name it had then. An empty default
// was never written, so it is not one.
func defaultsShipped() map[string][]string {
	all := map[string][]string{}
	for name, s := range removedSettings {
		if len(s.defaults) > 0 {
			all[name] = slices.Clone(s.defaults)
		}
	}
	for _, cfg := range CFGStrMap {
		if cfg.Default == "" {
			continue
		}
		for _, name := range cfg.names() {
			all[name] = append(slices.Clone(all[name]), cfg.Default)
		}
	}
	return all
}

// deleteNested removes a dotted key from a settings map, along with any parent
// left empty by the removal.
func deleteNested(settings map[string]any, path string) {
	first, rest, nested := strings.Cut(path, ".")
	if !nested {
		delete(settings, first)
		return
	}
	child, ok := settings[first].(map[string]any)
	if !ok {
		return
	}
	deleteNested(child, rest)
	if len(child) == 0 {
		delete(settings, first)
	}
}
