package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"
)

// servedAt is what the daemon served when DaemonRevision was last bumped: the
// revision and a fingerprint of its pages and Airflow theme.
var servedAt = struct {
	revision    int
	fingerprint string
}{1, "552d69bf241a68457e6e9a66f9d427eb6a354e4c6548fb4f49fdc4f20e98df19"}

// servedFingerprint hashes what a user sees from the daemon that is not
// code: the snippet it injects into every Airflow page and the pages it
// renders itself.
func servedFingerprint(t *testing.T) string {
	t.Helper()
	h := sha256.New()
	h.Write([]byte(airflowThemeSnippet))
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil || len(names) == 0 {
		t.Fatalf("no page templates to fingerprint: %v", err)
	}
	for _, name := range names { // fs.Glob returns them sorted
		body, err := fs.ReadFile(templateFS, name)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(name))
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// A running daemon is only replaced by one with a newer DaemonRevision, so a
// change to its pages or the Airflow theme without a bump stays invisible to
// everyone whose daemon is already running, which is every user of the
// desktop or `astro local`. This makes that bump a decision someone has to
// make.
func TestDaemonRevisionCoversWhatItServes(t *testing.T) {
	got := servedFingerprint(t)
	switch {
	case DaemonRevision == servedAt.revision && got != servedAt.fingerprint:
		t.Fatalf("the daemon's pages or Airflow theme changed since DaemonRevision %d. "+
			"Bump DaemonRevision so running daemons are replaced, then record it here: {%d, %q}",
			DaemonRevision, DaemonRevision+1, got)
	case DaemonRevision != servedAt.revision:
		t.Fatalf("DaemonRevision is %d but servedAt records %d. Record what it serves: {%d, %q}",
			DaemonRevision, servedAt.revision, DaemonRevision, got)
	}
}
