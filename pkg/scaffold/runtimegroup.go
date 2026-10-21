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
// image has that they pull in, at the versions the runtime pins hold.
type RuntimeGroup struct {
	// Build is the image's: "3.3-9".
	Build string
	// Names are the image's top-level packages, normalized. None means there
	// are none, and the group is taken out.
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
// Airflow's, which it lists already. nil g is not known: the group stays, less
// what the project now lists. A group with nothing left is taken out, with
// its place in default-groups.
//
// A group the project did not have joins default-groups, so uv installs it. One
// it had stays as default-groups has it: a project that took it out of there
// wants the group written and not installed. refused reports a group not
// written because its table cannot hold the comments that head it (an inline
// [dependency-groups]).
func setRuntimeGroup(ed tomledit.Editor, g *RuntimeGroup) (refused bool, err error) {
	current, exists := tomledit.GetLines(ed, runtimeGroupKey)
	listed := listedPackages(ed)
	var out []any
	if g == nil {
		for _, v := range current {
			if s, isString := v.(string); isString && listed[manifest.DistName(s)] {
				continue
			}
			out = append(out, v)
		}
	} else {
		var entries []string
		seen := map[string]bool{}
		for _, n := range g.Names {
			name := manifest.DistName(n)
			if listed[name] || slices.Contains(astroDists, name) || seen[name] {
				continue
			}
			seen[name] = true
			// A marker limits the package to the platforms it has a wheel for.
			if _, marker, ok := strings.Cut(n, ";"); ok {
				name += "; " + strings.TrimSpace(marker)
			}
			entries = append(entries, name)
		}
		slices.SortFunc(entries, func(a, b string) int { return strings.Compare(manifest.DistName(a), manifest.DistName(b)) })
		if len(entries) > 0 {
			for _, c := range manifest.RuntimeGroupComments(g.Build) {
				out = append(out, tomledit.Comment(c))
			}
			out = append(out, asAny(entries)...)
		}
	}
	if !slices.ContainsFunc(out, func(v any) bool { _, isString := v.(string); return isString }) {
		if exists {
			removeRuntimeGroup(ed)
		}
		return false, nil
	}
	if exists && reflect.DeepEqual(out, current) {
		return false, nil
	}
	if !exists {
		if err := ensureDevGroup(ed); err != nil {
			return false, err
		}
	}
	if err := ed.Set(runtimeGroupKey, out); err != nil {
		if errors.Is(err, tomledit.ErrCommentsUnsupported) {
			return true, nil
		}
		return false, err
	}
	if !exists {
		return false, addDefaultGroup(ed)
	}
	return false, nil
}

// removeRuntimeGroup takes the astro-runtime group out, with its place in
// default-groups (which uv refuses to name a group that is not there), and
// [dependency-groups] when nothing else is in it.
func removeRuntimeGroup(ed tomledit.Editor) {
	ed.Delete(runtimeGroupKey)
	current, _ := ed.Get(defaultGroupsKey)
	if list, isList := current.([]any); isList {
		if at := slices.Index(list, any(manifest.RuntimeGroupName)); at >= 0 {
			ed.Delete(append(slices.Clone(defaultGroupsKey), strconv.Itoa(at)))
			list = slices.Delete(slices.Clone(list), at, at+1)
		}
		// What is left is uv's own default, so the key says nothing, and an
		// empty dev group, which ensureDevGroup may have written for it, says
		// nothing either.
		if reflect.DeepEqual(list, uvDefaultGroups) {
			ed.Delete(defaultGroupsKey)
			if dev, ok := ed.Get(devGroupKey); ok {
				if l, isList := dev.([]any); isList && len(l) == 0 {
					ed.Delete(devGroupKey)
				}
			}
		}
	}
	if groups, ok := ed.Get(dependencyGroupsKey); ok {
		if m, isTable := groups.(map[string]any); isTable && len(m) == 0 {
			ed.Delete(dependencyGroupsKey)
		}
	}
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
