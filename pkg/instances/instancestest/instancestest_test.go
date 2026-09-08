package instancestest

import (
	"context"
	"strings"
	"testing"
)

// The two guards below are the only behavior this package adds to the copies
// it replaced, so they are the only things here worth a test. Everything else
// is the preamble and a parse, which the 57 tests across the three consumers
// exercise by using them.

// A caller that keeps mutating the map it passed cannot change what the
// environment reports, which is the difference between reading a snapshot and
// reading a live map through a closure.
func TestEnvReadsASnapshotOfTheMap(t *testing.T) {
	pairs := map[string]string{"AF_ID": "cli"}
	lookup := Env(pairs)

	pairs["AF_SECRET"] = "leaked-later"
	delete(pairs, "AF_ID")

	if v, ok := lookup("AF_ID"); !ok || v != "cli" {
		t.Errorf("AF_ID = %q/%v after the caller deleted it, want the value Env was given", v, ok)
	}
	if _, ok := lookup("AF_SECRET"); ok {
		t.Error("AF_SECRET resolved: a key added after Env was called reached the lookup")
	}
}

// A credential with no scheme cannot be sent by any caller, so reporting it as
// the empty header would let a source leak a value past a test asserting that
// nothing is sent.
func TestHeaderRefusesAValueWithNoScheme(t *testing.T) {
	_, err := header(func(context.Context) (string, string, error) {
		return "", "leaked-token", nil
	})
	if err == nil {
		t.Fatal("a value with no scheme produced no error, so it reads as sending nothing")
	}
	if strings.Contains(err.Error(), "leaked-token") {
		t.Errorf("err = %v: the refusal must not repeat the credential", err)
	}

	// The genuinely credential-less source keeps reporting the empty header,
	// which is what the 'none' and open-dev-server cases assert on.
	for name, src := range map[string]func(context.Context) (string, string, error){
		"no source":  nil,
		"empty pair": func(context.Context) (string, string, error) { return "", "", nil },
	} {
		got, err := header(src)
		if err != nil {
			t.Errorf("%s: err = %v, want the empty header", name, err)
		}
		if got != "" {
			t.Errorf("%s: header = %q, want empty", name, got)
		}
	}
}
