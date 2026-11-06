package secretenv

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeKeepsValuesByteForByte(t *testing.T) {
	in := map[string]string{
		"AIRFLOW_CONN_DB": `postgres://u:pa$$ 'w"o@h/db`,
		"MULTILINE":       "-----BEGIN KEY-----\nabc\n-----END KEY-----",
		"EMPTY":           "",
	}
	b, err := Encode(in)
	require.NoError(t, err)
	var out map[string]string
	require.NoError(t, json.Unmarshal(b, &out))
	assert.Equal(t, in, out)
}

// A pair no environment can hold is never sent.
func TestEncodeDropsInvalidPairs(t *testing.T) {
	b, err := Encode(map[string]string{"": "x", "A=B": "x", "NUL\x00": "x", "V": "a\x00b", "OK": "1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"OK":"1"}`, string(b))
}

func TestEncodeNothingIsNoHandoff(t *testing.T) {
	b, err := Encode(nil)
	require.NoError(t, err)
	assert.Nil(t, b)

	b, err = Encode(map[string]string{"A=B": "x"})
	require.NoError(t, err)
	assert.Nil(t, b, "nothing valid means no handoff at all")
}

func TestEnvironIsSorted(t *testing.T) {
	assert.Equal(t, []string{"A=1", "B=2"}, Environ(map[string]string{"B": "2", "A": "1", "C=": "3"}))
}
