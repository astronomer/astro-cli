package deployment

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"slices"
	"strings"
)

// emptyDigest is the digest of empty content, which an upload sends rather than moves or copies.
var emptyDigest = bundleDigestPrefix + hex.EncodeToString(sha256.New().Sum(nil))

// bundleSyncEdit places at to the content the bundle holds at from.
type bundleSyncEdit struct {
	from, to string
}

// reuseBundleContent changes an upload's plan so it sends no content the bundle
// already holds at another path in the synced directory: it moves a file the
// upload would delete, or else copies a file it keeps. remote holds the
// bundle's digests before the upload.
func reuseBundleContent(plan *bundleSyncPlan, local, remote map[string]string) {
	transfers := slices.Concat(plan.added, plan.updated)
	kept, deleted := bundleSources(plan, remote)
	movable := movableFrom(transfers)
	reused, movedAway := map[string]bool{}, map[string]bool{}
	for _, to := range transfers {
		digest := local[to]
		if digest == emptyDigest {
			continue
		}
		var moveFrom, copyFrom string
		for _, from := range deleted[digest] {
			switch {
			case movedAway[from]:
				copyFrom = cmp.Or(copyFrom, from)
			case moveFrom == "" && movable(from):
				moveFrom = from
			}
		}
		copyFrom = cmp.Or(kept[digest], copyFrom)
		switch {
		case moveFrom != "":
			movedAway[moveFrom] = true
			plan.moved = append(plan.moved, bundleSyncEdit{from: moveFrom, to: to})
		case copyFrom != "":
			plan.copied = append(plan.copied, bundleSyncEdit{from: copyFrom, to: to})
		default:
			continue
		}
		reused[to] = true
	}
	plan.added = slices.DeleteFunc(plan.added, func(relPath string) bool { return reused[relPath] })
	plan.updated = slices.DeleteFunc(plan.updated, func(relPath string) bool { return reused[relPath] })
	plan.deleted = slices.DeleteFunc(plan.deleted, func(relPath string) bool { return movedAway[relPath] })
	byDestination := func(a, b bundleSyncEdit) int { return strings.Compare(a.to, b.to) }
	slices.SortFunc(plan.moved, byDestination)
	slices.SortFunc(plan.copied, byDestination)
}

// bundleSources returns, by digest, the first file in path order that an
// upload leaves as it is, and every file it deletes, in path order.
func bundleSources(plan *bundleSyncPlan, remote map[string]string) (kept map[string]string, deleted map[string][]string) {
	changed := map[string]bool{}
	for _, relPath := range slices.Concat(plan.updated, plan.deleted) {
		changed[relPath] = true
	}
	kept = map[string]string{}
	for relPath, digest := range remote {
		if !changed[relPath] && (kept[digest] == "" || relPath < kept[digest]) {
			kept[digest] = relPath
		}
	}
	deleted = map[string][]string{}
	for _, relPath := range plan.deleted {
		deleted[remote[relPath]] = append(deleted[remote[relPath]], relPath)
	}
	return kept, deleted
}

// movableFrom reports whether an upload of transfers may move a file it would
// delete. It may not move one whose path is a directory above a transfer, or a
// file below one, because that file would be in the transfer's way.
func movableFrom(transfers []string) func(relPath string) bool {
	files, dirs := map[string]bool{}, map[string]bool{}
	for _, relPath := range transfers {
		files[relPath] = true
		for dir := path.Dir(relPath); dir != "."; dir = path.Dir(dir) {
			dirs[dir] = true
		}
	}
	return func(relPath string) bool {
		if dirs[relPath] {
			return false
		}
		for dir := path.Dir(relPath); dir != "."; dir = path.Dir(dir) {
			if files[dir] {
				return false
			}
		}
		return true
	}
}
