package local

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// devMappingDoc is the published full mapping from `astro dev` to the v2
// surface.
const devMappingDoc = "astro.sh/v2/dev-to-local"

// devReplacement maps one v1 `astro dev` subcommand to what replaces it.
type devReplacement struct {
	Command     string `json:"command"`
	Replacement string `json:"replacement"`
}

// devReplacements returns the mapping in the order shown to the user.
// Multi-word entries must come before their one-word prefix so lookup finds
// the longest match.
func devReplacements() []devReplacement {
	return []devReplacement{
		{nameStart, replaceStart},
		{nameStop, "astro local stop"},
		{nameRestart, "astro local restart"},
		{"ps", replaceStatus},
		{nameLogs, replaceLogs},
		{nameRun, "astro local run"},
		{"bash", "astro local shell"},
		{"parse", "astro local check"},
		{"kill", "astro local stop --clean"},
		{"pytest", "uv run pytest"},
		{nameInit, replaceInit},
		{nameObject + " import", replaceEnvSchema},
		{nameObject + " export", replaceEnvSchema},
		{nameObject, replaceEnvSchema},
	}
}

// devRemoved is the data behind the stub's output: the JSON payload in json
// mode, and the source the human message is rendered from.
type devRemoved struct {
	Error       string           `json:"error"`
	Typed       string           `json:"typed_command,omitempty"`
	Replacement string           `json:"replacement,omitempty"`
	Mapping     []devReplacement `json:"mapping"`
	Doc         string           `json:"doc"`
	// V1Project is set when the current directory looks like an astro v1
	// project, which v2 cannot run yet.
	V1Project bool `json:"v1_project,omitempty"`
}

// NewDevCmd builds the `astro dev` removal stub. The whole v1 dev tree is
// one command that accepts any subcommand, names the exact replacement for
// what was typed, and fails — so scripts and CI break loudly, and both
// humans and coding agents learn the new surface from the error text.
func NewDevCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := &cobra.Command{
		Use:     nameDev,
		Aliases: []string{"d"},
		Short:   "Removed in v2 — local Airflow lives under `astro local`",
		Args:    cobra.ArbitraryArgs,
		// Old invocations carry flags this stub does not know; parsing
		// them would fail before the guidance prints.
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(_ *cobra.Command, args []string) error {
			return c.runDevRemoved(args)
		},
	}
	markSkipPreRun(cmd)
	return cmd
}

func (c *cli) runDevRemoved(args []string) error {
	payload := buildDevRemoved(devTypedSubcommand(args), c.isV1Project())
	if devWantsJSON(args) {
		r := Renderer{Format: FormatJSON, Out: c.d.Stdout}
		if err := r.Emit(payload, func(w io.Writer) error {
			_, werr := fmt.Fprintln(w, renderDevRemoved(payload))
			return werr
		}); err != nil {
			return err
		}
		return errors.New(payload.Error)
	}
	return errors.New(renderDevRemoved(payload))
}

// devTypedSubcommand extracts what the user typed after `astro dev`: the
// leading words up to the first flag (two at most, so "object import"
// resolves as one command). Scanning stops at the first flag so a flag's
// value cannot leak into the echoed command.
func devTypedSubcommand(args []string) string {
	words := make([]string, 0, 2)
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		words = append(words, a)
		if len(words) == 2 {
			break
		}
	}
	return strings.Join(words, " ")
}

// devWantsJSON honors the v2 --output convention without flag parsing.
func devWantsJSON(args []string) bool {
	for i, a := range args {
		if a == "--output=json" || a == "-o=json" {
			return true
		}
		if (a == "--output" || a == "-o") && i+1 < len(args) && args[i+1] == "json" {
			return true
		}
	}
	return false
}

func buildDevRemoved(typed string, v1Project bool) devRemoved {
	mapping := devReplacements()
	p := devRemoved{
		Typed:     strings.TrimSpace("astro dev " + typed),
		Mapping:   mapping,
		Doc:       devMappingDoc,
		V1Project: v1Project,
	}
	if typed == "" {
		p.Error = "astro dev was removed in Astro CLI v2"
		return p
	}
	for _, m := range mapping {
		if typed == m.Command || strings.HasPrefix(typed, m.Command+" ") {
			p.Replacement = m.Replacement
			break
		}
	}
	p.Error = fmt.Sprintf("`%s` was removed in Astro CLI v2", p.Typed)
	return p
}

// renderDevRemoved is the human rendering of the same payload json mode
// emits.
func renderDevRemoved(p devRemoved) string {
	var b strings.Builder
	b.WriteString(p.Error)
	if p.Replacement != "" {
		fmt.Fprintf(&b, ". Use `%s` instead", p.Replacement)
	} else if p.Typed != "astro dev" {
		b.WriteString(" and has no direct replacement")
	}
	b.WriteString(".\nLocal Airflow now lives under `astro local`:\n\n")
	examples := []devReplacement{
		{nameStart, replaceStart},
		{nameLogs, replaceLogs},
		{nameInit, replaceInit},
	}
	if p.Replacement != "" {
		examples = append([]devReplacement{{strings.TrimPrefix(p.Typed, "astro dev "), p.Replacement}}, examples...)
	}
	seen := map[string]bool{}
	for _, e := range examples {
		if seen[e.Command] {
			continue
		}
		seen[e.Command] = true
		fmt.Fprintf(&b, "  %-24s # was: astro dev %s\n", e.Replacement, e.Command)
	}
	fmt.Fprintf(&b, "\nFull mapping: %s", p.Doc)
	if p.V1Project {
		b.WriteString("\n\nThis directory holds an astro v1 project (Dockerfile, no pyproject.toml). Migration ships in a later release; use astro CLI 1.x with this project for now.")
	}
	return b.String()
}

// isV1Project reports whether the working directory looks like a v1 astro
// project: a Dockerfile and no pyproject.toml.
func (c *cli) isV1Project() bool {
	wd, err := c.d.WorkingDir()
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(wd, "pyproject.toml")); err == nil {
		return false
	}
	_, err = os.Stat(filepath.Join(wd, "Dockerfile"))
	return err == nil
}
