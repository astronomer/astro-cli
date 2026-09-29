package vaultenv

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// seedLinks writes the link row of a global about to be created, before its
// value, so the value never exists without it: "projects": [] (no project
// until linked), or with NewEverywhere no row at all, removing any row an
// earlier entry of this key left behind. A value write that then fails leaves
// at most an empty row, which only narrows. An index that cannot be used
// refuses the create: the row could not be written, and without it the new
// global would reach every project.
func (w *Writer) seedLinks(vaultKey string) error {
	if w.scope != secrets.GlobalScope {
		return nil
	}
	refuse := func(err error) error {
		return fmt.Errorf("a new global starts out linked to no project, and its link state cannot be written. "+
			"Nothing was changed; repair %s first: %w", w.LinksPath(), err)
	}
	links, err := secrets.OpenLinks(w.dir)
	if err != nil {
		return refuse(err)
	}
	if w.NewEverywhere && links.ReachOf(vaultKey).Everywhere {
		return nil // no row to remove, so no index to write
	}
	if err := secrets.UpdateLinks(w.dir, func(rows map[string]secrets.Reach) error {
		if w.NewEverywhere {
			delete(rows, vaultKey)
		} else {
			rows[vaultKey] = secrets.Reach{Projects: []string{}}
		}
		return nil
	}); err != nil {
		return refuse(err)
	}
	return nil
}

// carryLinks gives newKey the link state of the spellings it is about to
// replace, before any value is written, so a set that renames a pinned global
// (link "region", then set "REGION") keeps it pinned instead of reaching every
// project under its new name. Where several replaced spellings are pinned, the
// new row reaches only the projects every one of them reached.
//
// The old rows stay until their values are gone (Set drops them then):
// removing them here, before the values, would leave the old values reaching
// every project if the write in between failed. Only a global scope with
// something to replace consults the index, and it is written only when a
// replaced spelling has a row. An index that cannot be read refuses the set:
// which projects the old value reached is unknown, and guessing "every
// project" is the leak.
func (w *Writer) carryLinks(newKey string, old []string) error {
	if w.scope != secrets.GlobalScope || len(old) == 0 {
		return nil
	}
	refuse := func(err error) error {
		return fmt.Errorf("it replaces %s, whose link state cannot be read, so the new entry could reach more "+
			"projects than the one it replaces. Nothing was changed; repair %s first: %w",
			strings.Join(old, ", "), w.LinksPath(), err)
	}
	links, err := secrets.OpenLinks(w.dir)
	if err != nil {
		return refuse(err)
	}
	pinned := false
	for _, k := range old {
		if !links.ReachOf(k).Everywhere {
			pinned = true
		}
	}
	if !pinned {
		return nil
	}
	err = secrets.UpdateLinks(w.dir, func(rows map[string]secrets.Reach) error {
		var carried []string
		restricted := false
		for _, k := range append([]string{newKey}, old...) {
			r, ok := rows[k]
			if !ok {
				continue
			}
			if !restricted {
				carried, restricted = slices.Clone(r.Projects), true
				continue
			}
			carried = slices.DeleteFunc(carried, func(p string) bool { return !slices.Contains(r.Projects, p) })
		}
		if restricted {
			rows[newKey] = secrets.Reach{Projects: carried}
		}
		return nil
	})
	if err != nil {
		return refuse(err)
	}
	return nil
}

// ErrNotGlobal reports a link edit on a writer for a project scope. Link state
// belongs to global entries only: a project's own secret already reaches that
// project and no other.
var ErrNotGlobal = errors.New("only a global vault entry has link state")

// ErrNotHeld reports a link read or edit for a name this scope holds no entry
// for.
var ErrNotHeld = errors.New("the vault holds no such entry")

// ErrLinkRowKept wraps the failure to remove a deleted global's link row. The
// value is gone either way; see Delete.
var ErrLinkRowKept = errors.New("its link row in the vault link index was kept")

// LinksPath is the link index this writer's vault keeps, for a message that
// names it.
func (w *Writer) LinksPath() string { return secrets.LinksPath(w.dir) }

// Reach is where the global entry (kind, name) is linked. It reads the link
// index only, so it needs no keyring. An index this build cannot use is the
// error OpenLinks gives, which wraps secrets.ErrLinksUnreadable or
// secrets.ErrLinksTooNew; while it stands no global resolves anywhere.
func (w *Writer) Reach(kind localenv.Kind, name string) (secrets.Reach, error) {
	vaultKey, err := w.linkKey(kind, name)
	if err != nil {
		return secrets.Reach{}, err
	}
	links, err := secrets.OpenLinks(w.dir)
	if err != nil {
		return secrets.Reach{}, err
	}
	return links.ReachOf(vaultKey), nil
}

// EditReach replaces the global entry's reach with what fn makes of it, under
// the index's writers' lock, and returns the reach before and after. A Reach
// with Everywhere set removes the row. fn's error, or UpdateLinks' refusal of
// an index it cannot safely rewrite, leaves the index as it was.
//
// Where two spellings of the name share an env key, the one the chain resolves
// is what fn sees, and every spelling gets the result, so no spelling the
// chain could fall back to keeps a wider reach.
func (w *Writer) EditReach(kind localenv.Kind, name string, fn func(secrets.Reach) (secrets.Reach, error)) (before, after secrets.Reach, err error) {
	matches, err := w.globalMatches(kind, name)
	if err != nil {
		return before, after, err
	}
	winner := matches[len(matches)-1]
	err = secrets.UpdateLinks(w.dir, func(rows map[string]secrets.Reach) error {
		before = secrets.Reach{Everywhere: true}
		if r, ok := rows[winner]; ok {
			before = r
		}
		next, ferr := fn(before)
		if ferr != nil {
			return ferr
		}
		after = next
		for _, k := range matches {
			if next.Everywhere {
				delete(rows, k)
				continue
			}
			rows[k] = secrets.Reach{Projects: append([]string(nil), next.Projects...)}
		}
		return nil
	})
	return before, after, err
}

// linkKey is the vault key whose row decides the global entry's reach: the
// spelling the chain resolves.
func (w *Writer) linkKey(kind localenv.Kind, name string) (string, error) {
	matches, err := w.globalMatches(kind, name)
	if err != nil {
		return "", err
	}
	return matches[len(matches)-1], nil
}

// globalMatches is sameEnvKey for a global writer, refusing a project scope
// and a name with no entry.
func (w *Writer) globalMatches(kind localenv.Kind, name string) ([]string, error) {
	if w.scope != secrets.GlobalScope {
		return nil, ErrNotGlobal
	}
	key, _, err := w.keys(kind, name)
	if err != nil {
		return nil, err
	}
	matches, err := w.sameEnvKey(kind, key)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, ErrNotHeld
	}
	return matches, nil
}

// dropLinkRows removes the link rows of global entries whose values were just
// deleted. Nothing to do in a project scope, for no keys, or when no removed
// key has a row, so a delete on a machine that has never linked anything
// writes no index.
func (w *Writer) dropLinkRows(removed []string) error {
	if w.scope != secrets.GlobalScope || len(removed) == 0 {
		return nil
	}
	links, err := secrets.OpenLinks(w.dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLinkRowKept, err)
	}
	has := false
	for _, k := range removed {
		if !links.ReachOf(k).Everywhere {
			has = true
		}
	}
	if !has {
		return nil
	}
	if err := secrets.UpdateLinks(w.dir, func(rows map[string]secrets.Reach) error {
		for _, k := range removed {
			delete(rows, k)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrLinkRowKept, err)
	}
	return nil
}
