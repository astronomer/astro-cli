package manifest

import "strings"

// nearMiss is the key in known that name most likely misspells, or "" when
// none is close: the same key once case and the _/- separators are set aside
// (client_registry for client-registry), or one within a small edit distance,
// 1 for a key of up to four letters and 2 for a longer one, a swap of two
// neighbors counting as one edit (defalt for default). The closest wins, and
// the first in known on a tie.
func nearMiss(name string, known []string) string {
	norm := normalizeKey(name)
	best, bestDist := "", -1
	for _, k := range known {
		if normalizeKey(k) == norm {
			return k
		}
		limit := 2
		if len(k) <= shortKey {
			limit = 1
		}
		if d := editDistance(strings.ToLower(name), k); d <= limit && (bestDist < 0 || d < bestDist) {
			best, bestDist = k, d
		}
	}
	return best
}

// shortKey is the longest key a near-miss is held to one edit of; two edits
// of a short key reach too many unrelated words.
const shortKey = 4

func normalizeKey(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "-")
}

// editDistance is the optimal string alignment distance between a and b: the
// insertions, deletions, substitutions and swaps of two neighboring letters
// that turn one into the other.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
