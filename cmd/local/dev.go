package local

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// The dev-to-local mapping lives in pkg/scaffold; the stub renders it.
type devReplacement = scaffold.DevReplacement

func devReplacements() []devReplacement { return scaffold.DevReplacements() }

// devRemoved is the data behind the stub's output: the JSON payload in json
// mode, and the source the human message is rendered from.
type devRemoved struct {
	Error       string           `json:"error"`
	Typed       string           `json:"typed_command,omitempty"`
	Replacement string           `json:"replacement,omitempty"`
	Mapping     []devReplacement `json:"mapping"`
	// Is1xProject is set when the working directory is inside a 1.x
	// project, its root included; V1Dir names that root. The replacements
	// only work there once it is converted.
	Is1xProject bool   `json:"v1_project,omitempty"`
	V1Dir       string `json:"v1_dir,omitempty"`
	// Notes say what became of a flag the typed command carried that has no
	// flag in the replacement.
	Notes []string `json:"notes,omitempty"`
	// Convert is the command that converts a 1.x project in place, set with
	// Is1xProject unless astro init refuses that project here.
	Convert string `json:"convert,omitempty"`
	// UnderAPC and ContextUnresolved say astro init refuses the 1.x project
	// the working directory is in or below (project.Convert1xBlocked): the
	// current context is Astro Private Cloud, or one the CLI cannot resolve.
	// Notes then say why and what to do instead, and the mapping is empty and
	// there is no Replacement or Convert, as the text names no astro local
	// command: none runs in that project.
	UnderAPC          bool `json:"under_apc,omitempty"`
	ContextUnresolved bool `json:"context_unresolved,omitempty"`
}

// devContext is what the stub reads about the directory it runs in and the
// command tree it points into.
type devContext struct {
	// in is the 1.x project the stub speaks of, if any, and why astro
	// init refuses it here, if it does (devProject).
	in project1x
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
		in:                 c.devProject(),
		dockerfile:         c.declaresDockerfile(),
		buildSecret:        takesFlag(root, []string{"local", nameStart}, "build-secret"),
		packageBuildSecret: takesFlag(root, []string{"package"}, "build-secret"),
	})
	if devWantsJSON(args) {
		r := cliout.Renderer{Format: cliout.FormatJSON, Out: c.d.Stdout, Style: c.d.JSONStyle}
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

// devReplacementFor finds the `astro local` command for an `astro dev` subcommand. The
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
		Typed:       strings.TrimSpace("astro dev " + typed),
		Mapping:     mapping,
		Is1xProject: dc.in.dir1x != "",
		V1Dir:       dc.in.dir1x,
	}
	// Where astro init refuses the 1.x project (project.Convert1xBlocked) it
	// stays as it is, so nothing offers astro init.
	stays := dc.in.dir1x != "" && dc.in.blocked != project.NotBlocked
	if dc.in.dir1x != "" {
		if !stays {
			// From below the root, astro init names the root, and so does
			// every place the stub offers it: the mapping, the replacement
			// for astro dev init, and the text's example row.
			p.Convert = dc.in.convert()
			p.Mapping = withInit(mapping, p.Convert)
		}
		// astro init keeps a Dockerfile that does more than pick a base image,
		// and one that mounts a build secret always does, so the converted
		// project builds it in Docker mode.
		if len(devFlagValues(args, "--build-secret", "--build-secrets")) > 0 {
			dc.dockerfile = true
		}
	}
	if stays {
		// No astro local command runs in this project, so none is named as
		// the replacement; the notes say why and what to do instead.
		p.UnderAPC, p.ContextUnresolved = dc.in.blocked == project.BlockedUnderAPC, dc.in.blocked == project.BlockedUnresolved
		p.Mapping = []devReplacement{}
		p.Notes = []string{
			project.Blocked1xMessage(dc.in.blocked, dc.in.dir1x),
			"The astro local commands work only in a converted project",
		}
	}
	if typed == "" {
		p.Error = "astro dev was removed in Astro CLI v2"
		return p
	}
	if replacement, ok := devReplacementFor(typed); ok && !stays {
		p.Replacement = replacement
		switch replacement {
		case replaceInit:
			if p.Convert != "" {
				p.Replacement = p.Convert
			}
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
	builds := dc.dockerfile || (dc.buildSecret && slices.ContainsFunc(devFlagValues(args, "--build-secret", "--build-secrets"), isRuntimeSecret))
	if builds && replacement == replaceStart {
		cmd = append(cmd, "--docker")
	}
	cmd, notes = withBuildSecrets(cmd, args, dc.buildSecret, dc)
	if builds && replacement == replaceRestart {
		notes = append(notes, fmt.Sprintf("With nothing running, restart starts in standalone mode, which builds no image; use `%s --docker` then", replaceStart))
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
// replacement takes the flag. Without a Dockerfile only the netrc secret
// reaches the build, and a note says the rest were left out.
func withBuildSecrets(cmd, args []string, takes bool, dc devContext) (withSecrets, notes []string) {
	secrets := devFlagValues(args, "--build-secret", "--build-secrets")
	if len(secrets) == 0 || !takes {
		return cmd, nil
	}
	if !dc.dockerfile {
		kept := slices.DeleteFunc(slices.Clone(secrets), func(spec string) bool { return !isRuntimeSecret(spec) })
		if len(kept) < len(secrets) {
			notes = []string{"Without [tool.astro] dockerfile, only the netrc --build-secret reaches the build"}
		}
		secrets = kept
	}
	for _, spec := range secrets {
		if !buildSecretSpecRe.MatchString(spec) {
			spec = "<spec>"
		}
		cmd = append(cmd, "--build-secret", spec)
	}
	return cmd, notes
}

func isRuntimeSecret(spec string) bool {
	s, err := manifest.ParseBuildSecret(spec)
	return err == nil && s.ID == manifest.RuntimeSecretID
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
	switch {
	case p.Replacement != "" && p.Convert != "" && p.Replacement != p.Convert:
		fmt.Fprintf(&b, ". Convert with `%s`, then use `%s`", p.Convert, p.Replacement)
	case p.Replacement != "":
		fmt.Fprintf(&b, ". Use `%s` instead", p.Replacement)
	case p.UnderAPC || p.ContextUnresolved:
		// The notes below say what to do instead.
	case p.Typed != "astro dev":
		b.WriteString(" and has no direct replacement")
	}
	b.WriteString(".")
	for _, n := range p.Notes {
		b.WriteString("\n" + n)
	}
	// Under APC, in a 1.x project no astro local command runs, so the notes
	// are the whole answer.
	if p.UnderAPC || p.ContextUnresolved {
		return b.String()
	}
	b.WriteString("\nLocal Airflow now lives under `astro local`:\n\n")
	examples := []devReplacement{
		{Command: nameStart, Replacement: replaceStart},
		{Command: nameLogs, Replacement: replaceLogs},
		{Command: nameInit, Replacement: cmp.Or(p.Convert, replaceInit)},
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
	if p.Is1xProject {
		fmt.Fprintf(&b, "\n\n%s holds a project made by Astro CLI 1.x (Dockerfile and .astro/). "+
			"Run %s to convert it in place: it moves requirements.txt and packages.txt into pyproject.toml, carries what airflow_settings.yaml declares, "+
			"and keeps the Dockerfile when it does more than pick a base image. The other commands above work once it is converted.", p.V1Dir, p.Convert)
	}
	return b.String()
}

// project1x is a 1.x project the stub speaks of: its directory, "" for
// none, and why astro init refuses it here, if it does.
type project1x struct {
	dir1x   string
	blocked project.Block
	// below is set when the working directory is below dir1x rather than
	// dir1x itself, so the command that converts it names dir1x.
	below bool
}

// convert is the command that converts the 1.x project in place: astro init,
// naming the project's directory when the working directory is below it, so
// it converts the root rather than making a project inside it.
func (p project1x) convert() string {
	if !p.below {
		return replaceInit
	}
	return replaceInit + " " + scaffold.ShellQuote(p.dir1x)
}

// withInit is mapping with astro dev init's replacement set to convert.
func withInit(mapping []devReplacement, convert string) []devReplacement {
	out := slices.Clone(mapping)
	for i := range out {
		if out[i].Replacement == replaceInit {
			out[i].Replacement = convert
		}
	}
	return out
}

// devProject is the 1.x project the stub speaks of: the nearest one the
// working directory is in or below, in every context, and why astro init
// refuses it there, if it does, from the one walk and decision init makes
// (project.Convert1xBlocked).
func (c *cli) devProject() project1x {
	wd, err := c.d.WorkingDir()
	if err != nil {
		return project1x{}
	}
	why, root := project.Convert1xBlocked(wd)
	abs, err := filepath.Abs(wd)
	if err != nil {
		abs = wd
	}
	return project1x{dir1x: root, blocked: why, below: root != "" && root != filepath.Clean(abs)}
}
