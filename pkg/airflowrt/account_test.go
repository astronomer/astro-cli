package airflowrt

import (
	"strings"
	"testing"
)

// The value is pinned, not just the agreement.
//
// Agreement between the Go callers is the compiler's job now. What no compiler
// can check is that the value still matches Airflow instances that already
// exist: this pair is written into the user table of every Airflow 2 the engines
// have ever provisioned, so changing it does not break a build — it locks people
// out of Airflows already running on their machine.
func TestAirflow2AccountValue(t *testing.T) {
	if Airflow2AdminUser != "admin" || Airflow2AdminPassword != "admin" {
		t.Errorf("account = %s/%s, want admin/admin; existing containers and standalone databases have this baked into their user table",
			Airflow2AdminUser, Airflow2AdminPassword)
	}
}

// The shim is the writer that cannot import anything.
//
// AF2DarwinShim is embedded Python, so its `users create` call is a string
// literal that no amount of Go plumbing can make reference the constant. It is
// also the reason the pair lives in this package rather than in pkg/localrt/rt,
// where the docker engine and the CLI could both reach it and this one could
// not. Scanning the script is the only check available, and without it a change
// to the constant moves docker mode and the CLI while standalone on macOS keeps
// seeding the old password — a 401 on every authenticated call to a standalone
// Airflow 2, on the one platform where that shim runs.
func TestDarwinShimSeedsTheSameAccount(t *testing.T) {
	// Whitespace-collapsed, because black owns this file's layout (prek.toml runs
	// it over standalone_scripts too). Matching literal indentation would turn any
	// reformat into a failure claiming the credentials had drifted.
	script := strings.Join(strings.Fields(string(AF2DarwinShim)), " ")

	// Anchored on the users-create argv, and bounded by the end of that list, so
	// the flag/value pairs below must belong to THAT call. Unanchored Contains
	// would pass if a second subprocess call elsewhere in the script happened to
	// carry a matching pair while the real one drifted.
	const anchor = `"users", "create",`
	start := strings.Index(script, anchor)
	if start < 0 {
		t.Fatalf("af2_darwin_shim.py no longer creates a user (no %s); this test is checking the wrong thing", anchor)
	}
	block := script[start:]
	if end := strings.Index(block, "],"); end >= 0 {
		block = block[:end]
	}

	for _, want := range []struct{ flag, value string }{
		{"--username", Airflow2AdminUser},
		{"--password", Airflow2AdminPassword},
	} {
		// The flag and the value that FOLLOWS it. The script says "admin" several
		// times besides — the email, a log line — so asserting the word appears
		// somewhere passes even when the password argument alone has changed.
		pair := `"` + want.flag + `", "` + want.value + `",`
		if !strings.Contains(block, pair) {
			t.Errorf("af2_darwin_shim.py's users-create does not pass %s %q; it seeds a different account than the constants say",
				want.flag, want.value)
		}
	}
}
