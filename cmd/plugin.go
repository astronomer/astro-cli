package cmd

import (
	"fmt"
	"io"
	"sort"

	"github.com/astronomer/astro-cli/pkg/plugin"
	"github.com/spf13/cobra"
)

func newPluginCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Astro CLI plugins",
		Long: `Plugins extend the Astro CLI with additional commands.
Plugins are executable files beginning with "astro-" that are found in your PATH.`,
	}
	cmd.AddCommand(
		newPluginListCmd(out),
	)
	return cmd
}

func newPluginListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all available plugins",
		Long: `List all available plugins found in your PATH.
Plugins are executable files beginning with "astro-".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listPlugins(out)
		},
	}
	return cmd
}

func listPlugins(out io.Writer) error {
	plugins, err := plugin.ListPlugins()
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Fprintln(out, "No plugins found.")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "To install a plugin, place an executable file beginning with 'astro-' in your PATH.")
		fmt.Fprintln(out, "For example: /usr/local/bin/astro-ai")
		return nil
	}

	// Sort plugins by name for consistent output
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	fmt.Fprintln(out, "Available plugins:")
	for _, p := range plugins {
		fmt.Fprintf(out, "  %-20s %s\n", p.BinaryName, p.Path)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "To use a plugin, run: astro %s [args]\n", plugins[0].Name)

	return nil
}
