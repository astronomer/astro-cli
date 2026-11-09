package cliout

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// suggestFlag is the one flag of cmd's to suggest for the unknown long flag
// err reports, spelled "--name", or "" when err is no such error or no flag
// is clearly the one meant.
//
// Close reads the same settings subcommand suggestions do
// (suggestionDistance): nothing when a command on the way disables
// suggestions, else within the root's distance by EditDistance, or the typed
// name a prefix of the flag's. A short name is held to fewer edits than half
// its length, so neither --ws nor --bogs (two edits from --id and --tags)
// becomes a guess. EditDistance counts swapped neighbours as one edit, so
// --froce is one from --force; subcommand suggestions keep cobra's own
// matcher (SubcommandSuggestions), where that is two. Clearly is one flag
// nearest: two equally close are a guess, and a wrong guess costs a second
// error. A shorthand gets nothing, since one letter is close to every other,
// and hidden and deprecated flags are never offered.
//
// By the time pflag reports a flag it has no such flag for, cobra has merged
// the parents' persistent flags into cmd.Flags(), so they are candidates too.
func suggestFlag(cmd *cobra.Command, err error) string {
	typed, isShorthand, spelling := UnknownFlag(err)
	if spelling == "" || isShorthand {
		return ""
	}
	distance, ok := suggestionDistance(cmd)
	if !ok {
		return ""
	}
	distance = min(distance, max(1, (utf8.RuneCountInString(typed)-1)/2))

	var names []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Deprecated == "" {
			names = append(names, f.Name)
		}
	})
	if matches := Closest(typed, names, distance); len(matches) == 1 {
		return "--" + matches[0]
	}
	return ""
}

// UnknownFlag reports the flag pflag has no such flag for: its name as typed
// (a shorthand's letter alone), whether it was a shorthand, and its spelling
// ("--force", "-f"). The spelling is "" for every other parse error: a
// missing value, or a value of the wrong type, is a mistake on a flag the
// command does have. pflag reports the name on its own, so
// --api-token=secret arrives here as --api-token.
func UnknownFlag(err error) (name string, isShorthand bool, spelling string) {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return "", false, ""
	}
	name = notExist.GetSpecifiedName()
	if notExist.GetSpecifiedShortnames() != "" {
		return name, true, "-" + name
	}
	return name, false, "--" + name
}

// Closest are the candidates nearest to name by cobra's rule for "Did you
// mean", ignoring case: within distance edits, or name a prefix of them, which
// counts as distance. Every candidate at the nearest distance, in the order
// given, not one picked by that order, so the caller decides what a tie means.
// None for an empty name, which is a prefix of everything.
func Closest(name string, candidates []string, distance int) []string {
	name = strings.ToLower(name)
	if name == "" {
		return nil
	}
	best := distance + 1
	var matches []string
	for _, candidate := range candidates {
		lower := strings.ToLower(candidate)
		d := EditDistance(name, lower)
		if strings.HasPrefix(lower, name) && d > distance {
			d = distance
		}
		switch {
		case d > distance:
			continue
		case d < best:
			best, matches = d, []string{candidate}
		case d == best:
			matches = append(matches, candidate)
		}
	}
	return matches
}

// EditDistance is the edit distance between a and b counting an insertion,
// a deletion, a substitution, or two neighbours swapped as one edit each
// (optimal string alignment), by character rather than byte, so an accented
// letter is one edit, not two. A swap is the commonest typo (--froce,
// apc.page_siez), and plain Levenshtein counts it as two.
func EditDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	// Three rows: the one two back is what a swap is measured from.
	back, prev, cur := make([]int, len(rb)+1), make([]int, len(rb)+1), make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], back[j-2]+1)
			}
		}
		back, prev, cur = prev, cur, back
	}
	return prev[len(rb)]
}
