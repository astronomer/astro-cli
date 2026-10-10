package scaffold

import (
	"runtime"
	"strings"
)

// ShellQuote is s as one shell word, for a path or argument the CLI prints in
// a command to run: unchanged when every rune is safe unquoted, else wrapped
// in single quotes, with an embedded single quote closed, escaped and
// reopened, as POSIX shells and PowerShell both read it. The one quoting
// every printed command uses, astro config's suggestions included.
func ShellQuote(s string) string { return shellQuote(s, runtime.GOOS) }

// shellQuote is ShellQuote for goos: on Windows a backslash is safe too, so
// an ordinary path such as C:\proj prints bare, as cmd.exe and PowerShell
// both read it.
func shellQuote(s, goos string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !isShellSafe(r, goos) }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune, goos string) bool {
	if goos == "windows" && r == '\\' {
		return true
	}
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.,:/=@+%", r)
}
