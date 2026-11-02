package local

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// NewInitCmd builds the root-level `astro init`. The same constructor backs
// `astro local init`, so both spellings stay one implementation.
func NewInitCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newInitCmd(c)
	cliout.AddOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

func newInitCmd(c *cli) *cobra.Command {
	var opts scaffold.Options
	cmd := &cobra.Command{
		Use:   "init [DIRECTORY]",
		Short: "Make a directory an Astro project",
		Long: "Create a pyproject.toml-based Astro project in the given directory (default: the current one).\n\n" +
			"Run it in the Airflow repo you already have: a directory with no pyproject.toml is scaffolded, and one that has a pyproject.toml gains a [tool.astro] section, leaving the rest of the file alone.\n\n" +
			"Run in an Astro CLI 1.x project, it converts it in place: requirements.txt and packages.txt are carried into pyproject.toml, what airflow_settings.yaml declares is carried over, " +
			"and a Dockerfile that only picks a runtime is removed, its runtime version and Python recorded in pyproject.toml. Any other Dockerfile is kept as the project's build, as written. " +
			"A 1.x file is removed once everything it said is in pyproject.toml, " +
			"except requirements.txt and packages.txt when a kept Dockerfile still installs them, and other files already there are kept. " +
			"What init could not carry over, such as a requirements.txt line that is not a plain requirement, is listed at the end, to move across by hand.",
		Example: "  # Make the current directory an Astro project\n" +
			"  astro init\n\n" +
			"  # Create a new project in its own directory, pinned to Airflow 3.1\n" +
			"  astro init my-project --airflow-version 3.1",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return c.runInit(cmd.Context(), dir, opts)
		},
	}
	cmd.Flags().StringVar(&opts.AirflowVersion, "airflow-version", "",
		"Airflow version to pin in the manifest (default: the pin already in the manifest, else the newest supported Airflow series)")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Project name (default: the directory name)")
	return cmd
}

func (c *cli) runInit(ctx context.Context, dir string, opts scaffold.Options) error {
	r := c.renderer()
	dir, err := c.resolveDir(dir)
	if err != nil {
		return err
	}
	// A 1.x airflow_settings.yaml's connection and variable values go to
	// the shared vault at this project's scope rather than into the manifest.
	// The writer is what knows that scope; see scaffold.SecretWriter.
	opts.SecretWriter = &lazyVaultWriter{dir: dir}
	// Called by scaffold only when nothing in the project states an Airflow,
	// so converting a pinned project makes no request. The lookup never fails
	// init: offline, it answers the built-in series.
	if c.d.AirflowDefault != nil {
		opts.Default = func() (string, string, runtimeversions.Source) {
			return c.d.AirflowDefault(ctx)
		}
	}
	// There is one project format, and init converts a 1.x project to it the
	// same way on Astro and on Astro Private Cloud, which deploys
	// pyproject.toml projects too. The one thing the context decides is the
	// kind of link a 1.x project's saved deploy target becomes: Houston's ids
	// and Astro's are both cuids, so under an APC context it is an apc link on
	// that context's domain, the one astro link add would write there.
	// A saved Deployment id with no workspace beside it was looked up, by
	// Astro CLI 1.x's deploy, in the context's current workspace, so that is
	// the link's, read from the config without resolving the login.
	if apc := c.d.APC; apc != nil {
		opts.APCDomain = apc.Domain
		if apc.CurrentWorkspace != nil {
			opts.APCWorkspace = func() string {
				ws, err := apc.CurrentWorkspace()
				if err != nil {
					return ""
				}
				return ws
			}
		}
	}

	res, err := scaffold.Run(dir, opts)
	if err != nil {
		return err
	}
	// After the manifest is written rather than inside scaffold.Plan, which
	// stays offline. A lookup that fails is not reported here, since init has
	// no to-do in it: `astro local start` makes the same lookup and says what
	// it found. uv.lock follows what it wrote; nothing written, nothing locked.
	if build := c.writeAstroBuild(ctx, res.Dir); build.changed {
		c.lockAfterInit(ctx, res.Dir)
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderInit(w, res, nextStart(res.Dir))
	})
}

// nextStart is the start command to suggest once init is done. Standalone
// mode builds no image, so a project whose manifest declares a Dockerfile or
// OS packages starts as it will run only in Docker mode.
func nextStart(dir string) string {
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	if err != nil || (m.Astro.Dockerfile == "" && len(m.Astro.Packages) == 0) {
		return replaceStart
	}
	return replaceStart + " --docker"
}

// defaultSourceNote says where a defaulted Airflow came from, for the line that
// names it: the runtime catalog, a cached copy of it, or the series built into
// this binary, and for that last one why. Nothing when a flag or the project's
// own pin decided it.
func defaultSourceNote(src runtimeversions.Source) string {
	switch src {
	case runtimeversions.SourceCatalog:
		return ", the latest from the runtime catalog"
	case runtimeversions.SourceCache:
		return ", the latest in the cached runtime catalog"
	case runtimeversions.SourceStaleCache:
		return ", the latest in an old cached copy of the runtime catalog; could not reach the catalog"
	case runtimeversions.SourceFallback:
		return ", the built-in default; could not reach the runtime catalog"
	case runtimeversions.SourceCatalogEmpty:
		return ", the built-in default; the runtime catalog lists no usable Airflow 3 release"
	case runtimeversions.SourceBuiltIn:
		return ", the built-in default"
	}
	return ""
}

// resolveDir turns a possibly relative directory into an absolute one against
// the process working directory, so commands never call os.Getwd themselves.
func (c *cli) resolveDir(dir string) (string, error) {
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	wd, err := c.d.WorkingDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, dir), nil
}

func renderInit(w io.Writer, res *scaffold.Result, next string) error {
	// "Adopted" and "Created" are the two shapes Run has: a manifest that was
	// already there and gained a section, or one this run wrote. Other files
	// can be updated on either path, so the manifest's own fate decides.
	verb := "Created"
	if res.Adopted {
		verb = "Adopted"
	}
	if _, err := fmt.Fprintf(w, "%s Astro project %s (Airflow %s%s) in %s\n", verb, res.Name, res.AirflowVersion, defaultSourceNote(res.AirflowDefaultSource), res.Dir); err != nil {
		return err
	}
	rows, changed := initRows(res)
	if err := renderRows(w, rows); err != nil {
		return err
	}
	if err := renderChanged(w, changed); err != nil {
		return err
	}
	if err := renderInfo(w, res.Info); err != nil {
		return err
	}
	if err := renderLeftToDo(w, res.Notes); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%s %s\n", cliout.NextLabel(w), next)
	return err
}

type initRow struct{ name, what string }

// Labels for the rows that gather several files.
const (
	rowCreated = "Created"
	rowRemoved = "Removed"
	rowKept    = "Kept"
)

// initRows is init's file list, one change a line with the file leading, and
// the advisories left for What changed. An advisory about one file the run
// changed ("pyproject.toml: …") is a line of that file. Files created with
// nothing more to say share one Created line, files only kept one Kept line
// (leaving out any the run also changed), and removed files a Removed line
// per reason.
func initRows(res *scaffold.Result) (rows []initRow, changed []string) {
	touched := map[string]bool{}
	var created []string
	add := func(label, fallback string) {
		name, what := splitLabel(label)
		touched[name] = true
		if what == "" && fallback == "" {
			created = append(created, name)
			return
		}
		rows = append(rows, initRow{name, cmp.Or(what, fallback)})
	}
	for _, l := range res.Updated {
		add(l, "updated")
	}
	for _, l := range res.Created {
		add(l, "")
	}
	for _, a := range res.Advisories {
		name, what, ok := strings.Cut(a, ": ")
		if !ok || !touched[name] {
			changed = append(changed, a)
			continue
		}
		rows = append(rows, initRow{name, what})
	}
	// Sorted by file, stable, so a file's lines sit together in the order
	// they were reported.
	first := map[string]int{}
	for i, r := range rows {
		if _, ok := first[r.name]; !ok {
			first[r.name] = i
		}
	}
	slices.SortStableFunc(rows, func(a, b initRow) int { return first[a.name] - first[b.name] })

	// A file created bare joins the Created line, unless lines above already
	// say what went into it.
	created = slices.DeleteFunc(created, func(name string) bool {
		return slices.ContainsFunc(rows, func(r initRow) bool { return r.name == name })
	})
	if len(created) > 0 {
		rows = append(rows, initRow{rowCreated, strings.Join(created, ", ")})
	}

	var reasons []string
	removed := map[string][]string{}
	for _, l := range res.Deleted {
		name, why := splitLabel(l)
		if _, ok := removed[why]; !ok {
			reasons = append(reasons, why)
		}
		removed[why] = append(removed[why], name)
	}
	for _, why := range reasons {
		what := strings.Join(removed[why], ", ")
		if why != "" {
			what += " (" + why + ")"
		}
		rows = append(rows, initRow{rowRemoved, what})
	}
	var kept []string
	for _, l := range res.Skipped {
		if name, _ := splitLabel(l); !touched[name] {
			kept = append(kept, name)
		}
	}
	if len(kept) > 0 {
		rows = append(rows, initRow{rowKept, strings.Join(kept, ", ")})
	}
	return rows, changed
}

// splitLabel splits a scaffold label, "file (what happened)", into its file
// and what happened; a bare label is all file.
func splitLabel(label string) (name, what string) {
	if i := strings.Index(label, " ("); i > 0 && strings.HasSuffix(label, ")") {
		return label[:i], label[i+2 : len(label)-1]
	}
	return label, ""
}

// renderRows prints the file list with its first column aligned.
func renderRows(w io.Writer, rows []initRow) error {
	width := 0
	for _, r := range rows {
		width = max(width, len(r.name))
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "  %-*s  %s\n", width, r.name, r.what); err != nil {
			return err
		}
	}
	return nil
}

// renderChanged prints the advisories no file line holds: things this run DID
// that now behave differently. They are not "left to do" and must not be
// rendered as it: a carried default is applied at start now, and a connection
// declared with no value will stop the project starting until it is set.
func renderChanged(w io.Writer, advisories []string) error {
	if len(advisories) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nWhat changed:\n"); err != nil {
		return err
	}
	for _, a := range advisories {
		if _, err := fmt.Fprintf(w, "  %s\n", a); err != nil {
			return err
		}
	}
	return nil
}

// renderInfo prints what a person should know and has nothing to do about,
// each as a Note.
func renderInfo(w io.Writer, info []string) error {
	if len(info) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	for _, n := range info {
		cliout.Note(w, n)
	}
	return nil
}

// renderLeftToDo prints init's notes, one per line, except the Deployment
// links a 1.x config listed: those come last, under one line, a command each.
func renderLeftToDo(w io.Writer, notes []string) error {
	if len(notes) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s\n", cliout.TodoLabel(w)); err != nil {
		return err
	}
	var links []string
	for _, note := range notes {
		if cmd, ok := strings.CutPrefix(note, scaffold.LinkNotePrefix); ok {
			links = append(links, cmd)
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s\n", note); err != nil {
			return err
		}
	}
	if len(links) == 0 {
		return nil
	}
	head := "  Link the Deployments .astro/config.yaml lists:\n"
	if len(links) == 1 {
		head = "  Link the Deployment .astro/config.yaml lists:\n"
	}
	if _, err := io.WriteString(w, head); err != nil {
		return err
	}
	for _, cmd := range links {
		if _, err := fmt.Fprintf(w, "    %s\n", cmd); err != nil {
			return err
		}
	}
	return nil
}

// lazyVaultWriter opens the vault on the first value it is asked about or
// asked to store, rather than when init starts.
//
// `astro init pipelines` scaffolds a directory that does not exist yet, and a
// vault scope is the CANONICAL path of the project directory — which cannot be
// resolved until there is a directory to resolve. Opening eagerly turned every
// init-into-a-new-directory into a failure about symlinks.
//
// A value is asked about only when a 1.x airflow_settings.yaml holds one, so the
// directory already exists. A project with nothing to carry never opens the
// vault at all, which is most of them.
type lazyVaultWriter struct {
	dir string
	w   *vaultenv.Writer
}

func (l *lazyVaultWriter) SetSecret(kind secrets.Kind, name, value string) error {
	w, err := l.writer()
	if err != nil {
		return err
	}
	return w.SetSecret(kind, name, value)
}

func (l *lazyVaultWriter) HasSecret(kind secrets.Kind, name string) (bool, error) {
	w, err := l.writer()
	if err != nil {
		return false, err
	}
	return w.HasSecret(kind, name)
}

func (l *lazyVaultWriter) HasSecretValue(kind secrets.Kind, name, value string) (scaffold.SecretHeld, error) {
	w, err := l.writer()
	if err != nil {
		return scaffold.SecretHeld{}, err
	}
	return w.HasSecretValue(kind, name, value)
}

func (l *lazyVaultWriter) writer() (*vaultenv.Writer, error) {
	if l.w == nil {
		w, err := vaultenv.NewWriter(l.dir)
		if err != nil {
			return nil, err
		}
		l.w = w
	}
	return l.w, nil
}
