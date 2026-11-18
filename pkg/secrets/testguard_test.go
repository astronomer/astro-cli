package secrets

import (
	"errors"
	"testing"
	"time"
)

// A test binary that has not installed the in-memory keyring cannot reach the
// OS one: both the plain backend and the one Logins wraps in a timeout refuse,
// with the error that says what to do, and count it.
func TestTheOSKeyringRefusesInATestBinaryThatDidNotMockIt(t *testing.T) {
	keyringMocked.Store(false)
	before := refusals.Load()
	t.Cleanup(func() {
		keyringMocked.Store(true)
		refusals.Store(before) // these refusals are the test's own, not a package's failure
	})

	for name, kr := range map[string]keyringAPI{
		"osKeyring":      osKeyring{},
		"timeoutKeyring": timeoutKeyring{inner: osKeyring{}, timeout: time.Second},
	} {
		if _, err := kr.Get("astro-guard-test", "master-key"); !errors.Is(err, errOSKeyringInTest) {
			t.Errorf("%s.Get err = %v, want errOSKeyringInTest", name, err)
		}
		if err := kr.Set("astro-guard-test", "master-key", "x"); !errors.Is(err, errOSKeyringInTest) {
			t.Errorf("%s.Set err = %v, want errOSKeyringInTest", name, err)
		}
	}
}

// With the mock in place, which a TestMain installs, the same backend works and
// never leaves memory.
func TestTheOSKeyringIsInMemoryOnceMocked(t *testing.T) {
	MockKeyringForTests()
	kr := osKeyring{}
	if err := kr.Set("astro-guard-test", "master-key", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := kr.Get("astro-guard-test", "master-key"); err != nil || got != "v" {
		t.Errorf("Get = %q, %v", got, err)
	}
}
