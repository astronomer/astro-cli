package local

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/scaffold"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/uv"
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
	var from string
	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Scaffold a new Astro project",
		Long: "Create a pyproject.toml-based Astro project in the given directory (default: the current one).\n\n" +
			"With --from <dir>, import a plain-Airflow repo (a dags/ folder and a requirements.txt) into a\n" +
			"new project instead: the manifest carries the requirements, the dags are copied, and uv locks\n" +
			"the result. The source repo is never touched.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if from != "" {
				return c.runImport(cmd.Context(), from, dir, opts)
			}
			return c.runInit(dir, opts)
		},
	}
	cmd.Flags().StringVar(&opts.AirflowVersion, "airflow-version", "",
		"Airflow version to pin in the manifest (default: "+scaffold.DefaultAirflowVersion+")")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Project name (default: the directory name)")
	cmd.Flags().StringVar(&from, "from", "", "Import a plain-Airflow repo from this directory instead of scaffolding an empty project")
	return cmd
}

func (c *cli) runInit(dir string, opts scaffold.Options) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, err = c.resolveDir(dir)
	if err != nil {
		return err
	}
	res, err := scaffold.Run(dir, opts)
	if err != nil {
		return err
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderInit(w, res)
	})
}

func (c *cli) runImport(ctx context.Context, src, dst string, opts scaffold.Options) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dst, err = c.resolveDir(dst)
	if err != nil {
		return err
	}
	iopts := scaffold.ImportOptions{Name: opts.Name, AirflowVersion: opts.AirflowVersion}
	if locker, err := newImportLocker(ctx); err == nil {
		iopts.Lock = locker
	}
	// Live uv output belongs only in text mode; json mode is one object.
	if r.Format == FormatText {
		iopts.LockOutput = c.d.Stdout
	}
	res, err := scaffold.Import(ctx, src, dst, iopts)
	if err != nil {
		return err
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderImport(w, res)
	})
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

// newImportLocker builds the uv-backed Locker for import. It is a package var
// so tests can drive the import path without a real uv binary.
var newImportLocker = func(ctx context.Context) (scaffold.Locker, error) {
	root, err := localrt.CacheRoot()
	if err != nil {
		return nil, err
	}
	return uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
}

func renderInit(w io.Writer, res *scaffold.Result) error {
	if _, err := fmt.Fprintf(w, "Created astro project %s (Airflow %s) in %s\n", res.Name, res.AirflowVersion, res.Dir); err != nil {
		return err
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
	_, err := fmt.Fprintf(w, "\nNext: %s\n", replaceStart)
	return err
}

func renderImport(w io.Writer, res *scaffold.ImportResult) error {
	if _, err := fmt.Fprintf(w, "Imported %s from %s into %s\n", res.Name, res.Source, res.Dir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Airflow %s (%s)\n", res.Airflow, res.AirflowFrom); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %d dags, %d dependencies carried into pyproject.toml\n", res.Dags, res.Dependencies); err != nil {
		return err
	}
	if res.Plugins > 0 {
		if _, err := fmt.Fprintf(w, "  %d plugin files copied\n", res.Plugins); err != nil {
			return err
		}
	}
	for _, warn := range res.Warnings {
		if _, err := fmt.Fprintf(w, "  note: %s\n", warn); err != nil {
			return err
		}
	}
	if err := renderLockReport(w, res.Lock); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nNext: %s\n", replaceStart)
	return err
}

func renderLockReport(w io.Writer, lock scaffold.LockReport) error {
	switch {
	case !lock.Attempted:
		_, err := fmt.Fprintf(w, "  lock: not attempted\n")
		return err
	case lock.Locked:
		_, err := fmt.Fprintf(w, "  lock: resolved; uv.lock written\n")
		return err
	}
	if _, err := fmt.Fprintf(w, "  lock: did not resolve — the project is scaffolded; fix pyproject.toml and run `%s`\n", replaceStart); err != nil {
		return err
	}
	if lock.Summary != "" {
		if _, err := fmt.Fprintf(w, "    %s\n", lock.Summary); err != nil {
			return err
		}
	}
	if len(lock.Packages) > 0 {
		if _, err := fmt.Fprintf(w, "    packages: %v\n", lock.Packages); err != nil {
			return err
		}
	}
	if len(lock.Constraints) > 0 {
		if _, err := fmt.Fprintf(w, "    constraints: %v\n", lock.Constraints); err != nil {
			return err
		}
	}
	if lock.Summary == "" && len(lock.Packages) == 0 && lock.Error != "" {
		if _, err := fmt.Fprintf(w, "    %s\n", lock.Error); err != nil {
			return err
		}
	}
	return nil
}
