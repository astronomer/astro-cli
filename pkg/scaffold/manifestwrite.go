package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// Every write to an existing pyproject.toml goes through EditManifest: the
// deployment links, the Airflow pin and the [tool.astro.env] declarations, from
// the CLI and from Astro Desktop alike. The rules live here once, so a writer
// cannot skip one of them:
//
//   - The edit is surgical, through tomledit, because the manifest is a file a
//     person owns and has commented. Re-emitting it from a parsed struct would
//     reformat everything around the one line that changed.
//   - The result is parsed before it is written, by manifest.Parse and by
//     envschema.ParseSchema, and a result either refuses is not written. The
//     parser refuses the whole file over one bad key, so a writer that skipped
//     the check could stop every other link or declaration in the project
//     loading. manifest.Parse leaves [tool.astro.env] untyped, which is why the
//     second parse is needed: without it an env write can save a section that
//     `astro local start` then refuses.
//   - The write is atomic, through pkg/fsatomic, at the file's existing mode. A
//     torn write leaves a manifest that does not parse, and a write at a fixed
//     mode would widen a file someone restricted.
//   - The whole read-modify-write runs inside the caller's wrapper, so a lock
//     taken there spans the read. A lock taken only around the write lets two
//     writers read the same old file, and the later atomic write silently drops
//     the other's edit.
//
// Replacing a table wholesale needs ReplaceTable, because tomledit refuses to
// Set over an existing table.

// ErrEditRefused reports an edit EditManifest did not write because its result
// would not load. The file on disk is unchanged. The error also wraps the
// parser's own error, so errors.As reaches the manifest.ValidationError or
// envschema.SchemaError that names each problem.
var ErrEditRefused = errors.New("that change was not written")

// ManifestEdit changes a manifest in place. before is the manifest as it was
// read, for the edits that need to keep what they are not setting; ed edits the
// same bytes. Returning an error abandons the edit and writes nothing.
type ManifestEdit func(before *manifest.Manifest, ed tomledit.Editor) error

// EditManifest applies edit to the pyproject.toml in dir and writes the result
// back, under the rules described at the top of this file.
//
// wrap runs the whole read-modify-write, and a nil wrap runs it directly. An
// embedder passes the lock its other writers of the same file take, plus
// anything else that has to hold while the file changes (Astro Desktop holds its
// dependency watcher, so its own write does not read as an outside edit). wrap
// must call the function it is given exactly once and return its error.
//
// It never creates a manifest. A missing file reports manifest.ErrNotFound, and
// one with no [tool.astro] reports manifest.ErrNoAstroSection, because a
// pyproject.toml without that table is someone else's Python project. A
// manifest that does not load already is refused as it stands, with the
// parser's error: an edit needs the parsed manifest to work from.
//
// An edit that changes no bytes writes nothing, so an idempotent caller does
// not disturb the file's mtime or wake anything watching it.
func EditManifest(dir string, wrap func(run func() error) error, edit ManifestEdit) error {
	return editManifestJudged(dir, wrap, edit, func(before *manifest.Manifest, out []byte) error {
		_, err := loadable(before, out)
		return err
	})
}

// judge decides whether an edit's result may be written: it returns nil, or
// the parser's error naming why not. before is the manifest as it was read.
type judge func(before *manifest.Manifest, out []byte) error

// editManifestJudged is EditManifest with the judge of the result as a
// parameter. EditManifest's is loadable's verdict; RemoveEnvDeclaration's is
// noNewEnvProblems, which lets a removal fix one of several problems.
func editManifestJudged(dir string, wrap func(run func() error) error, edit ManifestEdit, ok judge) error {
	ran := false
	run := func() error {
		ran = true
		return editManifest(dir, edit, ok)
	}
	if wrap == nil {
		return run()
	}
	err := wrap(run)
	if err == nil && !ran {
		return fmt.Errorf("the %s edit did not run: its wrapper returned without calling it", manifest.Marker)
	}
	return err
}

func editManifest(dir string, edit ManifestEdit, ok judge) error {
	path := filepath.Join(dir, manifest.Marker)
	// The write replaces the file by renaming onto it, and renaming onto a
	// symlink replaces the link with a regular file. Writing to what it points
	// at keeps the link.
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %w", manifest.ErrNotFound, err)
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	mode := info.Mode().Perm()
	// A rename replaces a file whatever its own permissions say, so without
	// this a manifest this user may not write would be rewritten anyway.
	if !writable(target, mode) {
		return fmt.Errorf("%s is read-only for this user, so it was not changed", path)
	}
	src, err := fsatomic.ReadFile(target)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	before, err := manifest.Parse(src)
	if err != nil {
		return withPath(err, path)
	}
	ed, err := tomledit.NewSurgical(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := edit(before, ed); err != nil {
		return fmt.Errorf("editing %s: %w", path, err)
	}
	out, err := ed.Bytes()
	if err != nil {
		return fmt.Errorf("rendering %s: %w", path, err)
	}
	if bytes.Equal(out, src) {
		return nil
	}
	// The baseline is parsed again rather than taken from before: the edit is
	// handed before and may change its maps in place, which would make the
	// comparison in the judge see the edit as the file's original state.
	baseline, err := manifest.Parse(src)
	if err != nil {
		return withPath(err, path)
	}
	if err := ok(baseline, out); err != nil {
		return fmt.Errorf("%w, because the result would not load: %w", ErrEditRefused, withPath(err, path))
	}
	return fsatomic.WriteFile(target, out, mode)
}

// loadable parses out the way every reader will, through manifest.Parse and
// then envschema.ParseSchema, and returns the parsed manifest or the parser's
// own error. Both errors name what they refuse, so a caller can pass them on.
//
// before is the manifest the edit started from, or nil for one being created.
// An existing [tool.astro.env] that the edit leaves exactly as it was is not
// held against the edit: a link change in a project whose declarations are
// already broken leaves them no more broken, and refusing it would block every
// unrelated write until someone fixed a section the edit never touched.
func loadable(before *manifest.Manifest, out []byte) (*manifest.Manifest, error) {
	after, err := manifest.Parse(out)
	if err != nil {
		return nil, err
	}
	if before != nil && reflect.DeepEqual(before.Astro.Env, after.Astro.Env) {
		return after, nil
	}
	if _, err := envschema.ParseSchema(after.Astro.Env); err != nil {
		return nil, err
	}
	return after, nil
}

// ReplaceTable writes value at key in place of whatever is there, deleting it
// first, because tomledit refuses to Set over an existing table. A table a
// person wrote under its own [header] arrives as one, so an edit that means
// "replace this link" has to delete before it sets.
//
// It replaces the table wholesale, comments and key order included, which is
// what makes it wrong for changing one key in a table a person may have
// commented: Set that key instead, and leave the rest of the table alone.
func ReplaceTable(ed tomledit.Editor, key []string, value any) error {
	ed.Delete(key)
	return ed.Set(key, value)
}

// noNewEnvProblems is loadable, except that a [tool.astro.env] which still
// does not load is accepted when every problem it has, the file already had.
//
// ParseSchema refuses the whole section over any one problem, so under
// loadable a section with two bad declarations could not lose either: each
// removal still leaves the other. Judging the problems as a set lets each
// removal through that makes the section no worse, and still refuses one that
// adds a problem the file did not have. The manifest itself is held to
// manifest.Parse as always.
func noNewEnvProblems(before *manifest.Manifest, out []byte) error {
	_, err := loadable(before, out)
	if err == nil {
		return nil
	}
	// A manifest error comes first from loadable and is never a SchemaError,
	// so it is refused here as it always is.
	var now *envschema.SchemaError
	if before == nil || !errors.As(err, &now) {
		return err
	}
	had := map[envschema.Problem]bool{}
	var was *envschema.SchemaError
	if _, berr := envschema.ParseSchema(before.Astro.Env); errors.As(berr, &was) {
		for _, p := range was.Problems {
			had[p] = true
		}
	}
	for _, p := range now.Problems {
		if !had[p] {
			return err
		}
	}
	return nil
}
