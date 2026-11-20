package airflowrt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each case is a .env line or lines and what Docker Compose's env_file loader
// (which 1.x handed every .env to) decodes it as.
func TestParseDotenvMatchesCompose(t *testing.T) {
	cases := []struct {
		name, in, key, want string
	}{
		{"unquoted", "FOO=bar", "FOO", "bar"},
		{"empty", "FOO=", "FOO", ""},
		{"spaces around =", "FOO = bar", "FOO", "bar"},
		{"export prefix", "export FOO=bar", "FOO", "bar"},
		{"export with tabs", "export\tFOO=bar", "FOO", "bar"},
		{"key named export", "export=bar", "export", "bar"},
		{"inline comment", "FOO=bar # a note", "FOO", "bar"},
		{"hash without space is value", "FOO=bar#baz", "FOO", "bar#baz"},
		{"unquoted backslash is literal", `FOO=a\nb`, "FOO", `a\nb`},
		{"value with =", "FOO=a=b", "FOO", "a=b"},
		{"single quoted is literal", `FOO='a\nb "c"'`, "FOO", `a\nb "c"`},
		{"single quoted keeps hash", "FOO='a # b'", "FOO", "a # b"},
		{"double quoted expands escapes", `FOO="a\nb\t\"c\"\\d"`, "FOO", "a\nb\t\"c\"\\d"},
		{"double quoted dollar escape", `FOO="\$HOME"`, "FOO", "$HOME"},
		{"double quoted unknown escape kept", `FOO="a\qb"`, "FOO", `a\qb`},
		{"comment after closing quote", `FOO="bar" # note`, "FOO", "bar"},
		{"crlf", "FOO=bar\r\n", "FOO", "bar"},
		{"multi-line single", "FOO='a\nb'", "FOO", "a\nb"},
		{"multi-line double", "FOO=\"{\n  \\\"a\\\": 1\n}\"", "FOO", "{\n  \"a\": 1\n}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, problems := ParseDotenvMap([]byte(tc.in))
			assert.Empty(t, problems)
			assert.Equal(t, tc.want, m[tc.key])
		})
	}
}

// The case that sent a 1.x project's connection missing under v2: JSON
// double-quoted with its inner quotes escaped decoded with the backslashes
// still in, which Airflow cannot parse, so it reported the connection as not
// defined.
func TestParseDotenvEscapedJSONConnection(t *testing.T) {
	in := `AIRFLOW_CONN_UCL_GCP="{\"conn_type\": \"google_cloud_platform\", \"extra\": {\"keyfile_dict\": {\"private_key\": \"-----BEGIN-----\\nabc\\n-----END-----\\n\"}}}"`
	m, problems := ParseDotenvMap([]byte(in))
	require.Empty(t, problems)
	var conn struct {
		ConnType string `json:"conn_type"`
		Extra    struct {
			KeyfileDict struct {
				PrivateKey string `json:"private_key"`
			} `json:"keyfile_dict"`
		} `json:"extra"`
	}
	require.NoError(t, json.Unmarshal([]byte(m["AIRFLOW_CONN_UCL_GCP"]), &conn))
	assert.Equal(t, "google_cloud_platform", conn.ConnType)
	assert.Equal(t, "-----BEGIN-----\nabc\n-----END-----\n", conn.Extra.KeyfileDict.PrivateKey)
}

func TestParseDotenvSpansAndOrder(t *testing.T) {
	in := "# head\nA=1\nB='x\ny'\n\nA=2\n"
	entries, problems := ParseDotenv([]byte(in))
	require.Empty(t, problems)
	assert.Equal(t, []DotenvEntry{
		{Key: "A", Value: "1", StartLine: 1, EndLine: 1},
		{Key: "B", Value: "x\ny", StartLine: 2, EndLine: 3},
		{Key: "A", Value: "2", StartLine: 5, EndLine: 5},
	}, entries)
	m, _ := ParseDotenvMap([]byte(in))
	assert.Equal(t, "2", m["A"], "the last assignment wins")
}

// A line that is no assignment, or a quote that never closes, costs only
// itself: it is reported and the lines after it still load.
func TestParseDotenvProblems(t *testing.T) {
	in := "GOOD=1\njust some words\nBAD NAME=x\nTRAIL=\"v\" extra\nOPEN='never closed\nLATER=2\n"
	m, problems := ParseDotenvMap([]byte(in))
	assert.Equal(t, map[string]string{"GOOD": "1", "TRAIL": "v", "LATER": "2"}, m)
	require.Len(t, problems, 4)
	assert.Equal(t, 2, problems[0].Line)
	assert.True(t, problems[0].Skipped)
	assert.Equal(t, 3, problems[1].Line)
	assert.True(t, problems[1].Skipped)
	assert.Equal(t, 4, problems[2].Line)
	assert.Contains(t, problems[2].Reason, "extra")
	assert.False(t, problems[2].Skipped, "TRAIL is kept, without the extra")
	assert.Equal(t, 5, problems[3].Line)
	assert.Contains(t, problems[3].Reason, "never closed")
	assert.True(t, problems[3].Skipped)
}

// A quote is closed by the next unescaped one of its kind, however many lines
// on, as Compose reads it: a stray opening quote takes the lines after it.
func TestParseDotenvQuoteSpansToNextQuote(t *testing.T) {
	m, problems := ParseDotenvMap([]byte("A=\"x\nB=1\nC=\"y\"\n"))
	assert.Equal(t, map[string]string{"A": "x\nB=1\nC="}, m)
	require.Len(t, problems, 2)
	// The assignment it took in is named, at the line the quote opened on.
	assert.Equal(t, 1, problems[0].Line)
	assert.Contains(t, problems[0].Reason, "taking in line 2")
	assert.False(t, problems[0].Skipped, "A is kept")
	assert.Equal(t, 3, problems[1].Line)
	assert.Contains(t, problems[1].Reason, "after its closing quote")
	assert.False(t, problems[1].Skipped, "A is kept")
}

// A value over several lines that takes in nothing that looks like an
// assignment, pasted JSON for one, is no problem.
func TestParseDotenvMultiLineJSONIsQuiet(t *testing.T) {
	_, problems := ParseDotenvMap([]byte("A='{\n  \"k\": \"v\",\n  \"n\": 1\n}'\n"))
	assert.Empty(t, problems)
}
