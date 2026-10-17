package proxy

import (
	"bytes"
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
}{1, "5d532abda52505ea2ab45691804c70505d5e83e8fac27bcf4fc056c1f5082a09"}

// servedFingerprint hashes what a user sees from the daemon that is not
// routing: the snippet it injects into every Airflow page, every file it
// embeds for its own pages, and those pages rendered with fixed data, which
// takes in the copy Go supplies to them (the default title and start hint).
func servedFingerprint(t *testing.T) string {
	t.Helper()
	h := sha256.New()
	h.Write([]byte(airflowThemeSnippet()))

	// The whole embedded tree rather than the embed's own pattern restated, so
	// an asset added beside the templates is covered without anyone editing
	// this test.
	err := fs.WalkDir(templateFS, ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		body, err := fs.ReadFile(templateFS, name)
		h.Write([]byte(name))
		h.Write(body)
		return err
	})
	if err != nil {
		t.Fatalf("fingerprinting the embedded pages: %v", err)
	}

	var pages bytes.Buffer
	for _, page := range []struct {
		name string
		data any
	}{
		{landingPageName, landingData{Title: Pages{}.title()}},
		{landingPageName, landingData{Title: Pages{}.title(), Routes: []LandingRoute{{Name: "demo", URL: "http://demo.localhost:6563", Port: "8080", ProjectDir: "/demo"}}}},
		{notFoundPageName, notFoundData{Hostname: "demo.localhost", Port: "6563"}},
		{unavailablePageName, nil},
	} {
		if err := pageTmpl.ExecuteTemplate(&pages, page.name, page.data); err != nil {
			t.Fatalf("rendering %s: %v", page.name, err)
		}
	}
	h.Write(pages.Bytes())
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
