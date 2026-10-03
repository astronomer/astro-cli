package local

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Link state decides which projects a GLOBAL vault entry reaches. It lives in
// the vault's link index, shared with Astro Desktop, keyed by canonical project
// path: no row reaches every project, a row reaches only the paths it names. A
// link to a project reaches its git worktrees too, because a checkout matches a
// link by its own path or by its project home (localrt.ProjectHome). See
// docs/v2-secrets.md.

const (
	envLinkStatusLinked   = "linked"
	envLinkStatusUnlinked = "unlinked"
)

// reachPath is one linked project in the JSON reach object.
type reachPath struct {
	Path string `json:"path"`
	// Exists is false for a path that is no longer there: a moved or deleted
	// project, which the link still names.
	Exists bool `json:"exists"`
}

// reachJSON is where a global vault entry is linked, as get and link report it.
type reachJSON struct {
	// AutoLink is true when the entry has no link row: it is auto-linked to every
	// project. Projects is then empty.
	AutoLink bool        `json:"auto_link"`
	Projects []reachPath `json:"projects"`
	// Error is set when the link index cannot be used. No global resolves in
	// any project while it stands, whatever the row would say.
	Error string `json:"error,omitempty"`
}

func newReachJSON(r secrets.Reach) *reachJSON {
	out := &reachJSON{AutoLink: r.Everywhere, Projects: []reachPath{}}
	for _, p := range r.Projects {
		_, err := os.Stat(p)
		out.Projects = append(out.Projects, reachPath{Path: p, Exists: err == nil})
	}
	return out
}

// text is the reach for a person: "auto-linked (every project)", the paths with the missing
// ones marked, or "not linked (no project)".
func (r *reachJSON) text() string {
	switch {
	case r.Error != "":
		return "unknown, because " + r.Error + "; no global resolves in any project until it is fixed"
	case r.AutoLink:
		return "auto-linked (every project)"
	case len(r.Projects) == 0:
		return "not linked (no project)"
	}
	parts := make([]string, len(r.Projects))
	for i, p := range r.Projects {
		parts[i] = p.Path
		if !p.Exists {
			parts[i] += " (missing)"
		}
	}
	return strings.Join(parts, ", ")
}

// globalReach is the reach of a global vault entry for get, or nil when the
// writer is not global or holds no such entry. An index that cannot be used
// is reported in the object rather than failing the get: the value is still
// the vault's, and the reach is the part that is unknown.
func globalReach(w *vaultenv.Writer, kind localenv.Kind, name string) *reachJSON {
	r, err := w.Reach(kind, name)
	switch {
	case errors.Is(err, vaultenv.ErrNotGlobal), errors.Is(err, vaultenv.ErrNotHeld):
		return nil
	case err != nil:
		return &reachJSON{Projects: []reachPath{}, Error: err.Error()}
	}
	return newReachJSON(r)
}

// envLinkResult is what link and unlink report.
type envLinkResult struct {
	Kind   localenv.Kind `json:"kind"`
	Name   string        `json:"name"`
	Status string        `json:"status"`
	Reach  reachJSON     `json:"reach"`
}

func newEnvLinkCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	var thisCheckout, everywhere bool
	noun := localenv.Noun(k.kind)
	cmd := &cobra.Command{
		Use:   "link " + k.arg + " [DIR...]",
		Short: "Limit a global " + k.label + " in the vault to specific projects",
		Long: "Link a global " + k.label + " in the encrypted vault to projects, so it reaches\n" +
			"those projects and no others. With no link it is auto-linked to every project.\n\n" +
			"Each DIR is a project directory; the default is the current project. A link\n" +
			"to a project also reaches its git worktrees. --this-checkout links the\n" +
			"checkout itself instead, so a worktree gets the value and its main project\n" +
			"does not. --auto-link removes the links, and the value is auto-linked to every project\n" +
			"again.\n\n" +
			"Only global vault entries have links: a project secret already reaches only\n" +
			"its own project.\n\n" +
			"Whatever reaches a project is passed to its Airflow at start, declared or not,\n" +
			"so linking is how to keep a global out of the projects that should not get\n" +
			"it. Astro Desktop reads the same links.",
		Example: `
  # limit it to the current project (and its worktrees)
  astro local env ` + noun + ` link ` + exampleName(k.kind) + `

  # add two more projects
  astro local env ` + noun + ` link ` + exampleName(k.kind) + ` ~/src/etl ~/src/reports

  # link only this worktree, not its main project
  astro local env ` + noun + ` link ` + exampleName(k.kind) + ` --this-checkout

  # auto-link it to every project again
  astro local env ` + noun + ` link ` + exampleName(k.kind) + ` --auto-link`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return c.runEnvLink(scope, k.kind, args[0], args[1:], thisCheckout, everywhere)
		},
	}
	cmd.Flags().BoolVar(&thisCheckout, "this-checkout", false, "Link each checkout's own path rather than its project, so a worktree does not share it with its main project")
	cmd.Flags().BoolVar(&everywhere, "auto-link", false, "Remove the links, so it is auto-linked to every project")
	cmd.MarkFlagsMutuallyExclusive("this-checkout", "auto-link")
	return cmd
}

func newEnvUnlinkCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	noun := localenv.Noun(k.kind)
	return &cobra.Command{
		Use:   "unlink " + k.arg + " [DIR...]",
		Short: "Stop a global " + k.label + " in the vault reaching specific projects",
		Long: "Unlink a global " + k.label + " in the encrypted vault from projects. Each DIR\n" +
			"is a project directory, or a linked path that no longer exists; the default\n" +
			"is the current project. Afterwards the checkout is not reached, whether it\n" +
			"was linked as a project or on its own.\n\n" +
			"An entry with no links is auto-linked to every project, so there is nothing to unlink\n" +
			"it from: link it to the projects it should reach instead. Unlinking the last\n" +
			"project leaves it reaching no project.",
		Example: `
  # stop it reaching the current project
  astro local env ` + noun + ` unlink ` + exampleName(k.kind) + `

  # drop a project that was moved away
  astro local env ` + noun + ` unlink ` + exampleName(k.kind) + ` /Users/me/old-etl`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return c.runEnvUnlink(scope, k.kind, args[0], args[1:])
		},
	}
}

// exampleName is a plausible global name of each kind, for the examples.
func exampleName(kind localenv.Kind) string {
	switch kind {
	case localenv.KindConn:
		return "warehouse"
	case localenv.KindVar:
		return "slack_channel"
	case localenv.KindEnv:
		return "SLACK_TOKEN"
	default:
		return "SLACK_TOKEN"
	}
}

func (c *cli) runEnvLink(scope *scopeFlags, kind localenv.Kind, name string, dirs []string, thisCheckout, everywhere bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	if everywhere && len(dirs) > 0 {
		return errors.New("--auto-link reaches every project, so it takes no directories")
	}
	w, err := c.linkWriter(scope, kind, name)
	if err != nil {
		return err
	}
	var paths []string
	if !everywhere {
		if paths, err = c.linkPaths(dirs, thisCheckout); err != nil {
			return err
		}
	}
	before, after, err := w.EditReach(kind, name, func(before secrets.Reach) (secrets.Reach, error) {
		if everywhere {
			return secrets.Reach{Everywhere: true}, nil
		}
		next := paths
		if !before.Everywhere {
			next = append(slices.Clone(before.Projects), paths...)
		}
		slices.Sort(next)
		return secrets.Reach{Projects: slices.Compact(next)}, nil
	})
	if err != nil {
		return linkEditErr(w, kind, name, err)
	}
	reach := newReachJSON(after)
	res := envLinkResult{Kind: kind, Name: name, Status: envLinkStatusLinked, Reach: *reach}
	return r.Emit(res, func(out io.Writer) error {
		var msg string
		switch {
		case everywhere && before.Everywhere:
			msg = fmt.Sprintf("%s %s is already auto-linked to every project\n", localenv.Noun(kind), name)
		case everywhere:
			msg = fmt.Sprintf("%s %s is now auto-linked to every project\n", localenv.Noun(kind), name)
		case before.Everywhere:
			msg = fmt.Sprintf("%s %s was auto-linked to every project; it now reaches only %s\n", localenv.Noun(kind), name, reach.text())
		default:
			msg = fmt.Sprintf("linked %s %s to %s\nReach: %s\n", localenv.Noun(kind), name, strings.Join(paths, ", "), reach.text())
		}
		_, werr := io.WriteString(out, msg)
		return werr
	})
}

func (c *cli) runEnvUnlink(scope *scopeFlags, kind localenv.Kind, name string, dirs []string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	w, err := c.linkWriter(scope, kind, name)
	if err != nil {
		return err
	}
	targets, err := c.unlinkTargets(dirs)
	if err != nil {
		return err
	}
	var removed []string
	_, after, err := w.EditReach(kind, name, func(before secrets.Reach) (secrets.Reach, error) {
		if before.Everywhere {
			return before, fmt.Errorf("%s %s has no links, so it is auto-linked to every project and there is nothing to unlink it from. "+
				"To limit it, link it to the projects it should reach: %s [DIR...]",
				localenv.Noun(kind), name, localenv.LinkHint(kind, name))
		}
		kept := make([]string, 0, len(before.Projects))
		for _, p := range before.Projects {
			if slices.Contains(targets, p) {
				removed = append(removed, p)
				continue
			}
			kept = append(kept, p)
		}
		if len(removed) == 0 {
			return before, fmt.Errorf("%s %s is not linked to %s; it reaches %s",
				localenv.Noun(kind), name, strings.Join(dirsOrCurrent(dirs), ", "), newReachJSON(before).text())
		}
		return secrets.Reach{Projects: kept}, nil
	})
	if err != nil {
		return linkEditErr(w, kind, name, err)
	}
	if len(after.Projects) == 0 {
		fmt.Fprintf(c.d.Stderr, "warning: %s %s now reaches no project. Link it to one with: %s, or auto-link it to every project with: %s --auto-link\n",
			localenv.Noun(kind), name, localenv.LinkHint(kind, name), localenv.LinkHint(kind, name))
	}
	reach := newReachJSON(after)
	res := envLinkResult{Kind: kind, Name: name, Status: envLinkStatusUnlinked, Reach: *reach}
	return r.Emit(res, func(out io.Writer) error {
		_, werr := fmt.Fprintf(out, "unlinked %s %s from %s\nReach: %s\n", localenv.Noun(kind), name, strings.Join(removed, ", "), reach.text())
		return werr
	})
}

func dirsOrCurrent(dirs []string) []string {
	if len(dirs) == 0 {
		return []string{"the current project"}
	}
	return dirs
}

// linkEditErr explains a link edit that did not happen. A link index this
// build cannot use is named, since the fix is to that file, and the index is
// never overwritten: rewriting it would lose what it holds, or drop
// restrictions a newer build wrote.
func linkEditErr(w *vaultenv.Writer, kind localenv.Kind, name string, err error) error {
	switch {
	case errors.Is(err, secrets.ErrLinksTooNew):
		return fmt.Errorf("cannot change which projects %s %s reaches: %w. Nothing was changed. "+
			"Use a newer astro (or the tool that wrote %s) to edit links", localenv.Noun(kind), name, err, w.LinksPath())
	case errors.Is(err, secrets.ErrLinksUnreadable):
		return fmt.Errorf("cannot change which projects %s %s reaches: %w. Nothing was changed. "+
			"Repair %s by hand; while it cannot be read, no global resolves in any project", localenv.Noun(kind), name, err, w.LinksPath())
	}
	return err
}

// linkWriter is the global vault writer for a name the global vault holds,
// or the reason the name cannot be linked: it is a project value, or not in
// the vault at all.
func (c *cli) linkWriter(scope *scopeFlags, kind localenv.Kind, name string) (*vaultenv.Writer, error) {
	noun := localenv.Noun(kind)
	if scope.project {
		return nil, fmt.Errorf("links apply to global vault entries only; a project's own %s already reaches that project and no other, so drop --project", noun)
	}
	w, err := vaultenv.NewWriter("")
	if err != nil {
		return nil, err
	}
	has, err := w.Has(kind, name)
	if err != nil {
		return nil, err
	}
	if has {
		return w, nil
	}
	if projectDir, perr := c.discoverProject(); perr == nil {
		if pw, err := vaultenv.NewWriter(projectDir); err == nil {
			if ok, _ := pw.Has(kind, name); ok { //nolint:errcheck // an unreadable project vault just skips this reason
				return nil, fmt.Errorf("%s %s is in this project's vault, so it already reaches only this project. "+
					"Links apply to global vault entries only", noun, name)
			}
		}
	}
	return nil, fmt.Errorf("the vault holds no global %s %s, so there is nothing to link. Set one with: astro local env %s set %s --global",
		noun, name, noun, name)
}

// linkPaths is the canonical path each directory links under: its project
// home, so the link reaches the project's worktrees too, or with
// thisCheckout the checkout's own path. No directories means the current
// project.
func (c *cli) linkPaths(dirs []string, thisCheckout bool) ([]string, error) {
	roots, err := c.linkRoots(dirs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(roots))
	for _, dir := range roots {
		fi, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("cannot link %s: %w", dir, err)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("cannot link %s: not a directory", dir)
		}
		resolve := localrt.ProjectHome
		if thisCheckout {
			resolve = localrt.CanonicalPath
		}
		p, err := resolve(dir)
		if err != nil {
			return nil, fmt.Errorf("cannot link %s: %w", dir, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// unlinkTargets is every spelling a link to each directory could have: the
// project home and the checkout's own path, so the checkout stops being
// reached whichever way it was linked. A directory that is gone cannot be
// canonicalized, and is matched as its absolute path, which is how a stale
// link is removed.
func (c *cli) unlinkTargets(dirs []string) ([]string, error) {
	roots, err := c.linkRoots(dirs)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, dir := range roots {
		path, err := localrt.CanonicalPath(dir)
		if err != nil {
			out = append(out, filepath.Clean(dir))
			continue
		}
		out = append(out, path)
		if home, err := localrt.ProjectHome(dir); err == nil {
			out = append(out, home)
		}
	}
	return out, nil
}

// linkRoots turns the positional directories into absolute project roots:
// each one's enclosing project when it is inside one, else the directory
// itself. None means the current project.
func (c *cli) linkRoots(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		dir, err := c.discoverProject()
		if err != nil {
			return nil, fmt.Errorf("not inside a project, so there is no current project to use: pass the project directory: %w", err)
		}
		return []string{dir}, nil
	}
	wd, err := c.d.WorkingDir()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if strings.HasPrefix(d, "~"+string(filepath.Separator)) || d == "~" {
			if home, herr := os.UserHomeDir(); herr == nil {
				d = filepath.Join(home, strings.TrimPrefix(d, "~"))
			}
		}
		if !filepath.IsAbs(d) {
			d = filepath.Join(wd, d)
		}
		d = filepath.Clean(d)
		// Only an existing directory is walked up from: a stale link names a
		// path that is gone, and walking up from it could land on an
		// enclosing project that was never linked.
		if _, serr := os.Stat(d); serr == nil {
			if proj, perr := project.Discover(d); perr == nil {
				d = proj.Dir
			}
		}
		out = append(out, d)
	}
	return out, nil
}
