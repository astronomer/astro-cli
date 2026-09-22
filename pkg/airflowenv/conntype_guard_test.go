package airflowenv

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// A value with no conn_type is not a connection: Airflow resolves a provider
// from conn_type, so there is nothing to connect with. Decoding it anyway would
// hand a caller a Connection that cannot work, and the desktop's copy of this
// codec has always refused it.
func TestDecodeConnEnvRefusesAValueWithNoConnType(t *testing.T) {
	key := EnvKeyForConnID("warehouse")
	for _, value := range []string{
		`{}`,
		`{"host":"h"}`,
		`{"conn_type":""}`,
		`{"conn_type":"","host":"h","login":"u","password":"p"}`,
	} {
		if c, ok := DecodeConnEnv(key, value); ok {
			t.Errorf("DecodeConnEnv accepted %q as connection %q", value, c.ConnID)
		}
	}

	// Not over-broad: a conn_type is all it asks for.
	for _, value := range []string{
		`{"conn_type":"postgres"}`,
		`{"conn_type":"http","host":"api"}`,
	} {
		if _, ok := DecodeConnEnv(key, value); !ok {
			t.Errorf("DecodeConnEnv refused %q, which carries a conn_type", value)
		}
	}
}

// The id is still readable when the value is refused, which is what lets a
// caller tell an unusable entry from an absent one.
func TestTheIDSurvivesARefusedValue(t *testing.T) {
	key := EnvKeyForConnID("warehouse")
	if _, ok := DecodeConnEnv(key, `{"host":"h"}`); ok {
		t.Fatal("expected the typeless value to be refused")
	}
	if got := ConnIDForEnvKey(key); got != "warehouse" {
		t.Errorf("ConnIDForEnvKey = %q, want %q", got, "warehouse")
	}
}

// NormalizeConn is the one definition of what a stored connection looks like,
// and it decodes through DecodeConnEnv — so the guard refuses a typeless
// connection at the WRITE boundary (`astro local env connection set --secret`, and the v1
// airflow_settings carry-over) rather than storing something unusable.
func TestNormalizeConnRefusesAValueWithNoConnType(t *testing.T) {
	for _, raw := range []string{`{}`, `{"host":"h"}`, `{"conn_type":""}`} {
		_, err := NormalizeConn("warehouse", raw)
		if err == nil {
			t.Errorf("NormalizeConn stored %q, which names no conn_type", raw)
			continue
		}
		// The message has to name what is missing: the value parses, so
		// "not valid JSON" would send the user looking for a syntax error.
		if !strings.Contains(err.Error(), "conn_type") {
			t.Errorf("NormalizeConn(%q) error does not name conn_type: %v", raw, err)
		}
	}
}

// What it still accepts. A URI always names a conn_type in its scheme, so that
// half is untouched by the guard.
func TestNormalizeConnStillAcceptsWhatItShould(t *testing.T) {
	for _, raw := range []string{
		`{"conn_type":"postgres","host":"db","login":"u","password":"p","port":5432}`,
		"postgres://u:p@db:5432/analytics",
		"snowflake://user:pw@account/db",
	} {
		val, err := NormalizeConn("warehouse", raw)
		if err != nil {
			t.Errorf("NormalizeConn(%q) = %v, want it accepted", raw, err)
			continue
		}
		// Whatever it stored must read back, or the two halves disagree.
		c, ok := DecodeConnEnv(EnvKeyForConnID("warehouse"), val)
		if !ok {
			t.Errorf("NormalizeConn(%q) stored %q, which DecodeConnEnv refuses", raw, val)
			continue
		}
		if c.ConnType == "" {
			t.Errorf("NormalizeConn(%q) stored a value with no conn_type: %q", raw, val)
		}
	}
}

// The encoder is unchanged, so it still emits conn_type even when empty. That
// asymmetry is deliberate for now: pkg/scaffold encodes a connection while
// carrying its conn_type separately, so refusing here needs its own look.
// Recorded as a test so the asymmetry is visible rather than surprising.
func TestEncodeStillEmitsATypelessConnection(t *testing.T) {
	_, val, ok := EncodeConnEnv(connmodel.Connection{ConnID: "warehouse", ConnHost: "h"})
	if !ok {
		t.Fatal("EncodeConnEnv refused a typeless connection; if that is intended, this test should change")
	}
	if _, ok := DecodeConnEnv(EnvKeyForConnID("warehouse"), val); ok {
		t.Error("DecodeConnEnv accepted the typeless value the encoder produced")
	}
}
