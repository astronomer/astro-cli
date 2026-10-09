package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

// The help every command prints is drawn here, once, for the whole tree. The
// root installs it with SetHelpFunc and SetUsageFunc and every command inherits
// both, so a page looks the same whichever package built its command.
//
// It replaces cobra's templates rather than amending them because the things
// worth fixing are layout, not wording: cobra never wraps, pads a command
// column for names but not aliases, and puts examples above the flags they use.
// A command that sets a template of its own is ignored — the inherited func
// wins — so a section a command needs belongs here, as flag groups are.

// The width help wraps to: the terminal's, but no wider than reads well, and
// a fixed width when output is not a terminal so a pipe gets the same page
// every time.
const (
	helpMaxWidth     = 100
	helpDefaultWidth = 80
	// helpMinWidth stops a very narrow terminal from wrapping a description
	// into one word per line; past it, lines overflow instead.
	helpMinWidth = 40
	// helpNameColumnMax caps the command column. A command whose spellings
	// are wider than this prints them on a line of their own, with its
	// description below, rather than pushing every sibling's description to
	// the right.
	helpNameColumnMax = 30
)

// helpWidth is a variable so a test pins the width instead of inheriting the
// terminal it happens to run in.
var helpWidth = func() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return helpDefaultWidth
	}
	return min(max(w, helpMinWidth), helpMaxWidth)
}

// flagGroupAnnotation names the section a flag is listed under. A flag without
// one is listed under "Flags:". The value is the section's title, without the
// trailing "Flags:" — "Image" renders as "Image Flags:".
const flagGroupAnnotation = "group"

// flagGroupOrderAnnotation is a command annotation naming the order its flag
// sections read in, as a comma-separated list of titles. Without it, sections
// follow the order their first flag was defined in.
const flagGroupOrderAnnotation = "flag-groups"

// verbosityFlag is listed on the root page only. Every command inherits it,
// and repeating it under every page's Global Flags made it the one flag most
// pages had in common, crowding out the ones that differ.
const verbosityFlag = "verbosity"

// helpFlagUsage is what -h/--help says on every page. Cobra's default is a
// lowercase "help for <name>", the one flag description in the tree that did
// not start with a capital.
const helpFlagUsage = "Show help for this command"

// helpName is the name cobra gives both its -h/--help flag and its help
// command.
const helpName = "help"

// rootBanner is drawn above the root page's description. It is kept out of
// Long, which help wraps: the art's lines are wider than a narrow terminal and
// are not prose.
const rootBanner = `
 ________   ______   _________  ______    ______             ______   __        ________
/_______/\ /_____/\ /________/\/_____/\  /_____/\           /_____/\ /_/\      /_______/\
\::: _  \ \\::::_\/_\__.::.__\/\:::_ \ \ \:::_ \ \   _______\:::__\/ \:\ \     \__.::._\/
 \::(_)  \ \\:\/___/\  \::\ \   \:(_) ) )_\:\ \ \ \ /______/\\:\ \  __\:\ \       \::\ \
  \:: __  \ \\_::._\:\  \::\ \   \: __ '\ \\:\ \ \ \\__::::\/ \:\ \/_/\\:\ \____  _\::\ \__
   \:.\ \  \ \ /____\:\  \::\ \   \ \ '\ \ \\:\_\ \ \          \:\_\ \ \\:\/___/\/__\::\__/\
    \__\/\__\/ \_____\/   \__\/    \_\/ \_\/ \_____\/           \_____\/ \_____\/\________\/
`

// installHelp makes rootCmd and every command under it render help here.
//
// platform and platformVersion are the context line: which control plane this
// machine points at, and Houston's version on APC. They are shown only on a
// page whose command talks to that control plane (contextMatters).
func installHelp(rootCmd *cobra.Command, platform, platformVersion string) {
	walkTree(rootCmd, func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		if f := c.Flags().Lookup(helpName); f != nil {
			f.Usage = helpFlagUsage
		}
	})
	rootCmd.SetUsageFunc(func(c *cobra.Command) error {
		w := c.OutOrStderr()
		_, err := io.WriteString(w, usageText(c, helpWidth(), ansi.ForWriter(w)))
		return err
	})
	rootCmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		w := c.OutOrStdout()
		io.WriteString(w, helpPage(c, platform, platformVersion, helpWidth(), ansi.ForWriter(w))) //nolint:errcheck // cobra's help func has nowhere to return an error, and cobra's own drops it too
	})
}

// helpPage is everything `--help` prints for c.
//
// The prose on it, a description and each command's and flag's, writes a
// command or flag the way the docs do, in backticks; code renders them for the
// stream the page goes to (ansi.Backticks): bold on a terminal, plain text
// anywhere else. Examples and the usage line are command lines, printed as
// written.
func helpPage(c *cobra.Command, platform, platformVersion string, width int, code ansi.Palette) string {
	var b strings.Builder
	if !c.HasParent() {
		b.WriteString(rootBanner + "\n")
	}
	if text := strings.TrimSpace(firstNonEmpty(commandLong(c), c.Short)); text != "" {
		b.WriteString(wrapText(code.Backticks(text), width) + "\n\n")
	}
	if c.Runnable() || c.HasSubCommands() {
		b.WriteString(usageText(c, width, code))
	}
	if contextMatters(c) {
		b.WriteString("\n" + contextLine(platform, platformVersion) + "\n")
	}
	return b.String()
}

func walkTree(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkTree(sub, fn)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// contextMatters reports whether a page should say which control plane the
// CLI points at. It is every command that runs the platform pre-run: the core
// tree (`astro local`, `astro init`, `astro af`, ...) skips it, works with no
// account, and reads the same whichever context is current, so there the line
// is noise.
func contextMatters(c *cobra.Command) bool {
	for p := c; p != nil; p = p.Parent() {
		if p.Annotations[telemetry.SkipPreRunAnnotation] == "true" {
			return false
		}
	}
	return true
}

func contextLine(platform, platformVersion string) string {
	line := "Current context: " + ansi.Bold(platform)
	if platform == apcPlatform && platformVersion != "" {
		line += " (platform version " + ansi.Bold(platformVersion) + ")"
	}
	return line
}

// usageText is everything below a page's description: usage line, aliases,
// commands, flags, examples, and the pointer to subcommand help. It is also
// what a usage error prints under the error.
//
// Each section is a block separated from the next by one blank line; a
// section with nothing to show is left out entirely.
func usageText(c *cobra.Command, width int, code ansi.Palette) string {
	var blocks []string

	usage := "Usage:"
	if c.Runnable() {
		usage += "\n  " + c.UseLine()
	}
	if c.HasAvailableSubCommands() {
		usage += "\n  " + c.CommandPath() + " [command]"
	}
	blocks = append(blocks, usage)

	if len(c.Aliases) > 0 {
		blocks = append(blocks, "Aliases:\n  "+c.NameAndAliases())
	}

	blocks = append(blocks, commandSections(c, width, code)...)
	blocks = append(blocks, flagSections(c, width, code)...)

	if c.HasExample() {
		blocks = append(blocks, "Examples:\n"+strings.Trim(c.Example, "\n"))
	}

	if c.HasAvailableSubCommands() {
		blocks = append(blocks, fmt.Sprintf("Use %q for more information about a command.", c.CommandPath()+" [command] --help"))
	}

	return strings.Join(blocks, "\n\n") + "\n"
}

// commandSections lists a command's children: one section per group the
// command declares, in declaration order, then the ungrouped ones, then the
// help topics (commands that only group others and run nothing).
func commandSections(c *cobra.Command, width int, code ansi.Palette) []string {
	var available, topics []*cobra.Command
	for _, sub := range c.Commands() {
		switch {
		// IsAvailableCommand turns down cobra's own help command, which
		// cobra's template lists by name; so does this.
		case sub.IsAvailableCommand() || sub.Name() == helpName:
			available = append(available, sub)
		case sub.IsAdditionalHelpTopicCommand():
			topics = append(topics, sub)
		}
	}
	if len(available) == 0 && len(topics) == 0 {
		return nil
	}

	column := 0
	for _, sub := range append(available, topics...) {
		column = max(column, len(commandSpellings(sub)))
	}
	column = min(column, helpNameColumnMax)

	var sections []string
	section := func(title string, cmds []*cobra.Command) {
		if len(cmds) == 0 {
			return
		}
		var b strings.Builder
		b.WriteString(title)
		for _, sub := range cmds {
			b.WriteString("\n" + commandRow(sub, column, width, code))
		}
		sections = append(sections, b.String())
	}

	grouped := map[string][]*cobra.Command{}
	var ungrouped []*cobra.Command
	for _, sub := range available {
		if sub.GroupID != "" && c.ContainsGroup(sub.GroupID) {
			grouped[sub.GroupID] = append(grouped[sub.GroupID], sub)
		} else {
			ungrouped = append(ungrouped, sub)
		}
	}
	for _, g := range c.Groups() {
		section(g.Title, grouped[g.ID])
	}
	ungroupedTitle := "Available Commands:"
	if len(c.Groups()) > 0 {
		ungroupedTitle = "Additional Commands:"
	}
	section(ungroupedTitle, ungrouped)
	section("Additional help topics:", topics)
	return sections
}

// commandRow is one line of a command list: its spellings padded to the
// column, then its description wrapped under itself. Spellings wider than the
// column take a line of their own.
func commandRow(sub *cobra.Command, column, width int, code ansi.Palette) string {
	const indent = 2
	const gap = 2
	names := commandSpellings(sub)
	descIndent := indent + column + gap
	desc := wrapHanging(code.Backticks(sub.Short), width, descIndent)
	pad := strings.Repeat(" ", indent)
	if len(names) > column {
		return pad + names + "\n" + strings.Repeat(" ", descIndent) + desc
	}
	return pad + names + strings.Repeat(" ", column-len(names)+gap) + desc
}

// flagSections lists a command's own flags, split into the sections their
// group annotation names, then the flags it inherits. Ungrouped flags come
// first under "Flags:"; the groups follow in the order their first flag was
// defined, which is the order the command's author wrote them in.
func flagSections(c *cobra.Command, width int, code ansi.Palette) []string {
	var sections []string

	local := c.LocalFlags()
	var order []string
	groups := map[string]*pflag.FlagSet{}
	local.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		group := ""
		if values := f.Annotations[flagGroupAnnotation]; len(values) > 0 {
			group = values[0]
		}
		if groups[group] == nil {
			groups[group] = pflag.NewFlagSet(group, pflag.ContinueOnError)
			order = append(order, group)
		}
		groups[group].AddFlag(f)
	})
	if set := groups[""]; set != nil {
		sections = append(sections, "Flags:\n"+flagUsages(set, width, code))
	}
	for _, group := range definitionOrder(c, order) {
		if group == "" {
			continue
		}
		sections = append(sections, group+" Flags:\n"+flagUsages(groups[group], width, code))
	}

	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == verbosityFlag {
			return
		}
		inherited.AddFlag(f)
	})
	if inherited.HasFlags() {
		sections = append(sections, "Global Flags:\n"+flagUsages(inherited, width, code))
	}
	return sections
}

// definitionOrder sorts groups into the order the command declares in its
// flagGroupOrderAnnotation, then any it does not name by where their first
// flag was defined. LocalFlags is rebuilt by name, so definition order has to
// be read from the command's own set, unsorted.
func definitionOrder(c *cobra.Command, groups []string) []string {
	first := map[string]int{}
	declared := strings.Split(c.Annotations[flagGroupOrderAnnotation], ",")
	for i, group := range declared {
		first[strings.TrimSpace(group)] = i - len(declared)
	}
	flags := c.Flags()
	sorted := flags.SortFlags
	flags.SortFlags = false
	i := 0
	flags.VisitAll(func(f *pflag.Flag) {
		group := ""
		if values := f.Annotations[flagGroupAnnotation]; len(values) > 0 {
			group = values[0]
		}
		if _, seen := first[group]; !seen {
			first[group] = i
		}
		i++
	})
	flags.SortFlags = sorted
	ordered := append([]string(nil), groups...)
	sort.SliceStable(ordered, func(a, b int) bool { return first[ordered[a]] < first[ordered[b]] })
	return ordered
}

// flagUsages lists a set's flags in two columns: the flag and its value, then
// its description wrapped under itself.
//
// pflag's own wrapping (FlagUsagesWrapped) gives up at the first word wider
// than the room left — a URL, usually — and prints the rest of the description
// on one line. Its column layout is private, so each flag is rendered alone,
// unwrapped, and split back into the two columns pflag joined: that keeps its
// spelling of value names, defaults and deprecations, which this does not
// restate.
func flagUsages(set *pflag.FlagSet, width int, code ansi.Palette) string {
	type row struct{ left, usage string }
	var rows []row
	column := 0
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		one := pflag.NewFlagSet("", pflag.ContinueOnError)
		one.AddFlag(f)
		left, usage := splitFlagUsage(one.FlagUsagesWrapped(0))
		rows = append(rows, row{left, usage})
		column = max(column, len(left))
	})
	const gap = 3
	var lines []string
	for _, r := range rows {
		var parts []string
		for _, line := range strings.Split(r.usage, "\n") {
			parts = append(parts, wrapHanging(code.Backticks(line), width, column+gap))
		}
		desc := strings.Join(parts, "\n"+strings.Repeat(" ", column+gap))
		lines = append(lines, r.left+strings.Repeat(" ", column-len(r.left)+gap)+desc)
	}
	return strings.Join(lines, "\n")
}

// splitFlagUsage splits pflag's one-flag line, "  -o, --output string  Output
// format", into the flag ("  -o, --output string") and its description. The
// flag column never holds two spaces in a row past its indent, and pflag puts
// exactly two before the description. A description pflag continued onto
// further lines comes back with their indent removed.
func splitFlagUsage(rendered string) (left, usage string) {
	rendered = strings.TrimRight(rendered, "\n")
	indent := len(rendered) - len(strings.TrimLeft(rendered, " "))
	at := strings.Index(rendered[indent:], "  ")
	if at < 0 {
		return rendered, ""
	}
	at += indent
	lines := strings.Split(rendered[at+2:], "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return rendered[:at], strings.Join(lines, "\n")
}

// wrapText wraps a description to width. Only unindented lines are wrapped —
// prose, and "- " list items, whose continuation lines are indented under the
// item's text. An indented line is a command or a literal someone may copy, so
// it is left whole however long it is.
func wrapText(text string, width int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if ansi.VisibleWidth(line) <= width || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		hang := 0
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			hang = 2
		}
		lines[i] = wrapHanging(line, width, hang)
	}
	return strings.Join(lines, "\n")
}

// wrapHanging breaks text into lines that end by column width and indents
// every line after the first by hang spaces. The first line is the caller's
// to place, and is taken to start at column hang too. A word longer than the
// room left is not broken. Widths are what a terminal shows, so a word in bold
// (a rendered backtick span) is as wide as its letters.
func wrapHanging(text string, width, hang int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	room := max(width-hang, helpMinWidth/2)
	var b strings.Builder
	lineLen := 0
	for i, word := range words {
		switch {
		case i == 0:
		case lineLen+1+ansi.VisibleWidth(word) > room:
			b.WriteString("\n" + strings.Repeat(" ", hang))
			lineLen = 0
		default:
			b.WriteString(" ")
			lineLen++
		}
		b.WriteString(word)
		lineLen += ansi.VisibleWidth(word)
	}
	return b.String()
}
