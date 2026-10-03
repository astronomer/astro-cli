package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testCipher(t *testing.T, fill byte) *vaultCipher {
	t.Helper()
	key := make([]byte, keyBytes)
	for i := range key {
		key[i] = byte(i) ^ fill
	}
	c, err := newVaultCipher(key)
	if err != nil {
		t.Fatalf("newVaultCipher: %v", err)
	}
	return c
}

// sealV1 writes the older envelope the way builds before v2 did: no AAD, no
// key id. Tests use it to stand in for a vault an older build left behind.
func sealV1(t *testing.T, c *vaultCipher, plain string) string {
	t.Helper()
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	return encPrefixV1 + base64.StdEncoding.EncodeToString(c.gcm.Seal(nonce, nonce, []byte(plain), nil))
}

func TestSealOpenRoundTrip(t *testing.T) {
	c := testCipher(t, 0)
	got, err := c.seal("env:global:A", "hunter2")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !strings.HasPrefix(got, encPrefixV2+c.kid+":") {
		t.Fatalf("want a v2 envelope carrying the key id, got %q", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatal("ciphertext contains the plaintext")
	}
	plain, legacy, err := c.open("env:global:A", got)
	if err != nil || legacy || plain != "hunter2" {
		t.Fatalf("open: legacy=%v err=%v, round trip matched=%v", legacy, err, plain == "hunter2")
	}
}

func TestSealEncryptsTheEmptyString(t *testing.T) {
	c := testCipher(t, 0)
	got, err := c.seal("env:global:A", "")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !strings.HasPrefix(got, encPrefixV2) {
		t.Fatalf("empty value stored as %q, want a v2 envelope", got)
	}
	plain, _, err := c.open("env:global:A", got)
	if err != nil || plain != "" {
		t.Fatalf("open: err=%v, empty=%v", err, plain == "")
	}
}

// An older build stored an empty secret as a bare "". It reads as empty, and
// is legacy so Upgrade rewrites it.
func TestOpenReadsABareEmptyAsLegacy(t *testing.T) {
	c := testCipher(t, 0)
	plain, legacy, err := c.open("env:global:A", "")
	if err != nil || plain != "" || !legacy {
		t.Fatalf("open(bare empty) = %d bytes, legacy %v, err %v; want empty, legacy, nil", len(plain), legacy, err)
	}
}

func TestOpenRefusesWhatIsNotAnEnvelope(t *testing.T) {
	c := testCipher(t, 0)
	for _, v := range []string{"postgres://attacker@evil/db", " ", "enc"} {
		plain, _, err := c.open("conn:global:warehouse", v)
		if !errors.Is(err, ErrUnencrypted) || !errors.Is(err, ErrTampered) {
			t.Errorf("open(unencrypted) err = %v, want ErrUnencrypted under ErrTampered", err)
		}
		if plain != "" {
			t.Errorf("open(unencrypted) returned %d bytes, want none", len(plain))
		}
	}
}

func TestOpenBindsTheValueToItsKey(t *testing.T) {
	c := testCipher(t, 0)
	enc, err := c.seal("conn:global:a", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.open("conn:global:b", enc); !errors.Is(err, ErrTampered) {
		t.Fatalf("opening a's ciphertext as b: err = %v, want ErrTampered", err)
	}
}

func TestOpenReportsADifferentMasterKey(t *testing.T) {
	enc, err := testCipher(t, 0).seal("k", "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = testCipher(t, 0xff).open("k", enc)
	if !errors.Is(err, ErrWrongMasterKey) {
		t.Fatalf("err = %v, want ErrWrongMasterKey", err)
	}
	if errors.Is(err, ErrTampered) {
		t.Fatal("a different key is not tampering; the remediations differ")
	}
}

func TestOpenReadsTheOlderEnvelope(t *testing.T) {
	c := testCipher(t, 0)
	plain, legacy, err := c.open("k", sealV1(t, c, "old"))
	if err != nil || !legacy || plain != "old" {
		t.Fatalf("open(v1): legacy=%v err=%v matched=%v", legacy, err, plain == "old")
	}
}

func TestOpenRefusesAnUnknownVersion(t *testing.T) {
	if _, _, err := testCipher(t, 0).open("k", "enc:v9:abc"); !errors.Is(err, ErrValueTooNew) {
		t.Fatalf("err = %v, want ErrValueTooNew", err)
	}
}

func TestOpenRejectsDamagedEnvelopes(t *testing.T) {
	c := testCipher(t, 0)
	for _, v := range []string{
		encPrefixV2 + c.kid + ":AAAA",
		encPrefixV2 + c.kid + ":!!!not-base64",
		encPrefixV2 + "no-separator",
		encPrefixV1 + "AAAA",
	} {
		if _, _, err := c.open("k", v); err == nil {
			t.Errorf("open(%q) succeeded, want an error", v)
		}
	}
}

func TestKeyIDDependsOnTheKey(t *testing.T) {
	a, b := testCipher(t, 0), testCipher(t, 1)
	if a.kid == b.kid {
		t.Fatal("two master keys share a key id")
	}
	if len(a.kid) != 2*kidBytes {
		t.Fatalf("key id is %d hex chars, want %d", len(a.kid), 2*kidBytes)
	}
	if again := testCipher(t, 0); again.kid != a.kid {
		t.Fatal("key id is not deterministic")
	}
}
