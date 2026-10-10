package scaffold

import "strings"

// ShellQuote is s as one shell word, for a path or argument the CLI prints in
// a command to run: unchanged when every rune is safe unquoted, else wrapped
// in single quotes, with an embedded single quote closed, escaped and
// reopened. POSIX shells and PowerShell both read a single-quoted word
// literally (PowerShell spells an embedded single quote differently, which a
// path rarely holds); a Windows path, with its backslashes, is always quoted.
// The one quoting every printed command uses, astro config's suggestions
// included.
func ShellQuote(s string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !isShellSafe(r) }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.,:/=@+%", r)
}
