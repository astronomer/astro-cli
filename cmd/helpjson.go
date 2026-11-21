package cmd

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// Machine-readable help: the same cobra metadata the text pages draw from,
// as one JSON document, so a script or a coding agent can discover the CLI
// without scraping. All of it is answered from the built tree, before any
// login, config read or network call.

// globalFlagGroup is the group of a flag the command inherits from an ancestor.
const globalFlagGroup = "global"

// searchLimit is how many results `astro help search` prints.
const searchLimit = 10

// helpSearchWord is the word after `astro help` that searches the commands,
// unless the root has a command of that name.
const helpSearchWord = "search"

// helpDocument is one command, as `astro <path> --help -o json` and
// `astro help <path> -o json` print it.
type helpDocument struct {
	Path        string        `json:"path"`
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	Usage       string        `json:"usage"`
	Aliases     []string      `json:"aliases"`
	Deprecated  string        `json:"deprecated,omitempty"`
	Subcommands []helpListing `json:"subcommands"`
	Arguments   []helpArg     `json:"arguments"`
	Flags       []helpFlag    `json:"flags"`
	Examples    string        `json:"examples"`
}

// helpListing is a child command in a list: its name and one line.
type helpListing struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// helpArg is a positional argument read from the command's Use line.
type helpArg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Repeated bool   `json:"repeated"`
}

// helpFlag is a visible flag. Group is the section the help page lists it
// under, "" for the plain Flags section and "global" for an inherited one.
type helpFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// helpAllDocument is `astro help --all -o json`: every visible command.
type helpAllDocument struct {
	Commands []helpDocument `json:"commands"`
}

// helpSearchResult is one match of `astro help search`.
type helpSearchResult struct {
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

// helpSearchDocument is `astro help search <query> -o json`.
type helpSearchDocument struct {
	Commands []helpSearchResult `json:"commands"`
}

// describeCommand builds the document for c.
func describeCommand(c *cobra.Command) helpDocument {
	addHelpFlag(c)
	doc := helpDocument{
		Path:        c.CommandPath(),
		Summary:     c.Short,
		Description: strings.TrimSpace(firstNonEmpty(commandLong(c), c.Short)),
		Usage:       c.UseLine(),
		Aliases:     append([]string{}, c.Aliases...),
		Deprecated:  c.Deprecated,
		Subcommands: []helpListing{},
		Arguments:   helpArguments(c.Use),
		Flags:       helpFlags(c),
		Examples:    strings.Trim(c.Example, "\n"),
	}
	for _, sub := range listedInOrder(c) {
		doc.Subcommands = append(doc.Subcommands, helpListing{Name: sub.Name(), Summary: sub.Short})
	}
	return doc
}

var useArgument = regexp.MustCompile(`<([^<>]+)>|\[([^\[\]]+)\]`)

// helpArguments reads the positional arguments out of a Use line: <NAME> is
// required, [NAME] optional, and a trailing "..." repeats. The [flags] marker
// and the "--" that opens a passthrough tail are not arguments.
func helpArguments(use string) []helpArg {
	args := []helpArg{}
	_, rest, _ := strings.Cut(use, " ")
	for _, m := range useArgument.FindAllStringSubmatch(rest, -1) {
		name, required := m[1], true
		if name == "" {
			name, required = m[2], false
		}
		name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "--"))
		repeated := strings.HasSuffix(name, "...")
		name = strings.TrimSpace(strings.TrimSuffix(name, "..."))
		if name == "" || name == "flags" {
			continue
		}
		args = append(args, helpArg{Name: name, Required: required, Repeated: repeated})
	}
	return args
}

// helpFlags lists c's visible flags the way its page does: its own, plain
// ones first, then each group in the page's order, then the inherited ones
// (without the root's --verbosity and --version, which the page leaves out).
func helpFlags(c *cobra.Command) []helpFlag {
	flags := []helpFlag{}
	groups := map[string][]*pflag.Flag{}
	var order []string
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		group := ""
		if values := f.Annotations[flagGroupAnnotation]; len(values) > 0 {
			group = values[0]
		}
		if groups[group] == nil {
			order = append(order, group)
		}
		groups[group] = append(groups[group], f)
	})
	for _, f := range groups[""] {
		flags = append(flags, describeFlag(f, ""))
	}
	for _, group := range definitionOrder(c, order) {
		if group == "" {
			continue
		}
		for _, f := range groups[group] {
			flags = append(flags, describeFlag(f, group))
		}
	}
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == verbosityFlag || f.Name == versionFlag {
			return
		}
		flags = append(flags, describeFlag(f, globalFlagGroup))
	})
	return flags
}

func describeFlag(f *pflag.Flag, group string) helpFlag {
	return helpFlag{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     f.DefValue,
		Description: f.Usage,
		Group:       group,
	}
}

// visibleCommands is c and every command under it a page lists, depth first,
// in the order the pages list them. A hidden command, and what is under it,
// is left out.
func visibleCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, sub := range listedInOrder(c) {
		out = append(out, visibleCommands(sub)...)
	}
	return out
}

func describeAll(root *cobra.Command) helpAllDocument {
	doc := helpAllDocument{Commands: []helpDocument{}}
	for _, c := range visibleCommands(root) {
		doc.Commands = append(doc.Commands, describeCommand(c))
	}
	return doc
}

// searchCommands matches every word of query, in any case, against the
// visible commands' paths, summaries, descriptions and examples, and returns
// up to limit, most relevant first: a command whose path holds every word,
// then one whose path and summary together do, then the rest, each in the
// order the pages list them.
func searchCommands(root *cobra.Command, query string, limit int) []helpSearchResult {
	words := strings.Fields(strings.ToLower(query))
	results := []helpSearchResult{}
	if len(words) == 0 {
		return results
	}
	holds := func(text string) bool {
		text = strings.ToLower(text)
		for _, w := range words {
			if !strings.Contains(text, w) {
				return false
			}
		}
		return true
	}
	var ranked [3][]helpSearchResult
	for _, c := range visibleCommands(root)[1:] {
		path := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
		result := helpSearchResult{Path: c.CommandPath(), Summary: c.Short}
		switch {
		case holds(path):
			ranked[0] = append(ranked[0], result)
		case holds(path + "\n" + c.Short):
			ranked[1] = append(ranked[1], result)
		case holds(path + "\n" + c.Short + "\n" + commandLong(c) + "\n" + c.Example):
			ranked[2] = append(ranked[2], result)
		}
	}
	for _, tier := range ranked {
		results = append(results, tier...)
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// writeListing prints commands as "<path>\t<summary>" lines.
func writeListing(w io.Writer, rows []helpSearchResult) error {
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", r.Path, r.Summary); err != nil {
			return err
		}
	}
	return nil
}

// runHelp is what `astro help` does with its words: search, or the help of
// the command they name, or with --all every command.
func runHelp(c *cobra.Command, args []string, all bool, format cliout.Format) error {
	root := c.Root()
	render := cliout.Renderer{Format: format, Out: c.OutOrStdout()}

	if all {
		if len(args) > 0 {
			return cliout.Usage(errors.New("help --all lists every command and takes no command name"))
		}
		doc := describeAll(root)
		return render.Emit(doc, func(w io.Writer) error {
			rows := make([]helpSearchResult, 0, len(doc.Commands))
			for i := 1; i < len(doc.Commands); i++ {
				rows = append(rows, helpSearchResult{Path: doc.Commands[i].Path, Summary: doc.Commands[i].Summary})
			}
			return writeListing(w, rows)
		})
	}

	if len(args) > 0 && args[0] == helpSearchWord && childNamed(root, helpSearchWord) == nil {
		query := strings.Join(args[1:], " ")
		if strings.TrimSpace(query) == "" {
			return cliout.Usage(errors.New("help search needs a query: astro help search <QUERY>"))
		}
		results := searchCommands(root, query, searchLimit)
		return render.Emit(helpSearchDocument{Commands: results}, func(w io.Writer) error {
			if len(results) == 0 {
				_, err := fmt.Fprintf(c.ErrOrStderr(), "No commands match %q.\n", query)
				return err
			}
			return writeListing(w, results)
		})
	}

	target := root
	for _, word := range args {
		next := childNamed(target, word)
		if next == nil {
			return cliout.PointAtHelpOf(cliout.UnknownSubcommand(target, word), target)
		}
		target = next
	}
	if format == cliout.FormatJSON {
		return render.Emit(describeCommand(target), nil)
	}
	return target.Help()
}

// printHelpJSONIfAsked prints the JSON help of the command args name when
// they ask for both --help and --output json, and reports whether it did. It
// runs before cobra parses anything, like --version, so a command that could
// not run here (no login, a project that does not parse) still describes
// itself. A line that names something that is not a command is left to the
// usual failure.
func printHelpJSONIfAsked(root *cobra.Command, args []string, out io.Writer) (bool, error) {
	if isShellCompletion(args) || !hasHelpFlag(args) || !cliout.ArgsAskForJSON(args, "o") {
		return false, nil
	}
	target, rest, err := root.Find(withoutOutputFlag(withoutHelpFlags(args)))
	if err != nil {
		return false, nil
	}
	for _, a := range rest {
		if !strings.HasPrefix(a, "-") {
			return false, nil
		}
	}
	return true, cliout.Renderer{Format: cliout.FormatJSON, Out: out}.Emit(describeCommand(target), nil)
}

// withoutOutputFlag drops -o/--output and its value from args.
func withoutOutputFlag(args []string) []string {
	kept := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(kept, args[i:]...)
		}
		switch {
		case a == "-o" || a == "--output":
			i++
		case strings.HasPrefix(a, "--output="), strings.HasPrefix(a, "-o"):
		default:
			kept = append(kept, a)
		}
	}
	return kept
}
