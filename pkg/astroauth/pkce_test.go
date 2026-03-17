package astroauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	require.NoError(t, err)
	assert.NotEmpty(t, verifier)
	assert.NotEmpty(t, challenge)
	assert.NotEqual(t, verifier, challenge)

	// Verify no padding or forbidden chars
	assert.NotContains(t, verifier, "=")
	assert.NotContains(t, verifier, "+")
	assert.NotContains(t, verifier, "/")
	assert.NotContains(t, challenge, "=")
	assert.NotContains(t, challenge, "+")
	assert.NotContains(t, challenge, "/")
}

func TestGeneratePKCE_Unique(t *testing.T) {
	v1, _, _ := GeneratePKCE()
	v2, _, _ := GeneratePKCE()
	assert.NotEqual(t, v1, v2, "two calls should produce different verifiers")
}

func TestBase64URLEncode(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte("hello"), "aGVsbG8"},
		{[]byte(""), ""},
		{[]byte{0xff, 0xfe}, "__4"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, Base64URLEncode(tt.input))
	}
}
