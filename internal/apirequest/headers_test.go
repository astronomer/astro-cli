package apirequest

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -H is "key:value", trimmed on both sides of the colon the way af and curl
// read it. Only the first colon splits, so a value may hold more.
func TestParseHeaders(t *testing.T) {
	got, err := ParseHeaders([]string{"X-Foo: bar", " X-Trace :a:b:c ", "accept:text/plain"})
	require.NoError(t, err)
	assert.Equal(t, "bar", got.Get("X-Foo"))
	assert.Equal(t, "a:b:c", got.Get("X-Trace"))
	assert.Equal(t, "text/plain", got.Get("Accept"))
}

func TestParseHeadersRefusesAHeaderWithNoColon(t *testing.T) {
	_, err := ParseHeaders([]string{"no-colon-here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid header format")
}

func TestWriteHeadIsAStatusLineHeadersAndABlankLine(t *testing.T) {
	var buf bytes.Buffer
	WriteHead(&buf, "HTTP/1.1 200 OK", http.Header{
		"X-Custom":     {"value"},
		"Content-Type": {"application/json"},
		"Status":       {"should-be-skipped"},
	}, false)
	assert.Equal(t, "HTTP/1.1 200 OK\nContent-Type: application/json\nX-Custom: value\n\n", buf.String())
}

func TestWriteHeadColorizes(t *testing.T) {
	var buf bytes.Buffer
	WriteHead(&buf, "HTTP/1.1 200 OK", http.Header{"X-Test": {"val"}}, true)
	assert.Contains(t, buf.String(), "\x1b[") // ANSI escape
	assert.Contains(t, buf.String(), "X-Test")
}

// Typed fields keep their type on the wire: a number, a bool, and a null reach
// the query as af sends them.
func TestQueryEncodesTypedFields(t *testing.T) {
	got := Query(map[string]any{
		"limit":       10,
		"only_active": true,
		"ratio":       1.5,
		"cursor":      nil,
		"tags":        []any{"a", "b"},
		"filter":      map[string]any{"state": "failed"},
	})
	want := url.Values{
		"limit":         {"10"},
		"only_active":   {"true"},
		"ratio":         {"1.5"},
		"cursor":        {""},
		"tags[]":        {"a", "b"},
		"filter[state]": {"failed"},
	}
	assert.Equal(t, want, got)
}
