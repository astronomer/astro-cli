package local

import "strconv"

// plural is n followed by the noun that fits it: "1 Dag", "3 Dags". Output
// counts things this way rather than as "Dag(s)" (docs/output-style.md).
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
