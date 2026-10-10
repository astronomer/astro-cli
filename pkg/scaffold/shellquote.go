package scaffold

import (
	"runtime"
	"strings"
)

// ShellQuote is s as one shell word, for a path or argument the CLI prints in
// a command to run: unchanged when every rune is safe unquoted, else quoted
// for the shell of the platform the CLI runs on (shellQuote). The one
// quoting every printed command uses, astro config's suggestions included.
func ShellQuote(s string) string { return shellQuote(s, runtime.GOOS) }

// shellQuote is ShellQuote for goos. On Windows it wraps s in double quotes,
// which cmd.exe and PowerShell both read as one word ("C:\a b"); a Windows
// path cannot hold a double quote. Elsewhere it wraps s in single quotes, with
// an embedded single quote closed, escaped and reopened, as POSIX shells read
// it.
func shellQuote(s, goos string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !isShellSafe(r) }) {
		return s
	}
	if goos == "windows" {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.,:/=@+%", r)
}
