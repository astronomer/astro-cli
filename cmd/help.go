package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/cmd/cliout"
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
 \::(_)  \ \\:\/___/\  \::\ \   \:(_) ) )_\:\ \ \ \ /______/\\:\ \    \:\ \       \::\ \
  \:: __  \ \\_::._\:\  \::\ \   \: __ '\ \\:\ \ \ \\__::::\/ \:\ \____\:\ \____  _\::\ \__
   \:.\ \  \ \ /____\:\  \::\ \   \ \ '\ \ \\:\_\ \ \          \:\/___/\\:\/___/\/__\::\__/\
    \__\/\__\/ \_____\/   \__\/    \_\/ \_\/ \_____\/           \_____\/ \_____\/\________\/
`

// installHelp makes rootCmd and every command under it render help here.
//
// platform and platformVersion are the context line: which control plane this
// machine points at, and Houston's version on APC. They are shown only on a
// page whose command talks to that control plane (contextMatters).
func installHelp(rootCmd *cobra.Command, platform, platformVersion string) {
	rootCmd.SetUsageFunc(func(c *cobra.Command) error {
		addHelpFlag(c)
		_, err := io.WriteString(c.OutOrStderr(), usageText(c, helpWidth()))
		return err
	})
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		// Cobra hands the help func the line as typed. A line that asks for
		// --output json gets the command's document, whatever else is on it.
		if cliout.ArgsAskForJSON(args, "o") {
			// A failed write ends the help: the text page is not a fallback
			// for a document the reader asked for, and the help func has no
			// way to return the error, so it is told on stderr.
			if err := (cliout.Renderer{Format: cliout.FormatJSON, Out: c.OutOrStdout(), Err: c.ErrOrStderr()}).Emit(describeCommand(c), nil); err != nil {
				c.PrintErrln(cliout.ErrorLabel(c.ErrOrStderr()), err)
			}
			return
		}
		addHelpFlag(c)
		io.WriteString(c.OutOrStdout(), helpPage(c, platform, platformVersion, helpWidth())) //nolint:errcheck // cobra's help func has nowhere to return an error, and cobra's own drops it too
	})
}

// addHelpFlag is cobra's InitDefaultHelpFlag with this CLI's usage, or sets
// that usage on the flag cobra added. It runs for the command a line runs
// (execute) and the one a help page draws, not across the tree: that walk was
// a quarter of the time it takes to build the tree, on every command.
func addHelpFlag(c *cobra.Command) {
	if f := c.Flags().Lookup(helpName); f != nil {
		f.Usage = helpFlagUsage
		return
	}
	for p := c; p != nil; p = p.Parent() {
		if p.PersistentFlags().Lookup(helpName) != nil {
			return
		}
	}
	c.Flags().BoolP(helpName, "h", false, helpFlagUsage)
	_ = c.Flags().SetAnnotation(helpName, cobra.FlagSetByCobraAnnotation, []string{"true"}) //nolint:errcheck // the flag was added on the line above
}

// helpPage is everything `--help` prints for c.
func helpPage(c *cobra.Command, platform, platformVersion string, width int) string {
	var b strings.Builder
	if !c.HasParent() {
		b.WriteString(rootBanner + "\n")
	}
	if text := strings.TrimSpace(firstNonEmpty(commandLong(c), c.Short)); text != "" {
		b.WriteString(wrapText(text, width) + "\n\n")
	}
	if c.Runnable() || c.HasSubCommands() {
		b.WriteString(usageText(c, width))
	}
	if contextMatters(c) {
		b.WriteString("\n" + contextLine(platform, platformVersion) + "\n")
	}
	return b.String()
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
// is noise. Nor does a removed command's stub, which runs none of it either.
func contextMatters(c *cobra.Command) bool {
	if cliout.IsRemovedCommand(c) {
		return false
	}
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
func usageText(c *cobra.Command, width int) string {
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

	blocks = append(blocks, commandSections(c, width)...)
	blocks = append(blocks, flagSections(c, width)...)

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
func commandSections(c *cobra.Command, width int) []string {
	available, topics := listedCommands(c)
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
			b.WriteString("\n" + commandRow(sub, column, width))
		}
		sections = append(sections, b.String())
	}

	grouped, ungrouped := splitByGroup(c, available)
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

// listedCommands is the children a page lists: the available ones, which
// include the help command (IsAvailableCommand turns down cobra's own, which
// cobra's template lists by name; so does this), and the help topics.
func listedCommands(c *cobra.Command) (available, topics []*cobra.Command) {
	for _, sub := range c.Commands() {
		switch {
		case sub.IsAvailableCommand() || sub.Name() == helpName:
			available = append(available, sub)
		case sub.IsAdditionalHelpTopicCommand():
			topics = append(topics, sub)
		}
	}
	return available, topics
}

// splitByGroup sorts available into the groups c declares and the rest.
func splitByGroup(c *cobra.Command, available []*cobra.Command) (grouped map[string][]*cobra.Command, ungrouped []*cobra.Command) {
	grouped = map[string][]*cobra.Command{}
	for _, sub := range available {
		if sub.GroupID != "" && c.ContainsGroup(sub.GroupID) {
			grouped[sub.GroupID] = append(grouped[sub.GroupID], sub)
		} else {
			ungrouped = append(ungrouped, sub)
		}
	}
	return grouped, ungrouped
}

// listedInOrder is listedCommands in the order a page reads them: each group
// in declaration order, then the ungrouped ones, then the topics.
func listedInOrder(c *cobra.Command) []*cobra.Command {
	available, topics := listedCommands(c)
	grouped, ungrouped := splitByGroup(c, available)
	var out []*cobra.Command
	for _, g := range c.Groups() {
		out = append(out, grouped[g.ID]...)
	}
	out = append(out, ungrouped...)
	return append(out, topics...)
}

// commandRow is one line of a command list: its spellings padded to the
// column, then its description wrapped under itself. Spellings wider than the
// column take a line of their own.
func commandRow(sub *cobra.Command, column, width int) string {
	const indent = 2
	const gap = 2
	names := commandSpellings(sub)
	descIndent := indent + column + gap
	desc := wrapHanging(sub.Short, width, descIndent)
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
func flagSections(c *cobra.Command, width int) []string {
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
	type section struct {
		title string
		rows  []flagRow
	}
	var titled []section
	if set := groups[""]; set != nil {
		titled = append(titled, section{"Flags:", flagRows(set)})
	}
	for _, group := range definitionOrder(c, order) {
		if group == "" {
			continue
		}
		titled = append(titled, section{group + " Flags:", flagRows(groups[group])})
	}

	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == verbosityFlag || f.Name == versionFlag {
			return
		}
		inherited.AddFlag(f)
	})
	if inherited.HasFlags() {
		titled = append(titled, section{"Global Flags:", flagRows(inherited)})
	}

	// One description column for every flag section, so the page reads as one
	// list under several headings rather than a column that moves with each.
	var all []flagRow
	for _, sec := range titled {
		all = append(all, sec.rows...)
	}
	column := flagColumn(all)
	for _, sec := range titled {
		sections = append(sections, sec.title+"\n"+renderFlagRows(sec.rows, column, width))
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

// flagRow is one flag as help lists it: the flag and its value, then its
// description.
type flagRow struct{ left, usage string }

// flagRows renders a set's visible flags into rows.
//
// pflag's own wrapping (FlagUsagesWrapped) gives up at the first word wider
// than the room left — a URL, usually — and prints the rest of the description
// on one line. Its column layout is private, so each flag is rendered alone,
// unwrapped, and split back into the two columns pflag joined: that keeps its
// spelling of value names, defaults and deprecations, which this does not
// restate.
func flagRows(set *pflag.FlagSet) []flagRow {
	var rows []flagRow
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		one := pflag.NewFlagSet("", pflag.ContinueOnError)
		one.AddFlag(f)
		left, usage := splitFlagUsage(one.FlagUsagesWrapped(0))
		rows = append(rows, flagRow{left, usage})
	})
	return rows
}

// flagColumn is the width of the flag column for rows: the widest flag, but
// no wider than helpNameColumnMax. A flag past the cap takes a line of its
// own rather than pushing every description to the right.
func flagColumn(rows []flagRow) int {
	column := 0
	for _, r := range rows {
		if len(r.left) <= helpNameColumnMax {
			column = max(column, len(r.left))
		}
	}
	return column
}

// renderFlagRows lists rows in two columns: the flag padded to column, then
// its description wrapped under itself. A flag wider than column puts its
// description on the next line, at the same indent as the rest.
func renderFlagRows(rows []flagRow, column, width int) string {
	const gap = 3
	hang := column + gap
	var lines []string
	for _, r := range rows {
		var parts []string
		for _, line := range strings.Split(r.usage, "\n") {
			parts = append(parts, wrapHanging(line, width, hang))
		}
		desc := strings.Join(parts, "\n"+strings.Repeat(" ", hang))
		if len(r.left) > column {
			lines = append(lines, r.left+"\n"+strings.Repeat(" ", hang)+desc)
			continue
		}
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
		if utf8.RuneCountInString(line) <= width || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
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
// room left is not broken.
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
		case lineLen+1+utf8.RuneCountInString(word) > room:
			b.WriteString("\n" + strings.Repeat(" ", hang))
			lineLen = 0
		default:
			b.WriteString(" ")
			lineLen++
		}
		b.WriteString(word)
		lineLen += utf8.RuneCountInString(word)
	}
	return b.String()
}
