package scaffold

import "testing"

// A word every shell reads as is stays bare; anything else is quoted for the
// shell of the platform: double quotes on Windows, single quotes elsewhere.
func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ in, goos, want string }{
		{"/opt/airflow", "linux", "/opt/airflow"},
		{"/my project", "linux", "'/my project'"},
		{"/it's", "darwin", `'/it'\''s'`},
		{"", "linux", "''"},
		{`C:\a b`, "windows", `"C:\a b"`},
		{`C:\proj`, "windows", `"C:\proj"`},
		{"astronomer.io", "windows", "astronomer.io"},
	} {
		if got := shellQuote(tc.in, tc.goos); got != tc.want {
			t.Errorf("shellQuote(%q, %s) = %s, want %s", tc.in, tc.goos, got, tc.want)
		}
	}
}
