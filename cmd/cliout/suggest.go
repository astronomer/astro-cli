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
// Close is cobra's rule for "Did you mean this?", read from the same settings
// UnknownSubcommand's suggestions are (suggestionsFor): nothing when a command
// on the way disables suggestions, else within the root's distance, or the
// typed name a prefix of the flag's. A name shorter than twice that distance
// is held to half its length, so --ws does not become --id. Clearly is one
// flag nearest: two equally close are a guess, and a wrong guess costs a
// second error. A shorthand gets nothing, since one letter is close to every
// other, and hidden and deprecated flags are never offered.
//
// By the time pflag reports a flag it has no such flag for, cobra has merged
// the parents' persistent flags into cmd.Flags(), so they are candidates too.
func suggestFlag(cmd *cobra.Command, err error) string {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) || notExist.GetSpecifiedShortnames() != "" {
		return ""
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.DisableSuggestions {
			return ""
		}
	}
	typed := notExist.GetSpecifiedName()
	distance := cmd.Root().SuggestionsMinimumDistance
	if distance <= 0 {
		distance = defaultSuggestionDistance
	}
	distance = min(distance, max(1, utf8.RuneCountInString(typed)/2))

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

// EditDistance is the Levenshtein distance between a and b, by character
// rather than byte, so an accented letter is one edit, not two.
func EditDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
