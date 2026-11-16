package secrets

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"
)

// errOSKeyringInTest is what the OS keyring answers inside a test binary that
// has not replaced it. Callers wrap it in ErrKeyringUnavailable, and some
// answer that by falling back (Logins keeps a login in the config), which is
// why the guard also counts its refusals and testenv fails the package on any
// (OSKeyringRefusals): a test that reached the real keychain must not pass.
var errOSKeyringInTest = errors.New("tests must not use the OS keychain; call keyring.MockInit (a package's TestMain does, through testenv.Isolate)")

// refusals counts the times the guard refused the OS keyring. A caller may
// answer the error by falling back (Logins keeps a login in the config), so a
// test that bypassed the mock could pass; the count lets the test binary's
// TestMain fail the run anyway (OSKeyringRefusals).
var refusals atomic.Int64

// OSKeyringRefusals is how many times the OS keyring was asked for in this test
// binary and refused. testenv.Isolate fails a package that has any.
func OSKeyringRefusals() int { return int(refusals.Load()) }

// keyringMocked records that go-keyring's default provider is its in-memory
// mock. go-keyring does not say which provider is installed, so the one
// function that installs it (MockKeyringForTests) records it.
var keyringMocked atomic.Bool

// MockKeyringForTests replaces go-keyring's default provider with its in-memory
// mock, and records that, so the OS backend below is not reached. A test
// binary's TestMain calls it through testenv.Isolate; a test that needs a
// keyring that fails calls keyring.MockInitWithError and restores
// keyring.MockInit in t.Cleanup. Installing the mock and recording it are one
// step on purpose: a setup that skipped the install could not still pass the
// guard. It panics outside a test binary, where replacing the OS keychain with
// memory would silently lose every credential written.
func MockKeyringForTests() {
	if !testing.Testing() {
		panic("secrets.MockKeyringForTests called outside a test binary")
	}
	keyring.MockInit()
	keyringMocked.Store(true)
}

// refuseInTests fails the OS keyring backend inside a test binary that never
// declared the mock. A test that reached the real keychain would prompt the
// developer (macOS shows "A keychain cannot be found to store 'master-key'"
// when HOME has moved) or write a credential into their login keychain, so it
// fails instead. testing.Testing is false in every shipped binary, so this
// costs the product nothing.
func refuseInTests() error {
	if testing.Testing() && !keyringMocked.Load() {
		refusals.Add(1)
		return errOSKeyringInTest
	}
	return nil
}
