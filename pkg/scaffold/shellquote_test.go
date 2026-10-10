package scaffold

import "testing"

// A word every shell reads as is stays bare, an ordinary Windows path
// included on Windows; anything else is single quoted.
func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ in, goos, want string }{
		{"/opt/airflow", "linux", "/opt/airflow"},
		{"/my project", "linux", "'/my project'"},
		{"/it's", "darwin", `'/it'\''s'`},
		{"", "linux", "''"},
		{`C:\proj`, "linux", `'C:\proj'`},
		{`C:\proj`, "windows", `C:\proj`},
		{`C:\my proj`, "windows", `'C:\my proj'`},
		{"astronomer.io", "windows", "astronomer.io"},
	} {
		if got := shellQuote(tc.in, tc.goos); got != tc.want {
			t.Errorf("shellQuote(%q, %s) = %s, want %s", tc.in, tc.goos, got, tc.want)
		}
	}
}
