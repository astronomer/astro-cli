package local

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/scaffold"
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
		Short: "Scaffold a new Astro project",
		Long:  "Create a pyproject.toml-based Astro project in the given directory (default: the current one).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return c.runInit(dir, opts)
		},
	}
	cmd.Flags().StringVar(&opts.AirflowVersion, "airflow-version", "",
		"Airflow version to pin in the manifest (default: "+scaffold.DefaultAirflowVersion+")")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Project name (default: the directory name)")
	return cmd
}

func (c *cli) runInit(dir string, opts scaffold.Options) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(dir) {
		wd, err := c.d.WorkingDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(wd, dir)
	}
	res, err := scaffold.Run(dir, opts)
	if err != nil {
		return err
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderInit(w, res)
	})
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
