package airflowenv

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// The value pair exists so storage is not bound by the env-var name rule. This
// is the contrast that justifies two pairs rather than one: the same connection
// is storable and not expressible as an env var.
func TestValueCodecAcceptsAnIDTheEnvFormCannot(t *testing.T) {
	c := connmodel.Connection{ConnID: "my-db", ConnType: "postgres", ConnHost: "h", ConnPassword: "p"}

	if _, _, ok := EncodeConnEnv(c); ok {
		t.Fatal("EncodeConnEnv accepted \"my-db\"; AIRFLOW_CONN_MY-DB is not a legal env-var name")
	}

	value, err := EncodeConnValue(c)
	if err != nil {
		t.Fatalf("EncodeConnValue refused a storable id: %v", err)
	}
	got, err := DecodeConnValue("my-db", value)
	if err != nil {
		t.Fatalf("DecodeConnValue could not read back what EncodeConnValue wrote: %v", err)
	}
	if got.ConnID != "my-db" || got.ConnType != "postgres" || got.ConnHost != "h" {
		t.Errorf("round-trip lost fields: %+v", got)
	}
	// Compared without echoing it.
	if got.ConnPassword != "p" {
		t.Errorf("password did not round-trip (%d bytes)", len(got.ConnPassword))
	}
}

// One definition of the shape: the value half of EncodeConnEnv is exactly what
// EncodeConnValue produces, so a vault row and an env var carry the same bytes.
func TestEncodeConnEnvUsesTheSameValueAsEncodeConnValue(t *testing.T) {
	for _, c := range []connmodel.Connection{
		{ConnID: "warehouse", ConnType: "postgres"},
		{ConnID: "warehouse", ConnType: "postgres", ConnHost: "h", ConnLogin: "u", ConnPassword: "p", ConnPort: 5432, ConnSchema: "s"},
		{ConnID: "warehouse", ConnType: "http", ConnExtra: map[string]any{"a": "b"}},
	} {
		_, envValue, ok := EncodeConnEnv(c)
		if !ok {
			t.Fatalf("EncodeConnEnv refused %+v", c)
		}
		bare, err := EncodeConnValue(c)
		if err != nil {
			t.Fatalf("EncodeConnValue: %v", err)
		}
		if envValue != bare {
			t.Errorf("the two encoders disagree:\n  env  =%q\n  value=%q", envValue, bare)
		}
	}
}

// DecodeConnValue is the one place the conn_type requirement lives, and its
// error names the record so a caller can say which stored row is unusable.
func TestDecodeConnValueRequiresAConnTypeAndNamesTheRecord(t *testing.T) {
	for _, value := range []string{`{}`, `{"host":"h"}`, `{"conn_type":""}`} {
		_, err := DecodeConnValue("warehouse", value)
		if err == nil {
			t.Errorf("DecodeConnValue accepted %q", value)
			continue
		}
		if !strings.Contains(err.Error(), "conn_type") {
			t.Errorf("error does not name conn_type: %v", err)
		}
		if !strings.Contains(err.Error(), "warehouse") {
			t.Errorf("error does not name the connection, so a caller cannot report which: %v", err)
		}
	}

	// Malformed JSON is a different error, and still an error.
	if _, err := DecodeConnValue("warehouse", "not json"); err == nil {
		t.Error("DecodeConnValue accepted a value that is not JSON")
	}
}

// The value pair is NOT symmetric, and that is worth pinning rather than
// discovering: the encoder takes a connection with no conn_type and the decoder
// refuses what it produced. A store using this pair without its own check can
// therefore write a row nothing can read back. The env pair avoids it only
// because NormalizeConn guards its write boundary.
func TestTheValuePairIsNotSymmetric(t *testing.T) {
	typeless := connmodel.Connection{ConnID: "warehouse", ConnHost: "h"}

	value, err := EncodeConnValue(typeless)
	if err != nil {
		t.Fatalf("EncodeConnValue refused a typeless connection; if that is now intended, this test should change: %v", err)
	}
	if _, err := DecodeConnValue("warehouse", value); err == nil {
		t.Fatal("DecodeConnValue accepted what EncodeConnValue produced; the asymmetry is gone and the doc should change")
	}
}

// The two doors report the id differently, on purpose. The env form is
// case-insensitive in Airflow so decode lowercases it; the value form takes the
// id from the store's own key and must hand it back unchanged.
//
// Pinned because the difference is invisible for the lowercase ids every other
// test here uses, and a caller keying a map on ConnID across both doors would
// see one connection as two.
func TestTheTwoDoorsReportTheIDDifferentlyOnPurpose(t *testing.T) {
	const mixed = "My_Conn"
	value := `{"conn_type":"postgres"}`

	viaValue, err := DecodeConnValue(mixed, value)
	if err != nil {
		t.Fatalf("DecodeConnValue: %v", err)
	}
	if viaValue.ConnID != mixed {
		t.Errorf("DecodeConnValue id = %q, want %q returned verbatim", viaValue.ConnID, mixed)
	}

	viaEnv, ok := DecodeConnEnv(EnvKeyForConnID(mixed), value)
	if !ok {
		t.Fatal("DecodeConnEnv refused a good value")
	}
	if viaEnv.ConnID != "my_conn" {
		t.Errorf("DecodeConnEnv id = %q, want the lowercased suffix", viaEnv.ConnID)
	}

	if viaValue.ConnID == viaEnv.ConnID {
		t.Error("the two doors now agree on a mixed-case id; if that is the new rule, the package doc should change")
	}
}

// Every error names the record, because the callers that get one are reporting
// which stored connection is unusable. An empty id has no record to name and is
// refused rather than returned as a nameless connection — the invariant both
// other entry points enforce through ValidConnID and IsConnEnvKey.
func TestEveryValueDecodeErrorNamesTheRecord(t *testing.T) {
	for _, value := range []string{`{}`, `{"host":"h"}`, `not json`, ``} {
		_, err := DecodeConnValue("warehouse", value)
		if err == nil {
			t.Errorf("DecodeConnValue accepted %q", value)
			continue
		}
		if !strings.Contains(err.Error(), "warehouse") {
			t.Errorf("error for %q does not name the record: %v", value, err)
		}
	}

	if _, err := DecodeConnValue("", `{"conn_type":"postgres"}`); err == nil {
		t.Error("DecodeConnValue accepted an empty id and returned a nameless connection")
	}
}

// The malformed-JSON error must not carry a piece of the value. It is decrypted
// connection JSON, and json.SyntaxError quotes the byte it stopped on — which
// for a corrupted row can be a byte of the password.
func TestTheJSONErrorDoesNotEchoTheValue(t *testing.T) {
	// A payload whose only distinctive content is the secret itself.
	_, err := DecodeConnValue("warehouse", `{"conn_type":"postgres","password":"sup3rsecret`)
	if err == nil {
		t.Fatal("expected an error for truncated JSON")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Errorf("the error echoed %d bytes of the value", len("sup3rsecret"))
	}
	// And it still says which record and what kind of fault.
	if !strings.Contains(err.Error(), "warehouse") || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("error should name the record and the fault: %v", err)
	}
}
