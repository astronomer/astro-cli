package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// Link state: which projects a global vault entry reaches.
//
// A global entry (scope GlobalScope) is eligible for every project unless the
// link index says otherwise. The index is one plain file beside the values,
// ~/.astro/secrets/links.idx, shared by the CLI and Astro Desktop:
//
//	{"version":1,"links":{
//	  "conn:global:warehouse":{"projects":["/Users/me/etl"]},
//	  "env:global:SLACK_TOKEN":{"projects":[]}}}
//
// Keys are vault keys. No row means every project; "projects": [] means no
// project; a list means only those, each a canonical path (the spelling
// localrt.CanonicalPath produces) compared byte for byte against a Checkout.
//
// Why a separate file rather than a field in the value file: every build that
// already shipped re-marshals only {key, value} on Set, so an older tool
// editing a pinned value would silently drop the pin and the entry would reach
// every project. Neither file name ends in valueExt, so ListMeta and hasValues
// in every build, old or new, skip both.
//
// Reading never touches the keyring. Reaching too few projects is the safe
// failure (a declared name is missing, with a diagnosis); reaching too many
// puts a credential into another project's Airflow. So every doubt on the read
// side resolves toward fewer: a file this build cannot parse, or one written by
// a newer version, is an error the caller fails closed on, and a row this build
// cannot fully read reaches no more than the paths it could read.

const (
	// linksFile is the index. Not ".json": see the note above.
	linksFile = "links.idx"
	// linksLock serializes writers. Readers never take it; fsatomic's atomic
	// replace gives them the old file or the new one.
	linksLock = "links.lock"
	// linksVersion is the format this build reads and writes. Any change that
	// could make an older reader reach MORE projects must bump it; an additive
	// field that an older reader may ignore safely does not.
	linksVersion = 1
	// linksPerm is owner-only, like the values beside it.
	linksPerm = 0o600
)

// ErrLinksUnreadable reports an index that exists but cannot be parsed. The
// caller fails its global tier closed: which entries reach which projects is
// unknown, and guessing "every project" is the unsafe direction.
var ErrLinksUnreadable = errors.New("vault link index is unreadable")

// ErrLinksTooNew reports an index written with a format version this build
// does not know. Failed closed like an unreadable one, and never rewritten: a
// newer version may carry restrictions this build would drop.
var ErrLinksTooNew = errors.New("vault link index was written by a newer version")

// LinksPath is the index file under a vault directory, for a message that
// needs to name it.
func LinksPath(dir string) string { return filepath.Join(dir, linksFile) }

// Reach is the set of checkouts one global entry is eligible for.
type Reach struct {
	// Everywhere is true when the index has no row for the entry: it reaches
	// every project, which is what every global did before link state existed.
	Everywhere bool
	// Projects are the canonical paths the entry is linked to when Everywhere
	// is false. Empty means no project.
	Projects []string
}

// Checkout is the directory a tool is resolving for, in the two spellings a
// link may name. Both are canonical; either may be empty.
type Checkout struct {
	// Path is the checkout's own canonical path.
	Path string
	// Home is the project the checkout belongs to: for a linked git worktree,
	// the main worktree's canonical root plus the checkout's offset within its
	// repository, and otherwise the same as Path. It is what makes a link to a
	// project reach that project's worktrees (localrt.ProjectHome computes it).
	Home string
}

// Includes reports whether the entry reaches c. THE predicate: both tools
// decide eligibility here, so they cannot disagree about it.
//
// A link matches the checkout's own path or its project home, byte for byte.
// No prefix matching: a worktree can live anywhere, and a directory under a
// linked project is not that project unless it resolves home to it. A checkout
// with neither spelling (a directory outside any project) is reached only by
// entries with no row.
func (r Reach) Includes(c Checkout) bool {
	if r.Everywhere {
		return true
	}
	for _, p := range r.Projects {
		if (c.Path != "" && p == c.Path) || (c.Home != "" && p == c.Home) {
			return true
		}
	}
	return false
}

// Links is the index as one read saw it.
type Links struct {
	rows map[string]Reach
}

// linksDoc is the file's top level. Unknown top-level fields are kept, as raw
// JSON, so a rewrite does not strip what a newer peer added.
type linksDoc struct {
	Version int                        `json:"version"`
	Links   map[string]json.RawMessage `json:"links"`
}

// OpenLinks reads the index under a vault directory (DefaultDir). A missing
// file is not an error: it is the state every machine starts in, and means
// every entry reaches every project. An index that cannot be parsed wraps
// ErrLinksUnreadable; one written by a newer format wraps ErrLinksTooNew.
func OpenLinks(dir string) (*Links, error) {
	if _, err := prepareDir(dir, false); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLinksUnreadable, err)
	}
	raw, err := fsatomic.ReadFile(LinksPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return &Links{rows: map[string]Reach{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrLinksUnreadable, LinksPath(dir), err)
	}
	doc, err := parseLinks(LinksPath(dir), raw)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]Reach, len(doc.Links))
	for key, rowRaw := range doc.Links {
		row, err := parseRow(rowRaw)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: row %q: %w", ErrLinksUnreadable, LinksPath(dir), key, err)
		}
		rows[key] = Reach{Projects: plausiblePaths(row.Projects)}
	}
	return &Links{rows: rows}, nil
}

// ReachOf is the reach of one vault key. A nil *Links is an index with no
// rows, so a caller that never opened one sees every entry reach everywhere.
func (l *Links) ReachOf(vaultKey string) Reach {
	if l == nil {
		return Reach{Everywhere: true}
	}
	r, ok := l.rows[vaultKey]
	if !ok {
		return Reach{Everywhere: true}
	}
	return Reach{Projects: slices.Clone(r.Projects)}
}

// parseLinks decodes the top level and checks the version.
func parseLinks(path string, raw []byte) (linksDoc, error) {
	var doc linksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, fmt.Errorf("%w: %s: %w", ErrLinksUnreadable, path, err)
	}
	switch {
	case doc.Version > linksVersion:
		return doc, fmt.Errorf("%w: %s is version %d, this build reads version %d", ErrLinksTooNew, path, doc.Version, linksVersion)
	case doc.Version < 1:
		// Missing or zero: not a file any version of this format wrote.
		return doc, fmt.Errorf("%w: %s has no valid version", ErrLinksUnreadable, path)
	}
	return doc, nil
}

// linkRow is the part of a row this build understands.
type linkRow struct {
	Projects []string `json:"projects"`
}

// parseRow decodes one row. A row that is present but names no projects —
// "projects": [], null, or absent — reaches no project: the row exists, so the
// entry was restricted, and reading it as unrestricted is the unsafe way round.
func parseRow(raw json.RawMessage) (linkRow, error) {
	var row linkRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return row, err
	}
	return row, nil
}

// plausiblePaths keeps the paths that could be a canonical project path on
// some platform (scopeIsPlausible, the read-side rule), dropping the rest. A
// dropped path matches nothing, so this only ever narrows a row.
func plausiblePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != GlobalScope && scopeIsPlausible(p) {
			out = append(out, p)
		}
	}
	return out
}

// updateLinksHook runs between UpdateLinks' read and its publish. Tests widen
// the race window with it; production leaves it nil.
var updateLinksHook func()

// UpdateLinks edits the index under dir: it takes the writers' lock, reads the
// current rows, hands them to fn to mutate, and publishes the result
// atomically at 0600.
//
// The map is keyed by vault key. A key fn deletes, or sets to a Reach with
// Everywhere true, loses its row and reaches every project; any other value is
// written as a row, an empty Projects as "projects": []. Rows fn leaves alone
// are written back unchanged, and a changed row keeps the fields this build
// does not know, so a rewrite never strips what a newer peer added.
//
// Refuses, writing nothing, when the lock cannot be taken within
// fsatomic.LockTimeout, when the current index is unreadable (overwriting it
// would lose whatever it held) or written by a newer version (rewriting it
// would drop restrictions this build cannot see), when fn returns an error, or
// when a changed row is not valid: its key must be a global vault key and each
// path must pass checkScope.
func UpdateLinks(dir string, fn func(map[string]Reach) error) error {
	if _, err := prepareDir(dir, true); err != nil {
		return err
	}
	unlock, err := fsatomic.Lock(filepath.Join(dir, linksLock))
	if err != nil {
		return fmt.Errorf("lock the vault link index: %w", err)
	}
	defer unlock()

	path := LinksPath(dir)
	doc := linksDoc{Version: linksVersion, Links: map[string]json.RawMessage{}}
	extra := map[string]json.RawMessage{}
	raw, err := fsatomic.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("%w: %s: %w", ErrLinksUnreadable, path, err)
	default:
		if doc, err = parseLinks(path, raw); err != nil {
			return err
		}
		if doc.Links == nil {
			doc.Links = map[string]json.RawMessage{}
		}
		// The whole top level again, for the fields this build does not know.
		if err := json.Unmarshal(raw, &extra); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrLinksUnreadable, path, err)
		}
		delete(extra, "version")
		delete(extra, "links")
	}

	before := make(map[string]Reach, len(doc.Links))
	for key, rowRaw := range doc.Links {
		row, err := parseRow(rowRaw)
		if err != nil {
			return fmt.Errorf("%w: %s: row %q: %w", ErrLinksUnreadable, path, key, err)
		}
		before[key] = Reach{Projects: row.Projects}
	}
	edit := make(map[string]Reach, len(before))
	for k, r := range before {
		edit[k] = Reach{Projects: slices.Clone(r.Projects)}
	}
	if err := fn(edit); err != nil {
		return err
	}

	next := make(map[string]json.RawMessage, len(edit))
	for key, r := range edit {
		if r.Everywhere {
			continue
		}
		old, existed := before[key]
		if existed && slices.Equal(old.Projects, r.Projects) {
			next[key] = doc.Links[key] // untouched: keep the row as found, unknown fields and all
			continue
		}
		rowRaw, err := buildRow(key, r.Projects, doc.Links[key])
		if err != nil {
			return err
		}
		next[key] = rowRaw
	}

	out, err := encodeLinks(extra, next)
	if err != nil {
		return err
	}
	if updateLinksHook != nil {
		updateLinksHook()
	}
	if err := fsatomic.WriteFile(path, out, linksPerm); err != nil {
		return fmt.Errorf("write the vault link index: %w", err)
	}
	return nil
}

// buildRow validates one changed row and encodes it over the row it replaces,
// so fields this build does not know survive the edit.
func buildRow(key string, projects []string, old json.RawMessage) (json.RawMessage, error) {
	_, scope, _, err := ParseKey(key)
	if err != nil {
		return nil, fmt.Errorf("link %q: %w", key, err)
	}
	if scope != GlobalScope {
		return nil, fmt.Errorf("%w: link %q: only a global entry has link state", ErrBadKey, key)
	}
	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		if p == GlobalScope {
			return nil, fmt.Errorf("%w: link %q: %q is not a project path", ErrBadKey, key, p)
		}
		if err := checkScope(p); err != nil {
			return nil, fmt.Errorf("link %q: %w", key, err)
		}
		paths = append(paths, p)
	}
	// Sorted and deduplicated, so the file does not churn with the order an
	// editor happened to add paths in.
	slices.Sort(paths)
	paths = slices.Compact(paths)

	fields := map[string]json.RawMessage{}
	if len(old) > 0 {
		if err := json.Unmarshal(old, &fields); err != nil {
			return nil, fmt.Errorf("%w: row %q: %w", ErrLinksUnreadable, key, err)
		}
	}
	enc, err := json.Marshal(paths)
	if err != nil {
		return nil, fmt.Errorf("encode link %q: %w", key, err)
	}
	fields["projects"] = enc
	return json.Marshal(fields)
}

// encodeLinks writes the top level: version, links, and any fields a newer
// peer added that this build carries through untouched.
func encodeLinks(extra, rows map[string]json.RawMessage) ([]byte, error) {
	top := make(map[string]json.RawMessage, len(extra)+2)
	for k, v := range extra {
		top[k] = v
	}
	version, err := json.Marshal(linksVersion)
	if err != nil {
		return nil, err
	}
	links, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("encode the vault link index: %w", err)
	}
	top["version"], top["links"] = version, links
	// encoding/json sorts map keys, so the file is deterministic.
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("encode the vault link index: %w", err)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return nil, fmt.Errorf("encode the vault link index: %w", err)
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}
