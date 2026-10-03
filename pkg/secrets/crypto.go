package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// The value envelope.
//
// Set writes every secret value as
//
//	enc:v2:<kid>:<base64(nonce || ciphertext || tag)>
//
// AES-256-GCM under the master key, with two additions over the older enc:v1:
// form that make the envelope an integrity check as well as a seal:
//
//   - The additional authenticated data is aadPrefix followed by the vault key
//     the value is stored under. A ciphertext copied from one entry's file into
//     another's fails to open, so a writer to the directory cannot swap values
//     between entries without the master key.
//   - kid identifies the master key that sealed the value: the first kidBytes of
//     HMAC-SHA256(masterKey, kidLabel), in hex. It is derived from the key
//     rather than stored beside it, so it needs no keyring change, and it is
//     not a secret: a MAC output reveals nothing about the key. With it, "this
//     value belongs to a different master key" is a fact read off the envelope
//     (ErrWrongMasterKey) rather than a guess from an authentication failure,
//     which a single damaged value produces too.
//
// The empty string is sealed like any other value. Older builds stored an
// empty secret as a bare "", which is read as the empty value it was and
// rewritten by Upgrade: it cannot redirect a connection, which is what refusing
// unencrypted values stops. Any other unprefixed value is never a valid secret.
//
// enc:v1:<base64(nonce || ciphertext || tag)> is the older form: same cipher,
// no AAD, no kid. It is still read, so a vault written by an older build keeps
// working, and Upgrade rewrites it as v2. Every other value in a secret entry,
// apart from the bare "" above, is refused with ErrUnencrypted.
//
// The change is one-way for builds that predate v2. Their reader returned any
// value without the enc:v1: prefix unchanged, so an older CLI or desktop reads
// a v2 value as its literal envelope text and serves that as the secret, and
// one that copies the value elsewhere seals that text. Running an older build
// against a vault this one has written or upgraded is therefore unsupported;
// no released build of either tool shipped the shared vault before v2.
const (
	encPrefixV1 = "enc:v1:"
	encPrefixV2 = "enc:v2:"
	// encMarker is what every envelope, of any version, starts with.
	encMarker = "enc:"

	// kidLabel is the HMAC message the key id is computed over. Changing it
	// changes every key id, and every existing v2 value then reports
	// ErrWrongMasterKey.
	kidLabel = "astro-vault-kid"
	kidBytes = 8
	// aadPrefix binds a v2 ciphertext to the envelope version and to the vault
	// key it is stored under; see aad.
	aadPrefix = "astro-vault/v2\x00"
)

// ErrTampered reports a secret entry whose stored value failed an integrity
// check: it is not an envelope at all (ErrUnencrypted), its file names a
// different key than the one requested, or its ciphertext does not
// authenticate under the master key that, by its key id, sealed it. The value
// is never returned.
//
// Per entry, unlike ErrKeyringUnavailable: the rest of the vault is
// unaffected, and a caller reading many keys skips this one.
var ErrTampered = errors.New("vault value failed its integrity check")

// ErrUnencrypted reports a secret entry holding a value that is not an
// envelope, the empty string included. It is refused rather than returned
// because returning it is what would let anyone who can write the vault
// directory plant a credential without the master key. A value meant to be
// unencrypted is stored with SetPlain, which marks it so, and every surface
// then treats it as plain rather than as a secret.
var ErrUnencrypted = fmt.Errorf("%w: a secret entry holds a value that is not encrypted", ErrTampered)

// ErrWrongMasterKey reports a value sealed under a different master key than
// the one in this machine's keyring, read off the envelope's key id. Every
// value written after a key was replaced reports it, which is what separates
// "the key changed" from "this one value is damaged" (ErrTampered). Per entry,
// like ErrTampered: whether the whole vault is affected is a question for a
// caller that read all of it.
var ErrWrongMasterKey = errors.New("vault value was encrypted under a different master key")

// ErrValueTooNew reports an envelope version this build does not know: a newer
// build of either tool wrote it. Neither tampered nor a key problem, and the
// fix is updating this tool.
var ErrValueTooNew = errors.New("vault value was written by a newer version")

// vaultCipher is the master key in the two forms the envelope needs: the AEAD
// that seals and opens values, and the key id stamped into every v2 value.
type vaultCipher struct {
	gcm cipher.AEAD
	kid string
}

func newVaultCipher(key []byte) (*vaultCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	return &vaultCipher{gcm: gcm, kid: keyID(key)}, nil
}

// keyID is the identifier of a master key: see the envelope note above.
func keyID(key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(kidLabel))
	return hex.EncodeToString(mac.Sum(nil)[:kidBytes])
}

// aad is the additional authenticated data for a v2 value stored under name.
// The NUL separates the fixed prefix from a key that is free to contain
// anything else.
func aad(name string) []byte {
	return []byte(aadPrefix + name)
}

// seal encrypts plain for storage under the vault key name, as a v2 envelope.
func (c *vaultCipher) seal(name, plain string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(plain), aad(name))
	return encPrefixV2 + c.kid + ":" + base64.StdEncoding.EncodeToString(ct), nil
}

// open decrypts a value stored under the vault key name. legacy reports a v1
// envelope, which opened without binding to name, or the bare "" an older build
// stored for an empty secret; either should be rewritten as v2.
//
// Nothing is returned for any other value that is not an envelope: see
// ErrUnencrypted.
func (c *vaultCipher) open(name, value string) (plain string, legacy bool, err error) {
	switch {
	case strings.HasPrefix(value, encPrefixV2):
		kid, body, ok := strings.Cut(strings.TrimPrefix(value, encPrefixV2), ":")
		if !ok {
			return "", false, fmt.Errorf("%w: malformed envelope", ErrTampered)
		}
		if kid != c.kid {
			return "", false, fmt.Errorf("%w (key id %s, this machine's is %s)", ErrWrongMasterKey, kid, c.kid)
		}
		plain, err := c.openRaw(body, aad(name))
		if err != nil {
			return "", false, fmt.Errorf("%w: %w", ErrTampered, err)
		}
		return plain, false, nil
	case strings.HasPrefix(value, encPrefixV1):
		// TODO(vault-v1-retire): drop this case and the bare "" one below; see
		// upgrade.go for when.
		//
		// No key id and no AAD: a failure here cannot say whether the key or the
		// value is wrong, so it carries neither sentinel.
		plain, err := c.openRaw(strings.TrimPrefix(value, encPrefixV1), nil)
		if err != nil {
			return "", false, err
		}
		return plain, true, nil
	case value == "":
		return "", true, nil
	case strings.HasPrefix(value, encMarker):
		version, _, _ := strings.Cut(strings.TrimPrefix(value, encMarker), ":")
		return "", false, fmt.Errorf("%w (envelope %q)", ErrValueTooNew, encMarker+version)
	default:
		return "", false, ErrUnencrypted
	}
}

func (c *vaultCipher) openRaw(body string, additional []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < c.gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:c.gcm.NonceSize()], raw[c.gcm.NonceSize():]
	plain, err := c.gcm.Open(nil, nonce, ct, additional)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
