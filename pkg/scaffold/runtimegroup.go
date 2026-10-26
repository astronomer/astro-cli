package scaffold

import (
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// RuntimeGroup is what SetAstroBuild writes into the project's astro-runtime
// dependency group (manifest.RuntimeGroupName): the top-level packages the
// image of one Astro Runtime build installs, which the CLI's
// runtimecatalog.RuntimeRoots reads. With the group in [tool.uv]
// default-groups, standalone mode installs them, and with them everything the
// image has that they pull in, at the versions the lock is seeded with.
type RuntimeGroup struct {
	// Names are the image's top-level packages, normalized. None means there
	// are none, and the group is emptied.
	Names []string
}

var (
	runtimeGroupKey  = []string{"dependency-groups", manifest.RuntimeGroupName}
	defaultGroupsKey = []string{"tool", "uv", "default-groups"}
)

// uvDefaultGroups is what uv installs when [tool.uv] default-groups is not
// set.
var uvDefaultGroups = []any{"dev"}

// setRuntimeGroup writes g as the astro-runtime group, less the packages the
// project lists itself (listedPackages), which choose their own version, and
// Airflow's, which it lists already, with the seed record first: seed's when
// seed is not nil (none for a zero Seed), else the one there. nil g is not
// known: the group stays, less what the project now lists.
//
// The group is never taken out. One with nothing left in it stays, empty, under
// comments that say so (manifest.RuntimeGroupEmptyComments): a project that
// took it out of default-groups keeps that choice when the group fills again.
// A group is created with packages in it, and then joins default-groups so uv
// installs it, or with a record alone; one the project had stays as
// default-groups has it. refused reports a group not written because its
// table cannot hold the comments that head it (an inline [dependency-groups]).
func setRuntimeGroup(ed tomledit.Editor, g *RuntimeGroup, seed *Seed) (refused bool, err error) {
	current, exists := tomledit.GetLines(ed, runtimeGroupKey)
	listed := listedPackages(ed)
	var entries []string
	if g == nil {
		for _, v := range current {
			if s, isString := v.(string); isString && !listed[manifest.DistName(s)] {
				entries = append(entries, s)
			}
		}
	} else {
		entries = groupEntries(g.Names, listed)
	}
	var record []any
	switch {
	case seed == nil:
		record = seedRecordIn(current)
	case seed.Build != "":
		record = []any{tomledit.Comment(manifest.SeedRecord(seed.Build, seed.Python))}
	}
	if !exists && len(entries) == 0 && len(record) == 0 {
		return false, nil
	}
	out := slices.Clone(record)
	if len(entries) > 0 {
		out = append(out, tomledit.Comment(manifest.RuntimeGroupComment))
		out = append(out, asAny(entries)...)
	} else {
		out = append(out, emptyGroup()...)
	}
	if exists && reflect.DeepEqual(out, current) {
		return false, nil
	}
	if err := ed.Set(runtimeGroupKey, out); err != nil {
		if errors.Is(err, tomledit.ErrCommentsUnsupported) {
			return true, nil
		}
		return false, err
	}
	if !exists && len(entries) > 0 {
		// Only now the group is there: a refused one leaves no dev group behind.
		if err := ensureDevGroup(ed); err != nil {
			return false, err
		}
		return false, addDefaultGroup(ed)
	}
	return false, nil
}

// groupEntries are names as the group lists them: normalized, a marker kept,
// sorted by name, less what the project lists and Airflow.
func groupEntries(names []string, listed map[string]bool) []string {
	var entries []string
	seen := map[string]bool{}
	for _, n := range names {
		name := manifest.DistName(n)
		if listed[name] || slices.Contains(astroDists, name) || seen[name] {
			continue
		}
		seen[name] = true
		// A marker limits the package to the platforms it installs on.
		if _, marker, ok := strings.Cut(n, ";"); ok {
			name += "; " + strings.TrimSpace(marker)
		}
		entries = append(entries, name)
	}
	slices.SortFunc(entries, func(a, b string) int { return strings.Compare(manifest.DistName(a), manifest.DistName(b)) })
	return entries
}

// emptyGroup is the group with nothing in it: its comments alone.
func emptyGroup() []any {
	var out []any
	for _, c := range manifest.RuntimeGroupEmptyComments() {
		out = append(out, tomledit.Comment(c))
	}
	return out
}

// devGroupKey is the group `uv add --dev` writes, uv's own default.
var devGroupKey = []string{"dependency-groups", "dev"}

// ensureDevGroup gives a project whose default-groups addDefaultGroup will
// write, from uv's own default, a dev group when it has none. uv installs dev
// by default without one, but refuses a default-groups that names a group the
// project does not define; and leaving dev out of the list would stop it
// installing the group `uv add --dev` writes later.
func ensureDevGroup(ed tomledit.Editor) error {
	if _, set := ed.Get(defaultGroupsKey); set {
		return nil
	}
	if _, ok := ed.Get(devGroupKey); ok {
		return nil
	}
	return ed.Set(devGroupKey, []any{})
}

// addDefaultGroup puts the astro-runtime group in [tool.uv] default-groups:
// after uv's own default when the key is not set, at the end of the list when
// it is. "all" installs every group already.
func addDefaultGroup(ed tomledit.Editor) error {
	current, ok := ed.Get(defaultGroupsKey)
	if !ok {
		return ed.Set(defaultGroupsKey, slices.Concat(uvDefaultGroups, []any{manifest.RuntimeGroupName}))
	}
	list, isList := current.([]any)
	if !isList || slices.Contains(list, any(manifest.RuntimeGroupName)) {
		return nil
	}
	return ed.Set(append(slices.Clone(defaultGroupsKey), strconv.Itoa(len(list))), manifest.RuntimeGroupName)
}
