package scaffold

import (
	"runtime"
	"strings"
)

// ShellQuote is s as one shell word, for a path or argument the CLI prints in
// a command to run: unchanged when every rune is safe unquoted, else quoted.
// The one quoting every printed command uses, astro config's suggestions
// included.
//
// Quoted means single quotes, with an embedded single quote closed, escaped
// and reopened, as POSIX shells and PowerShell both read it. Except on
// Windows, where a command may be pasted into cmd.exe as well as PowerShell,
// and cmd.exe does not read single quotes as quotes at all: there a word
// goes in double quotes, which both read the same way, whenever it holds
// nothing either expands inside them (see windowsDoubleQuotable).
//
// A word is an argument, never the command itself: PowerShell reads a quoted
// word in command position as a string, not a program to run.
func ShellQuote(s string) string { return shellQuote(s, runtime.GOOS) }

// shellQuote is ShellQuote for goos: on Windows a backslash is safe too, so
// an ordinary path such as C:\proj prints bare, as cmd.exe and PowerShell
// both read it.
func shellQuote(s, goos string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !isShellSafe(r, goos) }) {
		return s
	}
	if goos == windowsOS && windowsDoubleQuotable(s) {
		// A program reads backslashes before a quote as escaping it, so a
		// trailing run is doubled: C:\my dir\ would otherwise end in \", and
		// the word would run on into the rest of the line.
		trimmed := strings.TrimRight(s, `\`)
		return `"` + s + strings.Repeat(`\`, len(s)-len(trimmed)) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// windowsDoubleQuotable reports whether s means the same thing in double
// quotes to cmd.exe and to PowerShell: a space, &, |, <, >, a parenthesis or
// a single quote is literal inside them to both. What is not: a double quote
// ends the word; % expands a variable in cmd.exe even quoted; $ and the
// backtick expand in PowerShell; and a control character has no business in
// a pasted line.
//
// A word holding one of those keeps the single quotes. PowerShell reads them
// correctly; cmd.exe cannot, and no quoting is right in both shells for such
// a word. None turns up in a path or a variable name.
func windowsDoubleQuotable(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool {
		return r < ' ' || r == 0x7f || strings.ContainsRune("\"%$`", r)
	})
}

// isShellSafe reports whether r needs no quoting on goos. Windows adds the
// backslash, its path separator, and drops %, which cmd.exe expands as %VAR%
// even in a bare word.
func isShellSafe(r rune, goos string) bool {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.,:/=@+", r) {
		return true
	}
	if goos == windowsOS {
		return r == '\\'
	}
	return r == '%'
}
