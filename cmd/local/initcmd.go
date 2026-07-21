package local

import (
	"github.com/spf13/cobra"
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
	var opts struct {
		airflowVersion string
		name           string
	}
	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Scaffold a new Astro project",
		Long:  "Create a pyproject.toml-based Astro project in the given directory (default: the current one).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := c.renderer(); err != nil {
				return err
			}
			return notBuilt("astro init (project scaffold, an earlier fix)")
		},
	}
	cmd.Flags().StringVar(&opts.airflowVersion, "airflow-version", "", "Airflow version to pin in the manifest (default: latest)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Project name (default: the directory name)")
	return cmd
}
