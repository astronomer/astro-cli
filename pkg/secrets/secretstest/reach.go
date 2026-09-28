// Package secretstest holds the shared parity cases for vault link state, so
// every tool that decides which projects a global vault entry reaches runs the
// same table: pkg/secrets against Reach.Includes, the CLI through
// internal/vaultenv, and Astro Desktop through its own merged listings.
//
// A separate package in the httptest tradition: importing it from production
// code is visibly wrong, and a consumer in another module gets a supported
// fixture instead of a hand-copied one that drifts.
//
// The cases deliberately do not compute a checkout. That needs
// localrt.CanonicalPath and localrt.ProjectHome, which pkg/secrets does not
// import; each consumer builds the checkout its own way, and Layout.Checkout is
// what it must come to. Comparing the two is part of the parity.
package secretstest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Fixture permissions: owner-only files, ordinary directories.
const (
	dirPerm  = 0o755
	filePerm = 0o600
)

// Place names one directory in a Layout.
type Place string

// The places every ReachCase is evaluated from.
const (
	// PlaceMain is a main git worktree, the project a link usually names.
	PlaceMain Place = "main"
	// PlaceMainSub is a monorepo subdirectory of it: a project of its own.
	PlaceMainSub Place = "main-subdir"
	// PlaceWorktree is a linked worktree inside the repository's tree.
	PlaceWorktree Place = "worktree"
	// PlaceCustomWorktree is a linked worktree in a directory outside the
	// repository, as the desktop's WorktreeDir setting places them.
	PlaceCustomWorktree Place = "custom-worktree"
	// PlaceCustomWorktreeSub is the monorepo subdirectory inside that worktree.
	PlaceCustomWorktreeSub Place = "custom-worktree-subdir"
	// PlaceSymlink is PlaceMain reached through a symlink.
	PlaceSymlink Place = "symlink"
	// PlaceCaseVariant is PlaceMain spelled in a different case. Only on a
	// case-insensitive filesystem; elsewhere the case skips.
	PlaceCaseVariant Place = "case-variant"
	// PlaceOther is a second, unrelated project.
	PlaceOther Place = "other-project"
	// PlaceSandbox is a workflow sandbox: a project directory outside any
	// repository, which nothing links.
	PlaceSandbox Place = "sandbox"
	// PlaceOutside is a directory that is no project at all.
	PlaceOutside Place = "outside"
)

var allPlaces = []Place{
	PlaceMain, PlaceMainSub, PlaceWorktree, PlaceCustomWorktree, PlaceCustomWorktreeSub,
	PlaceSymlink, PlaceCaseVariant, PlaceOther, PlaceSandbox, PlaceOutside,
}

// Layout is the filesystem the cases run in, built by NewLayout.
type Layout struct {
	// Root is the temp directory everything lives under, canonical.
	Root string
	// dirs is the spelling a tool is handed for each place; checkouts is what
	// it must canonicalize that spelling to.
	dirs      map[Place]string
	checkouts map[Place]secrets.Checkout
	// stale is a linked path that no longer exists.
	stale string
}

// Dir is the directory to resolve from for a place, in the spelling the case
// is about (through the symlink, in the other case). Empty when this
// filesystem cannot express the place.
func (l Layout) Dir(p Place) string { return l.dirs[p] }

// Checkout is the checkout a tool must build for Dir(p): canonical path and
// project home.
func (l Layout) Checkout(p Place) secrets.Checkout { return l.checkouts[p] }

// NewLayout builds the directory tree under t.TempDir(): a main repository
// with a monorepo subdirectory, a linked worktree inside it and one outside
// it (laid out the way `git worktree add` leaves them, without running git),
// a symlink to the main checkout, a second project, a sandbox and a plain
// directory.
func NewLayout(t testing.TB) Layout {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(root, "repo")
	mainSub := filepath.Join(main, "services", "etl")
	worktree := filepath.Join(main, ".worktrees", "feat")
	custom := filepath.Join(root, "worktrees", "repo-feat2")
	customSub := filepath.Join(custom, "services", "etl")
	other := filepath.Join(root, "other")
	sandbox := filepath.Join(root, "cache", "sandboxes", "s1")
	outside := filepath.Join(root, "plain")

	mkdirs(t, filepath.Join(main, ".git"), mainSub, filepath.Join(other, ".git"), sandbox, outside, customSub)
	for _, dir := range []string{main, mainSub, other, sandbox, custom, customSub} {
		writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = 'p'\n")
	}
	linkWorktree(t, main, worktree, "feat")
	linkWorktree(t, main, custom, "repo-feat2")

	l := Layout{
		Root:  root,
		stale: filepath.Join(root, "moved-away"),
		dirs: map[Place]string{
			PlaceMain: main, PlaceMainSub: mainSub, PlaceWorktree: worktree,
			PlaceCustomWorktree: custom, PlaceCustomWorktreeSub: customSub,
			PlaceOther: other, PlaceSandbox: sandbox, PlaceOutside: outside,
		},
		checkouts: map[Place]secrets.Checkout{
			PlaceMain:              {Path: main, Home: main},
			PlaceMainSub:           {Path: mainSub, Home: mainSub},
			PlaceWorktree:          {Path: worktree, Home: main},
			PlaceCustomWorktree:    {Path: custom, Home: main},
			PlaceCustomWorktreeSub: {Path: customSub, Home: mainSub},
			PlaceSymlink:           {Path: main, Home: main},
			PlaceCaseVariant:       {Path: main, Home: main},
			PlaceOther:             {Path: other, Home: other},
			PlaceSandbox:           {Path: sandbox, Home: sandbox},
			PlaceOutside:           {Path: outside, Home: outside},
		},
	}
	// Symlinks need a privilege on Windows that CI runners may not grant.
	link := filepath.Join(root, "link-to-repo")
	if err := os.Symlink(main, link); err == nil {
		l.dirs[PlaceSymlink] = link
	}
	// Another case of the same directory is the same directory only on a
	// case-insensitive filesystem; asking the filesystem is the only test.
	variant := filepath.Join(root, "REPO")
	if a, err := os.Stat(variant); err == nil {
		if b, err := os.Stat(main); err == nil && os.SameFile(a, b) {
			l.dirs[PlaceCaseVariant] = variant
		}
	}
	return l
}

// linkWorktree writes a linked worktree's pointers: the admin directory under
// the main repository's .git/worktrees, with its commondir, and the .git FILE
// in the worktree naming it.
func linkWorktree(t testing.TB, main, wt, name string) {
	t.Helper()
	admin := filepath.Join(main, ".git", "worktrees", name)
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(wt, ".git")+"\n")
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+filepath.ToSlash(admin)+"\n")
}

func mkdirs(t testing.TB, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t testing.TB, path, body string) {
	t.Helper()
	mkdirs(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), filePerm); err != nil {
		t.Fatal(err)
	}
}

// The vault listing every case runs against: global keys only, since link
// state never applies to a project-scoped entry.
const (
	KeyEverywhere = "conn:global:everywhere_conn" // never has a row
	KeyWarehouse  = "conn:global:warehouse"       // linked to the main project
	// Linked to no project. A vault key naming a test entry, not a credential.
	KeySlackToken   = "env:global:SLACK_TOKEN"   //nolint:gosec // G101: see above
	KeyWorktreeOnly = "var:global:worktree_only" // linked to one worktree
	KeyStale        = "env:global:STALE_LINK"    // linked only to a path that is gone
	KeySubproject   = "env:global:SUBPROJECT"    // linked to the monorepo subdirectory
	KeyOther        = "var:global:other_only"    // linked to the other project
)

// ReachVault is the listing, sorted.
var ReachVault = []string{KeyEverywhere, KeyWarehouse, KeySlackToken, KeyStale, KeySubproject, KeyOther, KeyWorktreeOnly}

// ReachCase is one checkout under one index state.
type ReachCase struct {
	// Name is "<state>/<place>".
	Name string
	// Index is the links.idx content for a layout; nil means no file.
	Index func(Layout) []byte
	// Place is where the tool resolves from: Layout.Dir(Place), which must
	// canonicalize to Layout.Checkout(Place).
	Place Place
	// Want are the keys of ReachVault that reach the checkout, sorted.
	Want []string
	// WantErr is what OpenLinks reports for the index: nil, or wrapping
	// ErrLinksUnreadable or ErrLinksTooNew. A tool fails its global tier closed
	// on either, so Want is empty then.
	WantErr error
}

// Skip is a reason to skip this case on this filesystem, or "".
func (c *ReachCase) Skip(l Layout) string {
	if l.Dir(c.Place) == "" {
		return "this filesystem cannot express " + string(c.Place)
	}
	return ""
}

// WriteIndex publishes the case's index into a vault directory, or makes sure
// there is none.
func (c *ReachCase) WriteIndex(t testing.TB, vaultDir string, l Layout) {
	t.Helper()
	mkdirs(t, vaultDir)
	path := secrets.LinksPath(vaultDir)
	if c.Index == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return
	}
	writeFile(t, path, string(c.Index(l)))
}

// index encodes rows as a version 1 index, with paths from the layout.
func index(rows map[string][]string) []byte {
	type row struct {
		Projects []string `json:"projects"`
	}
	links := map[string]row{}
	for k, ps := range rows {
		links[k] = row{Projects: append([]string{}, ps...)}
	}
	out, err := json.Marshal(struct {
		Version int            `json:"version"`
		Links   map[string]row `json:"links"`
	}{1, links})
	if err != nil {
		panic(err) // a map of strings always marshals
	}
	return out
}

// Each state maps a place to the keys that reach it, written out by hand
// rather than derived: a table computed with the predicate it tests cannot
// fail.
func everything() []string { return sorted(ReachVault) }

func sorted(keys []string) []string {
	out := append([]string{}, keys...)
	sort.Strings(out)
	return out
}

// ReachCases is the whole table: every place under every index state.
func ReachCases() []ReachCase {
	type state struct {
		name    string
		index   func(Layout) []byte
		wantErr error
		want    map[Place][]string
	}
	allGet := func(keys []string) map[Place][]string {
		m := map[Place][]string{}
		for _, p := range allPlaces {
			m[p] = keys
		}
		return m
	}

	// Rows restricted to paths, but none naming the places, so only the
	// no-row key reaches anywhere.
	unrestricted := []string{KeyEverywhere}

	states := []state{
		{
			name: "no-file",
			want: allGet(everything()),
		},
		{
			// Every row "projects": [] — no project, not every project.
			name: "empty-list",
			index: func(Layout) []byte {
				return index(map[string][]string{
					KeyWarehouse: {}, KeySlackToken: {}, KeyWorktreeOnly: {},
					KeyStale: {}, KeySubproject: {}, KeyOther: {},
				})
			},
			want: allGet(unrestricted),
		},
		{
			// Every row names only a path that no longer exists.
			name: "stale-path",
			index: func(l Layout) []byte {
				return index(map[string][]string{
					KeyWarehouse: {l.stale}, KeySlackToken: {l.stale}, KeyWorktreeOnly: {l.stale},
					KeyStale: {l.stale}, KeySubproject: {l.stale}, KeyOther: {l.stale},
				})
			},
			want: allGet(unrestricted),
		},
		{
			name: "linked",
			index: func(l Layout) []byte {
				return index(map[string][]string{
					KeyWarehouse:    {l.Checkout(PlaceMain).Path},
					KeySlackToken:   {},
					KeyWorktreeOnly: {l.Checkout(PlaceCustomWorktree).Path},
					KeyStale:        {l.stale},
					KeySubproject:   {l.Checkout(PlaceMainSub).Path},
					KeyOther:        {l.Checkout(PlaceOther).Path, l.stale},
				})
			},
			want: map[Place][]string{
				// The project itself, however it is spelled.
				PlaceMain:        sorted([]string{KeyEverywhere, KeyWarehouse}),
				PlaceSymlink:     sorted([]string{KeyEverywhere, KeyWarehouse}),
				PlaceCaseVariant: sorted([]string{KeyEverywhere, KeyWarehouse}),
				// Its worktrees inherit the project's links, wherever they live;
				// a link naming one worktree reaches that one only.
				PlaceWorktree:       sorted([]string{KeyEverywhere, KeyWarehouse}),
				PlaceCustomWorktree: sorted([]string{KeyEverywhere, KeyWarehouse, KeyWorktreeOnly}),
				// A subdirectory is its own project, not the repository's, and
				// the same subdirectory of a worktree is that project too.
				PlaceMainSub:           sorted([]string{KeyEverywhere, KeySubproject}),
				PlaceCustomWorktreeSub: sorted([]string{KeyEverywhere, KeySubproject}),
				PlaceOther:             sorted([]string{KeyEverywhere, KeyOther}),
				// Nothing links these, so only the no-row entry reaches them.
				PlaceSandbox: unrestricted,
				PlaceOutside: unrestricted,
			},
		},
		{
			name:    "newer-version",
			index:   func(Layout) []byte { return []byte(`{"version":2,"links":{}}`) },
			wantErr: secrets.ErrLinksTooNew,
			want:    allGet(nil),
		},
		{
			name:    "corrupt",
			index:   func(Layout) []byte { return []byte(`{"version":1,"links":{"conn:global:warehouse":`) },
			wantErr: secrets.ErrLinksUnreadable,
			want:    allGet(nil),
		},
	}

	var out []ReachCase
	for _, s := range states {
		for _, p := range allPlaces {
			out = append(out, ReachCase{
				Name:    s.name + "/" + string(p),
				Index:   s.index,
				Place:   p,
				Want:    append([]string{}, s.want[p]...),
				WantErr: s.wantErr,
			})
		}
	}
	return out
}
