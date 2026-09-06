package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind is what Apply does for one change.
//
// Delete is produced by planRetirements, for the v1 files whose entire contents
// reached the manifest. airflow_settings.yaml is NOT among them, and no longer
// because this package cannot read it — it can, into the vault and the env
// schema both. Pools are why: neither tool stores them, so the file is the only
// record of a project's pools that survives, and planRetirements skips it by
// name rather than by whether some note happens to mention it.
type Kind string

const (
	CreateDir     Kind = "create-dir"
	CreateFile    Kind = "create-file"
	CreateSymlink Kind = "create-symlink"
	UpdateFile    Kind = "update-file"
	Delete        Kind = "delete"
)

// ErrChangedOnDisk reports that the project no longer matches what Plan saw, so
// Apply refused rather than doing something the preview did not describe.
var ErrChangedOnDisk = errors.New("the project changed since it was planned")

// ErrNoSecretWriter reports that the changeset carries values for the vault and
// no Options.SecretWriter was given to store them with.
var ErrNoSecretWriter = errors.New("no secret writer for a conversion that carries values")

// Change is one operation Apply performs on the project directory.
//
// Content is the FINAL bytes, not a description of an edit, and that is what
// makes a preview possible: a caller can diff Content against what is on disk
// and show a person exactly what changes. Computing it is why Plan reads the
// project even though it writes nothing.
type Change struct {
	Kind Kind `json:"kind"`
	// Path is relative to the project directory, in slash form, so it reads the
	// same in a UI on any platform. Apply refuses a path that leaves the project.
	Path string `json:"path"`
	// Content is the bytes to write, for CreateFile and UpdateFile.
	//
	// Not serialized: a change set crosses a process boundary only to be
	// DISPLAYED — the desktop calls this package in process, so the side
	// that applies is always the side that planned. Sending file contents to a
	// UI that only needs to list them is payload for nothing, and a diff view
	// can read the file itself. Apply refuses a write whose Content is nil, so a
	// round-tripped change set fails loudly instead of truncating the user's
	// files to zero bytes.
	Content []byte `json:"-"`
	// Target is the symlink destination, for CreateSymlink.
	Target string `json:"target,omitempty"`
	// Labels are how this change reads in the Result's lists, which is not
	// always the path: a directory reads "dags/", a symlink reads
	// "CLAUDE.md -> AGENTS.md", and an adopted manifest needs two lines to say
	// what was added to it. Result's lists are derived from these, so a change
	// cannot be reported and unlabelled at the same time.
	Labels []string `json:"labels,omitempty"`
}

// Changeset is the Result a run would produce, plus the operations that produce
// it. Plan returns one without touching the project; Apply performs the
// operations and returns the Result.
type Changeset struct {
	Result
	// Changes are performed in order, and the order is load-bearing in two
	// places.
	//
	// The manifest is written after everything that adds, because a manifest
	// carrying [tool.astro] is the one thing that makes a rerun refuse: a run
	// that dies before it is safe to repeat.
	//
	// Deletions come after the manifest, and last of all. Apply stops at the
	// first failure, so removing requirements.txt before the manifest that
	// replaces it means a run dying in between has taken the dependencies away
	// and put nothing in their place. Failing the other way round leaves the
	// project carrying both, which a person can sort out.
	Changes []Change `json:"changes"`
	// Secrets are the values that go to the shared vault rather than into the
	// project. Separate from Changes because they are neither a path nor bytes
	// a preview may show — see SecretWrite.
	Secrets []SecretWrite `json:"secrets,omitempty"`

	// secrets is the writer Apply stores them through, from
	// Options.SecretWriter.
	secrets SecretWriter
}

// report fills Created, Updated and Deleted from Changes, so the lists a person
// reads and the operations that produce them cannot disagree.
//
// They used to be appended side by side at each site, and they drifted exactly
// as you would expect: the adopted manifest was added to one list and not the
// other, so the single most important line in a conversion preview rendered
// blank. Skipped stays separate because nothing is done for it.
func (cs *Changeset) report() {
	cs.Created, cs.Updated, cs.Deleted = nil, nil, nil
	for i := range cs.Changes {
		c := &cs.Changes[i]
		switch c.Kind {
		case CreateDir, CreateFile, CreateSymlink:
			cs.Created = append(cs.Created, c.Labels...)
		case UpdateFile:
			cs.Updated = append(cs.Updated, c.Labels...)
		case Delete:
			// Its own list, and not folded into Updated, which is where it
			// started. A caller rendering "updated: requirements.txt" for a file
			// that no longer exists tells the user the opposite of what
			// happened, and every consumer of this Result — `astro init`'s
			// output, the desktop's preview — reads these lists rather than
			// Changes.
			cs.Deleted = append(cs.Deleted, c.Labels...)
		}
	}
}

// Apply performs a change set and reports what it did.
//
// It executes what Plan decided rather than deciding anything itself: a preview
// a person approved has to be what runs, so re-deriving a decision here — even
// the same decision — would make the preview advisory rather than binding.
//
// What it does check is that the preconditions still hold. The window between
// Plan and Apply is human-scale by design — the desktop shows the change set and
// waits — so a project that moved underneath is ordinary rather than exotic. An
// edit whose file has since been deleted, or a write with no content, fails with
// ErrChangedOnDisk instead of resurrecting or truncating something.
func (cs *Changeset) Apply() (*Result, error) {
	if err := os.MkdirAll(cs.Dir, dirPerm); err != nil {
		return nil, err
	}
	// Values first, files after. See applySecrets: a manifest that declares
	// connections whose values did not land leaves a project that will not
	// start, while a store that succeeded and a manifest that did not leaves a
	// vault holding values nothing yet declares, which the next run overwrites.
	if err := cs.applySecrets(); err != nil {
		return nil, err
	}
	for i := range cs.Changes {
		if err := cs.Changes[i].apply(cs.Dir); err != nil {
			return nil, err
		}
	}
	return cs.result(), nil
}

// result copies the Result out, including its slices. Handing back a struct that
// shares backing arrays with the Changeset means an append by either side can
// overwrite the other whenever there is spare capacity — silent, and dependent
// on lengths nobody is tracking.
func (cs *Changeset) result() *Result {
	res := cs.Result
	res.Created = append([]string(nil), cs.Created...)
	res.Skipped = append([]string(nil), cs.Skipped...)
	res.Updated = append([]string(nil), cs.Updated...)
	res.Deleted = append([]string(nil), cs.Deleted...)
	res.Notes = append([]string(nil), cs.Notes...)
	res.Advisories = append([]string(nil), cs.Advisories...)
	return &res
}

// resolve turns a change's project-relative path into an absolute one, refusing
// anything that leaves the project.
//
// Changeset is exported and JSON-tagged, so a change set can be built by hand or
// arrive from a UI, and filepath.Join would quietly Clean a "../" away rather
// than reject it — writing outside the directory the user approved. Delete makes
// it worse: an empty path or ".." names the project directory itself.
//
// A rooted path is refused separately from an absolute one because on Windows
// they are not the same question: filepath.IsAbs("/tmp/x") is FALSE there, a
// path being absolute on Windows meaning it carries a drive. Left to Join, that
// path would land at <project>\tmp\x — inside the project, so nothing escapes,
// but not the path the change set declared and not the one a preview showed.
// The rule this upholds is that Apply performs what Plan decided, so a path it
// cannot perform faithfully is refused rather than reinterpreted.
func (c *Change) resolve(dir string) (string, error) {
	if c.Path == "" {
		return "", fmt.Errorf("change of kind %q has no path", c.Kind)
	}
	clean := filepath.Clean(filepath.FromSlash(c.Path))
	rooted := strings.HasPrefix(clean, string(filepath.Separator))
	if filepath.IsAbs(clean) || rooted || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("change path %q leaves the project directory", c.Path)
	}
	return filepath.Join(dir, clean), nil
}

func (c *Change) apply(dir string) error {
	path, err := c.resolve(dir)
	if err != nil {
		return err
	}
	switch c.Kind {
	case CreateDir:
		// MkdirAll rather than Mkdir: the directory may have appeared since Plan
		// looked, and a person creating dags/ while deciding whether to accept
		// the preview should not abort the run half-applied.
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", c.Path, err)
		}
	case CreateFile:
		if c.Content == nil {
			return fmt.Errorf("%w: %s has no content to write", ErrChangedOnDisk, c.Path)
		}
		// The parent is made for the same reason CreateDir uses MkdirAll: a
		// change set is reviewed at human speed, and the project can move under
		// it. Every CreateFile used to target the project root, which Apply has
		// already made, so the write could assume its directory. dags/exampledag.py
		// is the first that cannot — its dags/ is only planned as a CreateDir when
		// it was absent at Plan time, so a project that HAD the directory and lost
		// it while the preview was on screen would fail the write instead.
		if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(c.Path), err)
		}
		if err := os.WriteFile(path, c.Content, filePerm); err != nil {
			return fmt.Errorf("creating %s: %w", c.Path, err)
		}
	case UpdateFile:
		if c.Content == nil {
			return fmt.Errorf("%w: %s has no content to write", ErrChangedOnDisk, c.Path)
		}
		// An update edits a file that is already there, and the distinction is
		// not pedantic: os.WriteFile would happily CREATE one, so a file the
		// user deleted while reviewing would come back — at filePerm rather than
		// its own mode, carrying content they had thrown away.
		//
		// Opened without O_CREATE for the same reason, and written in place
		// rather than through a rename: the file is the user's, and a rename
		// drops its mode, writes through a read-only bit, and turns a symlink
		// into a regular file.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, filePerm)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%w: %s was there when this was planned and is not now", ErrChangedOnDisk, c.Path)
			}
			return fmt.Errorf("writing %s: %w", c.Path, err)
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(c.Content); err != nil {
			return fmt.Errorf("writing %s: %w", c.Path, err)
		}
	case CreateSymlink:
		if err := os.Symlink(c.Target, path); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("linking %s: %w", c.Path, err)
		}
	case Delete:
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", c.Path, err)
		}
	default:
		// Kind is a plain string on an exported type, so an unknown one is
		// reachable: a hand-built change set, or a kind added after the caller
		// was compiled. Silently doing nothing would report a run as successful
		// while dropping changes the user approved.
		return fmt.Errorf("unknown change kind %q for %s", c.Kind, c.Path)
	}
	return nil
}
