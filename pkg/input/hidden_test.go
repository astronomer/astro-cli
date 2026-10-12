package input

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadHidden(t *testing.T) {
	long := `{"conn_type":"google_cloud_platform","extra":{"keyfile_dict":"` + strings.Repeat("x", 4000) + `"}}`
	cases := []struct {
		name, in, want string
	}{
		{"enter ends a plain value", "s3cret\rignored", "s3cret"},
		{"newline ends a plain value", "s3cret\n", "s3cret"},
		{"a long line is not truncated", long + "\r", long},
		{"enter inside open json is a newline", "{\r  \"conn_type\": \"http\"\r}\r", "{\n  \"conn_type\": \"http\"\n}"},
		{"a pasted crlf object", "{\r\n  \"a\": [1,\r\n 2]\r\n}\r\n", "{\n  \"a\": [1,\n 2]\n}"},
		{"brackets inside strings do not count", "{\"a\": \"}{\"\r}\r", "{\"a\": \"}{\"\n}"},
		{"an escaped quote stays in the string", "{\"a\": \"\\\"}\"\r}\r", "{\"a\": \"\\\"}\"\n}"},
		{"ctrl-d ends json that never closes", "{\"a\": 1\r\x04", "{\"a\": 1\n"},
		{"backspace removes a whole rune", "aé\x7f\x08b\r", "b"},
		{"ctrl-u clears the line", "wrong\x15right\r", "right"},
		{"arrow keys are dropped", "ab\x1b[Dc\r", "abc"},
		{"other control bytes are dropped", "a\x01b\tc\r", "ab\tc"},
		{"eof returns what was typed", "partial", "partial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readHidden(strings.NewReader(tc.in))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReadHiddenCtrlC(t *testing.T) {
	_, err := readHidden(strings.NewReader("half\x03rest\r"))
	require.ErrorIs(t, err, ErrInterrupted)
}

func TestReadHiddenEmptyEOF(t *testing.T) {
	for _, in := range []string{"", "\x04"} {
		_, err := readHidden(strings.NewReader(in))
		require.ErrorIs(t, err, io.EOF, "input %q", in)
	}
}
