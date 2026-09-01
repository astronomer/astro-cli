package env

import (
	"bytes"
	"strings"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
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
	objs := []astrov1.EnvironmentObject{
		{ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		{ObjectKey: "SECRET_KEY", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "shh", IsSecret: true}},
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

func (s *Suite) TestWriteVarDotenvEscapesSpecialChars() {
	objs := []astrov1.EnvironmentObject{
		{ObjectKey: "PLAIN", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "simple"}},
		{ObjectKey: "WITH_SPACES", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "two words"}},
		{ObjectKey: "WITH_NEWLINE", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "line1\nline2"}},
		{ObjectKey: "WITH_QUOTES", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: `say "hi"`}},
		{ObjectKey: "WITH_HASH", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "value#notacomment"}},
		{ObjectKey: "WITH_BACKSLASH", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: `path\to\thing`}},
		{ObjectKey: "WITH_DOLLAR", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "user $HOME"}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatDotenv, true, &buf))
	out := buf.String()

	s.Contains(out, "PLAIN=simple\n")
	s.Contains(out, `WITH_SPACES="two words"`)
	s.Contains(out, `WITH_NEWLINE="line1\nline2"`)
	s.Contains(out, `WITH_QUOTES="say \"hi\""`)
	s.Contains(out, `WITH_HASH="value#notacomment"`)
	s.Contains(out, `WITH_BACKSLASH="path\\to\\thing"`)
	s.Contains(out, `WITH_DOLLAR="user \$HOME"`)

	// A literal newline must never appear inside a value, since that would
	// silently split it across two lines for any dotenv parser.
	s.NotContains(out, "line1\nline2")
	// $ inside a double-quoted dotenv value is interpolated by most parsers
	// unless escaped, so an unescaped "$HOME" would round-trip incorrectly.
	s.NotContains(out, `"user $HOME"`)
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
	objs := []astrov1.EnvironmentObject{
		{ObjectKey: "LONG", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: long}},
		{ObjectKey: "MULTI", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "a\nb"}},
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
	objs := []astrov1.EnvironmentObject{
		{ObjectKey: "LONG", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: long}},
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
	objs := []astrov1.EnvironmentObject{
		{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatJSON, false, &buf))
	out := buf.String()
	s.Contains(out, `"objectKey": "FOO"`)
	s.Contains(out, id)
}

func (s *Suite) TestWriteVarLinksTableSecrets() {
	override := "secret-override"
	report := &VarLinksReport{
		ObjectKey:      "FOO",
		WorkspaceValue: "secret-value",
		IsSecret:       true,
		Links:          []VarLink{{DeploymentID: "dep1", OverrideValue: &override}},
	}

	s.Run("masks workspace value and override without --include-secrets", func() {
		var buf bytes.Buffer
		s.NoError(WriteVarLinks(report, FormatTable, false, &buf))
		out := buf.String()
		s.Contains(out, maskedSecret+" (secret)")
		s.Contains(out, "(hidden, use --include-secrets)")
		s.NotContains(out, "secret-value")
		s.NotContains(out, "secret-override")
	})

	s.Run("shows both with --include-secrets", func() {
		var buf bytes.Buffer
		s.NoError(WriteVarLinks(report, FormatTable, true, &buf))
		out := buf.String()
		s.Contains(out, "secret-value")
		s.Contains(out, "secret-override")
		s.NotContains(out, maskedSecret)
	})
}

// Hostile link values must not shred the table: long overrides are truncated
// and newlines are collapsed, same as the var list table.
func (s *Suite) TestWriteVarLinksTableClampsValues() {
	long := strings.Repeat("x", 500)
	report := &VarLinksReport{
		ObjectKey:      "FOO",
		WorkspaceValue: "line1\nline2",
		Links:          []VarLink{{DeploymentID: "dep1", OverrideValue: &long}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarLinks(report, FormatTable, false, &buf))
	out := buf.String()
	s.NotContains(out, long)
	s.Contains(out, "line1 ⏎ line2")
	for _, line := range strings.Split(out, "\n") {
		s.LessOrEqual(len([]rune(line)), 120, "rendered line exceeds bounded width: %q", line)
	}
}
