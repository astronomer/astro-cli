package env

import (
	"bytes"
	"strings"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
)

func (s *Suite) TestParseFormat() {
	cases := map[string]Format{
		"":       Format(""),
		"table":  FormatTable,
		"json":   FormatJSON,
		"yaml":   FormatYAML,
		"dotenv": FormatDotenv,
	}
	for in, want := range cases {
		got, err := ParseFormat(in)
		s.NoError(err, "input %q", in)
		s.Equal(want, got, "input %q", in)
	}

	_, err := ParseFormat("xml")
	s.Error(err)
}

func (s *Suite) TestWriteVarDotenv() {
	objs := []astrocore.EnvironmentObject{
		{ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		{ObjectKey: "SECRET_KEY", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "shh", IsSecret: true}},
	}

	s.Run("hides secrets by default", func() {
		var buf bytes.Buffer
		s.NoError(WriteVarList(objs, FormatDotenv, false, &buf))
		out := buf.String()
		s.Contains(out, "FOO=bar\n")
		s.Contains(out, "SECRET_KEY=  # secret, use --include-secrets")
		s.NotContains(out, "shh")
	})

	s.Run("includes secrets when asked", func() {
		var buf bytes.Buffer
		s.NoError(WriteVarList(objs, FormatDotenv, true, &buf))
		out := buf.String()
		s.Contains(out, "FOO=bar")
		s.Contains(out, "SECRET_KEY=shh")
	})
}

func (s *Suite) TestClampTableValue() {
	s.Equal("", clampTableValue(""))
	s.Equal("short", clampTableValue("short"))

	// Newlines are visualized so a single value never spans multiple table rows.
	s.Equal("a ⏎ b ⏎ c", clampTableValue("a\nb\rc"))

	// Long values are truncated to tableValueMax runes with an ellipsis.
	long := strings.Repeat("x", 200)
	got := clampTableValue(long)
	s.Equal(tableValueMax, len([]rune(got)))
	s.True(strings.HasSuffix(got, "…"))

	// Multibyte runes are counted by rune, not byte.
	multibyte := strings.Repeat("日", 100)
	got = clampTableValue(multibyte)
	s.Equal(tableValueMax, len([]rune(got)))
}

func (s *Suite) TestWriteVarTableTruncatesLongValues() {
	long := strings.Repeat("x", 500)
	objs := []astrocore.EnvironmentObject{
		{ObjectKey: "LONG", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: long}},
		{ObjectKey: "MULTI", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "a\nb"}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatTable, false, &buf))
	out := buf.String()
	s.NotContains(out, long)   // long value truncated
	s.Contains(out, "…")       // ellipsis present
	s.NotContains(out, "a\nb") // newline not preserved in cell
	s.Contains(out, "a ⏎ b")   // newline replaced with marker
}

func (s *Suite) TestWriteVarJSONNotTruncated() {
	long := strings.Repeat("x", 500)
	objs := []astrocore.EnvironmentObject{
		{ObjectKey: "LONG", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: long}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatJSON, false, &buf))
	s.Contains(buf.String(), long) // JSON output preserves the full value
}

func (s *Suite) TestWriteVarTableEmpty() {
	var buf bytes.Buffer
	s.NoError(WriteVarList(nil, FormatTable, false, &buf))
	s.Contains(strings.ToLower(buf.String()), "no environment variables")
}

func (s *Suite) TestWriteVarJSON() {
	id := "cabc12def0123456789012345"
	objs := []astrocore.EnvironmentObject{
		{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatJSON, false, &buf))
	out := buf.String()
	s.Contains(out, `"objectKey": "FOO"`)
	s.Contains(out, id)
}
