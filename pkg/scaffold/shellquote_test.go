package scaffold

import "testing"

// A word every shell reads as is stays bare, an ordinary Windows path
// included on Windows; anything else is quoted, in single quotes for POSIX
// shells and in double quotes on Windows wherever cmd.exe and PowerShell both
// read them the same way.
func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ in, goos, want string }{
		{"/opt/airflow", "linux", "/opt/airflow"},
		{"/my project", "linux", "'/my project'"},
		{"/it's", "darwin", `'/it'\''s'`},
		{"", "linux", "''"},
		{`C:\proj`, "linux", `'C:\proj'`},
		{"100%", "linux", "100%"},
		{`C:\proj`, "windows", `C:\proj`},
		{"astronomer.io", "windows", "astronomer.io"},
		// Double quotes: cmd.exe reads single quotes as plain characters, so
		// 'C:\my proj' split in two there, and 'x&calc&' ran calc.
		{`C:\my proj`, "windows", `"C:\my proj"`},
		{`C:\Users\RUNNER~1\x`, "windows", `"C:\Users\RUNNER~1\x"`},
		{"x&calc&", "windows", `"x&calc&"`},
		{"a|b<c>(d)", "windows", `"a|b<c>(d)"`},
		{"it's", "windows", `"it's"`},
		{"", "windows", `""`},
		{"wow!", "windows", `"wow!"`},
		{"café dir", "windows", `"café dir"`},
		// A trailing backslash run is doubled, or \" would escape the closing
		// quote; one before an inner character is left alone.
		{`C:\my dir\`, "windows", `"C:\my dir\\"`},
		{`C:\my dir\\`, "windows", `"C:\my dir\\\\"`},
		{`C:\my dir\x`, "windows", `"C:\my dir\x"`},
		// What either shell expands inside double quotes keeps single quotes,
		// which PowerShell still reads correctly.
		{"%PATH%", "windows", "'%PATH%'"},
		{"100%", "windows", "'100%'"},
		{"$HOME", "windows", "'$HOME'"},
		{"a`b", "windows", "'a`b'"},
		{`say "hi"`, "windows", `'say "hi"'`},
		{"tab\there", "windows", "'tab\there'"},
		{"line\nbreak", "windows", "'line\nbreak'"},
	} {
		if got := shellQuote(tc.in, tc.goos); got != tc.want {
			t.Errorf("shellQuote(%q, %s) = %s, want %s", tc.in, tc.goos, got, tc.want)
		}
	}
}
