package uv

import "testing"

// A requirement or index URL can carry credentials; uv's debug line masks them.
func TestLogLineMasksURLCredentials(t *testing.T) {
	got := logLine("uv", []string{"pip", "install", "pkg @ git+https://ghp_token@github.com/o/r", "--find-links=https://u:p@host/simple", "apache-airflow==3.1.0"})
	want := "uv pip install pkg @ git+https://***@github.com/o/r --find-links=https://***@host/simple apache-airflow==3.1.0"
	if got != want {
		t.Errorf("logLine() =\n%q\nwant\n%q", got, want)
	}
}
