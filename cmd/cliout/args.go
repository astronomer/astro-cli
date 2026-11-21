package cliout

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Cobra keeps a command's positionals in two places that nothing ties
// together: the Use line a reader sees, and an Args validator that decides
// what the command accepts. Args is the one declaration of both. A command
// names its positionals as data, once:
//
//	cliout.Args(cmd, cliout.Arg{Name: "DEPLOYMENT_ID", Optional: true})
//
// and from that one declaration the Use line shows exactly what is declared,
// Args enforces it, and `--help -o json` reads it back (DeclaredArgs). A
// command with no declaration takes no positionals; a group still refuses a
// word that names none of its subcommands (UndeclaredArgs).

// Arg is one positional argument of a command.
type Arg struct {
	// Name is the placeholder help shows: DEPLOYMENT_ID, KEY=VALUE.
	Name string
	// Optional is whether the command runs without it. An optional argument
	// follows every required one.
	Optional bool
	// Repeated is whether it takes one or more words (any number when
	// Optional). Nothing can follow a repeated argument but the words after --.
	Repeated bool
	// AfterDash is whether it is what follows a "--" on the line, as the
	// command `astro link add NAME -- COMMAND...` hands on. It is the last
	// argument declared.
	AfterDash bool
}

// Required is a required argument named name.
func Required(name string) Arg { return Arg{Name: name} }

// Optional is an optional argument named name.
func Optional(name string) Arg { return Arg{Name: name, Optional: true} }

// ArgsAnnotation holds a command's declaration, for DeclaredArgs.
const ArgsAnnotation = "astro.args"

// AnyWordsAnnotation marks a command that accepts any words after it, with
// the reason: its own RunE (or something it hands the words to) reads them.
const AnyWordsAnnotation = "astro.any-words"

// Args declares cmd's positionals, writes them after the name in its Use,
// and sets its Args validator to enforce the declared counts. Use is the
// command's name alone when it is declared; one that already carries a
// positional is a mistake, and panics where the tree is built so that every
// test run finds it. A command that declares none takes none. It returns cmd,
// so it wraps a literal as well as following a statement.
func Args(cmd *cobra.Command, args ...Arg) *cobra.Command {
	if strings.ContainsAny(cmd.Use, " \t") {
		panic(fmt.Sprintf("cliout.Args: Use %q already carries arguments; declare them with Args only", cmd.Use))
	}
	if err := checkDeclaration(args); err != nil {
		panic(fmt.Sprintf("cliout.Args: %s: %v", cmd.Use, err))
	}
	if len(args) > 0 {
		cmd.Use += " " + useLine(args)
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[ArgsAnnotation] = encodeArgs(args)
	cmd.Args = func(cmd *cobra.Command, words []string) error {
		return validateArgs(cmd, args, words)
	}
	return cmd
}

// CheckArgs adds a check of the words themselves to cmd's declared count
// check (a KEY=VALUE shape, a name that is not another word's alias). It runs
// after the count is right, so it never has to wonder how many words there are.
func CheckArgs(cmd *cobra.Command, check cobra.PositionalArgs) *cobra.Command {
	counted := cmd.Args
	if counted == nil {
		panic("cliout.CheckArgs: " + cmd.Use + " declares no arguments; call Args first")
	}
	cmd.Args = func(cmd *cobra.Command, words []string) error {
		if err := counted(cmd, words); err != nil {
			return err
		}
		return check(cmd, words)
	}
	return cmd
}

// CheckArgsBefore is CheckArgs for a check whose message says more than the
// count's would, so it is read first: a word that names something else than
// what the command takes (`logs triggerer dep-1`, where a Deployment goes)
// is told so, rather than called a stray argument. It runs on whatever
// number of words the line has, none included.
func CheckArgsBefore(cmd *cobra.Command, check cobra.PositionalArgs) *cobra.Command {
	counted := cmd.Args
	if counted == nil {
		panic("cliout.CheckArgsBefore: " + cmd.Use + " declares no arguments; call Args first")
	}
	cmd.Args = func(cmd *cobra.Command, words []string) error {
		if err := check(cmd, words); err != nil {
			return err
		}
		return counted(cmd, words)
	}
	return cmd
}

// AnyWords is for the command that reads the words after it itself and so
// takes any number of them: a group whose RunE names the replacement of a
// removed verb, a stub that only says what replaced it, an internal command
// that hands its argv on. why says what reads them. The words are not
// positionals of the command, so help shows none.
func AnyWords(cmd *cobra.Command, why string) *cobra.Command {
	if why == "" {
		panic("cliout.AnyWords: " + cmd.Use + " needs a reason")
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnyWordsAnnotation] = why
	if _, declared := cmd.Annotations[ArgsAnnotation]; declared {
		delete(cmd.Annotations, ArgsAnnotation)
		cmd.Use = cmd.Name()
	}
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

// Passthrough marks cmd, whose last declared argument and everything after it
// belong to a program it runs (`astro local run pytest --version`), so nothing
// after the first of them is the CLI's to read: not a flag, and not the
// root's --version. Flags before it still are.
func Passthrough(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[ArgvPassthroughAnnotation] = "true"
	cmd.Flags().SetInterspersed(false)
	return cmd
}

// DeclaredArgs is the positionals cmd declared with Args, in order, and none
// for a command that declared none.
func DeclaredArgs(cmd *cobra.Command) []Arg {
	return decodeArgs(cmd.Annotations[ArgsAnnotation])
}

// encodeArgs writes a declaration as the annotation holds it: one word per
// argument, NAME:flags, where flags are o (optional), r (repeated) and d
// (after a dash). A name holds no space or colon (checkDeclaration).
func encodeArgs(args []Arg) string {
	words := make([]string, 0, len(args))
	for _, a := range args {
		flags := ""
		if a.Optional {
			flags += "o"
		}
		if a.Repeated {
			flags += "r"
		}
		if a.AfterDash {
			flags += "d"
		}
		words = append(words, a.Name+":"+flags)
	}
	return strings.Join(words, " ")
}

func decodeArgs(encoded string) []Arg {
	var args []Arg
	for _, word := range strings.Fields(encoded) {
		name, flags, _ := strings.Cut(word, ":")
		args = append(args, Arg{
			Name:      name,
			Optional:  strings.Contains(flags, "o"),
			Repeated:  strings.Contains(flags, "r"),
			AfterDash: strings.Contains(flags, "d"),
		})
	}
	return args
}

// useLine is the positional part of a Use line for args: <NAME> required,
// [NAME] optional, ... after the name of a repeated one ([NAME...] when
// optional, <NAME>... when required), and [-- NAME...] for what follows a dash.
func useLine(args []Arg) string {
	words := make([]string, 0, len(args))
	for _, a := range args {
		dots := ""
		if a.Repeated {
			dots = "..."
		}
		switch {
		case a.AfterDash && a.Optional:
			words = append(words, "[-- "+a.Name+dots+"]")
		case a.AfterDash:
			words = append(words, "-- <"+a.Name+">"+dots)
		case a.Optional:
			words = append(words, "["+a.Name+dots+"]")
		default:
			words = append(words, "<"+a.Name+">"+dots)
		}
	}
	return strings.Join(words, " ")
}

// checkDeclaration refuses a declaration no line could satisfy.
func checkDeclaration(args []Arg) error {
	seenOptional, seenRepeated := false, false
	for i, a := range args {
		switch {
		case a.Name == "" || strings.ContainsAny(a.Name, " <>[]:") || strings.HasSuffix(a.Name, "..."):
			return fmt.Errorf("argument %d has the name %q, which is not a bare placeholder", i+1, a.Name)
		case a.AfterDash && i != len(args)-1:
			return fmt.Errorf("%s follows --, so it is the last argument", a.Name)
		case !a.AfterDash && seenRepeated:
			return fmt.Errorf("%s follows a repeated argument", a.Name)
		case !a.AfterDash && seenOptional && !a.Optional:
			return fmt.Errorf("required %s follows an optional argument", a.Name)
		}
		seenOptional = seenOptional || a.Optional
		seenRepeated = seenRepeated || (a.Repeated && !a.AfterDash)
	}
	return nil
}

// ArgsError is the usage error for a line that gives a command too many or too
// few arguments.
type ArgsError struct {
	// Command is the path of the command that refused the line.
	Command string
	// Unexpected is the first argument past what the command takes, "" when
	// there were too few.
	Unexpected string
	// Missing names the declared arguments the line left out, in order.
	Missing []string
	// Min and Max are how many arguments the command takes; Max is -1 for any
	// number.
	Min, Max int
}

func (e *ArgsError) Error() string {
	if len(e.Missing) > 0 {
		return e.Command + " requires " + joinNames(e.Missing)
	}
	return fmt.Sprintf("unexpected argument %q: %s %s", e.Unexpected, e.Command, takesText(e.Min, e.Max))
}

// joinNames reads names as prose: "A", "A and B", "A, B and C".
func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// takesText is how many arguments a command takes, as the refusal of a stray
// one says it.
func takesText(minimum, maximum int) string {
	plural := func(n int) string {
		if n == 1 {
			return "1 argument"
		}
		return fmt.Sprintf("%d arguments", n)
	}
	switch {
	case maximum == 0:
		return "takes no arguments"
	case minimum == maximum:
		return "takes exactly " + plural(maximum)
	case minimum == 0:
		return "takes at most " + plural(maximum)
	}
	return fmt.Sprintf("takes %d to %d arguments", minimum, maximum)
}

// validateArgs is the count check of a declaration, for the words a run was
// given. With an argument after --, the words before the dash are counted
// against the rest and the words after it against that one; a line with no
// dash counts every word against the first.
func validateArgs(cmd *cobra.Command, declared []Arg, words []string) error {
	var before []Arg
	var tail *Arg
	for i := range declared {
		if declared[i].AfterDash {
			tail = &declared[i]
			continue
		}
		before = append(before, declared[i])
	}
	if tail == nil {
		return countWords(cmd, before, words)
	}
	last := []Arg{{Name: tail.Name, Optional: tail.Optional, Repeated: tail.Repeated}}
	if dash := cmd.ArgsLenAtDash(); dash >= 0 && dash <= len(words) {
		if err := countWords(cmd, before, words[:dash]); err != nil {
			return err
		}
		return countWords(cmd, last, words[dash:])
	}
	if err := countWords(cmd, before, words); err != nil {
		return err
	}
	if !tail.Optional {
		return countWords(cmd, last, nil)
	}
	return nil
}

// countWords holds words to what declared takes: every required argument at
// least once, and no more than the declaration has room for.
func countWords(cmd *cobra.Command, declared []Arg, words []string) error {
	minimum, maximum := 0, len(declared)
	for _, a := range declared {
		if !a.Optional {
			minimum++
		}
		if a.Repeated {
			maximum = -1
		}
	}
	e := &ArgsError{Command: cmd.CommandPath(), Min: minimum, Max: maximum}
	switch {
	case len(words) < minimum:
		for _, a := range declared[len(words):minimum] {
			e.Missing = append(e.Missing, a.Name)
		}
	case maximum >= 0 && len(words) > maximum:
		e.Unexpected = words[maximum]
	default:
		return nil
	}
	return misinvoked(e)
}
