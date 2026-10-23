package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/uv"
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
			"and a Dockerfile that does more than pick a base image is kept as the project's build, moved onto its tag's -base image so it installs from pyproject.toml and uv.lock " +
			"(with .astro/install-project.py, to commit beside it). A 1.x file is removed once everything it said is in pyproject.toml, " +
			"except requirements.txt and packages.txt when a Dockerfile left on its ONBUILD tag still installs them, and other files already there are kept. " +
			"What init could not carry over, such as a requirements.txt line that is not a plain requirement, is listed at the end, to move across by hand.\n\n" +
			"Run in a project that is already an Astro project, it brings what astro writes there up to date and changes nothing else: " +
			"a declared Dockerfile still on an ONBUILD tag is moved onto its -base image as a conversion would, the .astro/install-project.py it runs is written or refreshed, " +
			"and the runtime pins in [tool.uv] move to the newest Astro Runtime build, with uv lock run so uv.lock follows them.",
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
	// A project that is already one is brought up to date rather than
	// refused: init is the one command that writes what astro keeps in a
	// project, so it is the one that refreshes it.
	opts.UpdateExisting = true
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
	// it found. What the runtime group left out is said here, though: a start
	// says it only when the group changes, which after a first init it does
	// not.
	build := c.writeAstroBuild(ctx, res.Dir)
	// On stderr as text in either format: init's stdout is its one result.
	notes := cliout.Renderer{Format: cliout.FormatText, Out: c.d.Stderr}
	if res.Existing {
		// Bringing a project up to date is where a Docker-only project moves
		// to a newer runtime build, so it says what moved.
		c.reportAstroBuild(notes, build)
		// The refresh changed the manifest, so the project was not up to date.
		if (build.pinsChanged || build.airflowChanged || build.groupChanged) && !slices.Contains(res.Updated, manifest.Marker) {
			res.Updated = append(res.Updated, manifest.Marker)
		}
	}
	// A uv.lock already there, in a project this run adopted or converted as
	// in one it brought up to date, was resolved without what init just
	// wrote into [tool.uv], so it follows.
	c.relockAfterInit(ctx, notes, res.Dir, build)
	// reportAstroBuild already said it when the group changed.
	if !res.Existing || !build.groupChanged {
		c.reportGroupLeftOut(build)
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderInit(w, res, nextStart(res.Dir))
	})
}

// relockAfterInit runs `uv lock` in a project that has a uv.lock, so the lock
// follows pyproject.toml as init left it: the runtime pins or the Airflow
// build may have moved, and the images install the lock's versions and build
// on the runtime the pins name, so a lock still resolved for the old build
// would install that build's versions over the new one. It runs whether or not
// init changed [tool.uv], which is what makes "run astro init" the one advice
// for a lock that is out of date; uv lock leaves a lock that is not as it is.
// With no uv to run, it says to run it when [tool.uv] changed. A project with
// no uv.lock has nothing to follow.
func (c *cli) relockAfterInit(ctx context.Context, r cliout.Renderer, dir string, build astroBuildWrite) {
	if _, err := os.Stat(filepath.Join(dir, "uv.lock")); err != nil {
		return
	}
	changed := build.pinsChanged || build.airflowChanged || build.groupChanged
	stale := fmt.Sprintf("uv.lock was resolved before %s's [tool.uv] changed: run uv lock so it follows", manifest.Marker)
	if c.d.Relock == nil {
		if changed {
			emitWarning(r, event{Event: "warning", Text: stale})
		}
		return
	}
	switch err := c.d.Relock(ctx, dir); {
	case errors.Is(err, uv.ErrNotFound):
		if changed {
			emitWarning(r, event{Event: "warning", Text: stale + ", once uv is installed"})
		}
	case err != nil:
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf("uv.lock could not be brought up to date with %s: uv lock failed: %v", manifest.Marker, err)})
	case changed:
		fmt.Fprintf(c.d.Stderr, "uv.lock: relocked for %s's [tool.uv]\n", manifest.Marker)
	}
}

// uvRelock is Deps.Relock in production: uv lock in dir, with the uv and the
// cache standalone mode uses.
func uvRelock(ctx context.Context, dir string) error {
	root, err := localrt.CacheRoot()
	if err != nil {
		return err
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
	if err != nil {
		return err
	}
	return client.Lock(ctx, dir, uv.Stdio{})
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
	switch {
	case res.Existing && res.OnBuildKept:
		if _, err := fmt.Fprintf(w, "Astro project %s in %s: its Dockerfile is kept on its ONBUILD base; see below\n", res.Name, res.Dir); err != nil {
			return err
		}
	case res.Existing && len(res.Created)+len(res.Updated)+len(res.Deleted) == 0:
		if _, err := fmt.Fprintf(w, "Astro project %s in %s is up to date\n", res.Name, res.Dir); err != nil {
			return err
		}
	case res.Existing:
		if _, err := fmt.Fprintf(w, "Brought Astro project %s in %s up to date\n", res.Name, res.Dir); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(w, "%s Astro project %s (Airflow %s%s) in %s\n", verb, res.Name, res.AirflowVersion, defaultSourceNote(res.AirflowDefaultSource), res.Dir); err != nil {
			return err
		}
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
