package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// The leaves here describe the Airflow itself rather than anything running on
// it: what version it is, what it has installed, and how it is configured.
// Each is one read, so each is a leaf of `af` rather than a family.

// newQueryLeaf finishes a leaf that acts on an Airflow: the leaf is built over
// its own query, then wired to the target as a family would be.
func newQueryLeaf(d Deps, t target, build func(*query) *cobra.Command) *cobra.Command {
	q := &query{cli: &cli{d: d}, t: t}
	cmd := build(q)
	attachTarget(q, cmd)
	return cmd
}

func newVersionCmd(d Deps, t target) *cobra.Command {
	return newQueryLeaf(d, t, func(q *query) *cobra.Command {
		return &cobra.Command{
			Use:   "version",
			Short: "Show an Airflow's version and API generation",
			Long:  "Show the version " + t.which() + " reports, and which generation of its REST API this CLI talks to.",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return q.runVersion(cmd.Context())
			},
		}
	})
}

func (q *query) runVersion(ctx context.Context) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	info, err := client.Version(ctx)
	if err != nil {
		return err
	}
	return emitDetail(r, newVersionRow(info), func(row versionRow) []field {
		return []field{
			{"version", row.Version},
			{"git version", row.GitVersion},
			{"api generation", row.Generation},
		}
	})
}

func newProvidersCmd(d Deps, t target) *cobra.Command {
	return newQueryLeaf(d, t, func(q *query) *cobra.Command {
		return &cobra.Command{
			Use:   "providers",
			Short: "List the provider packages an Airflow has installed",
			Long: "List every provider distribution installed on " + t.which() + ", with its version. It is what " +
				"that Airflow actually has, which is the thing to check when a DAG imports a provider that the " +
				"project pins one version of and the Airflow runs another.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return q.runProviders(cmd.Context())
			},
		}
	})
}

// providerRow is one installed provider.
type providerRow struct {
	PackageName string `json:"package_name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	// DocumentationURL is Airflow 3 only.
	DocumentationURL string `json:"documentation_url,omitempty"`
}

func newProviderRow(p airflowapi.Provider) providerRow {
	return providerRow{
		PackageName:      p.PackageName,
		Version:          p.Version,
		Description:      p.Description,
		DocumentationURL: p.DocumentationURL,
	}
}

func (q *query) runProviders(ctx context.Context) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	list, err := client.ListProviders(ctx)
	if err != nil {
		return notServed("its providers", err)
	}
	rows := mapRows(list.Providers, newProviderRow)
	return emitRows(r, rows, len(rows), newProviderList, func(w io.Writer, rows []providerRow) error {
		return renderTable(w, rows, "No providers on this Airflow.",
			[]string{"PACKAGE", "VERSION", "DESCRIPTION"},
			func(row providerRow) []string {
				return []string{row.PackageName, row.Version, firstLine(row.Description)}
			})
	})
}

func newPluginsCmd(d Deps, t target) *cobra.Command {
	return newQueryLeaf(d, t, func(q *query) *cobra.Command {
		var list listFlags
		cmd := &cobra.Command{
			Use:   "plugins",
			Short: "List the plugins an Airflow loaded, and what each contributes",
			Long: "List the plugins " + t.which() + " loaded: where each came from, and what it adds — macros, " +
				"listeners, timetables, UI views and apps. A plugin missing here did not load.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return q.runPlugins(cmd.Context(), list.options())
			},
		}
		addListFlags(cmd, &list, "")
		return cmd
	})
}

// pluginRow is one loaded plugin. Components is keyed by Airflow's own name
// for each kind of contribution, because the two generations list different
// kinds; see airflowapi.Plugin.
type pluginRow struct {
	Name       string              `json:"name"`
	Source     string              `json:"source,omitempty"`
	Components map[string][]string `json:"components,omitempty"`
}

func newPluginRow(p airflowapi.Plugin) pluginRow {
	row := pluginRow{Name: p.Name, Source: p.Source}
	if len(p.Components) > 0 {
		row.Components = p.Components
	}
	return row
}

func (q *query) runPlugins(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return notServed("its plugins", emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.Plugin, int, error) {
		list, err := client.ListPlugins(ctx, page)
		return list.Plugins, list.TotalEntries, err
	}, newPluginRow, newPluginList, func(w io.Writer, rows []pluginRow) error {
		return renderTable(w, rows, "No plugins on this Airflow.",
			[]string{"NAME", "SOURCE", "PROVIDES"},
			func(row pluginRow) []string {
				kinds := make([]string, 0, len(row.Components))
				for _, kind := range slices.Sorted(maps.Keys(row.Components)) {
					kinds = append(kinds, kind+": "+strings.Join(row.Components[kind], ","))
				}
				return []string{row.Name, row.Source, strings.Join(kinds, "; ")}
			})
	}))
}

func newConfigCmd(d Deps, t target) *cobra.Command {
	return newQueryLeaf(d, t, func(q *query) *cobra.Command {
		var section string
		cmd := &cobra.Command{
			Use:   "config",
			Short: "Show the configuration an Airflow runs with",
			Long: "Show the configuration " + t.which() + " runs with, one option per line under its section, as " +
				"Airflow resolved it from airflow.cfg, the environment, and its defaults. Airflow masks the " +
				"sensitive values itself.\n\nAirflow shows this only when it is told to expose it: [api] " +
				"expose_config on Airflow 3, [webserver] expose_config on Airflow 2. Both default to off, and " +
				"then this command is refused.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return q.runConfig(cmd.Context(), section)
			},
		}
		cmd.Flags().StringVar(&section, "section", "", "Show only this section, such as core or scheduler")
		return cmd
	})
}

// configSectionRow is one section of the configuration with its options
// inside, the nesting Airflow's config endpoint sends and af's `config show`
// prints, so `jq '.sections[] | select(.name == "core") | .options'` reads
// both the same way.
type configSectionRow struct {
	Name    string            `json:"name"`
	Options []configOptionRow `json:"options"`
}

// configOptionRow is one configuration option within its section.
type configOptionRow struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Source is where the value came from, when Airflow says; see
	// airflowapi.ConfigOption.
	Source string `json:"source,omitempty"`
}

// errConfigNotExposed is what a refused configuration read is reported as. The
// refusal is nearly always the setting, which is off by default, rather than
// the credential.
var errConfigNotExposed = errors.New("this Airflow does not expose its configuration: turn on [api] expose_config " +
	"(Airflow 3) or [webserver] expose_config (Airflow 2), or this token is not allowed to read it")

func (q *query) runConfig(ctx context.Context, section string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	config, err := client.Config(ctx, section)
	if errors.Is(err, airflowapi.ErrForbidden) {
		return fmt.Errorf("%w\n%w", errConfigNotExposed, err)
	}
	if err != nil {
		return err
	}
	rows := mapRows(config.Sections, func(s airflowapi.ConfigSection) configSectionRow {
		// mapRows never returns nil, so a section with no options publishes [].
		return configSectionRow{Name: s.Name, Options: mapRows(s.Options, func(o airflowapi.ConfigOption) configOptionRow {
			return configOptionRow{Key: o.Key, Value: o.Value, Source: o.Source}
		})}
	})
	return emitRows(r, rows, len(rows), newConfigSectionList, renderConfig)
}

// renderConfig writes the options the way airflow.cfg lays them out: a
// [section] header, then key = value under it. A section with no options
// prints nothing, and a configuration with no options at all says so.
func renderConfig(w io.Writer, rows []configSectionRow) error {
	printed := 0
	for _, row := range rows {
		if len(row.Options) == 0 {
			continue
		}
		if printed > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		printed++
		if _, err := fmt.Fprintf(w, "[%s]\n", row.Name); err != nil {
			return err
		}
		for _, o := range row.Options {
			line := o.Key + " = " + o.Value
			if o.Source != "" {
				line += "  # " + o.Source
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	}
	if printed == 0 {
		_, err := fmt.Fprintln(w, "This Airflow reported no configuration.")
		return err
	}
	return nil
}
