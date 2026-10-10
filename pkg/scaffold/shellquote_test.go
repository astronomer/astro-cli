package scaffold

import "testing"

// A word every shell reads as is stays bare; anything else is single quoted,
// on every platform.
func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/opt/airflow", "/opt/airflow"},
		{"/my project", "'/my project'"},
		{"/it's", `'/it'\''s'`},
		{"", "''"},
		{`C:\a b`, `'C:\a b'`},
		{"astronomer.io", "astronomer.io"},
	} {
		if got := ShellQuote(tc.in); got != tc.want {
			t.Errorf("ShellQuote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
