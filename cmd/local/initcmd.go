package local

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

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
	addOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

func newInitCmd(c *cli) *cobra.Command {
	var opts scaffold.Options
	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Make a directory an Astro project",
		Long: "Create a pyproject.toml-based Astro project in the given directory (default: the current one).\n\n" +
			"Run it in the Airflow repo you already have: a directory with no pyproject.toml is scaffolded, and one that has a pyproject.toml gains a [tool.astro] section, leaving the rest of the file alone.\n\n" +
			"Files already there are kept. What init found but could not carry over — a requirements.txt, a Dockerfile — is listed at the end, to move across by hand.",
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
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, err = c.resolveDir(dir)
	if err != nil {
		return err
	}
	// A v1 airflow_settings.yaml's connection and variable values go to
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

	res, err := scaffold.Run(dir, opts)
	if err != nil {
		return err
	}
	// After the manifest is written rather than inside scaffold.Plan, which
	// stays offline. A lookup that fails is not reported here, since init has
	// no to-do in it: `astro local start` makes the same lookup and says what
	// it found.
	c.writeAstroBuild(ctx, res.Dir)
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
	for _, entry := range res.Updated {
		if _, err := fmt.Fprintf(w, "  %s\n", entry); err != nil {
			return err
		}
	}
	for _, entry := range res.Created {
		if _, err := fmt.Fprintf(w, "  %s\n", entry); err != nil {
			return err
		}
	}
	for _, entry := range res.Skipped {
		if _, err := fmt.Fprintf(w, "  %s (already existed, kept)\n", entry); err != nil {
			return err
		}
	}
	// Removed files get their own line and their own word. They arrive in their
	// own list for the same reason: rendered beside Updated, in the unlabelled
	// block above, a destroyed file reads exactly like an edited one.
	for _, entry := range res.Deleted {
		if _, err := fmt.Fprintf(w, "  Removed %s\n", entry); err != nil {
			return err
		}
	}
	if err := renderChanged(w, res.Advisories); err != nil {
		return err
	}
	if err := renderLeftToDo(w, res.Notes); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nNext: %s\n", next)
	return err
}

// renderLeftToDo prints what init could not carry over, under a heading. Each
// line names a file and where its contents belong.
// renderChanged prints the advisories: things this run DID that now behave
// differently. They are not "left to do" and must not be rendered as it — a
// carried default is applied at start now, and a connection declared with no
// value will stop the project starting until it is set. Nothing else on screen
// says either, and until this existed the CLI computed them and dropped them,
// so the only reader was --json.
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

func renderLeftToDo(w io.Writer, notes []string) error {
	if len(notes) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nLeft to do:\n"); err != nil {
		return err
	}
	for _, note := range notes {
		if _, err := fmt.Fprintf(w, "  %s\n", note); err != nil {
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
// A value is asked about only when a v1 airflow_settings.yaml holds one, so the
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
