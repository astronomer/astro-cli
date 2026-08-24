package secrets

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestKeyRoundTrip(t *testing.T) {
	// A real absolute path for THIS platform, not a POSIX literal. Key's scope check
	// is filepath.IsAbs, which on Windows wants a volume name — "/Users/me/etl" is
	// rooted there but not absolute, so a hardcoded POSIX path makes every write-side
	// test in this file fail on the one platform where Docker is the desktop's only
	// runtime mode. Nothing caught that: CI's test-windows job runs `go test ./...`
	// from the repo root, which stops at module boundaries, so no pkg/* sub-module is
	// tested on Windows at all.
	checkout := filepath.Join(t.TempDir(), "etl")

	for _, tc := range []struct {
		name  string
		kind  Kind
		scope string
		item  string
	}{
		{"env in a checkout", KindEnv, checkout, "DATABASE_URL"},
		{"global var", KindVar, GlobalScope, "SLACK_CHANNEL"},
		{"connection", KindConn, checkout, "warehouse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := Key(tc.kind, tc.scope, tc.item)
			if err != nil {
				t.Fatalf("Key: %v", err)
			}
			kind, scope, name, err := ParseKey(key)
			if err != nil {
				t.Fatalf("ParseKey(%q): %v", key, err)
			}
			if kind != tc.kind || scope != tc.scope || name != tc.item {
				t.Errorf("round trip of %q = (%q, %q, %q), want (%q, %q, %q)",
					key, kind, scope, name, tc.kind, tc.scope, tc.item)
			}
		})
	}
}

// The wire format itself, spelled out. Every test above this one is a round trip,
// and a round trip is blind to exactly the change that would hurt most: rename the
// separator and Key and ParseKey agree with each other while every key already in
// a user's keyring becomes unreadable — keyring.go embeds the key in each value
// file and hashes it into the filename, so nothing errors, the secrets simply stop
// existing. These assertions fail instead. The colons here are literal on purpose,
// not built from sep.
func TestKeyFormatIsStable(t *testing.T) {
	global, err := Key(KindVar, GlobalScope, "SLACK_CHANNEL")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if global != "var:global:SLACK_CHANNEL" {
		t.Errorf("Key = %q, want %q", global, "var:global:SLACK_CHANNEL")
	}

	// The scope is a real absolute path on whatever platform this runs, so the
	// golden is composed rather than literal; the separators are still literal.
	scope := t.TempDir()
	scoped, err := Key(KindEnv, scope, "DATABASE_URL")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if want := "env:" + scope + ":DATABASE_URL"; scoped != want {
		t.Errorf("Key = %q, want %q", scoped, want)
	}
}

// The reason the split reads from both ends: a scope may contain colons. Built
// through Key with a path absolute on THIS platform — see checkScope on why the
// write side is GOOS-specific and the read side deliberately is not.
func TestKeyRoundTripsAColonInTheScope(t *testing.T) {
	scope := filepath.Join(t.TempDir(), "pro:ject")
	key, err := Key(KindConn, scope, "warehouse")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	kind, gotScope, name, err := ParseKey(key)
	if err != nil {
		t.Fatalf("ParseKey(%q): %v", key, err)
	}
	if gotScope != scope {
		t.Errorf("scope = %q, want %q — the split must not tear a colon-bearing path", gotScope, scope)
	}
	if kind != KindConn || name != "warehouse" {
		t.Errorf("got kind %q name %q, want conn/warehouse", kind, name)
	}
}

// The case that motivated the both-ends split, tested where it can be: parsing is
// platform-independent, so a Windows key is checked as a literal rather than built
// through Key. A left-to-right split parses this to a scope of "C", and Docker is
// the only runtime mode on Windows, so it would be every key on that platform.
func TestParseKeyHandlesAWindowsDriveColon(t *testing.T) {
	const key = `conn:C:\Users\me\projects\etl:warehouse`
	kind, scope, name, err := ParseKey(key)
	if err != nil {
		t.Fatalf("ParseKey(%q): %v", key, err)
	}
	if scope != `C:\Users\me\projects\etl` {
		t.Errorf("scope = %q; a drive letter is not a kind", scope)
	}
	if kind != KindConn || name != "warehouse" {
		t.Errorf("got kind %q name %q, want conn/warehouse", kind, name)
	}
}

// A scope that ends in a colon puts two of them side by side, which is the shape
// most likely to be "fixed" by a future reader collapsing empty parts. It must not
// be: a directory named "etl:" is legal on POSIX, and eating that colon files the
// secret under a directory that does not exist.
func TestParseKeyKeepsATrailingColonInTheScope(t *testing.T) {
	scope := filepath.Join(t.TempDir(), "etl:")
	key, err := Key(KindEnv, scope, "DATABASE_URL")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	_, gotScope, name, err := ParseKey(key)
	if err != nil {
		t.Fatalf("ParseKey(%q): %v", key, err)
	}
	if gotScope != scope || name != "DATABASE_URL" {
		t.Errorf("round trip of %q = (%q, %q), want (%q, %q)", key, gotScope, name, scope, "DATABASE_URL")
	}
}

// A colon in the name is refused rather than escaped: it would round-trip to a
// different key, which is worse than failing, and every name this grammar carries
// is already restricted to letters, digits and underscores upstream.
func TestKeyRejectsAColonInTheName(t *testing.T) {
	if _, err := Key(KindEnv, filepath.Join(t.TempDir(), "p"), "A:B"); !errors.Is(err, ErrBadKey) {
		t.Errorf("err = %v, want ErrBadKey for a name containing a colon", err)
	}
}

func TestKeyRejectsEmptyParts(t *testing.T) {
	if _, err := Key(KindEnv, "", "NAME"); !errors.Is(err, ErrBadKey) {
		t.Error("an empty scope must be refused; GlobalScope is the way to say 'no checkout'")
	}
	if _, err := Key(KindEnv, filepath.Join(t.TempDir(), "p"), ""); !errors.Is(err, ErrBadKey) {
		t.Error("an empty name must be refused")
	}
	if _, err := Key(Kind("secret"), filepath.Join(t.TempDir(), "p"), "NAME"); !errors.Is(err, ErrBadKey) {
		t.Error("an unknown kind must be refused rather than written into a key")
	}
}

// A truncated or foreign key must not parse as valid. GlobalScope being a literal
// rather than the empty string is what makes "env::NAME" and "env:NAME" both fail
// instead of one of them meaning "global".
func TestParseKeyRejectsMalformed(t *testing.T) {
	for _, key := range []string{
		"",
		"env",
		"env:NAME",
		"env::NAME",
		"env:/tmp/p:",
		"secret:/tmp/p:NAME",
		":/tmp/p:NAME",
	} {
		if _, _, _, err := ParseKey(key); !errors.Is(err, ErrBadKey) {
			t.Errorf("ParseKey(%q) = %v, want ErrBadKey", key, err)
		}
	}
}

// The read side has its own scope rule, and these are the keys that made it
// necessary: a peer tool writing a name with a colon in it produces a key whose
// scope is not a scope. Parsed without checking, "env:global:MY:VAR" resolves to a
// checkout called "global:MY" — so a caller filtering on GlobalScope drops the
// user's global secret, and one listing per-project secrets invents a directory.
// Refusing to parse turns a silent mis-attribution into an error.
func TestParseKeyRejectsAnImplausibleScope(t *testing.T) {
	for _, key := range []string{
		"env:global:MY:VAR", // a colon in the name, read as a scope
		"env:relative/p:N",  // never canonicalized
		"env::global:N",     // scope ":global"
		"env:C:N",           // a drive letter with no path after it
		"env:4:\\x:N",       // not a drive letter
	} {
		if _, _, _, err := ParseKey(key); !errors.Is(err, ErrBadKey) {
			t.Errorf("ParseKey(%q) = %v, want ErrBadKey", key, err)
		}
	}
}

// The other half of that rule: it must not reject a key merely because it was
// written on a different platform. filepath.IsAbs would — a Windows path is not
// absolute on Unix — and a vault two tools share cannot have a reader that refuses
// what the writer wrote.
func TestParseKeyAcceptsForeignAbsoluteScopes(t *testing.T) {
	for _, tc := range []struct{ key, scope string }{
		{`env:C:\Users\me\etl:DATABASE_URL`, `C:\Users\me\etl`},
		{"env:C:/Users/me/etl:DATABASE_URL", "C:/Users/me/etl"},
		{`env:\\srv\share\etl:DATABASE_URL`, `\\srv\share\etl`},
		{"env:/Users/me/etl:DATABASE_URL", "/Users/me/etl"},
	} {
		_, scope, _, err := ParseKey(tc.key)
		if err != nil {
			t.Errorf("ParseKey(%q): %v", tc.key, err)
			continue
		}
		if scope != tc.scope {
			t.Errorf("ParseKey(%q) scope = %q, want %q", tc.key, scope, tc.scope)
		}
	}
}

// A relative scope is the realistic version of "forgot to canonicalize": the key
// would be filed under whatever directory the process happened to be in, so the
// same project keys differently depending on where the tool was launched, and
// neither tool ever sees the other's values.
//
// This is the part Key can enforce without touching the filesystem. It cannot
// catch an unresolved symlink — hence the documented precondition rather than a
// promise.
func TestKeyRejectsARelativeScope(t *testing.T) {
	for _, scope := range []string{"relative/path", "./etl", "..", "etl"} {
		if _, err := Key(KindEnv, scope, "NAME"); !errors.Is(err, ErrBadKey) {
			t.Errorf("Key with scope %q = %v, want ErrBadKey", scope, err)
		}
	}
}

// GlobalScope is the one scope that is not a path, so the absolute-path check has
// to let it through.
func TestKeyAcceptsGlobalScope(t *testing.T) {
	key, err := Key(KindVar, GlobalScope, "SLACK_CHANNEL")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if _, scope, _, err := ParseKey(key); err != nil || scope != GlobalScope {
		t.Errorf("ParseKey(%q) scope = %q err = %v, want %q", key, scope, err, GlobalScope)
	}
}
