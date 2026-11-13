package astro

import "strings"

// withoutDeprecations is s without the lines that say a spelling is
// deprecated (cliout.WarnDeprecated). A test that spells a flag the way 1.x
// did, for what the flag does and not for the warning, asserts that nothing
// else was said.
func withoutDeprecations(s string) string {
	var kept strings.Builder
	for line := range strings.Lines(s) {
		if strings.HasPrefix(line, "Warning: ") && strings.Contains(line, " is deprecated: ") {
			continue
		}
		kept.WriteString(line)
	}
	return kept.String()
}
