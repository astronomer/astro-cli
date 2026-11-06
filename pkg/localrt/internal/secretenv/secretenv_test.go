package secretenv

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTripKeepsValuesByteForByte(t *testing.T) {
	in := map[string]string{
		"AIRFLOW_CONN_DB": `postgres://u:pa$$ 'w"o@h/db`,
		"MULTILINE":       "-----BEGIN KEY-----\nabc\n-----END KEY-----",
		"EMPTY":           "",
	}
	b, err := Encode(in)
	require.NoError(t, err)
	out, err := Decode(strings.NewReader(string(b)))
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

// A pair no environment can hold is dropped at both ends: Encode never sends
// it, and Decode never trusts a payload that has it.
func TestInvalidPairsAreDropped(t *testing.T) {
	b, err := Encode(map[string]string{"": "x", "A=B": "x", "NUL\x00": "x", "V": "a\x00b"})
	require.NoError(t, err)
	assert.Nil(t, b, "nothing valid means no handoff at all")

	out, err := Decode(strings.NewReader(`{"OK":"1","A=B":"2"}`))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"OK": "1"}, out)
}

func TestEncodeNothingIsNoHandoff(t *testing.T) {
	b, err := Encode(nil)
	require.NoError(t, err)
	assert.Nil(t, b)
}

func TestDecodeRefusesGarbage(t *testing.T) {
	_, err := Decode(strings.NewReader("not json"))
	assert.ErrorContains(t, err, "reading secret values")
}

func TestEnvironIsSorted(t *testing.T) {
	assert.Equal(t, []string{"A=1", "B=2"}, Environ(map[string]string{"B": "2", "A": "1", "C=": "3"}))
}
