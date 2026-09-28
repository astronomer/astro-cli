package local

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// The dev-to-local mapping lives in pkg/scaffold, which also publishes
// it in every scaffolded AGENTS.md; the stub renders the same data.
type devReplacement = scaffold.DevReplacement

func devReplacements() []devReplacement { return scaffold.DevReplacements() }

// devRemoved is the data behind the stub's output: the JSON payload in json
// mode, and the source the human message is rendered from.
type devRemoved struct {
	Error       string           `json:"error"`
	Typed       string           `json:"typed_command,omitempty"`
	Replacement string           `json:"replacement,omitempty"`
	Mapping     []devReplacement `json:"mapping"`
	// V1Project is set when the current directory looks like an astro v1
	// project, which v2 cannot run yet.
	V1Project bool `json:"v1_project,omitempty"`
	// Notes say what became of a flag the typed command carried that has no
	// flag in the replacement.
	Notes []string `json:"notes,omitempty"`
}

// devContext is what the stub reads about the directory it runs in and the
// command tree it points into.
type devContext struct {
	v1 bool
	// dockerfile is set when the current project declares [tool.astro]
	// dockerfile, which only Docker mode builds.
	dockerfile bool
	// buildSecret and packageBuildSecret are set when `astro local start` and
	// `astro package` take --build-secret, so a replacement never names a flag
	// the command would refuse.
	buildSecret        bool
	packageBuildSecret bool
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
		// Listed nowhere: the guidance is for someone who typed the old
		// command, not a menu entry teaching a command that is gone.
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		// Old invocations carry flags this stub does not know; parsing
		// them would fail before the guidance prints.
		DisableFlagParsing: true,
		// Usage is silenced (the guidance is the whole point); the error is
		// not, so the runner prints the tombstone message.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runDevRemoved(cmd.Root(), args)
		},
	}
	markSkipPreRun(cmd)
	return cmd
}

func (c *cli) runDevRemoved(root *cobra.Command, args []string) error {
	payload := buildDevRemoved(devTypedSubcommand(args), args, devContext{
		v1:                 c.isV1Project(),
		dockerfile:         c.declaresDockerfile(),
		buildSecret:        takesFlag(root, []string{"local", nameStart}, "build-secret"),
		packageBuildSecret: takesFlag(root, []string{"package"}, "build-secret"),
	})
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

// devReplacementFor finds the v2 command for an `astro dev` subcommand. The
// table is ordered longest-prefix first, so the first match wins.
//
// It is shared with the af group, which answers to `airflow` — the spelling
// that meant the local project before 2019 and became `astro dev`. An old
// invocation lands there, not here.
func devReplacementFor(typed string) (string, bool) {
	for _, m := range devReplacements() {
		if typed == m.Command || strings.HasPrefix(typed, m.Command+" ") {
			return m.Replacement, true
		}
	}
	return "", false
}

func buildDevRemoved(typed string, args []string, dc devContext) devRemoved {
	mapping := devReplacements()
	p := devRemoved{
		Typed:     strings.TrimSpace("astro dev " + typed),
		Mapping:   mapping,
		V1Project: dc.v1,
	}
	if typed == "" {
		p.Error = "astro dev was removed in Astro CLI v2"
		return p
	}
	if replacement, ok := devReplacementFor(typed); ok {
		p.Replacement = replacement
		switch replacement {
		case replaceStart, replaceRestart:
			p.Replacement, p.Notes = devStartReplacement(replacement, args, dc)
		case replacePackage:
			p.Replacement, p.Notes = devBuildReplacement(args, dc)
		}
	}
	p.Error = fmt.Sprintf("`%s` was removed in Astro CLI v2", p.Typed)
	return p
}

// buildSecretSpecRe is the shape of a --build-secret spec safe to repeat in a
// command line: id=, src= and env= pairs naming where a secret comes from.
var buildSecretSpecRe = regexp.MustCompile(`^[A-Za-z0-9_.,=/~:@+-]+$`)

// devStartReplacement fits the start or restart replacement to the project and
// to the flags typed. A project that declares a Dockerfile starts in Docker
// mode, since standalone mode does not build it, and keeps the --build-secret
// specs its build reads; restart has no --docker flag because it keeps the
// running mode, but a restart with nothing running starts in standalone mode,
// so it gets a note. --wait has no flag in v2 and becomes a note. Other flag values
// may be secrets, so only a build secret spec, which names a source and not a
// value, and a --wait value that parses as a duration are repeated.
func devStartReplacement(replacement string, args []string, dc devContext) (command string, notes []string) {
	cmd := []string{replacement}
	if dc.dockerfile && replacement == replaceStart {
		cmd = append(cmd, "--docker")
	}
	cmd, notes = withBuildSecrets(cmd, args, dc.buildSecret, dc)
	if dc.dockerfile && replacement == replaceRestart {
		notes = append(notes, fmt.Sprintf("With nothing running, restart starts in standalone mode, which does not build the Dockerfile; use `%s --docker` then", replaceStart))
	}
	if slices.ContainsFunc(args, isWaitFlag) {
		example := "10m"
		if waits := devFlagValues(args, "--wait"); len(waits) > 0 {
			if _, err := time.ParseDuration(waits[len(waits)-1]); err == nil {
				example = waits[len(waits)-1]
			}
		}
		notes = append(notes, fmt.Sprintf("--wait is now the %s environment variable, a Go duration: %s=%s %s",
			healthTimeoutEnv, healthTimeoutEnv, example, strings.Join(cmd, " ")))
	}
	return strings.Join(cmd, " "), notes
}

// devBuildReplacement carries the --build-secret specs typed to `astro package`
// in a project that declares a Dockerfile, as devStartReplacement does for start.
func devBuildReplacement(args []string, dc devContext) (command string, notes []string) {
	cmd, notes := withBuildSecrets([]string{replacePackage}, args, dc.packageBuildSecret, dc)
	command = strings.Join(cmd, " ")
	return command, notes
}

// withBuildSecrets appends the --build-secret specs in args to cmd when the
// replacement takes the flag, or returns a note when the project declares no
// Dockerfile for them to reach.
func withBuildSecrets(cmd, args []string, takes bool, dc devContext) (withSecrets, notes []string) {
	secrets := devFlagValues(args, "--build-secret", "--build-secrets")
	switch {
	case len(secrets) == 0 || !takes:
		return cmd, nil
	case !dc.dockerfile:
		return cmd, []string{"--build-secret applies only to a project that declares [tool.astro] dockerfile"}
	}
	for _, spec := range secrets {
		if !buildSecretSpecRe.MatchString(spec) {
			spec = "<spec>"
		}
		cmd = append(cmd, "--build-secret", spec)
	}
	return cmd, nil
}

func isWaitFlag(arg string) bool { return arg == "--wait" || strings.HasPrefix(arg, "--wait=") }

// devFlagValues collects the values given to any of names, in both the
// `--flag value` and `--flag=value` spellings.
func devFlagValues(args []string, names ...string) []string {
	var values []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		if !slices.Contains(names, name) {
			continue
		}
		if !hasValue {
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				continue
			}
			i++
			value = args[i]
		}
		values = append(values, value)
	}
	return values
}

// takesFlag reports whether the command at path under root defines flag.
func takesFlag(root *cobra.Command, path []string, flag string) bool {
	cmd, _, err := root.Find(path)
	return err == nil && cmd.Flags().Lookup(flag) != nil
}

// declaresDockerfile reports whether the current project declares its own
// Dockerfile. Any failure to read the manifest reads as no.
func (c *cli) declaresDockerfile() bool {
	dir, err := c.projectPath()
	if err != nil {
		return false
	}
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	return err == nil && m.Astro.Dockerfile != ""
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
	b.WriteString(".")
	for _, n := range p.Notes {
		b.WriteString("\n" + n)
	}
	b.WriteString("\nLocal Airflow now lives under `astro local`:\n\n")
	examples := []devReplacement{
		{Command: nameStart, Replacement: replaceStart},
		{Command: nameLogs, Replacement: replaceLogs},
		{Command: nameInit, Replacement: replaceInit},
	}
	if p.Replacement != "" {
		typed := devReplacement{Command: strings.TrimPrefix(p.Typed, "astro dev "), Replacement: p.Replacement}
		examples = append([]devReplacement{typed}, examples...)
	}
	seen := map[string]bool{}
	for _, e := range examples {
		if seen[e.Command] {
			continue
		}
		seen[e.Command] = true
		fmt.Fprintf(&b, "  %-24s # was: astro dev %s\n", e.Replacement, e.Command)
	}
	if p.V1Project {
		b.WriteString("\n\nThis directory holds an astro v1 project (Dockerfile and .astro/). Migration ships in a later release; use astro CLI 1.x with this project for now.")
	}
	return b.String()
}

// isV1Project reports whether the working directory holds a v1 astro project,
// using the shared predicate in internal/project so `astro dev` and
// `astro init` never disagree about what counts as v1.
func (c *cli) isV1Project() bool {
	wd, err := c.d.WorkingDir()
	if err != nil {
		return false
	}
	return project.IsV1(wd)
}
